package verify

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"log"
)

func zlibDecompress(data []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("zlib new reader: %w", err)
	}
	defer func(r io.ReadCloser) {
		err := r.Close()
		if err != nil {
			log.Printf("zlib reader close error: %s", err)
		}
	}(r)
	out, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("zlib read: %w", err)
	}
	return out, nil
}
