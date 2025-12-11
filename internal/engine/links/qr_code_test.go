package links

import (
	"testing"
)

func TestGenerateQRCode(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		size     int
		wantErr  bool
	}{
		{
			name:    "Valid QR Code",
			url:     "https://example.com",
			size:    512,
			wantErr: false,
		},
		{
			name:    "Size Too Small",
			url:     "https://example.com",
			size:    100,
			wantErr: true,
		},
		{
			name:    "Size Too Large",
			url:     "https://example.com",
			size:    5000,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GenerateQRCode(tt.url, tt.size)
			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateQRCode() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && len(got) == 0 {
				t.Errorf("GenerateQRCode() returned empty bytes")
			}
		})
	}
}
