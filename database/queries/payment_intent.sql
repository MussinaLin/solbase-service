-- name: CreatePaymentIntent :one
INSERT INTO payment_intents (
    id, intent_id, payer_chain, target_chain, merchant_recipient,
    receiver_email, amount, status, created_at, expires_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
) RETURNING *;

-- name: GetPaymentIntentByIntentID :one
SELECT * FROM payment_intents
WHERE intent_id = $1 AND deleted_at IS NULL;

-- name: UpdatePaymentIntentStatus :exec
UPDATE payment_intents
SET status = $2, error_message = $3
WHERE intent_id = $1;

-- name: UpdatePaymentIntentSolSettled :exec
UPDATE payment_intents
SET status = $2, amount = $3, payer_wallet = $4, sol_tx_hash = $5, sol_settled_at = $6
WHERE intent_id = $1;

-- name: UpdatePaymentIntentWithProof :exec
UPDATE payment_intents
SET sol_settle_proof = $2, status = $3
WHERE intent_id = $1;

-- name: UpdatePaymentIntentBaseSettled :exec
UPDATE payment_intents
SET status = $2, base_tx_hash = $3, base_settle_proof = $4, base_settled_at = $5, completed_at = $6
WHERE intent_id = $1;

-- name: UpdatePaymentIntentExpired :exec
UPDATE payment_intents
SET status = 'EXPIRED'
WHERE intent_id = $1;
