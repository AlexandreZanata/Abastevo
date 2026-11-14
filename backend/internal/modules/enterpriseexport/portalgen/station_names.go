package portalgen

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

// A canonical public label is primary; an equal or unknown legal name is not
// rendered a second time. Legacy snapshots simply have no secondary fact.
func stationLegalName(station enterpriseexport.Station) string {
	legal := strings.TrimSpace(kernel.PublicStationLegalName(station.LegalName))
	if strings.EqualFold(strings.Join(strings.Fields(legal), " "), strings.Join(strings.Fields(station.DisplayName), " ")) {
		return ""
	}
	return legal
}

var (
	stationParticles = map[string]bool{"de": true, "da": true, "do": true, "das": true, "dos": true, "e": true, "na": true, "no": true, "nas": true, "nos": true, "em": true}
	stationAcronyms  = map[string]bool{"BR": true, "GNV": true, "GLP": true, "ME": true, "EPP": true, "SA": true, "S/A": true, "S.A.": true, "S.A": true, "MEI": true}
	romanNumeral     = regexp.MustCompile(`^(?:I{1,3}|IV|VI{0,3}|IX|XI{0,3}|XIV|XV)$`)
)

// stationDisplayTitle renders an ALL-CAPS registry name in Portuguese title
// case for titles and H1s ("AUTO POSTO SÃO JOÃO LTDA" -> "Auto Posto São
// João Ltda"): particles lower-case after the first word; acronyms, roman
// numerals, single letters, vowel-less short tokens and tokens with digits
// stay upper-case. A name that already has lower-case letters is kept.
func stationDisplayTitle(name string) string {
	words := strings.Fields(name)
	if strings.ContainsFunc(name, unicode.IsLower) {
		return strings.Join(words, " ")
	}
	for i, word := range words {
		lower := strings.ToLower(word)
		runes := utf8.RuneCountInString(word)
		switch {
		case i > 0 && stationParticles[lower]:
			words[i] = lower
		case stationAcronyms[word] || romanNumeral.MatchString(word) || runes == 1 ||
			strings.ContainsFunc(word, unicode.IsDigit) || (runes <= 4 && !strings.ContainsFunc(word, isVowel)):
			// kept as written
		case i == 0 && strings.HasPrefix(lower, "d'") && len(lower) > 2:
			words[i] = "D'" + titleWord(lower[2:])
		default:
			words[i] = titleWord(lower)
		}
	}
	return strings.Join(words, " ")
}

func isVowel(r rune) bool {
	return strings.ContainsRune("AEIOUYÁÉÍÓÚÂÊÔÃÕÀÜaeiouyáéíóúâêôãõàü", r)
}

// stationRegistryName is the source display name, shown next to the
// normalized title when they differ so the registry name stays visible.
func stationRegistryName(station enterpriseexport.Station) string {
	raw := strings.Join(strings.Fields(station.DisplayName), " ")
	if raw == stationDisplayTitle(station.DisplayName) {
		return ""
	}
	return raw
}
