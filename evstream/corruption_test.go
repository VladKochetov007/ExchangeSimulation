package evstream_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash"
	"io"
	"testing"

	"exchange_sim/evstream"
)

type corruptionProbe struct{ value int64 }

func (corruptionProbe) SchemaID() uint16      { return evstream.FirstUserSchema }
func (corruptionProbe) SchemaVersion() uint16 { return 1 }
func (p corruptionProbe) AppendPayload(dst []byte) []byte {
	return evstream.AppendInt64(dst, p.value)
}

type markerProbe struct{ marker byte }

func (markerProbe) SchemaID() uint16      { return evstream.FirstUserSchema }
func (markerProbe) SchemaVersion() uint16 { return 1 }
func (p markerProbe) AppendPayload(dst []byte) []byte {
	return append(dst, p.marker)
}

type countingReader struct {
	*bytes.Reader
	bytesRead int
}

func (r *countingReader) Read(dst []byte) (int, error) {
	read, err := r.Reader.Read(dst)
	r.bytesRead += read
	return read, err
}

func oversizedBlockHeader(t *testing.T, uncompressedLen, storedLen uint32) []byte {
	t.Helper()
	stream := make([]byte, evstream.StreamHeaderSize+evstream.BlockHeaderSize)
	copy(stream[:8], evstream.Magic)
	binary.LittleEndian.PutUint16(stream[8:10], evstream.FormatMajor)
	binary.LittleEndian.PutUint32(stream[evstream.StreamHeaderSize:], evstream.BlockMagic)
	binary.LittleEndian.PutUint32(stream[evstream.StreamHeaderSize+4:], uncompressedLen)
	binary.LittleEndian.PutUint32(stream[evstream.StreamHeaderSize+8:], storedLen)
	return stream
}

func TestReaderRejectsOversizedBlockBeforePayloadRead(t *testing.T) {
	tooLarge := uint32(evstream.DefaultMaxBlockBytes + 1)
	tests := []struct {
		name            string
		uncompressedLen uint32
		storedLen       uint32
		maxUncompressed int
		maxStored       int
	}{
		{name: "uncompressed", uncompressedLen: tooLarge, storedLen: tooLarge},
		{name: "stored", uncompressedLen: 1, storedLen: tooLarge},
		{name: "configured-stored", uncompressedLen: 1, storedLen: 9, maxStored: 8, maxUncompressed: 8},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			input := oversizedBlockHeader(t, testCase.uncompressedLen, testCase.storedLen)
			counting := &countingReader{Reader: bytes.NewReader(input)}
			reader, err := evstream.NewReader(counting, evstream.ReaderOptions{
				MaxStoredBlockBytes:       testCase.maxStored,
				MaxUncompressedBlockBytes: testCase.maxUncompressed,
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := reader.Range(func(evstream.Frame) error { return nil }); !errors.Is(err, evstream.ErrCorrupt) {
				t.Fatalf("oversized block error = %v, want ErrCorrupt", err)
			}
			wantBytesRead := evstream.StreamHeaderSize + evstream.BlockHeaderSize
			if counting.bytesRead != wantBytesRead {
				t.Fatalf("reader consumed %d bytes before rejecting length, want %d", counting.bytesRead, wantBytesRead)
			}
		})
	}
}

type countingReaderAt struct {
	data  []byte
	reads int
}

func (reader *countingReaderAt) ReadAt(dst []byte, offset int64) (int, error) {
	reader.reads++
	return copy(dst, reader.data[offset:]), nil
}

func TestIndexedReaderRejectsOversizedDescriptorBeforeRead(t *testing.T) {
	source := &countingReaderAt{data: make([]byte, evstream.BlockHeaderSize)}
	reader := evstream.NewIndexedReader(source, evstream.CodecNone, evstream.NewDictionary(), nil)
	err := reader.RangeSelected([]evstream.BlockDescriptor{{StoredLen: uint32(evstream.DefaultMaxBlockBytes + 1)}}, evstream.Query{}, func(evstream.Frame) error {
		return nil
	})
	if !errors.Is(err, evstream.ErrCorrupt) {
		t.Fatalf("oversized indexed descriptor error = %v, want ErrCorrupt", err)
	}
	if source.reads != 0 {
		t.Fatalf("indexed reader performed %d source reads before rejecting descriptor", source.reads)
	}
}

func TestReadIndexRejectsOversizedDescriptorCountBeforeBodyRead(t *testing.T) {
	header := make([]byte, 8)
	binary.LittleEndian.PutUint32(header[0:4], evstream.IndexMagic)
	maxDescriptors := uint32((evstream.DefaultMaxIndexBytes - 8) / evstream.BlockDescriptorSize)
	binary.LittleEndian.PutUint32(header[4:8], maxDescriptors+1)
	if _, err := evstream.ReadIndex(bytes.NewReader(header)); !errors.Is(err, evstream.ErrCorrupt) {
		t.Fatalf("oversized index error = %v, want ErrCorrupt", err)
	}
}

type shortWriteSink struct {
	shortCall int
	calls     int
}

func (sink *shortWriteSink) Write(data []byte) (int, error) {
	sink.calls++
	if sink.calls == sink.shortCall {
		return len(data) - 1, nil
	}
	return len(data), nil
}

func TestWriterRejectsShortWrites(t *testing.T) {
	for _, shortCall := range []int{1, 2, 3, 4} {
		t.Run("write-"+string(rune('0'+shortCall)), func(t *testing.T) {
			sink := &shortWriteSink{shortCall: shortCall}
			writer := evstream.NewWriter(sink, evstream.WriterOptions{BlockBytes: 256})
			appendErr := writer.Append(1, 1, 0, corruptionProbe{value: 1})
			if shortCall == 1 {
				if !errors.Is(appendErr, io.ErrShortWrite) {
					t.Fatalf("append error = %v, want io.ErrShortWrite", appendErr)
				}
			} else if appendErr != nil {
				t.Fatalf("append error = %v", appendErr)
			}
			closeErr := writer.Close()
			if !errors.Is(closeErr, io.ErrShortWrite) {
				t.Fatalf("close error = %v, want io.ErrShortWrite", closeErr)
			}
			if sink.calls < shortCall {
				t.Fatalf("only %d writes reached, wanted short write call %d", sink.calls, shortCall)
			}
		})
	}
}

func TestIndexWriteToRejectsShortWrite(t *testing.T) {
	sink := &shortWriteSink{shortCall: 1}
	index := evstream.Index{Blocks: []evstream.BlockDescriptor{{Offset: 32}}}
	written, err := index.WriteTo(sink)
	if !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("index write error = %v, want io.ErrShortWrite", err)
	}
	if want := int64(8 + evstream.BlockDescriptorSize - 1); written != want {
		t.Fatalf("index reported %d bytes, want %d", written, want)
	}
}

func hashIgnoringLastByte(digest hash.Hash, frame []byte) {
	if len(frame) == 0 {
		return
	}
	_, _ = digest.Write(frame[:len(frame)-1])
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

func TestCustomFrameHashProjectionMatchesWriterAndReader(t *testing.T) {
	writeStream := func(marker byte) ([]byte, [32]byte) {
		var output bytes.Buffer
		writer := evstream.NewWriter(&output, evstream.WriterOptions{HashFrame: hashIgnoringLastByte})
		if err := writer.Append(1, 1, 0, markerProbe{marker: marker}); err != nil {
			t.Fatalf("append marker %d: %v", marker, err)
		}
		if err := writer.Close(); err != nil {
			t.Fatalf("close marker %d: %v", marker, err)
		}
		return output.Bytes(), writer.ExecutionHash()
	}

	first, firstHash := writeStream(1)
	second, secondHash := writeStream(2)
	if bytes.Equal(first, second) {
		t.Fatal("different projected fields produced identical streams")
	}
	if firstHash != secondHash {
		t.Fatalf("writer projection hashes differ: %x versus %x", firstHash, secondHash)
	}
	firstReader, err := evstream.NewReader(bytes.NewReader(first), evstream.ReaderOptions{
		VerifyHash: true,
		HashFrame:  hashIgnoringLastByte,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := firstReader.Range(func(evstream.Frame) error { return nil }); err != nil {
		t.Fatal(err)
	}
	secondReader, err := evstream.NewReader(bytes.NewReader(second), evstream.ReaderOptions{
		VerifyHash: true,
		HashFrame:  hashIgnoringLastByte,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := secondReader.Range(func(evstream.Frame) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if firstReader.RawExecutionHash() == secondReader.RawExecutionHash() {
		t.Fatal("raw canonical hashes ignored the projected marker")
	}
	for _, stream := range [][]byte{first, second} {
		reader, err := evstream.NewReader(bytes.NewReader(stream), evstream.ReaderOptions{
			VerifyHash: true,
			HashFrame:  hashIgnoringLastByte,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := reader.Range(func(evstream.Frame) error { return nil }); err != nil {
			t.Fatal(err)
		}
		if reader.ExecutionHash() != firstHash {
			t.Fatalf("reader projection hash = %x, want %x", reader.ExecutionHash(), firstHash)
		}
	}
}
