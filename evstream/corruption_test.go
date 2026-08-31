package evstream_test

import (
	"bytes"
	"errors"
	"testing"

	"exchange_sim/evstream"
	"exchange_sim/evstream/codecs"
)

// The format advertises corruption detection, streaming verification and
// gap-free sequencing. Until now those guarantees were exercised only
// indirectly, through schema tests in another package that happened to read
// what they wrote. A guarantee nothing tests directly is a guarantee nobody has
// checked: these tests attack the bytes themselves.

type probe struct{ value int64 }

func (probe) SchemaID() uint16      { return evstream.FirstUserSchema }
func (probe) SchemaVersion() uint16 { return 1 }

func (p probe) AppendPayload(dst []byte) []byte {
	return evstream.AppendInt64(dst, p.value)
}

func writeStream(t *testing.T, frames int, compressor evstream.BlockCompressor) []byte {
	t.Helper()
	var buf bytes.Buffer
	writer := evstream.NewWriter(&buf, evstream.WriterOptions{
		BlockBytes: 512, Compressor: compressor,
	})
	for i := 0; i < frames; i++ {
		if err := writer.Append(int64(i)*1e6, uint64(i%3), 0, probe{value: int64(i)}); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return buf.Bytes()
}

func readAll(data []byte) (uint64, error) { return readAllWith(data, nil) }

func readAllWith(data []byte, decompressor evstream.BlockDecompressor) (uint64, error) {
	reader, err := evstream.NewReader(bytes.NewReader(data), evstream.ReaderOptions{
		VerifyHash: true, Decompressor: decompressor,
	})
	if err != nil {
		return 0, err
	}
	frames := uint64(0)
	err = reader.Range(func(evstream.Frame) error {
		frames++
		return nil
	})
	return frames, err
}

// Every single-byte corruption in the payload region must be caught. A CRC that
// catches most flips is not corruption detection.
func TestEverySingleByteFlipIsDetected(t *testing.T) {
	clean := writeStream(t, 200, nil)
	if _, err := readAll(clean); err != nil {
		t.Fatalf("clean stream did not read: %v", err)
	}
	missed := 0
	for offset := evstream.StreamHeaderSize; offset < len(clean); offset++ {
		corrupt := append([]byte(nil), clean...)
		corrupt[offset] ^= 0x01
		if _, err := readAll(corrupt); err == nil {
			missed++
			if missed <= 3 {
				t.Errorf("a flipped bit at offset %d was not detected", offset)
			}
		}
	}
	if missed > 0 {
		t.Fatalf("%d of %d single-bit corruptions went undetected",
			missed, len(clean)-evstream.StreamHeaderSize)
	}
}

// Truncation at any point must fail rather than silently returning a prefix.
// A reader that reports success on a half-written file turns a crashed run into
// a shorter, apparently valid one.
func TestTruncationIsNeverSilent(t *testing.T) {
	clean := writeStream(t, 200, nil)
	full, err := readAll(clean)
	if err != nil {
		t.Fatalf("clean stream: %v", err)
	}
	for cut := 1; cut < len(clean); cut++ {
		frames, err := readAll(clean[:cut])
		if err == nil && frames != full {
			t.Fatalf("truncating to %d of %d bytes returned %d frames and no error",
				cut, len(clean), frames)
		}
	}
}

func TestBadMagicIsRejected(t *testing.T) {
	clean := writeStream(t, 10, nil)
	clean[0] ^= 0xFF
	if _, err := readAll(clean); !errors.Is(err, evstream.ErrBadMagic) {
		t.Fatalf("bad magic gave %v, want ErrBadMagic", err)
	}
}

// Corruption detection must not depend on the codec, since the codec is a
// storage choice and the guarantee is a format property.
func TestCorruptionIsDetectedUnderEveryCodec(t *testing.T) {
	zstd, err := codecs.NewZstdFastest()
	if err != nil {
		t.Fatalf("zstd: %v", err)
	}
	for name, compressor := range map[string]evstream.BlockCompressor{
		"none": nil, "lz4": codecs.NewLZ4(), "s2": codecs.NewS2(), "zstd": zstd,
	} {
		clean := writeStream(t, 200, compressor)
		decompressor, _ := compressor.(evstream.BlockDecompressor)
		if _, err := readAllWith(clean, decompressor); err != nil {
			t.Fatalf("%s: clean stream did not read: %v", name, err)
		}
		// One flip in the middle of the body, well past the header.
		corrupt := append([]byte(nil), clean...)
		corrupt[len(corrupt)/2] ^= 0x01
		if _, err := readAllWith(corrupt, decompressor); err == nil {
			t.Fatalf("%s: a flipped bit in the block body was not detected", name)
		}
	}
}

// A stream that ends without its trailer is a stream whose writer did not
// finish. Before the trailer existed, truncating at a block boundary produced a
// shorter stream that was entirely valid, because every block it still
// contained was intact — the tail simply vanished without trace.
func TestUnterminatedStreamIsRejectedUnlessAsked(t *testing.T) {
	clean := writeStream(t, 200, nil)
	withoutTrailer := clean[:len(clean)-evstream.TrailerSize]

	if _, err := readAll(withoutTrailer); !errors.Is(err, evstream.ErrShortBuffer) {
		t.Fatalf("a stream missing its trailer read as %v, want ErrShortBuffer", err)
	}
	frames, err := readAllWith2(withoutTrailer, evstream.ReaderOptions{AllowUnterminated: true})
	if err != nil {
		t.Fatalf("a run in progress must still be readable on request: %v", err)
	}
	if frames != 200 {
		t.Fatalf("read %d frames from the untrailed stream, want 200", frames)
	}
}

// A trailer that disagrees with what was read must fail, otherwise it is
// decoration rather than a check.
func TestTrailerMustAgreeWithTheFramesRead(t *testing.T) {
	clean := writeStream(t, 200, nil)
	tampered := append([]byte(nil), clean...)
	// The frame count sits immediately after the trailer magic.
	tampered[len(tampered)-evstream.TrailerSize+4] ^= 0x01
	if _, err := readAll(tampered); err == nil {
		t.Fatal("a trailer declaring the wrong frame count was accepted")
	}
}

func readAllWith2(data []byte, opts evstream.ReaderOptions) (uint64, error) {
	opts.VerifyHash = true
	reader, err := evstream.NewReader(bytes.NewReader(data), opts)
	if err != nil {
		return 0, err
	}
	frames := uint64(0)
	err = reader.Range(func(evstream.Frame) error {
		frames++
		return nil
	})
	return frames, err
}
