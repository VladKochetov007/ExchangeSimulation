package evstream

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"hash/crc32"
	"io"
)

// Frame is one decoded event handed to a visitor. Payload aliases the reader's
// block buffer and stops being valid when the visitor returns, so a visitor
// that keeps a field must copy it. Aliasing is the point: it is what lets a
// whole stream be read without allocating per event.
type Frame struct {
	Header  FrameHeader
	Payload []byte
	// Venue is the resolved dictionary string, empty when VenueRef is 0.
	Venue string
}

// ReaderOptions configures a Reader.
type ReaderOptions struct {
	// Decompressor must match the codec in the stream header. Nil is correct
	// for an uncompressed stream, and is an error for a compressed one — the
	// reader will not guess.
	Decompressor BlockDecompressor
	// VerifyHash maintains the rolling execution digest while reading, so a
	// stream can be checked against a published hash without a second pass.
	VerifyHash bool
	// SkipUnknownSchemas returns frames whose schema the caller does not
	// recognise rather than failing. Frames are length-prefixed precisely so
	// this is possible; it is what lets an old reader survive a new writer.
	SkipUnknownSchemas bool
	// AllowUnterminated permits reading a stream that is still being written.
	// Finished evidence must use the default false value so a missing tail is
	// reported rather than interpreted as a shorter successful run.
	AllowUnterminated bool
}

// Reader walks a stream, verifying structure as it goes.
//
// Verification is streaming and incremental: block CRCs, frame count per block,
// and gap-free monotonic sequence are all checked as the data arrives, so a
// truncated or corrupted stream is caught at the point of damage rather than
// after buffering the file.
type Reader struct {
	in           io.Reader
	decompressor BlockDecompressor
	codec        Codec
	epoch        uint32

	block             []byte
	stored            []byte
	blockHdr          [BlockHeaderSize]byte
	dict              *Dictionary
	lastSeq           uint64
	verify            bool
	rolling           [sha256.Size]byte
	hasher            hash.Hash
	frames            uint64
	blockFrames       uint32
	streamHdr         bool
	terminated        bool
	allowUnterminated bool
}

// NewReader validates the stream header and prepares to read blocks.
func NewReader(in io.Reader, opts ReaderOptions) (*Reader, error) {
	r := &Reader{
		in:                in,
		decompressor:      opts.Decompressor,
		dict:              NewDictionary(),
		verify:            opts.VerifyHash,
		allowUnterminated: opts.AllowUnterminated,
	}
	if opts.VerifyHash {
		r.hasher = sha256.New()
	}
	var header [StreamHeaderSize]byte
	if _, err := io.ReadFull(in, header[:]); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, ErrShortBuffer
		}
		return nil, err
	}
	if string(header[0:8]) != Magic {
		return nil, ErrBadMagic
	}
	major := binary.LittleEndian.Uint16(header[8:10])
	if major != FormatMajor {
		return nil, fmt.Errorf("%w: major %d, reader supports %d",
			ErrUnsupportedVersion, major, FormatMajor)
	}
	r.codec = Codec(header[12])
	r.epoch = binary.LittleEndian.Uint32(header[16:20])
	if r.codec != CodecNone {
		if r.decompressor == nil {
			return nil, fmt.Errorf("%w: stream is %s but no decompressor supplied",
				ErrUnsupportedVersion, r.codec)
		}
		if r.decompressor.Codec() != r.codec {
			return nil, fmt.Errorf("%w: stream is %s but decompressor is %s",
				ErrUnsupportedVersion, r.codec, r.decompressor.Codec())
		}
	}
	r.streamHdr = true
	return r, nil
}

// Codec reports the storage codec declared by the stream.
func (r *Reader) Codec() Codec { return r.codec }

// SchemaEpoch reports the writer's schema-set identity.
func (r *Reader) SchemaEpoch() uint32 { return r.epoch }

// ExecutionHash returns the rolling digest over the frames read so far. It is
// meaningful only when VerifyHash was set, and it is computed over uncompressed
// canonical frames, so it equals the writer's hash regardless of codec.
func (r *Reader) ExecutionHash() [sha256.Size]byte {
	var out [sha256.Size]byte
	if r.hasher == nil {
		return out
	}
	copy(out[:], r.hasher.Sum(nil))
	return out
}

// Terminated reports whether the reader observed a valid completion trailer.
func (r *Reader) Terminated() bool { return r.terminated }

// Count returns the number of frames read, including dictionary frames.
func (r *Reader) Count() uint64 { return r.frames }

// Range walks every frame in order, calling visit for each non-dictionary
// frame. Dictionary frames are consumed internally so callers never see them.
//
// visit returning an error stops the walk and returns that error.
func (r *Reader) Range(visit func(Frame) error) error {
	for {
		more, err := r.nextBlock()
		if err != nil {
			return err
		}
		if !more {
			if !r.terminated && !r.allowUnterminated {
				return fmt.Errorf("%w: stream ends without a completion trailer", ErrShortBuffer)
			}
			return nil
		}
		if err := r.rangeBlock(visit); err != nil {
			return err
		}
	}
}

// nextBlock reads and validates one block into r.block.
func (r *Reader) nextBlock() (bool, error) {
	if _, err := io.ReadFull(r.in, r.blockHdr[:]); err != nil {
		if errors.Is(err, io.EOF) {
			return false, nil
		}
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return false, ErrShortBuffer
		}
		return false, err
	}
	if magic := binary.LittleEndian.Uint32(r.blockHdr[0:4]); magic != BlockMagic {
		if magic == TrailerMagic {
			return false, r.readTrailer()
		}
		return false, fmt.Errorf("%w: block magic", ErrCorrupt)
	}
	uncompressedLen := int(binary.LittleEndian.Uint32(r.blockHdr[4:8]))
	storedLen := int(binary.LittleEndian.Uint32(r.blockHdr[8:12]))
	wantFrames := binary.LittleEndian.Uint32(r.blockHdr[12:16])
	wantCRC := binary.LittleEndian.Uint32(r.blockHdr[16:20])

	if uncompressedLen < 0 || storedLen < 0 {
		return false, fmt.Errorf("%w: negative block length", ErrCorrupt)
	}
	r.stored = growTo(r.stored, storedLen)
	if _, err := io.ReadFull(r.in, r.stored[:storedLen]); err != nil {
		return false, ErrShortBuffer
	}

	if r.codec == CodecNone {
		if storedLen != uncompressedLen {
			return false, fmt.Errorf("%w: uncompressed block length mismatch", ErrCorrupt)
		}
		r.block = r.stored[:storedLen]
	} else {
		block, err := r.decompressor.Decompress(r.block[:0], r.stored[:storedLen], uncompressedLen)
		if err != nil {
			return false, fmt.Errorf("%w: decompress: %v", ErrCorrupt, err)
		}
		if len(block) != uncompressedLen {
			return false, fmt.Errorf("%w: decompressed to %d, header says %d",
				ErrCorrupt, len(block), uncompressedLen)
		}
		r.block = block
	}

	// CRC is over the uncompressed bytes, so this check is the same whether the
	// block was stored raw or through any codec.
	if crc32.Checksum(r.block, crcTable) != wantCRC {
		return false, fmt.Errorf("%w: block CRC mismatch", ErrCorrupt)
	}
	r.blockFrames = wantFrames
	return true, nil
}

func (r *Reader) readTrailer() error {
	trailer := make([]byte, TrailerSize)
	copy(trailer, r.blockHdr[:])
	if _, err := io.ReadFull(r.in, trailer[len(r.blockHdr):]); err != nil {
		return ErrShortBuffer
	}
	declaredFrames := binary.LittleEndian.Uint64(trailer[4:12])
	if declaredFrames != r.frames {
		return fmt.Errorf("%w: trailer declares %d frames, read %d", ErrCorrupt, declaredFrames, r.frames)
	}
	if r.verify {
		actual := r.ExecutionHash()
		if !bytes.Equal(trailer[12:], actual[:]) {
			return fmt.Errorf("%w: trailer digest does not match frames", ErrCorrupt)
		}
	}
	r.terminated = true
	return nil
}

// rangeBlock walks the frames of the decoded block.
func (r *Reader) rangeBlock(visit func(Frame) error) error {
	var seen uint32
	for offset := 0; offset < len(r.block); {
		header, err := ParseFrameHeader(r.block[offset:])
		if err != nil {
			return err
		}
		end := offset + int(header.Length)
		if end > len(r.block) {
			return fmt.Errorf("%w: frame length %d overruns block", ErrCorrupt, header.Length)
		}
		frameBytes := r.block[offset:end]
		payload := frameBytes[FrameHeaderSize:]

		if header.Seq != r.lastSeq+1 {
			return fmt.Errorf("%w: expected %d, got %d", ErrSequence, r.lastSeq+1, header.Seq)
		}
		r.lastSeq = header.Seq
		r.frames++

		if r.verify {
			// Mirrors the writer: one continuous digest over concatenated
			// canonical frames, so the reader reproduces it exactly.
			r.hasher.Write(frameBytes)
		}

		if header.SchemaID == SchemaDictionary {
			if err := r.defineDictionary(payload); err != nil {
				return err
			}
		} else {
			venue := ""
			if header.VenueRef != 0 {
				value, ok := r.dict.Value(header.VenueRef)
				if !ok {
					return fmt.Errorf("%w: venue ref %d never defined", ErrCorrupt, header.VenueRef)
				}
				venue = value
			}
			if err := visit(Frame{Header: header, Payload: payload, Venue: venue}); err != nil {
				return err
			}
		}

		seen++
		offset = end
	}
	if seen != r.blockFrames {
		return fmt.Errorf("%w: block declares %d frames, found %d", ErrCorrupt, r.blockFrames, seen)
	}
	return nil
}

func (r *Reader) defineDictionary(payload []byte) error {
	cursor := NewCursor(payload)
	id := cursor.Uint32()
	value := cursor.String()
	if err := cursor.Err(); err != nil {
		return err
	}
	if cursor.Remaining() != 0 {
		return fmt.Errorf("%w: dictionary frame has trailing bytes", ErrCorrupt)
	}
	return r.dict.Define(id, value)
}

// Lookup resolves a dictionary id learned while reading.
func (r *Reader) Lookup(id uint32) (string, bool) { return r.dict.Value(id) }

func growTo(buf []byte, n int) []byte {
	if cap(buf) >= n {
		return buf[:n]
	}
	return make([]byte, n)
}
