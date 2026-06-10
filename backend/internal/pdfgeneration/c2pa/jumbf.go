// Package c2pa implements a minimal C2PA manifest writer for contract lifecycle
// provenance (DCS-OR-C2PA-001 through DCS-OR-C2PA-010).
//
// C2PA manifests are encoded as JUMBF (JPEG Universal Metadata Box Format, ISO 19566-5)
// containers. This package implements the subset needed to embed lifecycle assertions
// in contract PDFs with COSE_Sign1 signatures via the Crypto Provider Service.
package c2pa

import (
	"encoding/binary"
)

// boxType for a JUMBF Superbox (contains description box + content boxes).
var (
	jumbfBoxType = [4]byte{'j', 'u', 'm', 'b'} // JUMBF Superbox
	jumdBoxType  = [4]byte{'j', 'u', 'm', 'd'} // JUMBF Description Box
	jsonBoxType  = [4]byte{'j', 's', 'o', 'n'} // JSON content box
	cborBoxType  = [4]byte{'c', 'b', 'o', 'r'} // CBOR content box (COSE uses CBOR)
	uuidBoxType  = [4]byte{'u', 'u', 'i', 'd'} // UUID content box
)

// C2PA label UUIDs (as 16-byte arrays per ISO 19566-5 §7.2.1).
// Using the registered C2PA namespace UUID: 6a636231-6362-6f78-0000-000000000001 base.
var (
	// c2paBlockUUID is the top-level manifest store UUID (c2pa).
	c2paBlockUUID = [16]byte{
		0x63, 0x32, 0x70, 0x61, // "c2pa"
		0x00, 0x11, 0x00, 0x10,
		0x80, 0x00, 0x00, 0xAA,
		0x00, 0x38, 0x9B, 0x71,
	}

	// c2paManifestUUID is the manifest box UUID (c2ma).
	c2paManifestUUID = [16]byte{
		0x63, 0x32, 0x6D, 0x61, // "c2ma"
		0x00, 0x11, 0x00, 0x10,
		0x80, 0x00, 0x00, 0xAA,
		0x00, 0x38, 0x9B, 0x71,
	}

	// c2paAssertionStoreUUID is the assertion store UUID (c2as).
	c2paAssertionStoreUUID = [16]byte{
		0x63, 0x32, 0x61, 0x73, // "c2as"
		0x00, 0x11, 0x00, 0x10,
		0x80, 0x00, 0x00, 0xAA,
		0x00, 0x38, 0x9B, 0x71,
	}

	// c2paClaimUUID is the claim box UUID (c2cl).
	c2paClaimUUID = [16]byte{
		0x63, 0x32, 0x63, 0x6C, // "c2cl"
		0x00, 0x11, 0x00, 0x10,
		0x80, 0x00, 0x00, 0xAA,
		0x00, 0x38, 0x9B, 0x71,
	}

	// c2paSignatureUUID is the signature box UUID (c2cs).
	c2paSignatureUUID = [16]byte{
		0x63, 0x32, 0x63, 0x73, // "c2cs"
		0x00, 0x11, 0x00, 0x10,
		0x80, 0x00, 0x00, 0xAA,
		0x00, 0x38, 0x9B, 0x71,
	}

	// c2paJSONAssertionUUID is the JSON assertion box UUID.
	//nolint:unused
	c2paJSONAssertionUUID = [16]byte{
		0x6A, 0x73, 0x6F, 0x6E, // "json"
		0x00, 0x11, 0x00, 0x10,
		0x80, 0x00, 0x00, 0xAA,
		0x00, 0x38, 0x9B, 0x71,
	}

	// c2paCBORAssertionUUID is the CBOR assertion box UUID.
	c2paCBORAssertionUUID = [16]byte{
		0x63, 0x62, 0x6F, 0x72, // "cbor"
		0x00, 0x11, 0x00, 0x10,
		0x80, 0x00, 0x00, 0xAA,
		0x00, 0x38, 0x9B, 0x71,
	}

	// c2paUUIDAssertionUUID is the UUID assertion box UUID.
	c2paUUIDAssertionUUID = [16]byte{
		0x75, 0x75, 0x69, 0x64, // "uuid"
		0x00, 0x11, 0x00, 0x10,
		0x80, 0x00, 0x00, 0xAA,
		0x00, 0x38, 0x9B, 0x71,
	}
)

// Box represents a JUMBF box (header + content).
type Box struct {
	LBox  uint32 // total box length including LBox and TBox fields
	TBox  [4]byte
	XLBox uint64 // only used when LBox == 1
	Data  []byte
}

// WriteBox serialises a JUMBF box to bytes.
// If content length + 8 fits in a uint32, standard 8-byte header is used.
// Otherwise XLBox extended length header is used.
func WriteBox(tbox [4]byte, content []byte) []byte {
	totalLen := 8 + len(content) // 4 (LBox) + 4 (TBox) + content
	if totalLen <= 0xFFFFFFFF {
		buf := make([]byte, totalLen)
		binary.BigEndian.PutUint32(buf[0:4], uint32(totalLen))
		copy(buf[4:8], tbox[:])
		copy(buf[8:], content)
		return buf
	}
	// Extended length: LBox=1, then 8-byte XLBox.
	buf := make([]byte, 16+len(content))
	binary.BigEndian.PutUint32(buf[0:4], 1)
	copy(buf[4:8], tbox[:])
	binary.BigEndian.PutUint64(buf[8:16], uint64(16+len(content)))
	copy(buf[16:], content)
	return buf
}

// WriteDescriptionBox writes a JUMBF description box (jumd) with the given UUID and label.
// toggles is a bitmask; 0x03 means "requestable + label present".
func WriteDescriptionBox(uuid [16]byte, label string, toggles byte) []byte {
	content := make([]byte, 0, 16+1+len(label)+1)
	content = append(content, uuid[:]...)
	content = append(content, toggles)
	content = append(content, []byte(label)...)
	content = append(content, 0x00) // null terminator
	return WriteBox(jumdBoxType, content)
}

// WriteSuperbox writes a JUMBF superbox wrapping description + child boxes.
func WriteSuperbox(uuid [16]byte, label string, children ...[]byte) []byte {
	descBox := WriteDescriptionBox(uuid, label, 0x03)
	content := make([]byte, 0, len(descBox))
	content = append(content, descBox...)
	for _, child := range children {
		content = append(content, child...)
	}
	return WriteBox(jumbfBoxType, content)
}

// WriteJSONBox wraps JSON bytes in a JUMBF JSON content box.
func WriteJSONBox(jsonBytes []byte) []byte {
	return WriteBox(jsonBoxType, jsonBytes)
}

// WriteCBORBox wraps CBOR bytes (e.g. a COSE_Sign1 structure) in a JUMBF CBOR box.
func WriteCBORBox(cborBytes []byte) []byte {
	return WriteBox(cborBoxType, cborBytes)
}

// WriteUUIDBox wraps UUID assertion payload bytes in a UUID content box.
func WriteUUIDBox(contentUUID [16]byte, data []byte) []byte {
	content := make([]byte, 0, 16+len(data))
	content = append(content, contentUUID[:]...)
	content = append(content, data...)
	return WriteBox(uuidBoxType, content)
}
