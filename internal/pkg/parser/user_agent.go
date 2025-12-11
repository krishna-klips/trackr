package parser

import "strings"

func ParseUserAgent(ua string) (os, browser string) {
	uaLower := strings.ToLower(ua)

	// OS Detection
	if strings.Contains(uaLower, "windows") {
		os = "Windows"
	} else if strings.Contains(uaLower, "mac os") {
		os = "macOS"
	} else if strings.Contains(uaLower, "linux") {
		os = "Linux"
	} else if strings.Contains(uaLower, "android") {
		os = "Android"
	} else if strings.Contains(uaLower, "iphone") || strings.Contains(uaLower, "ipad") {
		os = "iOS"
	} else {
		os = "Unknown"
	}

	// Browser Detection
	if strings.Contains(uaLower, "chrome") && !strings.Contains(uaLower, "edge") {
		browser = "Chrome"
	} else if strings.Contains(uaLower, "safari") && !strings.Contains(uaLower, "chrome") {
		browser = "Safari"
	} else if strings.Contains(uaLower, "firefox") {
		browser = "Firefox"
	} else if strings.Contains(uaLower, "edge") {
		browser = "Edge"
	} else {
		browser = "Unknown"
	}

	return os, browser
}
