package links

import (
	"errors"
	"math/rand"
	"strings"
	"time"
)

const (
	shortCodeChars  = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	shortCodeLength = 7
)

type CodeAvailabilityChecker interface {
	ExistsByShortCode(code string) (bool, error)
}

func init() {
	rand.Seed(time.Now().UnixNano())
}

func GenerateShortCode(customCode string, checker CodeAvailabilityChecker) (string, error) {
	// Use custom code if provided
	if customCode != "" {
		if !isValidShortCode(customCode) {
			return "", errors.New("invalid short code format")
		}

		// Check availability
		exists, err := checker.ExistsByShortCode(customCode)
		if err != nil {
			return "", err
		}
		if exists {
			return "", errors.New("short code already taken")
		}

		return customCode, nil
	}

	// Generate random code with collision retry
	maxRetries := 5
	for i := 0; i < maxRetries; i++ {
		code := generateRandomCode(shortCodeLength)

		exists, err := checker.ExistsByShortCode(code)
		if err != nil {
			return "", err
		}
		if !exists {
			return code, nil
		}
	}

	// If collisions persist, increase length
	// Try one more time with +1 length
	code := generateRandomCode(shortCodeLength + 1)
	exists, err := checker.ExistsByShortCode(code)
	if err != nil {
		return "", err
	}
	if exists {
		return "", errors.New("failed to generate unique short code")
	}

	return code, nil
}

func generateRandomCode(length int) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = shortCodeChars[rand.Intn(len(shortCodeChars))]
	}
	return string(b)
}

func isValidShortCode(code string) bool {
	if len(code) < 3 || len(code) > 12 {
		return false
	}

	// Only alphanumeric
	for _, c := range code {
		if !strings.ContainsRune(shortCodeChars, c) {
			return false
		}
	}

	// Reserved codes
	reserved := []string{"api", "admin", "dashboard", "login", "signup", "health", "metrics"}
	for _, r := range reserved {
		if strings.EqualFold(code, r) {
			return false
		}
	}

	return true
}
