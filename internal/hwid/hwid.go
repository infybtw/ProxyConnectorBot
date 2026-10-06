// Package hwid generates stable device identifiers for subscriptions.
package hwid

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
)

// Generate returns a new random HWID in the format used by Happ/INCY:
// an uppercase UUID-like string (8-4-4-4-12). The value is generated once
// per subscription and stored in the database, so it stays stable across
// subscription refreshes.
func Generate() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	// Set UUID v4 version and variant bits.
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	h := hex.EncodeToString(b[:])
	h = strings.ToUpper(h)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32], nil
}
