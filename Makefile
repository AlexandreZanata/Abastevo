# Developer and CI entry points. Workflows call these targets instead of
# duplicating shell logic. quick-verify is the bounded change-aware gate run
# by the required "Quick verification" check.

.PHONY: quick-verify test-gate test-flow check-mobile check-rust check-infra test-infra test-deploy test-deploy-agent test-seo test-backup test-restore test-load check-security test-security test-rehearse test-portal-release rehearse-portal check-portal-capacity portal-capacity check-preview-probe preview-portal check-publication-gate preflight-portal-publication smoke-portal-public help

quick-verify:
	bash scripts/quick-verify.sh

check-mobile:
	bash scripts/check-mobile.sh

check-rust:
	bash scripts/check-rust-fast.sh

test-gate:
	bash scripts/tests/test-gate-selection.sh

test-flow:
	python3 scripts/tests/test-merge-checks.py
	bash scripts/tests/test-git-flow.sh
	bash scripts/tests/test-project-batch-flow.sh
	python3 scripts/tests/test-ci-cadence.py
	python3 scripts/tests/test-backend-integration-shard.py

check-infra:
	python3 infra/k8s/staging/test-manifests.py
	bash scripts/check-infra-config.sh staging
	bash scripts/check-infra-config.sh prod

test-infra:
	python3 infra/k8s/staging/test-manifests.py
	bash scripts/tests/test-infra-config.sh

test-deploy:
	bash scripts/tests/test-deploy.sh

test-deploy-agent:
	bash infra/deploy/agent/tests/test-status.sh
	python3 infra/scripts/test-rollout-probe.py
	python3 infra/deploy/agent/tests/test-surge-capacity.py
	python3 infra/deploy/agent/tests/test-indexnow.py
	bash infra/deploy/agent/tests/test-agent.sh

test-seo:
	python3 scripts/tests/test-seo-search-report.py

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

help:
	@echo "Targets:"
	@echo "  quick-verify    bounded task/integration checks (manifest + selection)"
	@echo "  check-mobile    bounded KMP/iOS selection (static always, toolchain-gated full)"
	@echo "  check-rust      bounded Rust station-prep selection (fmt, clippy, locked tests)"
	@echo "  test-gate       focused harness for gate selection/failure behavior"
	@echo "  test-flow       synthetic git/fake-gh lifecycle for the merge guard"
	@echo "  check-infra     staging/prod topology gate"
	@echo "  test-infra      infra gate failure-behavior harness"
	@echo "  test-deploy     deploy/rollback failure-behavior harness"
	@echo "  test-deploy-agent  signed staging planner failure-behavior harness"
	@echo "  test-seo        Search Console report script tests, no network"
	@echo "  test-backup     backup pipeline harness, real disposable DB"
	@echo "  test-restore    isolated restore drill harness, real disposable DB"
	@echo "  test-load       bounded load smoke + fault suite, real disposable DB"
	@echo "  check-security  STRIDE/dependency release review gate"
	@echo "  test-security   security gate failure-behavior harness"
	@echo "  test-rehearse   local release rehearsal, real disposable DB"
	@echo "  test-portal-release  portal publisher/verifier tests"
	@echo "  rehearse-portal isolated static portal Caddy rehearsal"
	@echo "  check-portal-capacity  capacity harness self-tests"
	@echo "  portal-capacity      bounded portal load/browser campaign"
	@echo "  check-preview-probe  preview/accessibility probe self-test"
	@echo "  preview-portal       isolated HTTPS preview/a11y/backup rehearsal"
	@echo "  check-publication-gate  public preflight/smoke self-tests"
	@echo "  preflight-portal-publication  local public-scope candidate gate"
	@echo "  smoke-portal-public  read-only public release smoke"
	@echo "  test-admin-staging  admin staging script guard harness, no network"

.PHONY: test-local-backend test-local-edge
test-local-backend:
	bash scripts/tests/test-local-backend.sh

test-local-edge:
	bash scripts/tests/test-local-edge.sh

.PHONY: test-admin-staging
test-admin-staging:
	bash scripts/tests/test-admin-staging-guard.sh
