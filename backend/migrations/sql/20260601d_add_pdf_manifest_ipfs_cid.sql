-- Stores CID of the standalone remote C2PA manifest object (JUMBF bytes)
-- for DCS-OR-C2PA-008 compliance.

ALTER TABLE contracts
    ADD COLUMN IF NOT EXISTS pdf_manifest_ipfs_cid TEXT;

ALTER TABLE contract_templates
    ADD COLUMN IF NOT EXISTS pdf_manifest_ipfs_cid TEXT;
