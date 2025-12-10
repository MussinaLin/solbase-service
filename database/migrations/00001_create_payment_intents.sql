-- +goose Up
CREATE TABLE payment_intents (
    id VARCHAR(36) PRIMARY KEY,
    intent_id VARCHAR(36) UNIQUE NOT NULL,
    payer_chain VARCHAR(50) NOT NULL,
    target_chain VARCHAR(50) NOT NULL,
    payer_wallet VARCHAR(100),
    merchant_recipient VARCHAR(42) NOT NULL,
    receiver_email VARCHAR(255),
    amount VARCHAR(50) NOT NULL,

    -- Solana proofs
    sol_commit_receipt TEXT,
    sol_settle_proof TEXT,
    sol_tx_hash VARCHAR(100),

    -- Base proofs
    base_commit_receipt TEXT,
    base_settle_proof TEXT,
    base_tx_hash VARCHAR(66),

    -- Status tracking
    status VARCHAR(30) NOT NULL,
    error_message TEXT,

    -- Timestamps
    created_at TIMESTAMP NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    sol_settled_at TIMESTAMP,
    base_settled_at TIMESTAMP,
    completed_at TIMESTAMP,
    deleted_at TIMESTAMP
);

CREATE INDEX idx_payment_intents_status ON payment_intents(status);
CREATE INDEX idx_payment_intents_intent_id ON payment_intents(intent_id);
CREATE INDEX idx_payment_intents_expires_at ON payment_intents(expires_at);
CREATE INDEX idx_payment_intents_deleted_at ON payment_intents(deleted_at);

-- +goose Down
DROP INDEX IF EXISTS idx_payment_intents_deleted_at;
DROP INDEX IF EXISTS idx_payment_intents_expires_at;
DROP INDEX IF EXISTS idx_payment_intents_intent_id;
DROP INDEX IF EXISTS idx_payment_intents_status;
DROP TABLE IF EXISTS payment_intents;
