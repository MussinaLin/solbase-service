CREATE TABLE email_wallets (
    id VARCHAR(36) PRIMARY KEY,
    email VARCHAR(255) UNIQUE NOT NULL,
    wallet_address VARCHAR(42) NOT NULL,
    privy_user_id VARCHAR(100) NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    deleted_at TIMESTAMP
);

CREATE INDEX idx_email_wallets_email ON email_wallets(email);
CREATE INDEX idx_email_wallets_deleted_at ON email_wallets(deleted_at);
