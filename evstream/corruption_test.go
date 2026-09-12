package evstream_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"testing"

	"exchange_sim/evstream"
)

type corruptionProbe struct{ value int64 }

func (corruptionProbe) SchemaID() uint16      { return evstream.FirstUserSchema }
func (corruptionProbe) SchemaVersion() uint16 { return 1 }
func (p corruptionProbe) AppendPayload(dst []byte) []byte {
	return evstream.AppendInt64(dst, p.value)
}

func writeCompleteProbeStream(t *testing.T, count int) []byte {
	t.Helper()
	var output bytes.Buffer
	writer := evstream.NewWriter(&output, evstream.WriterOptions{BlockBytes: 256})
	for index := 0; index < count; index++ {
		if err := writer.Append(int64(index), uint64(index), 0, corruptionProbe{value: int64(index)}); err != nil {
			t.Fatalf("append %d: %v", index, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return output.Bytes()
}

func readProbeStream(data []byte, allowUnterminated bool) (uint64, error) {
	reader, err := evstream.NewReader(bytes.NewReader(data), evstream.ReaderOptions{
		VerifyHash:        true,
		AllowUnterminated: allowUnterminated,
	})
	if err != nil {
		return 0, err
	}
	var events uint64
	err = reader.Range(func(evstream.Frame) error {
		events++
		return nil
	})
	return events, err
}

func TestCompleteStreamRequiresAndValidatesTrailer(t *testing.T) {
	complete := writeCompleteProbeStream(t, 200)
	if events, err := readProbeStream(complete, false); err != nil || events != 200 {
		t.Fatalf("complete stream = events %d, err %v", events, err)
	}

	withoutTrailer := complete[:len(complete)-evstream.TrailerSize]
	if _, err := readProbeStream(withoutTrailer, false); !errors.Is(err, evstream.ErrShortBuffer) {
		t.Fatalf("unterminated stream error = %v, want ErrShortBuffer", err)
	}
	if events, err := readProbeStream(withoutTrailer, true); err != nil || events != 200 {
		t.Fatalf("allowed unterminated stream = events %d, err %v", events, err)
	}
}

func TestEverySingleByteCorruptionIsRejected(t *testing.T) {
	clean := writeCompleteProbeStream(t, 200)
	for offset := evstream.StreamHeaderSize; offset < len(clean); offset++ {
		corrupt := append([]byte(nil), clean...)
		corrupt[offset] ^= 1
		if _, err := readProbeStream(corrupt, false); err == nil {
			t.Fatalf("byte flip at offset %d was accepted", offset)
		}
	}
}

func TestTrailerFrameCountAndMagicAreChecked(t *testing.T) {
	clean := writeCompleteProbeStream(t, 20)
	countTampered := append([]byte(nil), clean...)
	countTampered[len(countTampered)-evstream.TrailerSize+4] ^= 1
	if _, err := readProbeStream(countTampered, false); err == nil {
		t.Fatal("tampered trailer frame count was accepted")
	}

	badMagic := append([]byte(nil), clean...)
	badMagic[0] ^= 0xff
	if _, err := readProbeStream(badMagic, false); !errors.Is(err, evstream.ErrBadMagic) {
		t.Fatalf("bad stream magic error = %v, want ErrBadMagic", err)
	}
}

func TestReservedStreamHeaderBytesAreRejected(t *testing.T) {
	clean := writeCompleteProbeStream(t, 1)
	for _, offset := range []int{13, 15, 20, 31} {
		corrupt := append([]byte(nil), clean...)
		corrupt[offset] = 1
		if _, err := readProbeStream(corrupt, false); !errors.Is(err, evstream.ErrCorrupt) {
			t.Fatalf("reserved header byte %d error = %v, want ErrCorrupt", offset, err)
		}
	}
}

func TestWriterRejectsAppendAfterClose(t *testing.T) {
	var output bytes.Buffer
	writer := evstream.NewWriter(&output, evstream.WriterOptions{})
	if err := writer.Close(); err != nil {
		t.Fatalf("close empty stream: %v", err)
	}
	if err := writer.Append(1, 1, 0, corruptionProbe{}); err == nil {
		t.Fatal("append after close was accepted")
	}
	if _, err := writer.Intern("late"); err == nil {
		t.Fatal("intern after close was accepted")
	}
	if err := writer.Flush(); err == nil {
		t.Fatal("flush after close was accepted")
	}
}

func TestWriterRejectsClientIDThatCannotBeEncoded(t *testing.T) {
	var output bytes.Buffer
	writer := evstream.NewWriter(&output, evstream.WriterOptions{})
	if err := writer.Append(1, evstream.MaxEncodedClientID, 0, corruptionProbe{}); err != nil {
		t.Fatalf("maximum encodable client ID rejected: %v", err)
	}
	if err := writer.Append(2, evstream.MaxEncodedClientID+1, 0, corruptionProbe{}); !errors.Is(err, evstream.ErrClientIDOverflow) {
		t.Fatalf("overflow client ID error = %v, want ErrClientIDOverflow", err)
	}
	if writer.Count() != 1 {
		t.Fatalf("overflow append advanced frame count to %d, want 1", writer.Count())
	}
}

func TestReaderRejectsBytesAfterCompletionTrailer(t *testing.T) {
	complete := writeCompleteProbeStream(t, 1)
	complete = append(complete, 0x01)
	if _, err := readProbeStream(complete, false); err == nil {
		t.Fatal("trailing bytes after completion trailer were accepted")
	}
}

func TestReaderRejectsOversizedBlockBeforeReadingPayload(t *testing.T) {
	stream := writeCompleteProbeStream(t, 1)
	binary.LittleEndian.PutUint32(stream[evstream.StreamHeaderSize+4:evstream.StreamHeaderSize+8], ^uint32(0))
	binary.LittleEndian.PutUint32(stream[evstream.StreamHeaderSize+8:evstream.StreamHeaderSize+12], ^uint32(0))

	reader, err := evstream.NewReader(bytes.NewReader(stream[:evstream.StreamHeaderSize+evstream.BlockHeaderSize]), evstream.ReaderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := readReaderFrames(reader); !errors.Is(err, evstream.ErrCorrupt) {
		t.Fatalf("oversized block error = %v, want ErrCorrupt", err)
	}
}

type ratioProbeDecompressor struct{ called bool }

func (*ratioProbeDecompressor) Codec() evstream.Codec { return evstream.CodecLZ4 }

func (d *ratioProbeDecompressor) Decompress(dst, src []byte, uncompressedLen int) ([]byte, error) {
	d.called = true
	return dst, nil
}

func TestReaderRejectsCompressionExpansionBeforeDecompression(t *testing.T) {
	stream := make([]byte, evstream.StreamHeaderSize+evstream.BlockHeaderSize)
	copy(stream[:8], evstream.Magic)
	binary.LittleEndian.PutUint16(stream[8:10], evstream.FormatMajor)
	binary.LittleEndian.PutUint16(stream[10:12], evstream.FormatMinor)
	stream[12] = byte(evstream.CodecLZ4)
	binary.LittleEndian.PutUint32(stream[evstream.StreamHeaderSize:evstream.StreamHeaderSize+4], evstream.BlockMagic)
	binary.LittleEndian.PutUint32(stream[evstream.StreamHeaderSize+4:evstream.StreamHeaderSize+8], 3)
	binary.LittleEndian.PutUint32(stream[evstream.StreamHeaderSize+8:evstream.StreamHeaderSize+12], 1)

	decompressor := &ratioProbeDecompressor{}
	reader, err := evstream.NewReader(bytes.NewReader(stream), evstream.ReaderOptions{
		Decompressor: decompressor,
		Limits:       evstream.BlockReadLimits{MaxCompressionRatio: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := readReaderFrames(reader); !errors.Is(err, evstream.ErrCorrupt) {
		t.Fatalf("expansion-ratio error = %v, want ErrCorrupt", err)
	}
	if decompressor.called {
		t.Fatal("decompressor was called before the expansion ratio was validated")
	}
}

func TestIndexedReaderRejectsOversizedDescriptorBeforeReading(t *testing.T) {
	reader := evstream.NewIndexedReader(bytes.NewReader(nil), evstream.CodecNone, evstream.NewDictionary(), nil)
	err := reader.RangeSelected([]evstream.BlockDescriptor{{
		StoredLen:       ^uint32(0),
		UncompressedLen: ^uint32(0),
	}}, evstream.Query{}, func(evstream.Frame) error { return nil })
	if !errors.Is(err, evstream.ErrCorrupt) {
		t.Fatalf("oversized descriptor error = %v, want ErrCorrupt", err)
	}
}

func TestReaderRejectsDuplicateDictionaryValue(t *testing.T) {
	stream := writeDictionaryProbeStream(t)
	blockOffset := evstream.StreamHeaderSize
	storedLength := int(binary.LittleEndian.Uint32(stream[blockOffset+8 : blockOffset+12]))
	blockStart := blockOffset + evstream.BlockHeaderSize
	blockEnd := blockStart + storedLength
	frames := stream[blockStart:blockEnd]
	var firstValue []byte
	var dictionaryCount int
	for offset := 0; offset < len(frames); {
		header, err := evstream.ParseFrameHeader(frames[offset:])
		if err != nil {
			t.Fatalf("parse frame at %d: %v", offset, err)
		}
		frameLength := int(header.Length)
		if frameLength > len(frames)-offset {
			t.Fatalf("frame at %d overruns probe block", offset)
		}
		if header.SchemaID == evstream.SchemaDictionary {
			dictionaryCount++
			valueLength := int(binary.LittleEndian.Uint32(frames[offset+evstream.FrameHeaderSize+4 : offset+evstream.FrameHeaderSize+8]))
			valueStart := offset + evstream.FrameHeaderSize + 8
			valueEnd := valueStart + valueLength
			if valueEnd > offset+frameLength {
				t.Fatalf("dictionary value at %d overruns frame", offset)
			}
			if dictionaryCount == 1 {
				firstValue = append([]byte(nil), frames[valueStart:valueEnd]...)
			} else if dictionaryCount == 2 {
				if len(firstValue) != valueLength {
					t.Fatalf("probe dictionary values have different lengths: %d and %d", len(firstValue), valueLength)
				}
				copy(frames[valueStart:valueEnd], firstValue)
			}
		}
		offset += frameLength
	}
	if dictionaryCount != 2 {
		t.Fatalf("dictionary frame count = %d, want 2", dictionaryCount)
	}
	binary.LittleEndian.PutUint32(stream[blockOffset+16:blockOffset+20], crc32.Checksum(frames, crc32.MakeTable(crc32.Castagnoli)))
	digest := sha256.Sum256(frames)
	trailerOffset := blockEnd
	copy(stream[trailerOffset+12:trailerOffset+12+sha256.Size], digest[:])
	if _, err := readProbeStream(stream, false); !errors.Is(err, evstream.ErrCorrupt) {
		t.Fatalf("duplicate dictionary value error = %v, want ErrCorrupt", err)
	}
}

func writeDictionaryProbeStream(t *testing.T) []byte {
	t.Helper()
	var output bytes.Buffer
	writer := evstream.NewWriter(&output, evstream.WriterOptions{BlockBytes: 1024})
	first, err := writer.Intern("aa")
	if err != nil {
		t.Fatalf("intern first value: %v", err)
	}
	second, err := writer.Intern("bb")
	if err != nil {
		t.Fatalf("intern second value: %v", err)
	}
	if first != 1 || second != 2 {
		t.Fatalf("dictionary ids = %d, %d, want 1, 2", first, second)
	}
	if err := writer.Append(1, 1, second, corruptionProbe{value: 7}); err != nil {
		t.Fatalf("append probe: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close probe: %v", err)
	}
	return output.Bytes()
}

func readReaderFrames(reader *evstream.Reader) (uint64, error) {
	var frames uint64
	err := reader.Range(func(evstream.Frame) error {
		frames++
		return nil
	})
	return frames, err
}
