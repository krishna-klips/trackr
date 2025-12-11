package webhooks

import (
	"testing"
)

func TestSign(t *testing.T) {
	secret := "secret"
	payload := []byte("payload")

	// Calculated using: echo -n "payload" | openssl dgst -sha256 -hmac "secret"
	expected := "b82fcb791acec57859b989b430a826488ce2e479fdf92326bd0a2e8375a42ba4"

	got := Sign(secret, payload)

	if got != expected {
		t.Errorf("Sign() = %v, want %v", got, expected)
	}
}
