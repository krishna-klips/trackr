package links

import (
	"errors"
	"net/url"
)

func ValidateLink(link *Link) error {
	if link.DestinationURL == "" {
		return errors.New("destination_url is required")
	}

	u, err := url.Parse(link.DestinationURL)
	if err != nil {
		return errors.New("invalid destination_url format")
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("destination_url must start with http:// or https://")
	}

	// Validate Redirect Type
	if link.RedirectType != "" && link.RedirectType != "temporary" && link.RedirectType != "permanent" {
		return errors.New("redirect_type must be 'temporary' or 'permanent'")
	}

	// Validate Rules (basic check)
	if link.Rules != nil {
		// Could add deeper validation for country codes etc.
	}

	return nil
}
