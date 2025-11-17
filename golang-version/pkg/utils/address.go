package utils

import (
	"regexp"
	"strings"

	"github.com/go-playground/validator/v10"
)

var (
	// Ethereum address pattern: 0x followed by 40 hexadecimal characters
	ethAddressRegex = regexp.MustCompile(`^0x[a-fA-F0-9]{40}$`)
)

// IsValidEthAddress checks if a string is a valid Ethereum address
func IsValidEthAddress(address string) bool {
	return ethAddressRegex.MatchString(address)
}

// ValidateEthAddress is a custom validator for go-playground/validator
func ValidateEthAddress(fl validator.FieldLevel) bool {
	address := fl.Field().String()
	return IsValidEthAddress(address)
}

// NormalizeEthAddress normalizes an Ethereum address to lowercase
func NormalizeEthAddress(address string) string {
	return strings.ToLower(address)
}
