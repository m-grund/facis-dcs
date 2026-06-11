-- pdf_manifest_hash: SHA-256 hex of the latest JUMBF manifest bytes written to
-- the PDF.  Stored alongside the IPFS CID so the chain link is queryable without
-- an IPFS round-trip.
--
-- prev_manifest_hash: carry-forward chain link populated atomically when
-- pdf_ipfs_cid is cleared by a content-changing edit
-- (prev_manifest_hash = pdf_manifest_hash, pdf_manifest_hash = NULL).
-- Consumed by the next appendAndCache / appendOneTemplateManifest call and then
-- set back to NULL.  This preserves DCS-OR-C2PA-001 / DCS-FR-TR-08 chain
-- continuity across content edits (Gap E).

ALTER TABLE contracts
    ADD COLUMN IF NOT EXISTS pdf_manifest_hash   TEXT,
    ADD COLUMN IF NOT EXISTS prev_manifest_hash  TEXT;

ALTER TABLE contract_templates
    ADD COLUMN IF NOT EXISTS pdf_manifest_hash   TEXT,
    ADD COLUMN IF NOT EXISTS prev_manifest_hash  TEXT;
