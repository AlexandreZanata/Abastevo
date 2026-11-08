package domain

import (
	"strings"
	"testing"
)

func TestValidateTopicTitle(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr error
	}{
		{
			name:    "valid plain title",
			input:   "Preço da gasolina em São Paulo",
			want:    "Preço da gasolina em São Paulo",
			wantErr: nil,
		},
		{
			name:    "valid title with leading and trailing spaces",
			input:   "   Qual o melhor posto na Rodovia dos Bandeirantes?   ",
			want:    "Qual o melhor posto na Rodovia dos Bandeirantes?",
			wantErr: nil,
		},
		{
			name:    "valid title with accents and emojis",
			input:   "Etanol a R$ 3,19 compensa mais que gasolina? ⛽🚗",
			want:    "Etanol a R$ 3,19 compensa mais que gasolina? ⛽🚗",
			wantErr: nil,
		},
		{
			name:    "empty title",
			input:   "",
			want:    "",
			wantErr: ErrTopicTitleEmpty,
		},
		{
			name:    "whitespace only title",
			input:   "   \t\n  ",
			want:    "",
			wantErr: ErrTopicTitleEmpty,
		},
		{
			name:    "title exceeding 120 unicode runes",
			input:   strings.Repeat("a", 121),
			want:    "",
			wantErr: ErrTopicTitleTooLong,
		},
		{
			name:    "title exactly 120 unicode runes",
			input:   strings.Repeat("a", 120),
			want:    strings.Repeat("a", 120),
			wantErr: nil,
		},
		{
			name:    "title with null byte is rejected",
			input:   "Título com byte nulo\x00 no meio",
			want:    "",
			wantErr: ErrInvalidUTF8,
		},
		{
			name:    "title with invalid utf-8 byte sequence",
			input:   "Título inválido \xff\xfe",
			want:    "",
			wantErr: ErrInvalidUTF8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidateTopicTitle(tt.input)
			if tt.wantErr != nil {
				if err != tt.wantErr {
					t.Fatalf("expected error %v, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestValidateTopicBody(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr error
	}{
		{
			name:    "valid plain body with paragraphs",
			input:   "Primeiro parágrafo de discussão.\n\nSegundo parágrafo com detalhes e números: R$ 4,89.",
			want:    "Primeiro parágrafo de discussão.\n\nSegundo parágrafo com detalhes e números: R$ 4,89.",
			wantErr: nil,
		},
		{
			name:    "empty body",
			input:   "",
			want:    "",
			wantErr: ErrTopicBodyEmpty,
		},
		{
			name:    "whitespace only body",
			input:   "   \n\t  \n",
			want:    "",
			wantErr: ErrTopicBodyEmpty,
		},
		{
			name:    "body exceeding 4000 unicode runes",
			input:   strings.Repeat("a", 4001),
			want:    "",
			wantErr: ErrTopicBodyTooLong,
		},
		{
			name:    "body exactly 4000 unicode runes",
			input:   strings.Repeat("a", 4000),
			want:    strings.Repeat("a", 4000),
			wantErr: nil,
		},
		{
			name:    "body with null byte",
			input:   "Texto com \x00 caractere nulo",
			want:    "",
			wantErr: ErrInvalidUTF8,
		},
		{
			name:    "body with invalid utf-8",
			input:   "Texto com byte \x80 inválido",
			want:    "",
			wantErr: ErrInvalidUTF8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidateTopicBody(tt.input)
			if tt.wantErr != nil {
				if err != tt.wantErr {
					t.Fatalf("expected error %v, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestValidateTopicRegion(t *testing.T) {
	ibgeSP := 3550308       // São Paulo (35)
	ibgeRJ := 3304557       // Rio de Janeiro (33)
	ibgeBadLength := 355030 // 6 digits

	tests := []struct {
		name    string
		uf      string
		ibge    *int
		wantErr error
	}{
		{
			name:    "no region tags",
			uf:      "",
			ibge:    nil,
			wantErr: nil,
		},
		{
			name:    "valid UF only",
			uf:      "SP",
			ibge:    nil,
			wantErr: nil,
		},
		{
			name:    "valid UF and matching IBGE",
			uf:      "SP",
			ibge:    &ibgeSP,
			wantErr: nil,
		},
		{
			name:    "valid UF RJ and matching IBGE",
			uf:      "RJ",
			ibge:    &ibgeRJ,
			wantErr: nil,
		},
		{
			name:    "mismatched UF and IBGE prefix",
			uf:      "RJ",
			ibge:    &ibgeSP, // SP code under RJ UF
			wantErr: ErrInvalidRegion,
		},
		{
			name:    "invalid UF code",
			uf:      "XX",
			ibge:    nil,
			wantErr: ErrInvalidRegion,
		},
		{
			name:    "lowercase UF",
			uf:      "sp",
			ibge:    nil,
			wantErr: ErrInvalidRegion,
		},
		{
			name:    "IBGE invalid length (not 7 digits)",
			uf:      "SP",
			ibge:    &ibgeBadLength,
			wantErr: ErrInvalidRegion,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTopicRegion(tt.uf, tt.ibge)
			if tt.wantErr != nil {
				if err != tt.wantErr {
					t.Fatalf("expected error %v, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateTopic(t *testing.T) {
	t.Run("valid topic", func(t *testing.T) {
		err := ValidateTopic(
			CategoryFuels,
			"Preço em Campinas",
			"Posto na Av. Francisco Glicério com etanol a R$ 3,25.",
			"SP",
			nil,
			nil,
			nil,
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("invalid category", func(t *testing.T) {
		err := ValidateTopic(
			Category("INVALID_CAT"),
			"Título válido",
			"Corpo válido com mais de vinte caracteres.",
			"",
			nil,
			nil,
			nil,
		)
		if err != ErrInvalidCategory {
			t.Fatalf("expected ErrInvalidCategory, got %v", err)
		}
	})

	t.Run("invalid fuel product", func(t *testing.T) {
		badFuel := "KEROSENE_JET"
		err := ValidateTopic(
			CategoryFuels,
			"Título válido",
			"Corpo válido de discussão.",
			"",
			nil,
			&badFuel,
			nil,
		)
		if err == nil {
			t.Fatalf("expected error for invalid fuel product, got nil")
		}
	})

	t.Run("invalid station UUID", func(t *testing.T) {
		badUUID := "not-a-valid-uuid"
		err := ValidateTopic(
			CategoryFuels,
			"Título válido",
			"Corpo válido de discussão.",
			"",
			nil,
			nil,
			&badUUID,
		)
		if err == nil {
			t.Fatalf("expected error for invalid station UUID, got nil")
		}
	})
}
