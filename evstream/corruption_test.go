package evstream_test

import (
	"bytes"
	"errors"
	"hash"
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
