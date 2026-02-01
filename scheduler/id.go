package scheduler

import (
	"crypto/rand"
	"encoding/binary"
	"strings"
	"time"
)

const (
	idLength = 8
	alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
)

// GenerateID generates a short random ID using base62 encoding
func GenerateID() string {
	var sb strings.Builder
	sb.Grow(idLength)

	// Read random bytes
	buf := make([]byte, idLength*2) // Extra bytes for better randomness
	if _, err := rand.Read(buf); err != nil {
		// Fallback to less secure but functional ID
		return fallbackID()
	}

	for i := 0; i < idLength; i++ {
		// Use 2 bytes per character for better distribution
		val := binary.BigEndian.Uint16(buf[i*2:])
		idx := int(val) % len(alphabet)
		sb.WriteByte(alphabet[idx])
	}

	return sb.String()
}

// fallbackID generates an ID using time as entropy source
// This is only used if crypto/rand fails
func fallbackID() string {
	entropy := uint64(time.Now().UnixNano())
	var sb strings.Builder
	sb.Grow(idLength)

	for i := 0; i < idLength; i++ {
		idx := int(entropy) % len(alphabet)
		sb.WriteByte(alphabet[idx])
		entropy = entropy / uint64(len(alphabet))
	}

	return sb.String()
}
