package sia

import (
	"encoding/binary"
	"fmt"
	"time"

	"go.sia.tech/core/types"
	"go.sia.tech/indexd/slabs"
)

// Cached location metadata is written in a fixed binary layout rather than as
// JSON.
//
// Every field here is a hash, a key or a signature — bytes with no textual
// meaning — and a hex or base64 rendering doubles the size of the one thing a
// vault keeps a copy of per record. The layout is versioned so that a change
// invalidates old entries instead of misreading them, which costs one re-fetch
// and is what a cache is for.
const envelopeVersion = 1

// encodeObject writes the part of a location unique to one object.
func encodeObject(sealed slabs.SealedObject) []byte {
	out := make([]byte, 0, 256)
	out = append(out, envelopeVersion)
	out = appendBytes(out, sealed.EncryptedDataKey)
	out = append(out, sealed.DataSignature[:]...)
	out = appendBytes(out, sealed.EncryptedMetadataKey)
	out = appendBytes(out, sealed.EncryptedMetadata)
	out = append(out, sealed.MetadataSignature[:]...)
	out = binary.BigEndian.AppendUint64(out, uint64(sealed.CreatedAt.UnixNano()))
	out = binary.BigEndian.AppendUint64(out, uint64(sealed.UpdatedAt.UnixNano()))

	out = binary.BigEndian.AppendUint16(out, uint16(len(sealed.Slabs)))
	for _, slice := range sealed.Slabs {
		digest := slice.Digest()
		out = append(out, digest[:]...)
		out = binary.BigEndian.AppendUint32(out, slice.Offset)
		out = binary.BigEndian.AppendUint32(out, slice.Length)
	}
	return out
}

// decodeObject reads what encodeObject wrote, leaving the slab halves blank for
// the caller to fill in from the shared entries.
func decodeObject(b []byte) (slabs.SealedObject, []SlabID, error) {
	r := reader{b: b}
	if r.byteAt() != envelopeVersion {
		return slabs.SealedObject{}, nil, fmt.Errorf("%w: cached in layout %d, this build reads %d",
			ErrStaleLocation, b[0], envelopeVersion)
	}

	var sealed slabs.SealedObject
	sealed.EncryptedDataKey = r.bytes()
	copy(sealed.DataSignature[:], r.fixed(len(types.Signature{})))
	sealed.EncryptedMetadataKey = r.bytes()
	sealed.EncryptedMetadata = r.bytes()
	copy(sealed.MetadataSignature[:], r.fixed(len(types.Signature{})))
	sealed.CreatedAt = time.Unix(0, int64(r.uint64())).UTC()
	sealed.UpdatedAt = time.Unix(0, int64(r.uint64())).UTC()

	count := int(r.uint16())
	sealed.Slabs = make([]slabs.SlabSlice, count)
	ids := make([]SlabID, count)
	for i := range count {
		var digest types.Hash256
		copy(digest[:], r.fixed(len(types.Hash256{})))
		ids[i] = SlabID(slabs.SlabID(digest).String())
		sealed.Slabs[i].Offset = r.uint32()
		sealed.Slabs[i].Length = r.uint32()
	}
	if r.err != nil {
		return slabs.SealedObject{}, nil, fmt.Errorf("%w: %w", ErrStaleLocation, r.err)
	}
	return sealed, ids, nil
}

// encodeSlab writes the part of a location every object in a slab shares.
func encodeSlab(slice slabs.SlabSlice) []byte {
	out := make([]byte, 0, 32+4+len(slice.Sectors)*64)
	out = append(out, envelopeVersion)
	out = append(out, slice.EncryptionKey[:]...)
	out = binary.BigEndian.AppendUint16(out, uint16(slice.MinShards))
	out = binary.BigEndian.AppendUint16(out, uint16(len(slice.Sectors)))
	for _, sector := range slice.Sectors {
		out = append(out, sector.Root[:]...)
		out = append(out, sector.HostKey[:]...)
	}
	return out
}

// decodeSlab reads what encodeSlab wrote.
func decodeSlab(b []byte) (slabs.SlabSlice, error) {
	r := reader{b: b}
	if r.byteAt() != envelopeVersion {
		return slabs.SlabSlice{}, fmt.Errorf("%w: cached in layout %d, this build reads %d",
			ErrStaleLocation, b[0], envelopeVersion)
	}

	var slice slabs.SlabSlice
	copy(slice.EncryptionKey[:], r.fixed(len(slabs.EncryptionKey{})))
	slice.MinShards = uint(r.uint16())

	count := int(r.uint16())
	slice.Sectors = make([]slabs.PinnedSector, count)
	for i := range count {
		copy(slice.Sectors[i].Root[:], r.fixed(len(types.Hash256{})))
		copy(slice.Sectors[i].HostKey[:], r.fixed(len(types.PublicKey{})))
	}
	if r.err != nil {
		return slabs.SlabSlice{}, fmt.Errorf("%w: %w", ErrStaleLocation, r.err)
	}
	return slice, nil
}

func appendBytes(out, b []byte) []byte {
	out = binary.BigEndian.AppendUint32(out, uint32(len(b)))
	return append(out, b...)
}

// A reader walks a fixed layout, carrying the first failure rather than
// panicking on a short buffer. Cached bytes can be damaged or written by
// another version, and neither is a reason to end the process.
type reader struct {
	b   []byte
	err error
}

func (r *reader) take(n int) []byte {
	if r.err != nil {
		return make([]byte, n)
	}
	if len(r.b) < n {
		r.err = fmt.Errorf("cached location is truncated: wanted %d bytes, %d remain", n, len(r.b))
		return make([]byte, n)
	}
	out := r.b[:n]
	r.b = r.b[n:]
	return out
}

func (r *reader) byteAt() byte       { return r.take(1)[0] }
func (r *reader) fixed(n int) []byte { return r.take(n) }
func (r *reader) uint16() uint16     { return binary.BigEndian.Uint16(r.take(2)) }
func (r *reader) uint32() uint32     { return binary.BigEndian.Uint32(r.take(4)) }
func (r *reader) uint64() uint64     { return binary.BigEndian.Uint64(r.take(8)) }

func (r *reader) bytes() []byte {
	n := int(r.uint32())
	if r.err != nil || n == 0 {
		return nil
	}
	return append([]byte(nil), r.take(n)...)
}
