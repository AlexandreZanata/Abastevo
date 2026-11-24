package domain

import (
	"strings"
	"testing"
)

func TestPublicNameNormalizesNFCAndWhitespace(t *testing.T) {
	if got := NormalizePublicName("  A\u0301na\u00a0  d'Ávila-Silva.  "); got != "Ána d'Ávila-Silva." {
		t.Errorf("normalized public name = %q", got)
	}
}

func TestValidPublicName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		valid bool
	}{
		{"two letters", "É阿", true},
		{"40 runes", strings.Repeat("仁", 40), true},
		{"four digits", "João-٢٠26", true},
		{"combining mark", "Ймя", true},
		{"one rune", "É", false},
		{"41 runes", strings.Repeat("仁", 41), false},
		{"one letter", "É. 1234", false},
		{"five digits", "João 8٢٢٢٢", false},
		{"at sign", "João@Silva", false},
		{"slash", "João/Silva", false},
		{"colon", "João:Silva", false},
		{"emoji", "João🟢", false},
		{"control before trim", "\nJoão Silva", false},
		{"control among spaces", "João\tSilva", false},
		{"format", "João\u200dSilva", false},
		{"app name", "ABASTEVO", false},
		{"admin", " AdMiN ", false},
		{"administrator", "Administrador", false},
		{"moderator", "Moderador", false},
		{"moderation", "MoDeRaCaO", false},
		{"moderation accented", "MoDeRaC\u0327A\u0303O", false},
		{"support", "Suporte", false},
		{"ANP", "ANP", false},
		{"official", "Oficial", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidPublicName(tt.input); got != tt.valid {
				t.Errorf("ValidPublicName(%q) = %v, want %v", tt.input, got, tt.valid)
			}
		})
	}
}

func TestEmailHintKeepsOneCharacterAndNormalizedDomain(t *testing.T) {
	if got := EmailHint(" Ána@EXAMPLE.INVALID "); got != "á***@example.invalid" {
		t.Errorf("email hint = %q", got)
	}
}
