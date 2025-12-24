package util

import (
	"crypto/rand"
	"fmt"
)

const (
	// UUID v4 bit manipulation constants
	uuidVersionMask = 0x0f
	uuidVersion4    = 0x40
	uuidVariantMask = 0x3f
	uuidVariantRFC  = 0x80

	// UUID byte positions for version and variant
	uuidVersionByteIndex = 6
	uuidVariantByteIndex = 8

	// UUID byte slice sizes for formatting
	uuidBytesTotal  = 16
	uuidSlice1End   = 4
	uuidSlice2Start = 4
	uuidSlice2End   = 6
	uuidSlice3Start = 6
	uuidSlice3End   = 8
	uuidSlice4Start = 8
	uuidSlice4End   = 10
	uuidSlice5Start = 10
)

// GenerateUUID creates a simple UUID v4 without external dependencies
func GenerateUUID() (string, error) {
	b := make([]byte, uuidBytesTotal)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random bytes for UUID: %w", err)
	}
	// Set version (4) and variant bits
	b[uuidVersionByteIndex] = (b[uuidVersionByteIndex] & uuidVersionMask) | uuidVersion4
	b[uuidVariantByteIndex] = (b[uuidVariantByteIndex] & uuidVariantMask) | uuidVariantRFC
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		b[0:uuidSlice1End],
		b[uuidSlice2Start:uuidSlice2End],
		b[uuidSlice3Start:uuidSlice3End],
		b[uuidSlice4Start:uuidSlice4End],
		b[uuidSlice5Start:]), nil
}
