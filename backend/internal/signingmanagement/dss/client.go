// Package dss defines the interface for a remote Digital Signature Service
// (DCS-IR-SI-10). The DCS backend never holds private keys (DCS-IR-HI-01);
// all signing is delegated to an external DSS/PCM endpoint.
package dss

import "context"

// Client abstracts over any remote signing service.
type Client interface {
	// Sign signs the given payload using the specified credential type and
	// returns the raw signature bytes. The caller is responsible for encoding.
	Sign(ctx context.Context, payload []byte, credentialType string) ([]byte, error)
}

// StubClient returns deterministic placeholder bytes. It is the default v1
// implementation until a real DSS/PCM HTTP endpoint is configured.
type StubClient struct{}

// Sign implements Client. It returns a fixed placeholder so that Apply can
// store a record immediately without a real signing backend.
func (StubClient) Sign(_ context.Context, _ []byte, _ string) ([]byte, error) {
	// TODO(DCS-IR-SI-10): replace with HTTP call to DSS/PCM signing endpoint.
	return []byte("STUB_SIGNATURE_PLACEHOLDER"), nil
}
