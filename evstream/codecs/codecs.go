// Package codecs supplies block compressors for evstream.
//
// They are separate from evstream on purpose. Compression is a storage
// decision: the execution hash is taken over uncompressed canonical frames, so
// choosing a codec — or none — cannot change a scientific result. Keeping the
// codecs out of the format package makes that boundary structural rather than a
// convention someone has to remember.
package codecs

import (
	"fmt"

	"github.com/klauspost/compress/s2"
	"github.com/klauspost/compress/zstd"
	"github.com/pierrec/lz4/v4"

	"exchange_sim/evstream"
)

// LZ4 is the fastest of the three and the weakest compressor. It is the right
// default when a run is disk-bound on write throughput rather than capacity.
type LZ4 struct {
	compressor   lz4.Compressor
	decompressed []byte
}

// NewLZ4 returns an LZ4 block codec.
func NewLZ4() *LZ4 { return &LZ4{} }

func (c *LZ4) Codec() evstream.Codec { return evstream.CodecLZ4 }

func (c *LZ4) Compress(dst, src []byte) ([]byte, error) {
	bound := lz4.CompressBlockBound(len(src))
	if cap(dst) < bound {
		dst = make([]byte, bound)
	}
	dst = dst[:bound]
	n, err := c.compressor.CompressBlock(src, dst)
	if err != nil {
		return nil, err
	}
	if n == 0 {
		// LZ4 reports incompressible input as zero. Storing the raw bytes would
		// need a per-block flag the format does not have, so fall back to a
		// literal-only frame by refusing rather than silently changing shape.
		return nil, fmt.Errorf("lz4: block incompressible at %d bytes", len(src))
	}
	return dst[:n], nil
}

func (c *LZ4) Decompress(dst, src []byte, uncompressedLen int) ([]byte, error) {
	if cap(dst) < uncompressedLen {
		dst = make([]byte, uncompressedLen)
	}
	dst = dst[:uncompressedLen]
	n, err := lz4.UncompressBlock(src, dst)
	if err != nil {
		return nil, err
	}
	return dst[:n], nil
}

// S2 is Snappy's successor: Snappy-class speed with a better ratio, and it
// never fails on incompressible input.
type S2 struct{}

// NewS2 returns an S2 block codec.
func NewS2() *S2 { return &S2{} }

func (c *S2) Codec() evstream.Codec { return evstream.CodecS2 }

func (c *S2) Compress(dst, src []byte) ([]byte, error) {
	return s2.Encode(dst[:cap(dst)], src), nil
}

func (c *S2) Decompress(dst, src []byte, uncompressedLen int) ([]byte, error) {
	if cap(dst) < uncompressedLen {
		dst = make([]byte, uncompressedLen)
	}
	return s2.Decode(dst[:uncompressedLen], src)
}

// Zstd at its fastest level is the capacity option: markedly better ratio than
// LZ4 or S2 at a cost that is still small next to producing the events.
type Zstd struct {
	encoder *zstd.Encoder
	decoder *zstd.Decoder
}

// NewZstd returns a zstd block codec at the given level. Use
// zstd.SpeedFastest for the hot path; higher levels are for archival.
func NewZstd(level zstd.EncoderLevel) (*Zstd, error) {
	encoder, err := zstd.NewWriter(nil,
		zstd.WithEncoderLevel(level),
		zstd.WithEncoderConcurrency(1))
	if err != nil {
		return nil, err
	}
	decoder, err := zstd.NewReader(nil, zstd.WithDecoderConcurrency(1))
	if err != nil {
		return nil, err
	}
	return &Zstd{encoder: encoder, decoder: decoder}, nil
}

// NewZstdFastest is the configuration the benchmarks use.
func NewZstdFastest() (*Zstd, error) { return NewZstd(zstd.SpeedFastest) }

func (c *Zstd) Codec() evstream.Codec { return evstream.CodecZstd }

func (c *Zstd) Compress(dst, src []byte) ([]byte, error) {
	return c.encoder.EncodeAll(src, dst[:0]), nil
}

func (c *Zstd) Decompress(dst, src []byte, uncompressedLen int) ([]byte, error) {
	if cap(dst) < uncompressedLen {
		dst = make([]byte, 0, uncompressedLen)
	}
	return c.decoder.DecodeAll(src, dst[:0])
}

// Close releases the zstd worker state.
func (c *Zstd) Close() {
	c.encoder.Close()
	c.decoder.Close()
}
