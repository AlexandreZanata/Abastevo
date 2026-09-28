# P01-T13 verification entry points. Small compositions over existing gates;
# YAML workflows call these targets instead of duplicating shell logic.
# quick-verify is the bounded phase-integration gate (<5min warm budget,
# measured). verify-release reports the foundation subset plus explicit
# outstanding work and never certifies a release (P09 owns certification).

.PHONY: quick-verify verify-release test-gate test-flow help

quick-verify:
	bash scripts/quick-verify.sh

verify-release:
	bash scripts/verify-release.sh

test-gate:
	bash scripts/tests/test-gate-selection.sh

test-flow:
	bash scripts/tests/test-git-flow.sh

help:
	@echo "Targets:"
	@echo "  quick-verify    bounded task/integration checks (manifest + selection)"
	@echo "  verify-release  quick + full-matrix report (foundation subset, NOT CERTIFIED)"
	@echo "  test-gate       focused harness for gate selection/failure behavior"
	@echo "  test-flow       synthetic git/fake-gh lifecycle for phase controller"
