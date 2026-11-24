//go:build integration

package adapters

import (
	"io/fs"
	"testing"
	"testing/fstest"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

func TestPGKeyLoginAcrossRecoveryProfileExpansion(t *testing.T) {
	const profileMigration = "000302_account_recovery_profile.sql"
	previous := fstest.MapFS{}
	migrations, err := fs.Glob(dbmigrations.Files, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range migrations {
		if name >= profileMigration {
			continue
		}
		data, err := fs.ReadFile(dbmigrations.Files, name)
		if err != nil {
			t.Fatal(err)
		}
		previous[name] = &fstest.MapFile{Data: data}
	}

	svc, _, _ := freshServiceSchema(t, previous)
	ctx := t.Context()
	key, err := svc.CreateKeyAccount(ctx, "legacyowner")
	if err != nil {
		t.Fatal(err)
	}
	before, err := svc.LoginWithKey(ctx, key.Key)
	if err != nil {
		t.Fatalf("login before profile expansion: %v", err)
	}
	if before.Account.PublicName() != "legacyowner" || before.Account.DisplayName != "" {
		t.Fatal("the original schema must project the public alias")
	}

	profileSQL, err := fs.ReadFile(dbmigrations.Files, profileMigration)
	if err != nil {
		t.Fatal(err)
	}
	previous[profileMigration] = &fstest.MapFile{Data: profileSQL}
	store := svc.Store.(*PGStore)
	applied, err := migrate.Apply(ctx, store.pool.Config().ConnString(), previous)
	if err != nil || len(applied) != 1 || applied[0] != "000302" {
		t.Fatalf("profile migration: %v %v", applied, err)
	}
	if _, err := svc.UpdateName(ctx, before.Session.FamilyID, before.Session.AccessToken, "Nome de Teste"); err != nil {
		t.Fatal(err)
	}
	after, err := svc.LoginWithKey(ctx, key.Key)
	if err != nil {
		t.Fatalf("login after profile expansion: %v", err)
	}
	if after.Account.ID != before.Account.ID || after.Account.Alias != "legacyowner" || after.Account.PublicName() != "Nome de Teste" {
		t.Fatal("the expanded schema must project the new name and preserve the legacy identity")
	}
}
