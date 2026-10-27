# P01-T13 verification entry points. Small compositions over existing gates;
# YAML workflows call these targets instead of duplicating shell logic.
# quick-verify is the bounded phase-integration gate (<5min warm budget,
# measured). verify-release reports the foundation subset plus explicit
# outstanding work and never certifies a release (P09 owns certification).

.PHONY: quick-verify verify-release test-gate test-flow test-issues test-wiki check-mobile check-rust check-infra test-infra test-deploy test-deploy-agent test-backup test-restore test-load check-security test-security test-rehearse check-compat test-compat check-g09 test-g09 test-portal-release rehearse-portal check-portal-capacity portal-capacity check-preview-probe preview-portal check-publication-gate preflight-portal-publication smoke-portal-public help

quick-verify:
	bash scripts/quick-verify.sh

check-mobile:
	bash scripts/check-mobile.sh

check-rust:
	bash scripts/check-rust-fast.sh

verify-release:
	bash scripts/verify-release.sh

test-gate:
	bash scripts/tests/test-gate-selection.sh

test-flow:
	python3 scripts/tests/test-merge-checks.py
	bash scripts/tests/test-git-flow.sh
	bash scripts/tests/test-project-batch-flow.sh
	python3 scripts/tests/test-ci-cadence.py

test-issues:
	bash scripts/tests/test-issues.sh

test-wiki:
	bash scripts/tests/test-wiki.sh

check-infra:
	bash scripts/check-infra-config.sh staging
	bash scripts/check-infra-config.sh prod

test-infra:
	bash scripts/tests/test-infra-config.sh

test-deploy:
	bash scripts/tests/test-deploy.sh

test-deploy-agent:
	bash infra/deploy/agent/tests/test-agent.sh

test-backup:
	bash scripts/tests/test-backup.sh

test-restore:
	bash scripts/tests/test-restore.sh

test-load:
	bash scripts/tests/test-load.sh

check-security:
	bash scripts/check-security.sh

test-security:
	bash scripts/tests/test-security.sh

test-rehearse:
	bash scripts/tests/test-rehearse.sh

test-portal-release:
	cd backend && go test -race -count=1 ./internal/modules/enterpriseexport/portalgen

rehearse-portal:
	bash infra/scripts/rehearse-portal.sh

check-portal-capacity:
	python3 infra/scripts/load/stats.py --self-test
	python3 infra/scripts/load/drive.py --self-test
	python3 infra/scripts/load/portal_report.py --self-test
	node infra/scripts/load/browser_probe.mjs --self-test

portal-capacity:
	bash infra/scripts/load/portal_capacity.sh

check-preview-probe:
	node infra/scripts/load/preview_probe.mjs --self-test

preview-portal:
	bash infra/scripts/preview_portal.sh

check-publication-gate:
	python3 infra/scripts/load/publication_preflight.py --self-test
	bash infra/scripts/smoke_portal_public.sh --self-test

preflight-portal-publication:
	python3 infra/scripts/load/publication_preflight.py --artifact "$${PORTAL_PUBLICATION_ARTIFACT:?}" --scope "$${PORTAL_PUBLICATION_SCOPE:?}" --out "$${PORTAL_PUBLICATION_OUT:-/tmp/portal-preflight.json}"

smoke-portal-public:
	bash infra/scripts/smoke_portal_public.sh --origin "$${PORTAL_PUBLIC_ORIGIN:?}" --expected-sha "$${PORTAL_EXPECTED_CONTENT_SHA256:?}" --mode "$${PORTAL_SMOKE_MODE:-full}" --topology "$${PORTAL_SMOKE_TOPOLOGY:?}" --out "$${PORTAL_SMOKE_OUT:-/tmp/portal-smoke.txt}"

check-compat:
	bash scripts/check-compat.sh

test-compat:
	bash scripts/tests/test-compat.sh

check-g09:
	bash scripts/check-g09.sh

test-g09:
	bash scripts/tests/test-g09.sh

help:
	@echo "Targets:"
	@echo "  quick-verify    bounded task/integration checks (manifest + selection)"
	@echo "  check-mobile    bounded KMP/iOS selection (static always, toolchain-gated full)"
	@echo "  check-rust      bounded Rust station-prep selection (fmt, clippy, locked tests)"
	@echo "  verify-release  quick + full-matrix report (foundation subset, NOT CERTIFIED)"
	@echo "  test-gate       focused harness for gate selection/failure behavior"
	@echo "  test-flow       synthetic git/fake-gh lifecycle for phase controller"
	@echo "  test-issues     fake-API reconciliation for issues/milestones"
	@echo "  test-wiki       fixture-repo exporter/publisher for wiki mirror"
	@echo "  check-infra     staging/prod topology gate (P08-T01)"
	@echo "  test-infra      infra gate failure-behavior harness"
	@echo "  test-deploy     deploy/rollback failure-behavior harness (P08-T02)"
	@echo "  test-deploy-agent  signed staging planner failure-behavior harness (CD-02)"
	@echo "  test-backup     backup pipeline harness, real disposable DB (P08-T03)"
	@echo "  test-restore    isolated restore drill harness, real disposable DB (P08-T04)"
	@echo "  test-load       bounded load smoke + fault suite, real disposable DB (P08-T07)"
	@echo "  check-security  STRIDE/dependency release review gate (P08-T08)"
	@echo "  test-security   security gate failure-behavior harness (P08-T08)"
	@echo "  test-rehearse   local release rehearsal, real disposable DB (P09-T01)"
	@echo "  test-portal-release  portal publisher/verifier tests (ESP-14)"
	@echo "  rehearse-portal isolated static portal Caddy rehearsal (ESP-14)"
	@echo "  check-portal-capacity  capacity harness self-tests (ESP-16)"
	@echo "  portal-capacity      bounded portal load/browser campaign (ESP-16)"
	@echo "  check-preview-probe  preview/accessibility probe self-test (ESP-17)"
	@echo "  preview-portal       isolated HTTPS preview/a11y/backup rehearsal (ESP-17)"
	@echo "  check-publication-gate  public preflight/smoke self-tests (ESP-18)"
	@echo "  preflight-portal-publication  local public-scope candidate gate (ESP-18)"
	@echo "  smoke-portal-public  read-only public release smoke (ESP-18)"
	@echo "  check-compat    contract + frozen deltas + runbooks gate (P09-T02)"
	@echo "  test-compat     compat gate failure-behavior harness (P09-T02)"
	@echo "  check-g09       G09 readiness gate, refuses open blockers (P09-T03)"
	@echo "  test-g09        G09 gate failure-behavior harness (P09-T03)"
	@echo "  test-admin-staging  admin staging script guard harness, no network (A5a)"

.PHONY: test-local-backend test-local-edge
test-local-backend:
	bash scripts/tests/test-local-backend.sh

test-local-edge:
	bash scripts/tests/test-local-edge.sh

.PHONY: test-admin-staging
test-admin-staging:
	bash scripts/tests/test-admin-staging-guard.sh
