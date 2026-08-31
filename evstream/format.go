// Package evstream is a deterministic, typed, append-only binary event stream.
//
// It exists to replace JSON on the simulator's evidence hot path. Measurement
// on the integrated workload put encoding/json at 14.99 % of CPU with logging
// switched off entirely, every byte of it produced to feed the ordered
// execution-stream digest, plus 3.61 % in SHA-256 over 978 MB of JSON per
// simulated hour. Making that JSON faster was tried and rejected: a
// byte-identical hand-written encoder removed the reflection and changed
// nothing measurable, because the byte volume, not the encoder, is the cost.
// This format removes the bytes.
//
// # What the format guarantees
//
// Canonical bytes. Every value has exactly one encoding: fixed-width
// little-endian integers, no varints, no floats in payloads, no maps. Two runs
// that produce the same events produce identical streams, byte for byte, on
// any platform. That is what makes the stream itself hashable as the identity
// of a trajectory.
//
// Ordering. Frames carry a gap-free monotonic event_seq and are written in
// exactly the order they occurred. Global causal order is recoverable from the
// stream alone, and survives into every derived representation.
//
// Separation of identity from storage. The execution hash chains over
// UNCOMPRESSED canonical frames. Compression is applied per block afterwards
// and is a storage decision only: the same run compressed with zstd, with lz4,
// or not at all has the same hash. Changing codec must never change a
// scientific result.
//
// Evolution. Every frame is length-prefixed and carries a schema id and
// version, so a reader that does not know a schema skips it exactly rather
// than failing. Adding an event family does not invalidate old streams, and
// old readers do not break on new ones.
//
// Corruption detection and streaming verification. Each block carries its
// frame count and a CRC-32C over its uncompressed bytes, so a reader can
// verify incrementally, without buffering the stream and without trusting the
// compressor.
//
// # What this package deliberately does not do
//
// It contains no schema registry that has to be edited to add an event.
// Payload encoding lives on the value being written (PayloadAppender), and
// decoding is supplied by the caller as a SchemaSet. A new event family is
// added entirely outside this package.
package evstream

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// Wire constants. These are the format; changing any of them is a format
// version change, not a refactor.
const (
	// Magic identifies the stream and its byte order.
	Magic = "EXSIMEVS"

	// FormatMajor changes only when old readers must refuse the stream.
	FormatMajor uint16 = 1
	// FormatMinor changes when a reader that ignores unknown frames still works.
	FormatMinor uint16 = 0

	// StreamHeaderSize is fixed so a reader can seek past it without parsing.
	StreamHeaderSize = 32

	// BlockHeaderSize precedes every block's stored payload.
	BlockHeaderSize = 20
	// BlockMagic marks a block boundary, so a truncated stream is detectable
	// and a corrupted length cannot be silently mistaken for a valid block.
	BlockMagic uint32 = 0x42535645 // "EVSB" little-endian
	// TrailerMagic marks the end of a complete stream. Without a terminator a
	// stream truncated at a block boundary can look like a valid shorter run.
	TrailerMagic uint32 = 0x54535645 // "EVST" little-endian
	// TrailerSize is magic, frame count and the final execution digest.
	TrailerSize = 4 + 8 + 32

	// FrameHeaderSize is the fixed prefix of every frame.
	FrameHeaderSize = 32

	// SchemaDictionary is reserved: dictionary entries travel as frames in the
	// same ordered stream, so interning is covered by the execution hash and
	// needs no side channel.
	SchemaDictionary uint16 = 0

	// SchemaOpaqueJSON carries a payload that has no typed schema yet, as
	// length-prefixed JSON inside an ordinary frame. It exists so a stream is
	// uniformly binary from the first day: rare families ride opaque, keeping
	// ordering, framing, indexing and the digest uniform, and each is promoted
	// to a typed schema later without disturbing anything around it.
	SchemaOpaqueJSON uint16 = 1

	// FirstUserSchema is the lowest id available to callers.
	FirstUserSchema uint16 = 16
)

// Codec identifies the block compression applied to stored bytes. It is
// recorded in the stream header for readers; it never affects the hash.
type Codec uint8

const (
	CodecNone Codec = 0
	CodecLZ4  Codec = 1
	CodecS2   Codec = 2
	CodecZstd Codec = 3
)

func (c Codec) String() string {
	switch c {
	case CodecNone:
		return "none"
	case CodecLZ4:
		return "lz4"
	case CodecS2:
		return "s2"
	case CodecZstd:
		return "zstd"
	}
	return fmt.Sprintf("codec(%d)", uint8(c))
}

var (
	// ErrBadMagic means the bytes are not an evstream, or the stream is
	// misaligned — distinct from a corrupt block, which is ErrCorrupt.
	ErrBadMagic = errors.New("evstream: bad magic")
	// ErrUnsupportedVersion means the major version is newer than this reader.
	ErrUnsupportedVersion = errors.New("evstream: unsupported format version")
	// ErrCorrupt covers a CRC mismatch, an impossible length, or a frame count
	// that disagrees with the frames actually present.
	ErrCorrupt = errors.New("evstream: corrupt stream")
	// ErrShortBuffer means the stream ended mid-structure.
	ErrShortBuffer = errors.New("evstream: truncated stream")
	// ErrSequence means event_seq was not gap-free and monotonic, which would
	// make global ordering unrecoverable.
	ErrSequence = errors.New("evstream: non-monotonic event sequence")
)

// FrameHeader is the fixed prefix every frame carries.
//
// Length first, so an unknown schema can be skipped without understanding it —
// that is what lets a stream outlive the reader that was current when it was
// written.
type FrameHeader struct {
	// Length counts the whole frame, including this field.
	Length uint32
	// Seq is the gap-free global event number.
	Seq uint64
	// SimTS is simulated time in nanoseconds.
	SimTS int64
	// SchemaID names the event family; SchemaVersion its layout revision.
	SchemaID      uint16
	SchemaVersion uint16
	// VenueRef is a dictionary id, or 0 for none. Interning a venue or symbol
	// costs four bytes per event instead of the string.
	VenueRef uint32
	// ClientID is the account the event belongs to, or 0 for venue-level.
	ClientID uint64
}

// AppendFrameHeader writes the header in canonical form. The frame length is
// patched by the writer once the payload length is known.
func AppendFrameHeader(dst []byte, h FrameHeader) []byte {
	var scratch [FrameHeaderSize]byte
	binary.LittleEndian.PutUint32(scratch[0:4], h.Length)
	binary.LittleEndian.PutUint64(scratch[4:12], h.Seq)
	binary.LittleEndian.PutUint64(scratch[12:20], uint64(h.SimTS))
	binary.LittleEndian.PutUint16(scratch[20:22], h.SchemaID)
	binary.LittleEndian.PutUint16(scratch[22:24], h.SchemaVersion)
	binary.LittleEndian.PutUint32(scratch[24:28], h.VenueRef)
	// ClientID's high half is not stored: the remaining four bytes hold its low
	// word, and the field is documented as a 32-bit account space. Keeping the
	// header at 32 bytes keeps frames cache-friendly.
	binary.LittleEndian.PutUint32(scratch[28:32], uint32(h.ClientID))
	return append(dst, scratch[:]...)
}

// ParseFrameHeader reads a header from the front of src.
func ParseFrameHeader(src []byte) (FrameHeader, error) {
	if len(src) < FrameHeaderSize {
		return FrameHeader{}, ErrShortBuffer
	}
	h := FrameHeader{
		Length:        binary.LittleEndian.Uint32(src[0:4]),
		Seq:           binary.LittleEndian.Uint64(src[4:12]),
		SimTS:         int64(binary.LittleEndian.Uint64(src[12:20])),
		SchemaID:      binary.LittleEndian.Uint16(src[20:22]),
		SchemaVersion: binary.LittleEndian.Uint16(src[22:24]),
		VenueRef:      binary.LittleEndian.Uint32(src[24:28]),
		ClientID:      uint64(binary.LittleEndian.Uint32(src[28:32])),
	}
	if h.Length < FrameHeaderSize {
		return FrameHeader{}, fmt.Errorf("%w: frame length %d below header size", ErrCorrupt, h.Length)
	}
	return h, nil
}

// Interner assigns a dictionary id to a repeated string, emitting a dictionary
// frame the first time it sees one. Passed to an encoder so a value can intern
// its own strings without knowing anything about framing.
type Interner interface {
	Intern(string) (uint32, error)
}

// Resolver turns a dictionary id back into the string it stands for. The reader
// implements it; decoders take it as an interface so a schema can be decoded
// without a live stream.
type Resolver interface {
	Lookup(uint32) (string, bool)
}

// InterningAppender is a payload that resolves its own strings while encoding.
//
// Most event families carry symbols, assets, wallets or reasons drawn from a
// small fixed set, and interning them is what turns a twenty-byte symbol into
// four bytes. Encoding and interning happen together rather than in two passes
// because a value type has nowhere to store the ids between them.
//
// The writer encodes into a scratch buffer first, so dictionary frames emitted
// during encoding land in the stream ahead of the event frame that references
// them — a reader always learns an id before it is used.
type InterningAppender interface {
	SchemaID() uint16
	SchemaVersion() uint16
	// AppendPayloadInterning appends the canonical payload to dst, resolving
	// strings through in.
	AppendPayloadInterning(dst []byte, in Interner) ([]byte, error)
}

// PayloadAppender is implemented by the value being written. Encoding lives on
// the type rather than in a table here, so a new event family is added without
// editing this package.
//
// The appended bytes must be canonical: identical values must produce identical
// bytes on every platform and every run. Use the Append* helpers below, and do
// not encode a Go map, a float, or anything whose iteration order or
// representation is not fixed.
type PayloadAppender interface {
	// AppendPayload appends this value's canonical payload to dst.
	AppendPayload(dst []byte) []byte
	// SchemaID and SchemaVersion identify the layout of those bytes.
	SchemaID() uint16
	SchemaVersion() uint16
}

// Canonical primitives. Fixed width, little-endian, no varints: a varint has
// more than one representation for the same number unless minimality is
// enforced everywhere, and the size it saves is recovered by block compression,
// which is where size belongs.

// AppendInt64 appends a signed 64-bit value — the representation for every
// price and every fixed-point quantity in this simulator.
func AppendInt64(dst []byte, v int64) []byte {
	return binary.LittleEndian.AppendUint64(dst, uint64(v))
}

// AppendUint64 appends an unsigned 64-bit value.
func AppendUint64(dst []byte, v uint64) []byte {
	return binary.LittleEndian.AppendUint64(dst, v)
}

// AppendUint32 appends an unsigned 32-bit value, used for dictionary refs and
// enum-like fields.
func AppendUint32(dst []byte, v uint32) []byte {
	return binary.LittleEndian.AppendUint32(dst, v)
}

// AppendUint16 appends an unsigned 16-bit value.
func AppendUint16(dst []byte, v uint16) []byte {
	return binary.LittleEndian.AppendUint16(dst, v)
}

// AppendBool appends a boolean as one byte, 0 or 1. No other value is legal,
// so a decoder can reject anything else as corruption.
func AppendBool(dst []byte, v bool) []byte {
	if v {
		return append(dst, 1)
	}
	return append(dst, 0)
}

// AppendBytes appends a length-prefixed byte string. Used for values that
// genuinely vary; anything drawn from a small fixed set should be interned
// through the dictionary instead.
func AppendBytes(dst []byte, v []byte) []byte {
	dst = binary.LittleEndian.AppendUint32(dst, uint32(len(v)))
	return append(dst, v...)
}

// AppendString is AppendBytes for a string, without copying it first.
func AppendString(dst []byte, v string) []byte {
	dst = binary.LittleEndian.AppendUint32(dst, uint32(len(v)))
	return append(dst, v...)
}

// PresenceBits sizes the optional-field bitmap for a schema with n optional
// fields. Optionality is explicit and positional: a decoder never has to guess
// whether a zero means absent or means zero.
func PresenceBits(n int) int { return (n + 7) / 8 }

// Cursor reads canonical primitives from a payload, tracking position and the
// first error so a decoder reads straight through and checks once at the end.
type Cursor struct {
	buf []byte
	pos int
	err error
}

// NewCursor starts reading src.
func NewCursor(src []byte) *Cursor { return &Cursor{buf: src} }

// Err returns the first error encountered, if any.
func (c *Cursor) Err() error { return c.err }

// Offset reports how many bytes have been consumed from the cursor.
func (c *Cursor) Offset() int { return c.pos }

// Remaining reports how many bytes are unread. A decoder should require this
// to be zero at the end: trailing bytes mean the payload does not match the
// schema version it claims.
func (c *Cursor) Remaining() int { return len(c.buf) - c.pos }

func (c *Cursor) take(n int) []byte {
	if c.err != nil {
		return nil
	}
	if c.pos+n > len(c.buf) {
		c.err = ErrShortBuffer
		return nil
	}
	out := c.buf[c.pos : c.pos+n]
	c.pos += n
	return out
}

// Int64 reads a signed 64-bit value.
func (c *Cursor) Int64() int64 {
	b := c.take(8)
	if b == nil {
		return 0
	}
	return int64(binary.LittleEndian.Uint64(b))
}

// Uint64 reads an unsigned 64-bit value.
func (c *Cursor) Uint64() uint64 {
	b := c.take(8)
	if b == nil {
		return 0
	}
	return binary.LittleEndian.Uint64(b)
}

// Uint32 reads an unsigned 32-bit value.
func (c *Cursor) Uint32() uint32 {
	b := c.take(4)
	if b == nil {
		return 0
	}
	return binary.LittleEndian.Uint32(b)
}

// Uint16 reads an unsigned 16-bit value.
func (c *Cursor) Uint16() uint16 {
	b := c.take(2)
	if b == nil {
		return 0
	}
	return binary.LittleEndian.Uint16(b)
}

// Uint8 reads a single byte, used for enum-like fields that will never need
// more than 256 values.
func (c *Cursor) Uint8() uint8 {
	b := c.take(1)
	if b == nil {
		return 0
	}
	return b[0]
}

// Bool reads a boolean, rejecting any byte other than 0 or 1 as corruption
// rather than silently coercing it.
func (c *Cursor) Bool() bool {
	b := c.take(1)
	if b == nil {
		return false
	}
	switch b[0] {
	case 0:
		return false
	case 1:
		return true
	}
	if c.err == nil {
		c.err = fmt.Errorf("%w: boolean byte %d", ErrCorrupt, b[0])
	}
	return false
}

// Bytes reads a length-prefixed byte string. The result aliases the payload
// buffer and is only valid until the caller is handed the next frame.
func (c *Cursor) Bytes() []byte {
	n := c.Uint32()
	if c.err != nil {
		return nil
	}
	return c.take(int(n))
}

// String reads a length-prefixed string, copying it so the result outlives the
// frame buffer.
func (c *Cursor) String() string {
	return string(c.Bytes())
}

// Presence reads the optional-field bitmap for n optional fields.
func (c *Cursor) Presence(n int) PresenceSet {
	return PresenceSet(c.take(PresenceBits(n)))
}

// PresenceSet answers whether an optional field is present.
type PresenceSet []byte

// Has reports whether optional field i was written.
func (p PresenceSet) Has(i int) bool {
	index := i / 8
	if index >= len(p) {
		return false
	}
	return p[index]&(1<<(uint(i)%8)) != 0
}

// SetPresence marks optional field i as present in a bitmap being built.
func SetPresence(bitmap []byte, i int) {
	index := i / 8
	if index < len(bitmap) {
		bitmap[index] |= 1 << (uint(i) % 8)
	}
}
