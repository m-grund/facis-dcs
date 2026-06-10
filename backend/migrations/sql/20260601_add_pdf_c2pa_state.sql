ALTER TABLE contracts
    ADD COLUMN IF NOT EXISTS pdf_c2pa_state TEXT;

ALTER TABLE contract_templates
    ADD COLUMN IF NOT EXISTS pdf_c2pa_state TEXT;
