package base

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

type DIDDocument map[string]interface{}

func NewDIDDocument(didFileContent []byte) (*DIDDocument, error) {
	var didFile DIDDocument
	err := json.Unmarshal(didFileContent, &didFile)
	if err != nil {
		return nil, err
	}
	return &didFile, nil
}

func (d DIDDocument) ExtractDomainAndPath() (domain string, err error) {
	raw, ok := d["id"]
	if !ok {
		return "", errors.New(`did document does not contain "id"`)
	}
	idStr, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf(`"id" field is not a string (got %T)`, raw)
	}

	domain, segments, _, err := parse(idStr)
	if err != nil {
		return "", err
	}

	var domainAndPath string
	path := pathString(segments)
	if path != "" {
		domainAndPath = fmt.Sprintf("%s/%s", domain, path)
	} else {
		domainAndPath = domain
	}

	return domainAndPath, nil
}

func pathString(path []string) string {
	return strings.Join(path, "/")
}

func parse(did string) (domain string, path []string, fragment string, err error) {
	const prefix = "did:web:"
	if !strings.HasPrefix(did, prefix) {
		return "", nil, "", fmt.Errorf("not a did:web identifier: %q", did)
	}
	rest := strings.TrimPrefix(did, prefix)
	if rest == "" {
		return "", nil, "", fmt.Errorf("did:web identifier has no method-specific id")
	}

	if idx := strings.Index(rest, "#"); idx != -1 {
		fragment = rest[idx+1:]
		rest = rest[:idx]
	}
	if rest == "" {
		return "", nil, "", fmt.Errorf("did:web identifier has empty method-specific id before fragment")
	}

	segments := strings.Split(rest, ":")

	domainSegment := segments[0]
	domain, err = url.QueryUnescape(domainSegment)
	if err != nil {
		return "", nil, "", fmt.Errorf("decode domain segment %q: %w", domainSegment, err)
	}
	if domain == "" {
		return "", nil, "", fmt.Errorf("did:web identifier has empty domain segment")
	}

	if len(segments) > 1 {
		path = segments[1:]
	}

	return domain, path, fragment, nil
}

// UUID v4
func generateUUID() (string, error) {
	uuid := make([]byte, 16)
	_, err := rand.Read(uuid)
	if err != nil {
		return "", err
	}

	uuid[6] = (uuid[6] & 0x0f) | 0x40
	uuid[8] = (uuid[8] & 0x3f) | 0x80

	return fmt.Sprintf("%x-%x-%x-%x-%x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:]), nil
}

func GenerateID() (*string, error) {
	uuid, err := generateUUID()
	if err != nil {
		return nil, err
	}

	return &uuid, nil
}
