package domain

import (
	"strings"
	"unicode"
)

// StationDisplayName implements B-BR-STATION-NAME-01. Correction is supplied
// only by the approved projection, never by an official source parser. Legal
// and trade source values remain separate; casing is never title-cased.
func StationDisplayName(legalName, tradeName, cnpj, correction string) string {
	clean := func(value string) string { return strings.Join(strings.Fields(value), " ") }
	if value := clean(correction); value != "" {
		return value
	}
	trade := clean(tradeName)
	if !usableTradeName(trade, clean(legalName), cnpj) {
		return clean(legalName)
	}
	return trade
}

func usableTradeName(trade, legal, cnpj string) bool {
	alphanumeric := func(value string) string {
		return strings.Map(func(r rune) rune {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				return unicode.ToUpper(r)
			}
			return -1
		}, value)
	}
	identity := alphanumeric(trade)
	if identity == "" || strings.EqualFold(trade, legal) ||
		(cnpj != "" && identity == alphanumeric(cnpj)) {
		return false
	}
	placeholder := strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Mn, r) {
			return -1
		}
		switch upper := unicode.ToUpper(r); upper {
		case 'À', 'Á', 'Â', 'Ã', 'Ä', 'Å':
			return 'A'
		case 'È', 'É', 'Ê', 'Ë':
			return 'E'
		case 'Ì', 'Í', 'Î', 'Ï':
			return 'I'
		case 'Ò', 'Ó', 'Ô', 'Õ', 'Ö':
			return 'O'
		case 'Ù', 'Ú', 'Û', 'Ü':
			return 'U'
		default:
			return upper
		}
	}, trade)
	switch placeholder {
	case "NAO POSSUI", "SEM NOME", "NAO INFORMADO":
		return false
	}
	return true
}
