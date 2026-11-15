//go:build integration

package adapters_test

import (
	"testing"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	ma "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/domain/checklist"
	"github.com/jackc/pgx/v5"
)

// B-BR-EMP-331/335 in the database: forced RLS, insert-only history, the
// guarded head, consecutive numbering and closed committed versions — a
// prior version cannot be rewritten after publication, even by SQL.
func TestChecklistTemplateSchemaGuardsAndIsolation(t *testing.T) {
	fx, assets, unit := lifecycleFixture(t)
	s, _ := checklistStore(t, fx)
	asset, category := checklistAsset(t, fx, assets, unit, "Synthetic schema", fleet.Active)
	created, err := createTemplate(t, fx, s, checklist.Departure, categoryTarget(category))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := templateRun(t, fx, s, templateCommand(t, ma.TemplateCommandInput{Kind: ma.CommandPublish, TemplateID: created.TemplateID, ExpectedRevision: 1, Name: "Versão 2", Target: categoryTarget(category), Items: checklistItems(3)})); err != nil {
		t.Fatal(err)
	}
	org, id := fx.scope.OrgID().String(), created.TemplateID
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT DELETE,TRUNCATE ON org_checklist_templates,org_checklist_template_commands,org_checklist_template_versions,org_checklist_template_items,org_checklist_template_targets,org_checklist_policy_revisions TO "+role+
		";GRANT UPDATE ON org_checklist_template_commands,org_checklist_template_versions,org_checklist_template_items,org_checklist_template_targets,org_checklist_policy_revisions TO "+role); err != nil {
		t.Fatal(err)
	}
	for label, statement := range map[string]string{
		"rewrite a published version name":  `UPDATE org_checklist_template_versions SET name='Reescrita' WHERE org_id=$1 AND template_id=$2 AND version=1`,
		"rewrite a published item":          `UPDATE org_checklist_template_items SET critical=false WHERE org_id=$1 AND template_id=$2 AND version=1`,
		"rewrite a published target":        `UPDATE org_checklist_template_targets SET category_id=NULL WHERE org_id=$1 AND template_id=$2`,
		"rewrite a policy revision":         `UPDATE org_checklist_policy_revisions SET mode='WARN' WHERE org_id=$1 AND template_id=$2`,
		"rewrite a receipt":                 `UPDATE org_checklist_template_commands SET request_hash=repeat('0',64) WHERE org_id=$1 AND template_id=$2`,
		"delete a version":                  `DELETE FROM org_checklist_template_versions WHERE org_id=$1 AND template_id=$2 AND version=1`,
		"delete an item":                    `DELETE FROM org_checklist_template_items WHERE org_id=$1 AND template_id=$2`,
		"delete a target":                   `DELETE FROM org_checklist_template_targets WHERE org_id=$1 AND template_id=$2`,
		"delete a policy revision":          `DELETE FROM org_checklist_policy_revisions WHERE org_id=$1 AND template_id=$2`,
		"delete a receipt":                  `DELETE FROM org_checklist_template_commands WHERE org_id=$1 AND template_id=$2`,
		"delete the template":               `DELETE FROM org_checklist_templates WHERE org_id=$1 AND id=$2`,
		"change the context":                `UPDATE org_checklist_templates SET context='RETURN',revision=revision+1 WHERE org_id=$1 AND id=$2`,
		"move back to version 1":            `UPDATE org_checklist_templates SET version=1,revision=revision+1 WHERE org_id=$1 AND id=$2`,
		"skip a revision":                   `UPDATE org_checklist_templates SET policy_revision=policy_revision+1,revision=revision+2 WHERE org_id=$1 AND id=$2`,
		"publish and change policy at once": `UPDATE org_checklist_templates SET version=version+1,policy_revision=policy_revision+1,revision=revision+1 WHERE org_id=$1 AND id=$2`,
		"add an item to version 1":          `INSERT INTO org_checklist_template_items(org_id,template_id,version,position,label,critical,optional,photo_on_not_ok) VALUES($1,$2,1,3,'Extra',false,false,false)`,
		"refill a version 1 position":       `INSERT INTO org_checklist_template_items(org_id,template_id,version,position,label,critical,optional,photo_on_not_ok) VALUES($1,$2,1,2,'Extra',false,false,false)`,
		"add a target to version 2":         `INSERT INTO org_checklist_template_targets(org_id,template_id,version,category_id) SELECT $1,$2,2,category_id FROM org_assets WHERE org_id=$1 LIMIT 1`,
		"insert version 1 again":            `INSERT INTO org_checklist_template_versions(org_id,template_id,version,name,target_kind,target_count,item_count,operation_id,published_by) SELECT org_id,template_id,1,'Again','ORGANIZATION',0,1,operation_id,published_by FROM org_checklist_template_versions WHERE org_id=$1 AND template_id=$2 AND version=2`,
		"insert a version gap":              `INSERT INTO org_checklist_template_versions(org_id,template_id,version,name,target_kind,target_count,item_count,operation_id,published_by) SELECT org_id,template_id,4,'Gap','ORGANIZATION',0,1,operation_id,published_by FROM org_checklist_template_versions WHERE org_id=$1 AND template_id=$2 AND version=2`,
		"insert a policy revision gap":      `INSERT INTO org_checklist_policy_revisions(org_id,template_id,revision,mode,validity_seconds,operation_id,set_by) SELECT org_id,template_id,3,'WARN',3600,operation_id,set_by FROM org_checklist_policy_revisions WHERE org_id=$1 AND template_id=$2`,
	} {
		want := "55000"
		if label == "refill a version 1 position" {
			want = "23505"
		}
		allocationWant(t, label, allocationExec(t, fx, statement, org, id), want)
	}
	allocationWant(t, "truncate versions", allocationExec(t, fx, `TRUNCATE org_checklist_template_versions CASCADE`), "55000")
	// A committed version is closed before any row check; the flag rule itself
	// is a table constraint.
	allocationWant(t, "critical optional item", allocationExec(t, fx, `INSERT INTO org_checklist_template_items(org_id,template_id,version,position,label,critical,optional,photo_on_not_ok) VALUES($1,$2,2,9,'x',true,true,false)`, org, id), "55000")
	var flags string
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conname='org_checklist_template_items_flags_check'`).Scan(&flags); err != nil || flags != "CHECK ((NOT (critical AND optional)))" {
		t.Fatal("critical items are never optional in the schema", flags, err)
	}
	allocationWant(t, "asset target on a category version", allocationExec(t, fx, `INSERT INTO org_checklist_template_targets(org_id,template_id,version,asset_id) VALUES($1,$2,2,$3)`, org, id, asset), "23514")
	// Forced RLS: another scope sees and changes nothing; a cross-organization
	// row is refused.
	other := legalFixtureFor(t, fx.f, fx.onboarding, fx.actor, fx.flags)
	var visible int
	if err := otherScopeCount(t, other, `SELECT(SELECT count(*) FROM org_checklist_templates)+(SELECT count(*) FROM org_checklist_template_versions)+(SELECT count(*) FROM org_checklist_template_items)+(SELECT count(*) FROM org_checklist_template_targets)+(SELECT count(*) FROM org_checklist_policy_revisions)+(SELECT count(*) FROM org_checklist_template_commands)`, &visible); err != nil || visible != 0 {
		t.Fatal("rows visible to another organization", visible, err)
	}
	allocationWant(t, "cross-organization head", allocationExec(t, other, `INSERT INTO org_checklist_templates(org_id,id,context,state,revision,version,policy_revision,operation_id,created_by) VALUES($1,gen_random_uuid(),'DEPARTURE','ACTIVE',1,1,1,gen_random_uuid(),gen_random_uuid())`, org), "42501")
	var rows int
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_checklist_template_versions WHERE org_id=$1 AND template_id=$2`, org, id).Scan(&rows); err != nil || rows != 2 {
		t.Fatal("version history", rows, err)
	}
	var name string
	var critical bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT v.name,i.critical FROM org_checklist_template_versions v JOIN org_checklist_template_items i USING(org_id,template_id,version) WHERE v.org_id=$1 AND v.template_id=$2 AND v.version=1 AND i.position=1`, org, id).Scan(&name, &critical); err != nil || name != "Checklist sintético" || !critical {
		t.Fatal("version 1 rewritten", name, critical, err)
	}
	var unprotected bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT bool_or(NOT(relrowsecurity AND relforcerowsecurity)) FROM pg_class WHERE relname IN('org_checklist_templates','org_checklist_template_commands','org_checklist_template_versions','org_checklist_template_items','org_checklist_template_targets','org_checklist_policy_revisions')`).Scan(&unprotected); err != nil || unprotected {
		t.Fatal("forced RLS", err)
	}
}

func otherScopeCount(t *testing.T, fx legalFixtureState, query string, out *int) error {
	t.Helper()
	tx, err := fx.f.Runtime.Begin(t.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(t.Context())
	if _, err := tx.Exec(t.Context(), `SELECT set_config('anpfuel.org_id',$1,true)`, fx.scope.OrgID().String()); err != nil {
		return err
	}
	return tx.QueryRow(t.Context(), query).Scan(out)
}
