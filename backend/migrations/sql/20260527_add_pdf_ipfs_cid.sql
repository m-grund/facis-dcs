ALTER TABLE contracts
    ADD COLUMN IF NOT EXISTS pdf_ipfs_cid TEXT;

ALTER TABLE contract_templates
    ADD COLUMN IF NOT EXISTS pdf_ipfs_cid TEXT;
