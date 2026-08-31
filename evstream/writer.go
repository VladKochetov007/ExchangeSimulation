package evstream

import (
	"crypto/sha256"
	"encoding"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"hash/crc32"
	"io"
)

// crcTable is Castagnoli, which has hardware support on this architecture and
// is therefore cheap enough to run over every block on every write.
var crcTable = crc32.MakeTable(crc32.Castagnoli)

// BlockCompressor compresses a block's canonical bytes for storage.
//
// Supplied by the caller rather than selected from a table here, so a new codec
// needs no change to this package. Whatever it does, it cannot affect the
// execution hash: the hash is taken over the uncompressed frames before this is
// ever called.
type BlockCompressor interface {
	// Codec identifies the codec in the stream header.
	Codec() Codec
	// Compress appends the compressed form of src to dst.
	Compress(dst, src []byte) ([]byte, error)
}

// BlockDecompressor is the reading half.
type BlockDecompressor interface {
	Codec() Codec
	// Decompress appends the decompressed form of src to dst, where the caller
	// already knows the exact uncompressed size.
	Decompress(dst, src []byte, uncompressedLen int) ([]byte, error)
}

// WriterOptions configures a Writer. The zero value writes uncompressed blocks
// of the default size, which is the configuration whose bytes are the
// canonical ones.
type WriterOptions struct {
	// Compressor is applied per block for storage. Nil writes raw blocks.
	Compressor BlockCompressor
	// BlockBytes is the target uncompressed size of a block. A block is closed
	// once it reaches this, so blocks are bounded and a reader's working set is
	// bounded with them. Zero uses DefaultBlockBytes.
	BlockBytes int
	// SchemaEpoch identifies the caller's schema set, recorded in the header so
	// a reader can refuse a stream written against an incompatible registry.
	SchemaEpoch uint32
}

// DefaultBlockBytes trades compression ratio against reader working set. Large
// enough for the codecs to find redundancy across many events, small enough
// that a reader holds one block rather than a file.
const DefaultBlockBytes = 1 << 20

// Writer appends events to a stream and maintains the execution hash.
//
// Not safe for concurrent use: the stream is a total order, and serialising
// access is the caller's job precisely because the caller is the one that knows
// what the order should be.
type Writer struct {
	out io.Writer
	// closed guards against appending after the trailer, which would produce a
	// stream that reads as complete and then has bytes after its own end.
	closed     bool
	compressor BlockCompressor
	blockBytes int

	// frames accumulates canonical, uncompressed bytes for the open block.
	frames     []byte
	frameCount uint32
	// stored is reused for the compressed form so a block costs no allocation.
	stored []byte

	seq     uint64
	rolling [sha256.Size]byte
	hasher  hash.Hash

	dict     *Dictionary
	scratch  []byte
	payload  []byte
	wroteHdr bool
	epoch    uint32
	err      error

	// offset tracks bytes written so the index can record where each block
	// begins without the writer needing a seekable destination.
	offset uint64
	stats  blockStats
	index  Index
}

// NewWriter starts a stream on out.
func NewWriter(out io.Writer, opts WriterOptions) *Writer {
	blockBytes := opts.BlockBytes
	if blockBytes <= 0 {
		blockBytes = DefaultBlockBytes
	}
	w := &Writer{
		out:        out,
		compressor: opts.Compressor,
		blockBytes: blockBytes,
		frames:     make([]byte, 0, blockBytes+blockBytes/8),
		hasher:     sha256.New(),
		dict:       NewDictionary(),
		epoch:      opts.SchemaEpoch,
	}
	w.stats.reset()
	return w
}

// Intern returns the dictionary id for s, emitting a dictionary frame the first
// time it is seen.
//
// Ids are assigned in first-use order, which is deterministic because the event
// order is. The dictionary travels as frames in the same stream, so it is
// covered by the execution hash rather than living in a side channel that could
// drift from the events that reference it.
func (w *Writer) Intern(s string) (uint32, error) {
	if id, ok := w.dict.Lookup(s); ok {
		return id, nil
	}
	id := w.dict.Assign(s)
	payload := w.scratch[:0]
	payload = AppendUint32(payload, id)
	payload = AppendString(payload, s)
	w.scratch = payload
	if err := w.appendFrame(FrameHeader{
		SchemaID:      SchemaDictionary,
		SchemaVersion: 1,
	}, payload); err != nil {
		return 0, err
	}
	return id, nil
}

// Append writes one event. venueRef is a dictionary id from Intern, or 0.
func (w *Writer) Append(simTS int64, clientID uint64, venueRef uint32, payload PayloadAppender) error {
	if w.err != nil {
		return w.err
	}
	return w.appendFrame(FrameHeader{
		SimTS:         simTS,
		ClientID:      clientID,
		VenueRef:      venueRef,
		SchemaID:      payload.SchemaID(),
		SchemaVersion: payload.SchemaVersion(),
	}, nil, payload)
}

// AppendInterning writes one event whose payload resolves its own strings.
//
// The payload is encoded into a scratch buffer before the frame is opened, so
// any dictionary frame emitted while interning is written first and a reader
// never meets an id it has not yet learned. The cost is one copy of a payload
// that is tens of bytes; the alternative — interning mid-frame — would
// interleave a dictionary frame into the middle of an event frame and corrupt
// the stream.
func (w *Writer) AppendInterning(simTS int64, clientID uint64, venueRef uint32,
	payload InterningAppender) error {
	if w.err != nil {
		return w.err
	}
	if err := w.ensureHeader(); err != nil {
		return err
	}
	encoded, err := payload.AppendPayloadInterning(w.payload[:0], w)
	if err != nil {
		return err
	}
	w.payload = encoded
	return w.appendFrame(FrameHeader{
		SimTS:         simTS,
		ClientID:      clientID,
		VenueRef:      venueRef,
		SchemaID:      payload.SchemaID(),
		SchemaVersion: payload.SchemaVersion(),
	}, encoded)
}

// appendFrame writes a frame whose payload is either supplied directly (the
// dictionary case) or produced by an appender.
func (w *Writer) appendFrame(header FrameHeader, raw []byte, appender ...PayloadAppender) error {
	if w.closed {
		return errors.New("evstream: append after Close")
	}
	if w.err != nil {
		return w.err
	}
	if err := w.ensureHeader(); err != nil {
		return err
	}

	w.seq++
	header.Seq = w.seq

	start := len(w.frames)
	// Reserve the header, append the payload, then patch the length. Encoding
	// straight into the block buffer means a frame costs no allocation of its
	// own.
	w.frames = AppendFrameHeader(w.frames, header)
	if raw != nil {
		w.frames = append(w.frames, raw...)
	}
	for _, a := range appender {
		w.frames = a.AppendPayload(w.frames)
	}
	length := len(w.frames) - start
	binary.LittleEndian.PutUint32(w.frames[start:start+4], uint32(length))

	// The digest is taken over the concatenated canonical frames, absorbed in
	// block-sized writes at flush rather than one write per frame.
	//
	// The first version reset the hasher, absorbed the previous digest and the
	// frame, and summed — for every event. Measurement put that at 54 % of
	// encode cost for a complex event and 68 % for a simple one.
	//
	// Streaming each frame into one long-lived hasher fixed that, and an
	// earlier comment here rejected going further on the grounds that block
	// boundaries depend on a storage parameter. That reasoning conflated two
	// different things. A per-block digest that is then chained would indeed
	// depend on the block size. Absorbing the same bytes into the same
	// continuous hasher in larger Write calls does not: SHA-256 is a stream, so
	// Write(a); Write(b) and Write(a||b) produce the identical digest. The
	// result stays a pure function of the canonical byte sequence, independent
	// of block size, codec and buffering, and the per-call overhead and the
	// copy into the hasher's 64-byte staging buffer both disappear. Frames
	// accumulate in w.frames and are absorbed by flushBlock; ExecutionHash
	// covers whatever has not been flushed yet.

	w.stats.observe(header)
	w.frameCount++
	if len(w.frames) >= w.blockBytes {
		return w.flushBlock()
	}
	return nil
}

func (w *Writer) ensureHeader() error {
	if w.wroteHdr {
		return nil
	}
	var header [StreamHeaderSize]byte
	copy(header[0:8], Magic)
	binary.LittleEndian.PutUint16(header[8:10], FormatMajor)
	binary.LittleEndian.PutUint16(header[10:12], FormatMinor)
	codec := CodecNone
	if w.compressor != nil {
		codec = w.compressor.Codec()
	}
	header[12] = byte(codec)
	binary.LittleEndian.PutUint32(header[16:20], w.epoch)
	if _, err := w.out.Write(header[:]); err != nil {
		w.err = err
		return err
	}
	w.offset += StreamHeaderSize
	w.wroteHdr = true
	return nil
}

// flushBlock writes the open block: CRC over the uncompressed bytes, then the
// stored (possibly compressed) form.
func (w *Writer) flushBlock() error {
	if w.frameCount == 0 {
		return nil
	}
	uncompressed := w.frames
	// Absorb the block's frames into the execution digest before the buffer is
	// handed to a compressor or reset. This is the same byte sequence, in the
	// same order, that the per-frame version absorbed.
	w.hasher.Write(uncompressed)
	stored := uncompressed
	if w.compressor != nil {
		var err error
		stored, err = w.compressor.Compress(w.stored[:0], uncompressed)
		if err != nil {
			w.err = fmt.Errorf("evstream: compress block: %w", err)
			return w.err
		}
		w.stored = stored
	}

	var header [BlockHeaderSize]byte
	binary.LittleEndian.PutUint32(header[0:4], BlockMagic)
	binary.LittleEndian.PutUint32(header[4:8], uint32(len(uncompressed)))
	binary.LittleEndian.PutUint32(header[8:12], uint32(len(stored)))
	binary.LittleEndian.PutUint32(header[12:16], w.frameCount)
	// CRC over the UNCOMPRESSED bytes: it then verifies the data rather than
	// the compressor's output, and holds whatever codec was used.
	binary.LittleEndian.PutUint32(header[16:20], crc32.Checksum(uncompressed, crcTable))

	if _, err := w.out.Write(header[:]); err != nil {
		w.err = err
		return err
	}
	if _, err := w.out.Write(stored); err != nil {
		w.err = err
		return err
	}

	w.index.Blocks = append(w.index.Blocks,
		w.stats.descriptor(w.offset, uint32(len(stored)), uint32(len(uncompressed))))
	w.offset += uint64(BlockHeaderSize + len(stored))

	w.frames = w.frames[:0]
	w.frameCount = 0
	w.stats.reset()
	return nil
}

// Flush closes the open block. Call before relying on bytes being readable.
func (w *Writer) Flush() error {
	if w.err != nil {
		return w.err
	}
	if err := w.ensureHeader(); err != nil {
		return err
	}
	return w.flushBlock()
}

// ExecutionHash returns the digest over every frame written so far.
//
// Snapshotting mid-stream clones the hasher state rather than disturbing it, so
// a checkpoint costs one clone and the running digest continues undisturbed.
// It covers uncompressed canonical bytes, so it is identical whatever codec the
// stream was stored with and whatever block size was configured.
// Close flushes the open block and writes the stream trailer, which is what
// marks the stream complete. A stream without a trailer is a stream whose
// writer did not finish, and a reader is entitled to say so.
//
// It does not close the underlying writer: the caller owns that, and often
// wants to write an index beside it first.
func (w *Writer) Close() error {
	if err := w.Flush(); err != nil {
		return err
	}
	if err := w.ensureHeader(); err != nil {
		return err
	}
	digest := w.ExecutionHash()
	trailer := make([]byte, 0, TrailerSize)
	trailer = AppendUint32(trailer, TrailerMagic)
	trailer = AppendUint64(trailer, w.seq)
	trailer = append(trailer, digest[:]...)
	if _, err := w.out.Write(trailer); err != nil {
		return err
	}
	w.closed = true
	return nil
}

// ExecutionHash returns the digest over every canonical frame written so far,
// including frames still sitting in the open block. The hasher itself has only
// absorbed flushed blocks, so the running state is cloned and the unflushed
// tail absorbed into the clone — the caller gets the digest as of now without
// the writer's own state depending on when it was asked.
func (w *Writer) ExecutionHash() [sha256.Size]byte {
	var out [sha256.Size]byte
	snapshot, err := w.hasher.(encoding.BinaryMarshaler).MarshalBinary()
	if err != nil {
		// The standard library's SHA-256 does not fail here. A hasher that
		// cannot snapshot its state cannot have the unflushed tail absorbed
		// into a clone, so absorb it into the hasher itself: the digest is
		// correct, at the cost of the running state, which is the safer of the
		// two failures.
		w.hasher.Write(w.frames)
		w.frames = w.frames[:0]
		copy(out[:], w.hasher.Sum(nil))
		return out
	}
	clone := sha256.New()
	if err := clone.(encoding.BinaryUnmarshaler).UnmarshalBinary(snapshot); err != nil {
		// Same fallback as above: absorb the tail directly rather than return
		// a digest that silently omits it.
		w.hasher.Write(w.frames)
		w.frames = w.frames[:0]
		copy(out[:], w.hasher.Sum(nil))
		return out
	}
	clone.Write(w.frames)
	copy(out[:], clone.Sum(nil))
	return out
}

// Count returns the number of frames written, including dictionary frames.
func (w *Writer) Count() uint64 { return w.seq }

// Index returns the block directory built while writing. Valid after Flush.
func (w *Writer) Index() *Index { return &w.index }

// Dictionary exposes the interning table, which a selective reader needs in
// order to resolve references in blocks it did not read sequentially.
func (w *Writer) Dictionary() *Dictionary { return w.dict }
