# Single source of truth for per-package coverage minimums, consumed by
# both `just test`/`testGui` and CI so the numbers never drift apart.
# Source this file (`source scripts/coverage-thresholds.sh`) rather than
# hardcoding thresholds elsewhere.
CORE_COVERAGE_MIN=90
CLI_COVERAGE_MIN=70
GUI_COVERAGE_MIN=25
