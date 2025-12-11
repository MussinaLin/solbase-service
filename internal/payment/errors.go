package payment

import "errors"

var (
	// ErrNotFound is returned when a payment intent is not found.
	ErrNotFound = errors.New("payment intent not found")

	// ErrInvalidInput is returned when input validation fails.
	ErrInvalidInput = errors.New("invalid input")

	// ErrExpired is returned when a payment intent has expired.
	ErrExpired = errors.New("payment intent has expired")

	// ErrInvalidStatus is returned when the intent status doesn't allow the operation.
	ErrInvalidStatus = errors.New("invalid intent status for this operation")

	// ErrProofValidation is returned when proof validation fails.
	ErrProofValidation = errors.New("proof validation failed")

	// ErrInsufficientFunds is returned when there are insufficient funds.
	ErrInsufficientFunds = errors.New("insufficient funds")

	// ErrEmailRequired is returned when email is required but not provided.
	ErrEmailRequired = errors.New("either email or recipient is required")

	// ErrBothProvided is returned when both email and recipient are provided.
	ErrBothProvided = errors.New("cannot provide both email and recipient")

	// ErrInvalidPayerChain is returned when an invalid payer chain is specified.
	ErrInvalidPayerChain = errors.New("invalid payer_chain: must be solana, base, or bsc")

	// ErrConcurrentUpdate is returned when a concurrent update conflicts with the operation.
	ErrConcurrentUpdate = errors.New("concurrent update detected: intent status has changed")

	// ErrInvalidEmail is returned when email format is invalid.
	ErrInvalidEmail = errors.New("invalid email format")

	// ErrInvalidRecipient is returned when recipient address format is invalid.
	ErrInvalidRecipient = errors.New("invalid recipient address format")

	// ErrInvalidAmount is returned when amount is invalid (negative, zero, or malformed).
	ErrInvalidAmount = errors.New("invalid amount: must be a positive number with up to 6 decimal places")
)
