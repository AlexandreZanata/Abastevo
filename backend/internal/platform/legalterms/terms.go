// Package legal publishes the exact text reviewed by the Android owner.
package legal

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
)

const Version = "training-terms-v1"

//go:embed text/training-consent-pt-BR.txt
var Text string

var Digest = digest(Text)

func digest(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}
