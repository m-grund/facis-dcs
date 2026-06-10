CREATE TABLE contract_signatures (
    id              BIGSERIAL PRIMARY KEY,
    contract_did    VARCHAR(255) NOT NULL REFERENCES contracts(did),
    signer_did      VARCHAR(255) NOT NULL,
    credential_type VARCHAR(255) NOT NULL DEFAULT 'stub',
    signature_bytes BYTEA,
    ipfs_cid        TEXT,
    status          VARCHAR(32)  NOT NULL DEFAULT 'PENDING',
    signed_at       TIMESTAMP,
    revoked_at      TIMESTAMP,
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_contract_signatures_did    ON contract_signatures(contract_did);
CREATE INDEX idx_contract_signatures_status ON contract_signatures(status);
