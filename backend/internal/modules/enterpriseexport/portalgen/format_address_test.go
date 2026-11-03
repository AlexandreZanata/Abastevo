package portalgen

import "testing"

func TestStationAddressLine(t *testing.T) {
	cases := []struct {
		name    string
		address map[string]string
		want    string
	}{
		{
			name: "verbatim street and locality",
			address: map[string]string{
				"street": "Rua Exemplo", "number": "10", "complement": "Loja 2", "district": "Centro",
				"postal_code": "80000-000", "municipio": "Curitiba", "uf": "PR",
			},
			want: "Rua Exemplo, 10 · Loja 2 · Centro · 80000-000 · Curitiba/PR",
		},
		{
			name:    "municipality fallback",
			address: map[string]string{"municipio": "Curitiba", "uf": "PR"},
			want:    "Curitiba/PR",
		},
		{
			name: "coordinate and code literals",
			address: map[string]string{
				"street": "-25.42, -49.27", "district": "geo:-25.42,-49.27",
				"municipio": "4106902", "uf": "PR",
			},
		},
		{
			name:    "coordinates never used as number",
			address: map[string]string{"street": "Rua Exemplo", "number": "lat: -25.42, lon: -49.27"},
			want:    "Rua Exemplo",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := addressLine(tc.address); got != tc.want {
				t.Errorf("addressLine = %q, want %q", got, tc.want)
			}
		})
	}
}
