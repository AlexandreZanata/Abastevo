package jobs

import (
	"strings"
	"unicode"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
)

// normalizeText removes accents and converts characters to lowercase ASCII equivalents.
func normalizeText(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range strings.ToLower(s) {
		switch r {
		case 'á', 'à', 'ã', 'â', 'ä':
			b.WriteRune('a')
		case 'é', 'è', 'ê', 'ë':
			b.WriteRune('e')
		case 'í', 'ì', 'î', 'ï':
			b.WriteRune('i')
		case 'ó', 'ò', 'õ', 'ô', 'ö':
			b.WriteRune('o')
		case 'ú', 'ù', 'û', 'ü':
			b.WriteRune('u')
		case 'ç':
			b.WriteRune('c')
		default:
			if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) || r == '-' {
				b.WriteRune(r)
			} else {
				b.WriteRune(' ')
			}
		}
	}
	return b.String()
}

// containsAnyWord checks if the normalized haystack contains any of the search stems.
func containsAnyWord(haystack string, stems []string) bool {
	for _, stem := range stems {
		if strings.Contains(haystack, stem) {
			return true
		}
	}
	return false
}

var electricKeywords = []string{
	"eletric", "hibrid", "recarga", "bateria", "eletroposto",
	"plug-in", "veiculo eletrico", "carro eletrico", "kw",
}

var trafficRulesKeywords = []string{
	"transito", "multa", "cnh", "ctb", "detran", "senatran",
	"infracao", "radar", "velocidade", "habilitacao", "leis de transito",
	"pontos na carteira",
}

var roadsMobilityKeywords = []string{
	"rodovia", "dnit", "pedagio", "trafego", "mobilidade",
	"estrada", "concessionaria", "asfalto", "br-", "sp-", "rj-", "mg-",
	"interdicao", "faixa reversivel",
}

var driverEconomyKeywords = []string{
	"economia", "consumo", "km/l", "gasto", "poupar", "calibragem",
	"economizar", "custo por km",
}

var vehiclesTechKeywords = []string{
	"veiculo", "mecanica", "motor", "manutencao", "seguranca veicular",
	"montadora", "recall", "pneu", "airbag", "freio", "carro",
}

var fuelsKeywords = []string{
	"combustivel", "combustiveis", "gasolina", "etanol", "diesel",
	"gnv", "glp", "anp", "posto", "postos", "petrobras",
	"refinaria", "brent", "barril", "aditivada", "alcool",
	"bomba de combustivel", "preco dos combustiveis",
}

var abastevoDataKeywords = []string{
	"abastevo data", "dados abastevo", "relatorio abastevo", "pesquisa abastevo",
}

// ClassifyCategory maps title, excerpt and source tags to a domain category deterministically.
func ClassifyCategory(title, excerpt string, sourceCategories []string) domain.Category {
	combined := title + " " + excerpt + " " + strings.Join(sourceCategories, " ")
	norm := normalizeText(combined)

	// Highest specificity first
	if containsAnyWord(norm, abastevoDataKeywords) {
		return domain.CategoryAbastevoData
	}
	if containsAnyWord(norm, electricKeywords) {
		return domain.CategoryElectric
	}
	if containsAnyWord(norm, driverEconomyKeywords) {
		return domain.CategoryDriverEconomy
	}
	if containsAnyWord(norm, trafficRulesKeywords) {
		return domain.CategoryTrafficRules
	}
	if containsAnyWord(norm, roadsMobilityKeywords) {
		return domain.CategoryRoadsMobility
	}
	if containsAnyWord(norm, fuelsKeywords) {
		return domain.CategoryFuels
	}
	if containsAnyWord(norm, vehiclesTechKeywords) {
		return domain.CategoryVehiclesTech
	}

	// Default baseline
	return domain.CategoryFuels
}
