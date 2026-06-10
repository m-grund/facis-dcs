ALTER TABLE contracts
    ADD COLUMN IF NOT EXISTS pdf_renderer_version TEXT;

ALTER TABLE contract_templates
    ADD COLUMN IF NOT EXISTS pdf_renderer_version TEXT;
