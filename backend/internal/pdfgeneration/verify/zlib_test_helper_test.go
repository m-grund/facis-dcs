package verify

import (
	"bytes"
	"compress/zlib"
)

// zlibCompress compresses data with zlib for use in test PDF construction.
func zlibCompress(data []byte) []byte {
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	_, _ = w.Write(data)
	_ = w.Close()
	return buf.Bytes()
}
