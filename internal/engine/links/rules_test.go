package links

import "testing"

func TestRedirectRules_Evaluate(t *testing.T) {
	tests := []struct {
		name     string
		rules    *RedirectRules
		ctx      *RequestContext
		expected string
	}{
		{
			name: "Geo Match",
			rules: &RedirectRules{
				Geo: map[string]string{"US": "https://us.example.com"},
			},
			ctx:      &RequestContext{CountryCode: "US"},
			expected: "https://us.example.com",
		},
		{
			name: "Device Match Priority",
			rules: &RedirectRules{
				Geo:    map[string]string{"US": "https://us.example.com"},
				Device: map[string]string{"mobile": "https://m.example.com"},
			},
			ctx:      &RequestContext{CountryCode: "US", DeviceType: "mobile"},
			expected: "https://m.example.com",
		},
		{
			name:     "No Match",
			rules:    &RedirectRules{Geo: map[string]string{"US": "https://us.example.com"}},
			ctx:      &RequestContext{CountryCode: "GB"},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.rules.Evaluate(tt.ctx)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}
