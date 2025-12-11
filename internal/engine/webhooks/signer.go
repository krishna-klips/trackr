package webhooks

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// Signer is responsible for generating HMAC signatures
// We already have GenerateHMAC in dispatcher.go, but for separation of concerns as per plan:

func Sign(secret string, payload []byte) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))
}
