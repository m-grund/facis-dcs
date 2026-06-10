package verify

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/digitorus/pkcs7"
)

var (
	byteRangeRe = regexp.MustCompile(`/ByteRange\s*\[\s*(\d+)\s+(\d+)\s+(\d+)\s+(\d+)\s*\]`)
	contentsRe  = regexp.MustCompile(`/Contents\s*<([0-9A-Fa-f\s]+)>`)
)

// VerifyPDFSignatures validates detached PDF signatures (PAdES/PKCS#7) found
// via /ByteRange + /Contents pairs.
// Returns (signatureCount, allValid, error).
// When no signatures are present: count=0, allValid=false, err=nil.
func VerifyPDFSignatures(pdf []byte) (int, bool, error) {
	matches := byteRangeRe.FindAllSubmatchIndex(pdf, -1)
	if len(matches) == 0 {
		return 0, false, nil
	}

	allValid := true
	for i, m := range matches {
		if len(m) < 10 {
			return 0, false, fmt.Errorf("invalid ByteRange match at index %d", i)
		}

		a, err := parseInt(pdf[m[2]:m[3]])
		if err != nil {
			return 0, false, fmt.Errorf("parse ByteRange[0]: %w", err)
		}
		b, err := parseInt(pdf[m[4]:m[5]])
		if err != nil {
			return 0, false, fmt.Errorf("parse ByteRange[1]: %w", err)
		}
		c, err := parseInt(pdf[m[6]:m[7]])
		if err != nil {
			return 0, false, fmt.Errorf("parse ByteRange[2]: %w", err)
		}
		d, err := parseInt(pdf[m[8]:m[9]])
		if err != nil {
			return 0, false, fmt.Errorf("parse ByteRange[3]: %w", err)
		}

		if a < 0 || b < 0 || c < 0 || d < 0 || a+b > len(pdf) || c+d > len(pdf) {
			allValid = false
			continue
		}

		signedContent := make([]byte, 0, b+d)
		signedContent = append(signedContent, pdf[a:a+b]...)
		signedContent = append(signedContent, pdf[c:c+d]...)

		dict, found := signatureDictAround(pdf, m[0], m[1])
		if !found {
			allValid = false
			continue
		}

		cms, err := signatureContentsBytes(dict)
		if err != nil {
			allValid = false
			continue
		}

		p7, err := pkcs7.Parse(cms)
		if err != nil {
			allValid = false
			continue
		}
		p7.Content = signedContent
		if err := p7.Verify(); err != nil {
			allValid = false
		}
	}

	return len(matches), allValid, nil
}

func parseInt(b []byte) (int, error) {
	v, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		return 0, err
	}
	return v, nil
}

func signatureDictAround(pdf []byte, start, end int) ([]byte, bool) {
	dictStart := bytes.LastIndex(pdf[:start], []byte("<<"))
	if dictStart == -1 {
		return nil, false
	}
	rel := bytes.Index(pdf[end:], []byte(">>"))
	if rel == -1 {
		return nil, false
	}
	dictEnd := end + rel + 2
	if dictEnd <= dictStart {
		return nil, false
	}
	return pdf[dictStart:dictEnd], true
}

func signatureContentsBytes(dict []byte) ([]byte, error) {
	m := contentsRe.FindSubmatch(dict)
	if len(m) < 2 {
		return nil, fmt.Errorf("signature /Contents not found")
	}
	hexStr := strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\n', '\r', '\t':
			return -1
		default:
			return r
		}
	}, string(m[1]))
	if len(hexStr)%2 == 1 {
		hexStr += "0"
	}
	b, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, fmt.Errorf("decode /Contents hex: %w", err)
	}

	// PDF signature containers are often padded with trailing 0x00 bytes.
	b = bytes.TrimRight(b, "\x00")
	if len(b) == 0 {
		return nil, fmt.Errorf("empty signature container after trimming padding")
	}
	return b, nil
}
