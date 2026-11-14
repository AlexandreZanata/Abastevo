package config

import "testing"

func TestDevelopmentStorageServiceIsScoped(t *testing.T) {
	for _, tc := range []struct {
		env, host, value string
		valid            bool
	}{
		{"development", "", "true", true}, {"staging", "abastevo.com.br", "true", true},
		{"staging", "other.example.invalid", "true", false}, {"staging", "teste.abastevo.com.br", "true", false}, {"production", "abastevo.com.br", "true", false},
		{"development", "", "invalid", false},
	} {
		cfg, err := load(envFunc(map[string]string{"ANPFUEL_ENV": tc.env, "ANPFUEL_DATABASE_URL": "postgres://fixture:fixture@localhost/fixture", "ANPFUEL_CANONICAL_HOST": tc.host, "ANPFUEL_DEV_STORAGE_SERVICE": tc.value, "ANPFUEL_CURSOR_SECRET": "synthetic-cursor-secret-with-sufficient-length"}))
		if (err == nil) != tc.valid {
			t.Fatalf("%+v: %v", tc, err)
		}
		if err == nil && !cfg.DevelopmentStorageService {
			t.Fatal("service forwarding ignored")
		}
	}
}
