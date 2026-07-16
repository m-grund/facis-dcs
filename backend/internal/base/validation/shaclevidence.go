package validation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

// SHACLEvidence runs the Semantic Hub SHACL validation pass
// (validateAgainstHubShapes, ADR-9) for a document and returns the schema
// version it validated against (pinned per ADR-8 when the document already
// carries sh:shapesGraph, otherwise the hub's active version) and a stable
// hash of the resulting findings.
//
// Phase 4: this is what gets embedded into signed evidence (the
// ContractSigningSummaryCredential, the C2PA manifest) — an external
// verifier resolves sh:shapesGraph to fetch the exact pinned shapes from
// the public hub endpoints, re-runs validation, and compares the hash it
// gets against the one embedded at signing time. A mismatch means the
// document was mutated (or the hub's pinned version became unavailable)
// after the evidence was produced.
func SHACLEvidence(ctx context.Context, contractDocument any) (schemaVersion int, reportHash string, err error) {
	contract, err := normalizeObject(contractDocument)
	if err != nil {
		return 0, "", err
	}
	findings, version, err := validateAgainstHubShapes(ctx, contract)
	if err != nil {
		return 0, "", err
	}
	return version, ValidationReportHash(findings), nil
}

// ValidationReportHash computes a stable SHA-256 hash (hex) of a set of
// SHACL findings — deterministic regardless of the order goRDFlib produced
// them in, so the same document validated twice against the same hub
// version always hashes identically.
func ValidationReportHash(findings []PolicyFinding) string {
	type reportEntry struct {
		RuleID   string `json:"ruleId"`
		Severity string `json:"severity"`
		Path     string `json:"path"`
		Message  string `json:"message"`
	}
	entries := make([]reportEntry, 0, len(findings))
	for _, f := range findings {
		entries = append(entries, reportEntry{RuleID: f.RuleID, Severity: f.Severity, Path: f.Path, Message: f.Message})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].RuleID != entries[j].RuleID {
			return entries[i].RuleID < entries[j].RuleID
		}
		return entries[i].Path < entries[j].Path
	})
	// Errors are never expected to be raised by json.Marshal on this
	// concrete, non-cyclic struct slice.
	canonical, _ := json.Marshal(entries)
	sum := sha256.Sum256(canonical)
	return "sha256:" + hex.EncodeToString(sum[:])
}
