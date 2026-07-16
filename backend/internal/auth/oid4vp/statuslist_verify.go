package oid4vp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"digital-contracting-service/internal/auth/oid4vp/status"
	"digital-contracting-service/internal/auth/oid4vp/status/handler"
)

var statusListVerifier *status.Verifier

// ConfigureStatusListVerification wires the status-list verifier used by OID4VP.
func ConfigureStatusListVerification(
	trustDataPath string,
	xfscAllowUnsignedFallback bool,
) error {
	return configureStatusListVerification(trustDataPath, xfscAllowUnsignedFallback, os.Getenv)
}

func configureStatusListVerification(
	trustDataPath string,
	xfscAllowUnsignedFallback bool,
	getenv func(string) string,
) error {

	var trust *status.TrustConfig
	path := strings.TrimSpace(trustDataPath)
	if path != "" {
		cfg, err := status.LoadTrustConfig(path)
		if err != nil {
			return fmt.Errorf("load status list trust config %q: %w", path, err)
		}
		trust = cfg
	}

	statusListVerifier = handler.NewVerifier(trust, handler.Options{
		XFSCAllowUnsignedFallback: xfscAllowUnsignedFallback,
	})
	return nil
}

func checkStatusList(rawClaims json.RawMessage) error {
	if statusListVerifier == nil {
		return fmt.Errorf("status list verifier is not configured")
	}

	if len(rawClaims) == 0 {
		return fmt.Errorf("credential claims are empty")
	}

	dec := json.NewDecoder(strings.NewReader(string(rawClaims)))
	dec.UseNumber()
	var claims map[string]any
	if err := dec.Decode(&claims); err != nil {
		return fmt.Errorf("parse credential claims for status list check: %w", err)
	}

	result, err := statusListVerifier.VerifyStatus(context.Background(), status.VerifiedCredential{
		Format: "sd-jwt",
		Claims: claims,
	})
	if err != nil {
		return fmt.Errorf("status list check: %w", err)
	}
	if !result.Accepted {
		return mapStatusListRejection(result)
	}
	return nil
}

func mapStatusListRejection(result status.CredentialVerificationResult) error {
	if len(result.StatusResults) > 0 {
		ref := result.StatusResults[0]
		switch ref.State {
		case status.StateInvalid:
			return fmt.Errorf("credential status list index %d is revoked", ref.Index)
		case status.StateSuspended:
			return fmt.Errorf("credential status list index %d is suspended", ref.Index)
		}
	}
	if reason := strings.TrimSpace(result.Reason); reason != "" {
		return fmt.Errorf("status list check: %s", reason)
	}
	return fmt.Errorf("status list check: credential rejected")
}
