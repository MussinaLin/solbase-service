-- name: GetEmailWalletByEmail :one
SELECT * FROM email_wallets
WHERE email = $1 AND deleted_at IS NULL;

-- name: CreateEmailWallet :one
INSERT INTO email_wallets (
    id, email, wallet_address, privy_user_id, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6
) RETURNING *;
