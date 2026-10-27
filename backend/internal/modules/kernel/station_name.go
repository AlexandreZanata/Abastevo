package kernel

import (
	"regexp"
	"strings"
)

// cpfShapedToken matches a standalone 11-digit or NNN.NNN.NNN-NN token, the
// same shape the portal privacy check refuses.
var cpfShapedToken = regexp.MustCompile(`(^|[^0-9A-Za-z])([0-9]{11}|[0-9]{3}\.[0-9]{3}\.[0-9]{3}-[0-9]{2})([^0-9A-Za-z]|$)`)

// PublicStationName minimizes an official station name for every public
// surface (B-BR-017): MEI legal names often carry the owner's CPF, which is
// never public. A name left empty falls back to the establishment CNPJ.
func PublicStationName(name, cnpj string) string {
	original := strings.TrimSpace(name)
	for cpfShapedToken.MatchString(name) {
		name = cpfShapedToken.ReplaceAllString(name, "$1$3")
	}
	name = strings.Join(strings.Fields(name), " ")
	if name == "" && original != "" {
		if cnpj != "" {
			return "Posto " + cnpj
		}
		return "Posto"
	}
	return name
}
