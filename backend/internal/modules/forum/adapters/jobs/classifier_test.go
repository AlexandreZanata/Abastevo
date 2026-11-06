package jobs

import (
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
)

func TestClassifyCategory_DeterministicRules(t *testing.T) {
	tests := []struct {
		name       string
		title      string
		excerpt    string
		categories []string
		expected   domain.Category
	}{
		{
			name:     "Gasolina price change",
			title:    "Preço da gasolina cai nos postos brasileiros segundo pesquisa da ANP",
			excerpt:  "Média nacional do combustível recua após reajuste nas refinarias.",
			expected: domain.CategoryFuels,
		},
		{
			name:     "Electric vehicles & charging",
			title:    "Novo corredor elétrico com eletropostos ultra-rápidos é inaugurado",
			excerpt:  "Motoristas de veículos elétricos e híbridos plug-in ganham novos pontos de recarga.",
			expected: domain.CategoryElectric,
		},
		{
			name:     "Traffic rules and CNH",
			title:    "Senatran alerta sobre novas regras para renovação da CNH e limites de velocidade",
			excerpt:  "Mudanças no Código de Trânsito Brasileiro preveem fiscalização eletrônica reforçada por radar.",
			expected: domain.CategoryTrafficRules,
		},
		{
			name:     "Roads and mobility",
			title:    "DNIT anuncia obras de pavimentação na BR-163 para desafogar o tráfego de carretas",
			excerpt:  "Concessionária inicia recuperação de asfalto em rodovias federais.",
			expected: domain.CategoryRoadsMobility,
		},
		{
			name:     "Driver economy",
			title:    "Dicas de calibragem e manutenção para economizar combustível no trânsito urbano",
			excerpt:  "Como melhorar a média de km/l e reduzir o custo por km rodado.",
			expected: domain.CategoryDriverEconomy,
		},
		{
			name:     "Vehicles tech",
			title:    "Montadoras anunciam recall de airbags e sistemas de freio em modelos 2025",
			excerpt:  "Segurança veicular é prioridade na atualização mecânica dos novos motores.",
			expected: domain.CategoryVehiclesTech,
		},
		{
			name:     "Abastevo data",
			title:    "Relatório Abastevo Data analisa dispersão de preços entre bairros da capital",
			excerpt:  "Pesquisa Abastevo revela diferenças de até 20% no litro do etanol.",
			expected: domain.CategoryAbastevoData,
		},
		{
			name:     "Fallback default to FUELS",
			title:    "Governo discute medidas para o setor automotivo em reunião ministerial",
			excerpt:  "Encontro em Brasília aborda incentivos e regulamentação geral.",
			expected: domain.CategoryFuels,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyCategory(tc.title, tc.excerpt, tc.categories)
			if got != tc.expected {
				t.Errorf("ClassifyCategory(%q, %q) = %q, expected %q", tc.title, tc.excerpt, got, tc.expected)
			}
		})
	}
}
