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

func (d DIDDocument) GetID() (string, error) {
	raw, ok := d["id"]
	if !ok {
		return "", errors.New(`did document does not contain "id"`)
	}

	return raw.(string), nil
}

func (d DIDDocument) GetHostname() (string, error) {
	id, err := d.GetID()
	if err != nil {
		return "", err
	}
	return didWebToHostname(id)
}

func didWebToHostname(did string) (string, error) {
	const prefix = "did:web:"
	if !strings.HasPrefix(did, prefix) {
		return "", fmt.Errorf("not a did:web identifier: %q", did)
	}

	rest := strings.TrimPrefix(did, prefix)

	// Alles nach dem ersten ":" wären Pfad-Komponenten (did:web:host:path:to:res),
	// die uns hier nicht interessieren - wir wollen nur den Host-Teil.
	hostEncoded, _, _ := strings.Cut(rest, ":")
	if hostEncoded == "" {
		return "", errors.New("did:web identifier has empty host component")
	}

	host, err := url.QueryUnescape(hostEncoded) // %3A -> ":"
	if err != nil {
		return "", fmt.Errorf("invalid percent-encoding in did:web host: %w", err)
	}

	return host, nil
}

func HostnameToDIDWeb(host string) string {
	return "did:web:" + strings.ReplaceAll(host, ":", "%3A")
}

func DIDWebToHostname(did string) (string, error) {
	const prefix = "did:web:"
	if !strings.HasPrefix(did, prefix) {
		return "", fmt.Errorf("not a did:web identifier: %q", did)
	}
	rest := strings.TrimPrefix(did, prefix)
	hostEncoded, _, _ := strings.Cut(rest, ":")
	if hostEncoded == "" {
		return "", errors.New("did:web identifier has empty host component")
	}
	return strings.ReplaceAll(hostEncoded, "%3A", ":"), nil
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
