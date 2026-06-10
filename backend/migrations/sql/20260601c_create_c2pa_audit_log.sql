CREATE TABLE IF NOT EXISTS c2pa_audit_log (
    id              BIGSERIAL PRIMARY KEY,
    entity_type     TEXT         NOT NULL CHECK (entity_type IN ('contract', 'template')),
    entity_did      VARCHAR(255) NOT NULL CHECK (entity_did <> ''),
    from_state      TEXT,
    to_state        TEXT         NOT NULL,
    actor_did       TEXT         NOT NULL,
    reason          TEXT,
    vc_id           TEXT,
    manifest_hash   TEXT,
    occurred_at     TIMESTAMP    NOT NULL,
    created_at      TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_c2pa_audit_log_entity_did
    ON c2pa_audit_log (entity_did);

CREATE INDEX IF NOT EXISTS idx_c2pa_audit_log_occurred_at
    ON c2pa_audit_log (occurred_at DESC);

CREATE OR REPLACE FUNCTION c2pa_audit_log_no_modify()
    RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'c2pa_audit_log is append-only';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_c2pa_audit_log_no_update ON c2pa_audit_log;
CREATE TRIGGER trg_c2pa_audit_log_no_update
    BEFORE UPDATE ON c2pa_audit_log
    FOR EACH ROW
EXECUTE FUNCTION c2pa_audit_log_no_modify();

DROP TRIGGER IF EXISTS trg_c2pa_audit_log_no_delete ON c2pa_audit_log;
CREATE TRIGGER trg_c2pa_audit_log_no_delete
    BEFORE DELETE ON c2pa_audit_log
    FOR EACH ROW
EXECUTE FUNCTION c2pa_audit_log_no_modify();
