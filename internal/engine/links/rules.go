package links

import (
	"strings"
	"time"
)

type RequestContext struct {
	IPAddress   string
	UserAgent   string
	CountryCode string
	DeviceType  string
	OS          string
	Browser     string
	Referrer    string
	RequestTime time.Time
}

func (r *RedirectRules) Evaluate(ctx *RequestContext) string {
	if r == nil {
		return ""
	}

	// Priority: Device > Geo > Default

	// 1. Device-based routing
	if r.Device != nil && ctx.DeviceType != "" {
		if url, ok := r.Device[ctx.DeviceType]; ok {
			return url
		}
	}

	// 2. Geo-based routing
	if r.Geo != nil && ctx.CountryCode != "" {
		if url, ok := r.Geo[ctx.CountryCode]; ok {
			return url
		}
	}

	return ""
}

// Simple device detection logic
func ParseDeviceType(ua string) string {
	ua = strings.ToLower(ua)
	if strings.Contains(ua, "mobile") || strings.Contains(ua, "android") || strings.Contains(ua, "iphone") {
		if strings.Contains(ua, "ipad") || strings.Contains(ua, "tablet") {
			return "tablet"
		}
		return "mobile"
	}
	return "desktop"
}
