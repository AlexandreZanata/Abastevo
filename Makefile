# P01-T13 verification entry points. Small compositions over existing gates;
# YAML workflows call these targets instead of duplicating shell logic.
# quick-verify is the bounded phase-integration gate (<5min warm budget,
# measured). verify-release reports the foundation subset plus explicit
# outstanding work and never certifies a release (P09 owns certification).

.PHONY: quick-verify verify-release test-gate test-flow test-issues test-wiki check-infra test-infra test-deploy test-backup test-restore test-load check-security test-security test-rehearse check-compat test-compat check-g09 test-g09 help

quick-verify:
	bash scripts/quick-verify.sh

verify-release:
	bash scripts/verify-release.sh

test-gate:
	bash scripts/tests/test-gate-selection.sh

test-flow:
	bash scripts/tests/test-git-flow.sh

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
	@echo "  verify-release  quick + full-matrix report (foundation subset, NOT CERTIFIED)"
	@echo "  test-gate       focused harness for gate selection/failure behavior"
	@echo "  test-flow       synthetic git/fake-gh lifecycle for phase controller"
	@echo "  test-issues     fake-API reconciliation for issues/milestones"
	@echo "  test-wiki       fixture-repo exporter/publisher for wiki mirror"
	@echo "  check-infra     staging/prod topology gate (P08-T01)"
	@echo "  test-infra      infra gate failure-behavior harness"
	@echo "  test-deploy     deploy/rollback failure-behavior harness (P08-T02)"
	@echo "  test-backup     backup pipeline harness, real disposable DB (P08-T03)"
	@echo "  test-restore    isolated restore drill harness, real disposable DB (P08-T04)"
	@echo "  test-load       bounded load smoke + fault suite, real disposable DB (P08-T07)"
	@echo "  check-security  STRIDE/dependency release review gate (P08-T08)"
	@echo "  test-security   security gate failure-behavior harness (P08-T08)"
	@echo "  test-rehearse   local release rehearsal, real disposable DB (P09-T01)"
	@echo "  check-compat    contract + frozen deltas + runbooks gate (P09-T02)"
	@echo "  test-compat     compat gate failure-behavior harness (P09-T02)"
	@echo "  check-g09       G09 readiness gate, refuses open blockers (P09-T03)"
	@echo "  test-g09        G09 gate failure-behavior harness (P09-T03)"
