package oid4vp

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

const maxStatusListDecodedBytes = 4 << 20 // 4 MiB uncompressed bitstring cap.

type bitPacking int

const (
	bitPackingMSB bitPacking = iota // W3C MSB
	bitPackingLSB                   // IETF LSB
)

func encodedListFromBody(body []byte) (string, error) {
	return encodedListFromBodyForPurpose(body, "")
}

func encodedListFromBodyForPurpose(body []byte, expectedPurpose string) (string, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil {
		return "", fmt.Errorf("parse status list response: %w", err)
	}

	if raw, ok := root["credentialSubject"]; ok {
		var subject struct {
			EncodedList   string `json:"encodedList"`
			StatusPurpose any    `json:"statusPurpose"`
		}
		if err := json.Unmarshal(raw, &subject); err != nil {
			return "", fmt.Errorf("parse status list credentialSubject: %w", err)
		}
		if expectedPurpose != "" && !statusPurposeMatches(subject.StatusPurpose, expectedPurpose) {
			return "", fmt.Errorf("status list purpose does not match credential status purpose %q", expectedPurpose)
		}
		if strings.TrimSpace(subject.EncodedList) != "" {
			return strings.TrimSpace(subject.EncodedList), nil
		}
	}

	// XFSC: {"list":"<base64 compressed bitstring>"}
	var direct struct {
		List string `json:"list"`
	}
	if err := json.Unmarshal(body, &direct); err == nil && strings.TrimSpace(direct.List) != "" {
		return strings.TrimSpace(direct.List), nil
	}

	return "", fmt.Errorf("status list response has no credentialSubject.encodedList or list field")
}

func statusPurposeMatches(raw any, expected string) bool {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return true
	}

	switch v := raw.(type) {
	case string:
		return strings.TrimSpace(v) == expected
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) == expected {
				return true
			}
		}
	}

	return false
}

func decompressEncodedList(encoded string) ([]byte, error) {
	compressed, err := decodeStatusListEncoding(encoded)
	if err != nil {
		return nil, err
	}

	readCompressed := func(r io.ReadCloser) ([]byte, error) {
		defer func() { _ = r.Close() }()
		limited := io.LimitReader(r, maxStatusListDecodedBytes+1)
		out, err := io.ReadAll(limited)
		if err != nil {
			return nil, err
		}
		if len(out) > maxStatusListDecodedBytes {
			return nil, fmt.Errorf("decoded status list exceeds %d bytes", maxStatusListDecodedBytes)
		}
		return out, nil
	}

	if len(compressed) >= 2 && compressed[0] == 0x1f && compressed[1] == 0x8b {
		r, err := gzip.NewReader(bytes.NewReader(compressed))
		if err != nil {
			return nil, fmt.Errorf("create gzip reader for bitstring: %w", err)
		}
		out, err := readCompressed(r)
		if err != nil {
			return nil, fmt.Errorf("read gzip bitstring: %w", err)
		}
		return out, nil
	}

	if r, err := zlib.NewReader(bytes.NewReader(compressed)); err == nil {
		out, err := readCompressed(r)
		if err != nil {
			return nil, fmt.Errorf("read zlib bitstring: %w", err)
		}
		return out, nil
	}

	// fallback if compression magic bytes are wrong
	if r, err := gzip.NewReader(bytes.NewReader(compressed)); err == nil {
		out, err := readCompressed(r)
		if err != nil {
			return nil, fmt.Errorf("read gzip bitstring: %w", err)
		}
		return out, nil
	}

	return nil, fmt.Errorf("unsupported status list compression; expected gzip or zlib")
}

func decodeStatusListEncoding(encoded string) ([]byte, error) {
	s := strings.TrimSpace(encoded)
	if s == "" {
		return nil, fmt.Errorf("empty encoded status list")
	}

	s = strings.TrimPrefix(s, "u") // multibase base64url

	return decodeBase64URLOrStd(s)
}

func decodeBase64URLOrStd(s string) ([]byte, error) {
	encodings := []*base64.Encoding{
		base64.RawURLEncoding,
		base64.URLEncoding,
		base64.RawStdEncoding,
		base64.StdEncoding,
	}

	var lastErr error

	for _, enc := range encodings {
		b, err := enc.DecodeString(s)
		if err == nil {
			return b, nil
		}
		lastErr = err
	}

	return nil, fmt.Errorf("base64 decode status list: %w", lastErr)
}

func bitRevoked(bitstring []byte, index uint32) bool {
	status, err := statusValueAt(bitstring, index, defaultStatusSize, bitPackingMSB)
	if err != nil {
		return true
	}

	return status != 0
}

func queryEntryStatusFromBody(body []byte, index uint32) (string, error) {
	return queryEntryStatusFromBodyWithOptions(body, index, defaultStatusSize, "")
}

func queryEntryStatusFromBodyWithOptions(body []byte, index uint32, statusSize uint32, purpose string) (string, error) {
	var (
		encoded string
		err     error
	)

	if purpose == "" {
		encoded, err = encodedListFromBody(body)
	} else {
		encoded, err = encodedListFromBodyForPurpose(body, purpose)
	}

	if err != nil {
		return "", err
	}

	return queryEntryStatusFromEncodedList(encoded, index, statusSize)
}

// queryEntryStatusFromEncodedList uses W3C MSB bit ordering.
func queryEntryStatusFromEncodedList(encoded string, index uint32, bitsPerEntry uint32) (string, error) {
	bitstring, err := decompressEncodedList(encoded)
	if err != nil {
		return "", err
	}

	if bitsPerEntry == 1 {
		if bitRevoked(bitstring, index) {
			return "revoked", nil
		}
		return "active", nil
	}

	value, err := multiBitStatusValue(bitstring, index, bitsPerEntry)
	if err != nil {
		return "", err
	}

	if value == 0 {
		return "active", nil
	}

	return "revoked", nil
}

func queryTokenStatusFromEncodedList(encoded string, index uint32, bitsPerEntry uint32) (string, error) {
	return queryEntryStatusFromEncodedListWithPacking(encoded, index, bitsPerEntry, bitPackingLSB)
}

func queryEntryStatusFromEncodedListWithPacking(encoded string, index uint32, bitsPerEntry uint32, packing bitPacking) (string, error) {
	bitstring, err := decompressEncodedList(encoded)
	if err != nil {
		return "", err
	}

	statusValue, err := statusValueAt(bitstring, index, bitsPerEntry, packing)

	if err != nil {
		return "", err
	}

	if statusValue == 0 {
		return "active", nil
	}

	return "revoked", nil
}

func multiBitStatusValue(bitstring []byte, index, bitsPerEntry uint32) (uint32, error) {
	value, err := statusValueAt(bitstring, index, bitsPerEntry, bitPackingMSB)
	if err != nil {
		return 0, err
	}

	if value > uint64(^uint32(0)) {
		return 0, fmt.Errorf("status value overflows uint32")
	}

	return uint32(value), nil
}

func statusValueAt(bitstring []byte, index, bitsPerEntry uint32, packing bitPacking) (uint64, error) {
	if bitsPerEntry == 0 || bitsPerEntry > maxSupportedStatusSize {
		return 0, fmt.Errorf("unsupported bits per entry: %d", bitsPerEntry)
	}

	start := uint64(index) * uint64(bitsPerEntry)
	end := start + uint64(bitsPerEntry)

	if end > uint64(len(bitstring))*8 {
		return 0, fmt.Errorf("status index %d out of range", index)
	}

	var value uint64
	for i := uint32(0); i < bitsPerEntry; i++ {
		pos := start + uint64(i)
		b := bitstring[pos/8]
		var bit uint64
		switch packing {
		case bitPackingMSB:
			bit = uint64((b >> (7 - (pos % 8))) & 1)
			value = (value << 1) | bit
		case bitPackingLSB:
			bit = uint64((b >> (pos % 8)) & 1)
			value |= bit << i
		default:
			return 0, fmt.Errorf("unsupported bit packing")
		}
	}

	return value, nil
}

func ensureActiveStatus(status string, index uint32) error {
	if status == "revoked" {
		return fmt.Errorf("credential status list index %d is revoked", index)
	}

	return nil
}
