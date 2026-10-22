package domain

import (
	"strings"
	"time"
)

// Interpret resolves one Portuguese question into a typed query,
// alternatives or an unsupported reason. Reference date supplies the
// latest survey week; the interpreter never reads a clock, network,
// database or model. Adversarial text is matched literally, never run.
func Interpret(question string, now time.Time) Interpretation {
	normalized, ok := normalize(question)
	if !ok || normalized == "" {
		return Interpretation{State: StateUnsupported, Reason: ReasonUnsupportedIntent}
	}
	price := priceScore(normalized)
	official := officialScore(normalized)
	electric := evScore(normalized)
	if price == 0 && official == 0 && electric == 0 {
		return Interpretation{State: StateUnsupported, Reason: ReasonUnsupportedIntent}
	}
	if electric > price && electric > official {
		return interpretEV(normalized, now)
	}
	if official > price {
		return interpretOfficial(normalized)
	}
	return interpretPrice(normalized, now)
}

// priceScore counts allowlisted price signals. Fuel names weigh most;
// generic price words weigh least.
func priceScore(normalized string) int {
	score := 0
	for _, f := range fuels {
		for _, fragment := range f.fragments {
			if strings.Contains(normalized, fragment) {
				score += 3
				break
			}
		}
	}
	if contains(normalized, "preco", "precos", "caro", "cara", "barato", "barata", "media", "ranking", "valor", "valores", "combustivel", "combustiveis") {
		score++
	}
	if contains(normalized, "maior", "menor", "mais caro", "mais barata", "mais barato") {
		score++
	}
	return score
}

// officialScore counts allowlisted official-record signals.
func officialScore(normalized string) int {
	score := 0
	if contains(normalized, "fiscalizacao", "fiscalizacoes", "acao fiscal", "acoes fiscais", "documento de fiscalizacao") {
		score += 3
	}
	if contains(normalized, "auto de infracao", "autos de infracao", "infracao", "infracoes", "multa", "multas", "autuado", "autuacao") {
		score += 3
	}
	if contains(normalized, "analise de qualidade", "analises", "ensaio", "ensaios", "qualidade do combustivel", "amostra") {
		score += 3
	}
	if contains(normalized, "registros da anp", "registros oficiais", "acoes da anp") {
		score++
	}
	return score
}

// extractFuel maps normalized text to an exact fuel/unit pair, preferring
// the longest match across all fuels ("gasolina aditivada" beats
// "gasolina"). Empty fuel with unknown fuel-like words is an unknown fuel;
// empty without them is a missing fuel.
func extractFuel(normalized string) (fuel, unit string, unknown, missing bool) {
	bestFuel, bestUnit, bestLen := "", "", 0
	for _, f := range fuels {
		for _, fragment := range f.fragments {
			if strings.Contains(normalized, fragment) && len(fragment) > bestLen {
				bestFuel, bestUnit, bestLen = f.fuel, f.unit, len(fragment)
			}
		}
	}
	if bestFuel != "" {
		return bestFuel, bestUnit, false, false
	}
	if contains(normalized, unknownFuels...) {
		return "", "", true, false
	}
	return "", "", false, true
}

// extractGeography maps normalized text to an exact geography. Country is
// the national default; UF matches codes or full names accent-free; a city
// after "em" passes through for API-side exact matching (unknown cities
// page empty, never guessed). Country is always BR.
func extractGeography(normalized string) Geography {
	for code := range ufCodes() {
		if containsWord(normalized, strings.ToLower(code)) {
			return Geography{Level: "STATE", Country: "BR", UF: code}
		}
	}
	for name, code := range ufs {
		if strings.Contains(normalized, name) {
			return Geography{Level: "STATE", Country: "BR", UF: code}
		}
	}
	if city := extractCity(normalized); city != "" {
		return Geography{Level: "MUNICIPALITY", Country: "BR", Municipality: city}
	}
	return Geography{Level: "COUNTRY", Country: "BR"}
}

func ufCodes() map[string]bool {
	codes := map[string]bool{}
	for _, code := range ufs {
		codes[code] = true
	}
	for _, code := range []string{"AC", "AL", "AM", "AP", "BA", "CE", "DF", "ES", "GO", "MA", "MG", "MS", "MT", "PA", "PB", "PE", "PI", "PR", "RJ", "RN", "RO", "RR", "RS", "SC", "SE", "SP", "TO"} {
		codes[code] = true
	}
	return codes
}

// containsWord matches whole words only, so "sp" does not fire inside
// "hospital" and "am" does not fire inside "programa".
func containsWord(normalized, word string) bool {
	for _, token := range strings.Fields(normalized) {
		if token == word {
			return true
		}
	}
	return false
}

// extractCity captures the words after "em" as a municipality candidate.
// Accents were stripped by normalization; the API matches source-stated
// names exactly, so unknown spellings page empty instead of guessing.
func extractCity(normalized string) string {
	fields := strings.Fields(normalized)
	for i, token := range fields {
		if token == "em" && i+1 < len(fields) {
			candidate := strings.Join(fields[i+1:], " ")
			candidate = strings.TrimRight(candidate, " ?!.")
			if candidate != "" && len(candidate) <= 80 {
				return candidate
			}
		}
	}
	return ""
}
