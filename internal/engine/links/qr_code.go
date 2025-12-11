package links

import (
	"errors"

	"github.com/skip2/go-qrcode"
)

func GenerateQRCode(shortURL string, size int) ([]byte, error) {
	// Default size
	if size == 0 {
		size = 512
	}

	// Validate size
	if size < 128 || size > 2048 {
		return nil, errors.New("invalid size: must be between 128 and 2048")
	}

	// Generate QR code
	qr, err := qrcode.New(shortURL, qrcode.Medium)
	if err != nil {
		return nil, err
	}

	qr.DisableBorder = false

	// Return PNG bytes
	return qr.PNG(size)
}
