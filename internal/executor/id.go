package executor

import (
	"crypto/rand"
	"encoding/hex"
)

func newRunID() string {
	var b [8]byte
	_, _ = rand.Read(b[:]) // crypto/rand.Read never returns an error on supported platforms
	return hex.EncodeToString(b[:])
}
