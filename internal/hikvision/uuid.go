package hikvision

import (
	"crypto/rand"
	"fmt"
)

// newSearchID generates a random UUID v4 string for use as ISAPI searchID.
// CHK072: searchID is always generated internally — never accepted from HTTP requests
// to prevent ISAPI injection. The value is non-deterministic (different each call).
func newSearchID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failure is extremely rare; fall back to a zeroed UUID v4 skeleton
		// rather than propagating an error into the search path. The device will accept
		// any non-empty searchID string.
		b = make([]byte, 16)
	}
	// Set UUID v4 version bits (RFC 4122 §4.4)
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant bits
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
