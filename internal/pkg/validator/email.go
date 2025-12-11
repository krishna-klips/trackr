package validator

import (
	"errors"
	"net"
	"strings"
)

var blockedDomains = []string{
	"gmail.com", "yahoo.com", "hotmail.com", "outlook.com",
	"aol.com", "icloud.com", "protonmail.com", "mail.com",
	"zoho.com", "yandex.com", "gmx.com", "live.com",
}

func IsCorporateEmail(email string) error {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return errors.New("invalid email format")
	}

	domain := strings.ToLower(parts[1])

	// Check against blocked list
	for _, blocked := range blockedDomains {
		if domain == blocked {
			return errors.New("consumer email domains not allowed")
		}
	}

	// Additional validation: check MX records
	mx, err := net.LookupMX(domain)
	if err != nil || len(mx) == 0 {
		return errors.New("invalid email domain or no MX records found")
	}

	return nil
}
