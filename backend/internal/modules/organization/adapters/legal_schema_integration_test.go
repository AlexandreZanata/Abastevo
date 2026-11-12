//go:build integration

package adapters_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func syntheticLegalCPF(n int) string {
	value := fmt.Sprintf("%09d", 341572000+n)
	for len(value) < 11 {
		sum := 0
		for i, ch := range value {
			sum += int(ch-'0') * (len(value) + 1 - i)
		}
		digit := (sum * 10) % 11
		if digit == 10 {
			digit = 0
		}
		value += fmt.Sprint(digit)
	}
	return value
}
func TestLegalSchemaScopesIdentifiersAndBoundsDatabaseFailures(t *testing.T) {
	f, _, service, actor, _ := onboardingFixture(t)
	var safe bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT count(*)=3 AND bool_and(relrowsecurity AND relforcerowsecurity) FROM pg_class WHERE oid IN (to_regclass('org_legal_entities'),to_regclass('org_establishments'),to_regclass('org_legal_commands'))`).Scan(&safe); err != nil || !safe {
		t.Fatal("scoped legal tables absent", err)
	}
	role := pgx.Identifier{f.RuntimeRole}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE,DELETE ON org_legal_entities,org_establishments TO "+role+"; GRANT SELECT,INSERT ON org_legal_commands TO "+role); err != nil {
		t.Fatal(err)
	}
	created, _, err := service.Create(t.Context(), actor, onboardingCommand(t, onboardingID(t)))
	if err != nil {
		t.Fatal(err)
	}
	scope := scopeFor(t, created.OrgID.String())
	cpf := syntheticLegalCPF(5)
	entity := onboardingID(t)
	insert := func(id, value, name string) error {
		return database.WithOrgTx(t.Context(), f.Runtime, scope, func(tx pgx.Tx) error {
			_, err := tx.Exec(t.Context(), `INSERT INTO org_legal_entities(org_id,id,kind,legal_name,identifier,purpose,state) VALUES($1,$2,'NATURAL_PERSON',$3,$4,'VERIFICATION_AND_AGREEMENTS','DECLARED')`, scope.OrgID().String(), id, name, value)
			return err
		})
	}
	if err := insert(entity, cpf, "Synthetic private party"); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ value, name, code string }{{cpf, "Another synthetic party", "P7601"}, {cpf[:10] + string(byte('0'+(cpf[10]-'0'+1)%10)), "Synthetic party", "23514"}, {cpf, "unsafe\nname", "23514"}} {
		err := insert(onboardingID(t), test.value, test.name)
		var failure *pgconn.PgError
		detail := ""
		if err != nil {
			detail = err.Error()
		}
		if errors.As(err, &failure) {
			detail += failure.Detail + failure.Hint + failure.Where + failure.InternalQuery
		}
		if strings.Contains(detail, cpf) || strings.Contains(detail, test.value) || strings.Contains(detail, test.name) {
			t.Fatal("database failure exposed private content")
		}
		schemaCode(t, err, test.code)
	}
	var count int
	if err := f.Runtime.QueryRow(t.Context(), `SELECT count(*) FROM org_legal_entities`).Scan(&count); err != nil || count != 0 {
		t.Fatal("unscoped legal read", err)
	}
	other := scopeFor(t, onboardingID(t))
	if err := database.WithOrgTx(t.Context(), f.Runtime, other, func(tx pgx.Tx) error {
		return tx.QueryRow(t.Context(), `SELECT count(*) FROM org_legal_entities WHERE id=$1`, entity).Scan(&count)
	}); err != nil || count != 0 {
		t.Fatal("foreign legal read", err)
	}
	err = database.WithOrgTx(t.Context(), f.Runtime, scope, func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `UPDATE org_legal_entities SET identifier=$1,revision=revision+1 WHERE org_id=$2 AND id=$3`, syntheticLegalCPF(6), scope.OrgID().String(), entity)
		return err
	})
	schemaCode(t, err, "55000")
}
