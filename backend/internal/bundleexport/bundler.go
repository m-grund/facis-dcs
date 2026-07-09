// Package bundleexport assembles a single ZIP bundle for a contract or a
// template from data the instance already holds — it re-uses the existing
// retrieval paths (contract/template read, PDF export, C2PA manifest
// extraction, signature load) rather than re-implementing any of them
// (FR-TR-24, FR-CWE-30).
//
// Contract bundle layout (per contract, recursively for the parent chain
// UPWARD — never downward, so siblings are structurally absent):
//
//	contract.jsonld        the machine-readable source (incl. dcs:parentContract)
//	contract.pdf           the current signed PDF/A-3 (from the IPFS export path)
//	manifest-store.c2pa    the embedded C2PA manifest store
//	credentials/…          lifecycle credentials extracted from the C2PA chain
//	signatures.json        contract_signatures rows incl. states
//	parents/<parent-did>/… the same structure for each parent, recursively
//	bundle-manifest.json   index of every entry with its SHA-256 (root only)
//
// A FR-PACM-06 structural-integrity pre-flight runs BEFORE zipping: if a
// referenced component is missing (e.g. a contract without an exported PDF),
// the export is refused with a findings list instead of shipping an
// incomplete ZIP.
package bundleexport

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/base/ipfs"
	cwedb "digital-contracting-service/internal/contractworkflowengine/db"
	cweevent "digital-contracting-service/internal/contractworkflowengine/event"
	"digital-contracting-service/internal/pdfgeneration/manifest"
	"digital-contracting-service/internal/pdfgeneration/pdfcore"
	"digital-contracting-service/internal/pdfgeneration/provenance"
	pdfquery "digital-contracting-service/internal/pdfgeneration/query"
	signdb "digital-contracting-service/internal/signingmanagement/db"
	tpldb "digital-contracting-service/internal/templaterepository/db"
)

// ErrRefused is returned when the structural-integrity pre-flight finds a
// missing/inconsistent component. The accompanying Findings list explains why.
type ErrRefused struct {
	Findings []string
}

func (e *ErrRefused) Error() string {
	return "bundle export refused: " + strings.Join(e.Findings, "; ")
}

// AsRefused reports whether err is (or wraps) a refusal and returns it.
func AsRefused(err error) (*ErrRefused, bool) {
	var refused *ErrRefused
	if errors.As(err, &refused) {
		return refused, true
	}
	return nil, false
}

// SignatureLoader is the narrow slice of the signing-management repository the
// bundler needs (contract_signatures rows for signatures.json).
type SignatureLoader interface {
	LoadSignatures(ctx context.Context, tx *sqlx.Tx, did string) ([]signdb.SignatureRecord, error)
}

// Bundler builds contract/template ZIP bundles. All dependencies mirror the
// existing PDF export / C2PA / signing wiring.
type Bundler struct {
	DB         *sqlx.DB
	CRepo      cwedb.ContractRepo
	TRepo      tpldb.ContractTemplateRepo
	SignRepo   SignatureLoader
	IPFSClient *ipfs.APIClient
	PDFCore    *pdfcore.Client
	VCIssuer   provenance.VCIssuer
	IssuerDID  string
}

// ExportContext carries the caller identity used for the FR-CSA-18 audit event.
type ExportContext struct {
	ExportedBy string
	HolderDID  string
	UserRoles  userrole.UserRoles
}

var pathSanitizer = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func sanitizeSegment(s string) string {
	cleaned := pathSanitizer.ReplaceAllString(s, "_")
	cleaned = strings.Trim(cleaned, "_")
	if cleaned == "" {
		cleaned = "entry"
	}
	return cleaned
}

// bundleFiles is a path->bytes set for one bundle (no bundle-manifest.json,
// which the root adds last).
type bundleFiles map[string][]byte

// manifestEntry is one row in bundle-manifest.json.
type manifestEntry struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Bytes  int    `json:"bytes"`
}

// bundleManifest is the bundle-manifest.json index.
type bundleManifest struct {
	BundleType  string          `json:"bundle_type"`
	RootDID     string          `json:"root_did"`
	GeneratedAt string          `json:"generated_at"`
	Components  []componentInfo `json:"components"`
	Entries     []manifestEntry `json:"entries"`
}

// componentInfo records per-contract metadata surfaced in the manifest.
type componentInfo struct {
	DID             string `json:"did"`
	ContractVersion int    `json:"contract_version"`
	State           string `json:"state"`
	Role            string `json:"role"`
	ParentDID       string `json:"parent_did,omitempty"`
}

// ExportContract builds the contract bundle ZIP for did. On a structural
// integrity failure it returns an *ErrRefused. On success it also records an
// EXPORT audit event (FR-CSA-18).
func (b *Bundler) ExportContract(ctx context.Context, did string, ec ExportContext) (io.ReadCloser, error) {
	files := bundleFiles{}
	var components []componentInfo
	if err := b.collectContract(ctx, did, "", map[string]bool{}, files, &components); err != nil {
		return nil, err
	}

	zipBytes, err := zipWithManifest(files, "contract", did, components)
	if err != nil {
		return nil, err
	}

	if err := b.recordExportEvent(ctx, did, ec); err != nil {
		// The bundle is already assembled; failing to persist the audit event
		// must not corrupt the response. Log and continue.
		log.Printf("bundleexport: could not record EXPORT audit event for %s: %v", did, err)
	}

	return io.NopCloser(bytes.NewReader(zipBytes)), nil
}

// collectContract gathers one contract's files under pathPrefix and recurses
// into its parent chain. Findings accumulate into an *ErrRefused, which is
// returned as soon as the whole tree has been walked for the root call.
func (b *Bundler) collectContract(ctx context.Context, did, pathPrefix string, visited map[string]bool, files bundleFiles, components *[]componentInfo) error {
	if visited[did] {
		return nil
	}
	visited[did] = true

	tx, err := b.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("bundleexport: rollback: %v", err)
		}
	}(tx)

	contract, err := b.CRepo.ReadDataByDID(ctx, tx, did)
	if err != nil {
		return &ErrRefused{Findings: []string{fmt.Sprintf("contract %s: not found or unreadable", did)}}
	}

	pdfState, err := b.CRepo.ReadPDFState(ctx, tx, did)
	if err != nil {
		return fmt.Errorf("read PDF state for %s: %w", did, err)
	}
	if pdfState.IPFSCID == "" {
		return &ErrRefused{Findings: []string{fmt.Sprintf("contract %s: no exported PDF (export the contract PDF before bundling)", did)}}
	}

	signatures, err := b.SignRepo.LoadSignatures(ctx, tx, did)
	if err != nil {
		return fmt.Errorf("load signatures for %s: %w", did, err)
	}

	role := "root"
	if pathPrefix != "" {
		role = "parent"
	}
	parentDID := extractParentContractDID(contract.ContractData)
	*components = append(*components, componentInfo{
		DID:             did,
		ContractVersion: contract.ContractVersion,
		State:           contract.State,
		Role:            role,
		ParentDID:       parentDID,
	})

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit read tx for %s: %w", did, err)
	}

	// contract.jsonld
	var jsonldBytes []byte
	if contract.ContractData != nil {
		jsonldBytes = []byte(*contract.ContractData)
	}
	files[pathPrefix+"contract.jsonld"] = jsonldBytes

	// contract.pdf (via the existing IPFS export path)
	pdfBytes, err := b.fetchContractPDF(ctx, did)
	if err != nil {
		return &ErrRefused{Findings: []string{fmt.Sprintf("contract %s: exported PDF could not be fetched: %v", did, err)}}
	}
	files[pathPrefix+"contract.pdf"] = pdfBytes

	// manifest-store.c2pa + credentials/ (from the same C2PA chain)
	manifestBytes, err := b.PDFCore.ExtractManifest(ctx, pdfBytes)
	if err != nil {
		return &ErrRefused{Findings: []string{fmt.Sprintf("contract %s: C2PA manifest extraction failed: %v", did, err)}}
	}
	if len(manifestBytes) == 0 {
		return &ErrRefused{Findings: []string{fmt.Sprintf("contract %s: exported PDF carries no C2PA manifest store", did)}}
	}
	files[pathPrefix+"manifest-store.c2pa"] = manifestBytes

	chain, err := manifest.ParseChain(manifestBytes)
	if err != nil {
		return &ErrRefused{Findings: []string{fmt.Sprintf("contract %s: C2PA manifest chain unparsable: %v", did, err)}}
	}
	chainJSON, err := json.MarshalIndent(chain, "", "  ")
	if err != nil {
		return fmt.Errorf("encode manifest chain for %s: %w", did, err)
	}
	files[pathPrefix+"credentials/manifest-chain.json"] = chainJSON
	for i, entry := range chain {
		entryJSON, err := json.MarshalIndent(entry, "", "  ")
		if err != nil {
			return fmt.Errorf("encode credential %d for %s: %w", i, did, err)
		}
		name := fmt.Sprintf("%02d-%s.json", i, sanitizeSegment(entry.Label))
		files[pathPrefix+"credentials/"+name] = entryJSON
	}

	// signatures.json
	sigJSON, err := json.MarshalIndent(signaturesToJSON(signatures), "", "  ")
	if err != nil {
		return fmt.Errorf("encode signatures for %s: %w", did, err)
	}
	files[pathPrefix+"signatures.json"] = sigJSON

	// parents/<parent-did>/… recursively upward (no downward traversal).
	if parentDID != "" {
		parentPrefix := pathPrefix + "parents/" + parentDID + "/"
		if err := b.collectContract(ctx, parentDID, parentPrefix, visited, files, components); err != nil {
			// A non-local parent surfaces as a "not found" refusal — a benign
			// cross-instance parent, so keep only the link, not the files. Any
			// other error (incl. a locally-resolvable parent that is itself
			// incomplete) fails the whole export.
			if !isNotFoundRefusal(err, parentDID) {
				return err
			}
		}
	}

	return nil
}

func isNotFoundRefusal(err error, did string) bool {
	refused, ok := AsRefused(err)
	if !ok {
		return false
	}
	for _, f := range refused.Findings {
		if strings.Contains(f, did) && strings.Contains(f, "not found") {
			return true
		}
	}
	return false
}

// fetchContractPDF returns the current signed PDF bytes through the existing
// export path.
func (b *Bundler) fetchContractPDF(ctx context.Context, did string) ([]byte, error) {
	handler := pdfquery.ExportContractPdfHandler{
		DB:         b.DB,
		CRepo:      b.CRepo,
		IPFSClient: b.IPFSClient,
		PDFCore:    b.PDFCore,
		VCIssuer:   b.VCIssuer,
		IssuerDID:  b.IssuerDID,
	}
	reader, err := handler.Handle(ctx, pdfquery.ExportContractPdfQry{DID: did})
	if err != nil {
		return nil, err
	}
	defer func() { _ = reader.Close() }()
	return io.ReadAll(reader)
}

// ExportTemplate builds the flat template bundle ZIP: template JSON-LD,
// rendered PDF, and bundle manifest. No parent/frame chain (no frame-type
// taxonomy exists at template level).
func (b *Bundler) ExportTemplate(ctx context.Context, did string) (io.ReadCloser, error) {
	tx, err := b.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	tpl, err := b.TRepo.ReadDataByID(ctx, tx, did)
	if err != nil {
		_ = tx.Rollback()
		return nil, &ErrRefused{Findings: []string{fmt.Sprintf("template %s: not found or unreadable", did)}}
	}
	if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
		log.Printf("bundleexport: rollback: %v", err)
	}

	files := bundleFiles{}
	if tpl.TemplateData != nil {
		files["template.jsonld"] = []byte(*tpl.TemplateData)
	} else {
		files["template.jsonld"] = []byte("{}")
	}

	pdfBytes, err := b.fetchTemplatePDF(ctx, did)
	if err != nil {
		return nil, &ErrRefused{Findings: []string{fmt.Sprintf("template %s: rendered PDF could not be produced: %v", did, err)}}
	}
	files["template.pdf"] = pdfBytes

	components := []componentInfo{{DID: did, State: tpl.State, Role: "root"}}
	zipBytes, err := zipWithManifest(files, "template", did, components)
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(zipBytes)), nil
}

func (b *Bundler) fetchTemplatePDF(ctx context.Context, did string) ([]byte, error) {
	handler := pdfquery.ExportTemplatePdfHandler{
		DB:         b.DB,
		TRepo:      b.TRepo,
		IPFSClient: b.IPFSClient,
		PDFCore:    b.PDFCore,
		VCIssuer:   b.VCIssuer,
		IssuerDID:  b.IssuerDID,
	}
	reader, err := handler.Handle(ctx, pdfquery.ExportTemplatePdfQry{DID: did})
	if err != nil {
		return nil, err
	}
	defer func() { _ = reader.Close() }()
	return io.ReadAll(reader)
}

func (b *Bundler) recordExportEvent(ctx context.Context, did string, ec ExportContext) error {
	tx, err := b.DB.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("bundleexport: rollback audit tx: %v", err)
		}
	}(tx)

	evt := cweevent.ExportEvent{
		DID:        did,
		HolderDID:  ec.HolderDID,
		ExportedBy: ec.ExportedBy,
		Format:     "zip",
		OccurredAt: time.Now().UTC(),
		UserRoles:  ec.UserRoles,
	}
	if err := event.Create(ctx, tx, evt, componenttype.ContractWorkflowEngine); err != nil {
		return err
	}
	return tx.Commit()
}

// zipWithManifest computes the bundle-manifest.json over files and writes the
// full ZIP. Every non-manifest entry is listed in the manifest with the
// SHA-256 of its packaged bytes.
func zipWithManifest(files bundleFiles, bundleType, rootDID string, components []componentInfo) ([]byte, error) {
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	entries := make([]manifestEntry, 0, len(paths))
	for _, p := range paths {
		sum := sha256.Sum256(files[p])
		entries = append(entries, manifestEntry{
			Path:   p,
			SHA256: hex.EncodeToString(sum[:]),
			Bytes:  len(files[p]),
		})
	}

	manifestJSON, err := json.MarshalIndent(bundleManifest{
		BundleType:  bundleType,
		RootDID:     rootDID,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Components:  components,
		Entries:     entries,
	}, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode bundle manifest: %w", err)
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, p := range paths {
		w, err := zw.Create(p)
		if err != nil {
			return nil, fmt.Errorf("zip create %s: %w", p, err)
		}
		if _, err := w.Write(files[p]); err != nil {
			return nil, fmt.Errorf("zip write %s: %w", p, err)
		}
	}
	mw, err := zw.Create("bundle-manifest.json")
	if err != nil {
		return nil, fmt.Errorf("zip create manifest: %w", err)
	}
	if _, err := mw.Write(manifestJSON); err != nil {
		return nil, fmt.Errorf("zip write manifest: %w", err)
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("close zip: %w", err)
	}
	return buf.Bytes(), nil
}

func signaturesToJSON(records []signdb.SignatureRecord) []map[string]any {
	out := make([]map[string]any, 0, len(records))
	for _, r := range records {
		entry := map[string]any{
			"signer_did":      r.SignerDID,
			"credential_type": r.CredentialType,
			"status":          r.Status,
		}
		if r.SignedAt != nil {
			entry["signed_at"] = r.SignedAt.UTC().Format(time.RFC3339)
		}
		if r.RevokedAt != nil {
			entry["revoked_at"] = r.RevokedAt.UTC().Format(time.RFC3339)
		}
		out = append(out, entry)
	}
	return out
}

// extractParentContractDID returns the single dcs:parentContract @id from a
// contract document (object or one-element array form), or "".
func extractParentContractDID(data *datatype.JSON) string {
	if data == nil || !data.IsNotNullValue() {
		return ""
	}
	var doc map[string]any
	if err := json.Unmarshal(*data, &doc); err != nil {
		return ""
	}
	value, ok := doc["dcs:parentContract"]
	if !ok {
		value = doc["parentContract"]
	}
	switch typed := value.(type) {
	case map[string]any:
		id, _ := typed["@id"].(string)
		return id
	case []any:
		if len(typed) == 0 {
			return ""
		}
		if first, ok := typed[0].(map[string]any); ok {
			id, _ := first["@id"].(string)
			return id
		}
	}
	return ""
}
