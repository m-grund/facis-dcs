CREATE TYPE contract_state AS ENUM ('DRAFT', 'NEGOTIATION', 'SUBMITTED', 'REJECTED', 'REVIEWED', 'APPROVED', 'TERMINATED', 'EXPIRED');
CREATE TYPE contract_expiration_policy AS ENUM ('RENEWAL', 'TERMINATION', 'ARCHIVING');


CREATE TABLE IF NOT EXISTS contracts
(
    did               VARCHAR(255),

    created_by        VARCHAR(255)   NOT NULL,
    created_at        TIMESTAMP      NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        TIMESTAMP      NOT NULL DEFAULT CURRENT_TIMESTAMP,

    start_date          TIMESTAMP,
    exp_date          TIMESTAMP,
    exp_policy        contract_expiration_policy,
    exp_notice_period INT,

    responsible_persons     JSONB DEFAULT '{}'::jsonb,

    state             contract_state NOT NULL,

    contract_version  INT NOT NULL DEFAULT 1,

    name              VARCHAR(255),
    description       TEXT,
    contract_data     JSONB DEFAULT '{}'::jsonb,
    search_vector     tsvector GENERATED ALWAYS AS (
        to_tsvector('english', contract_data::text)
        ) STORED,

    CONSTRAINT pk_contracts PRIMARY KEY (did),
    CONSTRAINT chk_did_not_empty CHECK (did <> '')
);


CREATE INDEX idx_contract_contracts_search ON contracts
    USING GIN (search_vector);


CREATE OR REPLACE FUNCTION update_updated_at_column()
    RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER contract_contracts_update_updated_at
    BEFORE UPDATE ON contracts
    FOR EACH ROW
EXECUTE FUNCTION update_updated_at_column();

------------------------------------------------------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS contract_history
(
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),

    did               VARCHAR(255),

    created_by        VARCHAR(255)   NOT NULL,
    created_at        TIMESTAMP      NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        TIMESTAMP      NOT NULL DEFAULT CURRENT_TIMESTAMP,

    start_date          TIMESTAMP,
    exp_date          TIMESTAMP,
    exp_policy        contract_expiration_policy,
    exp_notice_period INT,

    responsible_persons     JSONB DEFAULT '{}'::jsonb,

    state             contract_state NOT NULL,

    contract_version  INT NOT NULL DEFAULT 1,

    name              VARCHAR(255),
    description       TEXT,
    contract_data     JSONB DEFAULT '{}'::jsonb,
    search_vector     tsvector GENERATED ALWAYS AS (
        to_tsvector('english', contract_data::text)
        ) STORED,

    CONSTRAINT chk_did_not_empty CHECK (did <> '')
);

------------------------------------------------------------------------------------------------------------------------

CREATE OR REPLACE VIEW contracts_effective AS
SELECT
    did,
    created_by,
    created_at,
    updated_at,
    start_date,
    exp_date,
    exp_policy,
    exp_notice_period,
    CASE
        WHEN exp_date <= CURRENT_TIMESTAMP
            AND state NOT IN ('TERMINATED', 'REJECTED', 'EXPIRED')
            THEN 'EXPIRED'::contract_state
        ELSE state
        END AS state,
    contract_version,
    name,
    description,
    contract_data,
    search_vector,
    responsible_persons
FROM contracts;

------------------------------------------------------------------------------------------------------------------------

CREATE OR REPLACE VIEW contracts_effective_metadata AS
SELECT
    did,
    created_by,
    created_at,
    updated_at,
    start_date,
    exp_date,
    exp_policy,
    exp_notice_period,
    CASE
        WHEN exp_date <= CURRENT_TIMESTAMP
            AND state NOT IN ('TERMINATED', 'REJECTED', 'EXPIRED')
            THEN 'EXPIRED'::contract_state
        ELSE state
        END AS state,
    contract_version,
    name,
    description,
    responsible_persons
FROM contracts;

------------------------------------------------------------------------------------------------------------------------

CREATE OR REPLACE VIEW contracts_effective_process_data AS
SELECT
    did,
    created_by,
    created_at,
    updated_at,
    start_date,
    exp_date,
    exp_policy,
    exp_notice_period,
    CASE
        WHEN exp_date <= CURRENT_TIMESTAMP
            AND state NOT IN ('TERMINATED', 'REJECTED', 'EXPIRED')
            THEN 'EXPIRED'::contract_state
        ELSE state
        END AS state,
    contract_version
FROM contracts;

------------------------------------------------------------------------------------------------------------------------

CREATE TYPE contract_review_task_state AS ENUM ('OPEN', 'APPROVED', 'REJECTED');

CREATE TABLE IF NOT EXISTS contract_review_task
(
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),

    did             VARCHAR(255) NOT NULL CHECK (did <> ''),

    state    contract_review_task_state NOT NULL,
    reviewer VARCHAR(255)      NOT NULL CHECK (reviewer <> ''),

    created_by VARCHAR(255) NOT NULL,
    created_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT fk_review_task_contract
        FOREIGN KEY (did)
            REFERENCES contracts (did)
);

------------------------------------------------------------------------------------------------------------------------

CREATE TYPE contract_approval_task_state AS ENUM ('OPEN', 'APPROVED', 'REJECTED');

CREATE TABLE IF NOT EXISTS contract_approval_task
(
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),

    did             VARCHAR(255) NOT NULL CHECK (did <> ''),

    state    contract_approval_task_state NOT NULL,
    approver VARCHAR(255)        NOT NULL CHECK (approver <> ''),

    created_by VARCHAR(255) NOT NULL,
    created_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT fk_approval_task_contract
        FOREIGN KEY (did)
            REFERENCES contracts (did)
);

------------------------------------------------------------------------------------------------------------------------

CREATE TYPE contract_negotiation_task_state AS ENUM ('OPEN', 'ACCEPTED');

CREATE TABLE IF NOT EXISTS contract_negotiation_task
(
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),

    did             VARCHAR(255) NOT NULL CHECK (did <> ''),

    state    contract_negotiation_task_state NOT NULL,
    negotiator VARCHAR(255)        NOT NULL CHECK (negotiator <> ''),

    created_by VARCHAR(255) NOT NULL,
    created_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT fk_contract_negotiation_task
        FOREIGN KEY (did)
            REFERENCES contracts (did)
);

------------------------------------------------------------------------------------------------------------------------

CREATE TYPE contract_negotiation_decision AS ENUM ('ACCEPTED', 'REJECTED', 'CLOSED');

CREATE TABLE IF NOT EXISTS contract_negotiations
(
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),

    did                 VARCHAR(255) NOT NULL CHECK (did <> ''),
    contract_version    INT NOT NULL,

    change_request      JSONB DEFAULT '{}'::jsonb,

    created_by VARCHAR(255) NOT NULL,
    created_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT fk_contracts
        FOREIGN KEY (did)
            REFERENCES contracts (did)
);

CREATE TABLE IF NOT EXISTS contract_negotiation_decisions
(
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),

    negotiation_id      uuid,

    negotiator         VARCHAR(255) NOT NULL,
    decision            contract_negotiation_decision,
    rejection_reason    TEXT,

    CONSTRAINT fk_contract_negotiations
        FOREIGN KEY (negotiation_id)
            REFERENCES contract_negotiations (id)
            ON DELETE CASCADE
);