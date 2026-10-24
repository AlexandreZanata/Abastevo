package portalgen

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

func brl(milli int64) string {
	sign := ""
	if milli < 0 {
		sign = "-"
		milli = -milli
	}
	return fmt.Sprintf("R$ %s%s,%03d", sign, groupThousands(milli/1000), milli%1000)
}

func plural(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}

func brlPtr(milli *int64) string {
	if milli == nil {
		return "—"
	}
	return brl(*milli)
}

func groupThousands(value int64) string {
	raw := strconv.FormatInt(value, 10)
	if len(raw) <= 3 {
		return raw
	}
	var out strings.Builder
	first := len(raw) % 3
	if first > 0 {
		out.WriteString(raw[:first])
	}
	for i := first; i < len(raw); i += 3 {
		if out.Len() > 0 {
			out.WriteByte('.')
		}
		out.WriteString(raw[i : i+3])
	}
	return out.String()
}

func dateBR(value string) string {
	day, err := time.Parse("2006-01-02", value)
	if err != nil {
		return value
	}
	return day.Format("02/01/2006")
}

func periodLabel(start, end string) string {
	if start == "" || end == "" {
		return "período não informado"
	}
	return dateBR(start) + " a " + dateBR(end)
}

func publicationLabel(value string) string {
	at, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return value
	}
	return at.UTC().Format("02/01/2006 15:04")
}

func cnpjBR(value string) string {
	if value == "" {
		return "CNPJ não informado"
	}
	clean := strings.ToUpper(strings.TrimSpace(value))
	if len(clean) != 14 {
		return value
	}
	return clean[0:2] + "." + clean[2:5] + "." + clean[5:8] + "/" + clean[8:12] + "-" + clean[12:14]
}

func fuelLabel(value string) string {
	switch value {
	case "GASOLINE_REGULAR":
		return "Gasolina comum"
	case "GASOLINE_ADDITIVED":
		return "Gasolina aditivada"
	case "GASOLINE_PREMIUM_GRADE":
		return "Gasolina premium"
	case "ETHANOL":
		return "Etanol hidratado"
	case "DIESEL_S500":
		return "Diesel S500"
	case "DIESEL_S10":
		return "Diesel S10"
	case "CNG":
		return "GNV"
	case "LPG_P13":
		return "GLP"
	default:
		return value
	}
}

func statusLabel(value string) string {
	switch value {
	case "active":
		return "Ativo (informado pela fonte)"
	case "suspended":
		return "Suspenso (informado pela fonte)"
	case "revoked":
		return "Revogado (informado pela fonte)"
	default:
		return value + " (informado pela fonte)"
	}
}

func locationLabel(value string) string {
	switch value {
	case "reviewed":
		return "Localização revisada"
	case "city-centroid":
		return "Localização aproximada por município"
	case "unknown", "":
		return "Localização não informada"
	default:
		return value
	}
}

func ufName(value string) string {
	names := map[string]string{
		"AC": "Acre", "AL": "Alagoas", "AP": "Amapá", "AM": "Amazonas",
		"BA": "Bahia", "CE": "Ceará", "DF": "Distrito Federal", "ES": "Espírito Santo",
		"GO": "Goiás", "MA": "Maranhão", "MT": "Mato Grosso", "MS": "Mato Grosso do Sul",
		"MG": "Minas Gerais", "PA": "Pará", "PB": "Paraíba", "PR": "Paraná",
		"PE": "Pernambuco", "PI": "Piauí", "RJ": "Rio de Janeiro", "RN": "Rio Grande do Norte",
		"RS": "Rio Grande do Sul", "RO": "Rondônia", "RR": "Roraima", "SC": "Santa Catarina",
		"SP": "São Paulo", "SE": "Sergipe", "TO": "Tocantins",
	}
	if name, ok := names[value]; ok {
		return name
	}
	return value
}

func addressLine(address map[string]string) string {
	parts := make([]string, 0, 7)
	street := strings.TrimSpace(address["street"])
	number := strings.TrimSpace(address["number"])
	if street != "" {
		if number != "" {
			street += ", " + number
		}
		parts = append(parts, street)
	}
	for _, key := range []string{"complement", "district", "postal_code"} {
		if value := strings.TrimSpace(address[key]); value != "" {
			parts = append(parts, value)
		}
	}
	return strings.Join(parts, " · ")
}

func isUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for i, r := range value {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if r != '-' {
				return false
			}
			continue
		}
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F') {
			return false
		}
	}
	return true
}
