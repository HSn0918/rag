package redis

import (
	"crypto/sha256"
	"fmt"
)

func hashText(text string) string {
	hash := sha256.Sum256([]byte(text))
	return fmt.Sprintf("%x", hash)
}
