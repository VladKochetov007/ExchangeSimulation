package evstream

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

// BlockDescriptor summarises one block so a query can decide whether to read it
// without reading it.
//
// This is the whole selective-query strategy in one struct. A columnar store
// gets selectivity by splitting records into columns; here it comes from
// bounding what a block can possibly contain — a sequence range, a time range,
// which event families are inside, and a filter over which symbols are. A query
// with a time window and an event family touches only the blocks whose bounds
// overlap it.
//
// Exactly 64 bytes, so a descriptor array is cache-line aligned and can be
// mmapped and indexed arithmetically rather than parsed.
type BlockDescriptor struct {
	// Offset is the byte position of the block header within the stream.
	Offset uint64
	// StoredLen is the on-disk size; UncompressedLen the size after decoding.
	StoredLen       uint32
	UncompressedLen uint32
	// FirstSeq and LastSeq bound the block's event sequence, inclusive.
	FirstSeq uint64
	LastSeq  uint64
	// MinTS and MaxTS bound simulated time, inclusive.
	MinTS int64
	MaxTS int64
	// Families is an exact bitmap of the schema ids present, for the 64 ids
	// starting at FirstUserSchema. Exactness matters: a false negative would
	// silently drop events from a result.
	Families uint64
	// Symbols is a probabilistic filter over the dictionary ids referenced.
	// False positives cost a wasted block read; false negatives are impossible,
	// which is the only direction that would corrupt an answer.
	Symbols uint64
}

// BlockDescriptorSize is fixed so an index is a flat array.
const BlockDescriptorSize = 64

// DefaultMaxIndexBytes bounds the body allocated while loading an index. An
// index is auxiliary evidence, so a corrupt descriptor count must not be able
// to turn a small sidecar into an unbounded allocation request.
const DefaultMaxIndexBytes = 64 << 20

// IndexMagic marks a persisted index.
const IndexMagic uint32 = 0x58444945 // "EIDX"

// familyBit maps a schema id onto the family bitmap. Ids outside the 64-wide
// window fold back into it: folding costs a false positive, never a false
// negative, so a query stays correct and only loses a little selectivity.
func familyBit(schemaID uint16) uint64 {
	if schemaID < FirstUserSchema {
		return 1 << 0
	}
	return 1 << (uint64(schemaID-FirstUserSchema)%63 + 1)
}

// symbolBit maps a dictionary id into the symbol filter.
func symbolBit(ref uint32) uint64 {
	if ref == 0 {
		return 0
	}
	// Multiplicative hash, so ids that are dense and sequential — which
	// dictionary ids are — spread across the word instead of clustering in the
	// low bits.
	return 1 << ((uint64(ref) * 0x9E3779B97F4A7C15) >> 58)
}

// AppendDescriptor writes a descriptor in canonical form.
func AppendDescriptor(dst []byte, d BlockDescriptor) []byte {
	var scratch [BlockDescriptorSize]byte
	binary.LittleEndian.PutUint64(scratch[0:8], d.Offset)
	binary.LittleEndian.PutUint32(scratch[8:12], d.StoredLen)
	binary.LittleEndian.PutUint32(scratch[12:16], d.UncompressedLen)
	binary.LittleEndian.PutUint64(scratch[16:24], d.FirstSeq)
	binary.LittleEndian.PutUint64(scratch[24:32], d.LastSeq)
	binary.LittleEndian.PutUint64(scratch[32:40], uint64(d.MinTS))
	binary.LittleEndian.PutUint64(scratch[40:48], uint64(d.MaxTS))
	binary.LittleEndian.PutUint64(scratch[48:56], d.Families)
	binary.LittleEndian.PutUint64(scratch[56:64], d.Symbols)
	return append(dst, scratch[:]...)
}

// ParseDescriptor reads one descriptor.
func ParseDescriptor(src []byte) (BlockDescriptor, error) {
	if len(src) < BlockDescriptorSize {
		return BlockDescriptor{}, ErrShortBuffer
	}
	return BlockDescriptor{
		Offset:          binary.LittleEndian.Uint64(src[0:8]),
		StoredLen:       binary.LittleEndian.Uint32(src[8:12]),
		UncompressedLen: binary.LittleEndian.Uint32(src[12:16]),
		FirstSeq:        binary.LittleEndian.Uint64(src[16:24]),
		LastSeq:         binary.LittleEndian.Uint64(src[24:32]),
		MinTS:           int64(binary.LittleEndian.Uint64(src[32:40])),
		MaxTS:           int64(binary.LittleEndian.Uint64(src[40:48])),
		Families:        binary.LittleEndian.Uint64(src[48:56]),
		Symbols:         binary.LittleEndian.Uint64(src[56:64]),
	}, nil
}

// Query selects blocks. A zero Query matches everything, so a full scan and a
// selective scan use the same code path and the same decoder — there is no
// separate "fast path" that could drift from the slow one.
type Query struct {
	// FromTS and ToTS bound simulated time, inclusive. Zero ToTS means no upper
	// bound.
	FromTS int64
	ToTS   int64
	// FromSeq and ToSeq bound event sequence, inclusive. Zero ToSeq means no
	// upper bound.
	FromSeq uint64
	ToSeq   uint64
	// Families lists the schema ids of interest. Empty means all.
	Families []uint16
	// SymbolRefs lists dictionary ids of interest. Empty means all. Filtering
	// is probabilistic, so a block may still be read and yield nothing.
	SymbolRefs []uint32
}

// familyMask is the query's family bitmap, or zero for "all".
func (q Query) familyMask() uint64 {
	var mask uint64
	for _, id := range q.Families {
		mask |= familyBit(id)
	}
	return mask
}

func (q Query) symbolMask() uint64 {
	var mask uint64
	for _, ref := range q.SymbolRefs {
		mask |= symbolBit(ref)
	}
	return mask
}

// Selects reports whether a block can possibly contain a matching event.
//
// Conservative in one direction only: it may say yes to a block that turns out
// to hold nothing, which costs a read, and it must never say no to a block that
// holds a match, which would silently truncate a result.
func (q Query) Selects(d BlockDescriptor) bool {
	if q.FromTS != 0 && d.MaxTS < q.FromTS {
		return false
	}
	if q.ToTS != 0 && d.MinTS > q.ToTS {
		return false
	}
	if q.FromSeq != 0 && d.LastSeq < q.FromSeq {
		return false
	}
	if q.ToSeq != 0 && d.FirstSeq > q.ToSeq {
		return false
	}
	if mask := q.familyMask(); mask != 0 && d.Families&mask == 0 {
		return false
	}
	if mask := q.symbolMask(); mask != 0 && d.Symbols&mask == 0 {
		return false
	}
	return true
}

// Matches reports whether a decoded frame satisfies the query's exact
// predicates. The block filter is coarse and probabilistic; this is the precise
// check applied per event.
func (q Query) Matches(h FrameHeader) bool {
	if q.FromTS != 0 && h.SimTS < q.FromTS {
		return false
	}
	if q.ToTS != 0 && h.SimTS > q.ToTS {
		return false
	}
	if q.FromSeq != 0 && h.Seq < q.FromSeq {
		return false
	}
	if q.ToSeq != 0 && h.Seq > q.ToSeq {
		return false
	}
	if len(q.Families) != 0 {
		found := false
		for _, id := range q.Families {
			if id == h.SchemaID {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if len(q.SymbolRefs) != 0 {
		found := false
		for _, ref := range q.SymbolRefs {
			if ref == h.VenueRef {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// Index is the block directory for one stream.
type Index struct {
	Blocks []BlockDescriptor
}

// WriteTo persists the index. It is written as a sidecar rather than a footer
// so that a stream truncated by a crash keeps a usable index for the blocks
// that did land, and so an index can be rebuilt and replaced without rewriting
// the evidence.
func (ix *Index) WriteTo(w io.Writer) (int64, error) {
	buf := make([]byte, 0, 16+len(ix.Blocks)*BlockDescriptorSize)
	buf = binary.LittleEndian.AppendUint32(buf, IndexMagic)
	buf = binary.LittleEndian.AppendUint32(buf, uint32(len(ix.Blocks)))
	for _, block := range ix.Blocks {
		buf = AppendDescriptor(buf, block)
	}
	n, err := writeExact(w, buf)
	return int64(n), err
}

// ReadIndex loads a persisted index.
func ReadIndex(r io.Reader) (*Index, error) {
	var header [8]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, ErrShortBuffer
	}
	if binary.LittleEndian.Uint32(header[0:4]) != IndexMagic {
		return nil, ErrBadMagic
	}
	count := binary.LittleEndian.Uint32(header[4:8])
	maxDescriptors := uint64((DefaultMaxIndexBytes - 8) / BlockDescriptorSize)
	if uint64(count) > maxDescriptors {
		return nil, fmt.Errorf("%w: index descriptor count %d exceeds limit %d", ErrCorrupt, count, maxDescriptors)
	}
	body := make([]byte, int(count)*BlockDescriptorSize)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, ErrShortBuffer
	}
	index := &Index{Blocks: make([]BlockDescriptor, count)}
	for i := range index.Blocks {
		block, err := ParseDescriptor(body[i*BlockDescriptorSize:])
		if err != nil {
			return nil, err
		}
		index.Blocks[i] = block
	}
	return index, nil
}

// Select returns the blocks a query must read, and the number skipped. The skip
// count is what makes the index's value measurable rather than assumed.
func (ix *Index) Select(q Query) (selected []BlockDescriptor, skipped int) {
	for _, block := range ix.Blocks {
		if q.Selects(block) {
			selected = append(selected, block)
		} else {
			skipped++
		}
	}
	return selected, skipped
}

// blockStats accumulates a descriptor while a block is being written.
type blockStats struct {
	firstSeq uint64
	lastSeq  uint64
	minTS    int64
	maxTS    int64
	families uint64
	symbols  uint64
	started  bool
}

func (s *blockStats) reset() {
	*s = blockStats{minTS: math.MaxInt64, maxTS: math.MinInt64}
}

func (s *blockStats) observe(h FrameHeader) {
	if !s.started {
		s.firstSeq = h.Seq
		s.started = true
	}
	s.lastSeq = h.Seq
	if h.SimTS < s.minTS {
		s.minTS = h.SimTS
	}
	if h.SimTS > s.maxTS {
		s.maxTS = h.SimTS
	}
	s.families |= familyBit(h.SchemaID)
	s.symbols |= symbolBit(h.VenueRef)
}

func (s *blockStats) descriptor(offset uint64, storedLen, uncompressedLen uint32) BlockDescriptor {
	return BlockDescriptor{
		Offset: offset, StoredLen: storedLen, UncompressedLen: uncompressedLen,
		FirstSeq: s.firstSeq, LastSeq: s.lastSeq,
		MinTS: s.minTS, MaxTS: s.maxTS,
		Families: s.families, Symbols: s.symbols,
	}
}

// IndexedReader reads only the blocks a query selects, seeking past the rest.
//
// It shares the frame decoding and validation of Reader; what it drops is the
// requirement to read every byte. Sequence continuity cannot be checked across
// a skipped block, so it verifies continuity within each block instead and
// reports the gap boundaries it crossed.
type IndexedReader struct {
	source       io.ReaderAt
	decompressor BlockDecompressor
	codec        Codec
	dict         *Dictionary
	block        []byte
	stored       []byte
}

// NewIndexedReader prepares random-access reads over a stream.
//
// The dictionary must be supplied: interning ids are learned by reading the
// stream in order, and a reader that skips blocks may skip the frame that
// defined an id it later needs. Building the dictionary once with a full
// sequential pass, then reusing it for many selective queries, is the intended
// pattern — and it is why the dictionary is small enough to keep resident.
func NewIndexedReader(source io.ReaderAt, codec Codec, dict *Dictionary,
	decompressor BlockDecompressor) *IndexedReader {
	return &IndexedReader{
		source: source, codec: codec, dict: dict, decompressor: decompressor,
	}
}

// RangeSelected walks the frames of the given blocks in order, calling visit
// only for frames matching the query exactly.
func (r *IndexedReader) RangeSelected(blocks []BlockDescriptor, q Query, visit func(Frame) error) error {
	for _, descriptor := range blocks {
		if err := r.readBlock(descriptor); err != nil {
			return err
		}
		for offset := 0; offset < len(r.block); {
			header, err := ParseFrameHeader(r.block[offset:])
			if err != nil {
				return err
			}
			end := offset + int(header.Length)
			if end > len(r.block) {
				return fmt.Errorf("%w: frame overruns block", ErrCorrupt)
			}
			if header.SchemaID != SchemaDictionary && q.Matches(header) {
				venue := ""
				if header.VenueRef != 0 {
					venue, _ = r.dict.Value(header.VenueRef)
				}
				if err := visit(Frame{
					Header:  header,
					Payload: r.block[offset+FrameHeaderSize : end],
					Venue:   venue,
				}); err != nil {
					return err
				}
			}
			offset = end
		}
	}
	return nil
}

func (r *IndexedReader) readBlock(d BlockDescriptor) error {
	if d.StoredLen > uint32(DefaultMaxBlockBytes) {
		return fmt.Errorf("%w: indexed stored block length %d exceeds limit %d", ErrCorrupt, d.StoredLen, DefaultMaxBlockBytes)
	}
	if d.UncompressedLen > uint32(DefaultMaxBlockBytes) {
		return fmt.Errorf("%w: indexed uncompressed block length %d exceeds limit %d", ErrCorrupt, d.UncompressedLen, DefaultMaxBlockBytes)
	}
	total := BlockHeaderSize + int(d.StoredLen)
	r.stored = growTo(r.stored, total)
	if _, err := r.source.ReadAt(r.stored[:total], int64(d.Offset)); err != nil {
		return ErrShortBuffer
	}
	if binary.LittleEndian.Uint32(r.stored[0:4]) != BlockMagic {
		return fmt.Errorf("%w: block magic at offset %d", ErrCorrupt, d.Offset)
	}
	stored := r.stored[BlockHeaderSize:total]
	if r.codec == CodecNone {
		r.block = stored
		return nil
	}
	block, err := r.decompressor.Decompress(r.block[:0], stored, int(d.UncompressedLen))
	if err != nil {
		return fmt.Errorf("%w: decompress at offset %d: %v", ErrCorrupt, d.Offset, err)
	}
	r.block = block
	return nil
}
