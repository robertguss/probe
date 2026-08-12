.PHONY: build test fmt vet hostile sentinel race integration product-e2e perf flaky docs-validate

# Default fast feedback loop (mirrors CI unit job).
build:
	CGO_ENABLED=0 go build -o /tmp/foundry ./cmd/foundry

test:
	go test -count=1 ./...

fmt:
	gofmt -l .

vet:
	go vet ./...

# go-foundry-cli-wet.4.6 — one-command local runner for the tagged hostile
# fsx suite. Kept out of the default `make test` / `go test ./...` because
# REQ-213 probes may exercise multi-FS mounts (FOUNDRY_FSX_ROOTS) that are
# not universally available on developer machines. See docs/dev/testing.md
# ("Default-suite policy: hostile fsx & sentinel") for the full rationale.
hostile:
	go test -tags=hostile -count=1 -timeout 180s -v ./integration/hostile/fsx/
	go test -tags=hostile -count=1 -timeout 120s ./internal/fsx/ -run 'Hostile|Commit|Transaction|Preserve|Static|Parent|Stage|Classify'

# Sentinel (REQ-214) already runs under default `go test ./...` on unix
# (build-constrained by `//go:build unix`, no mounts/sudo required). This
# target is a convenience for running just the sentinel/E2 slice in
# isolation, e.g. while iterating on toolrun env construction.
sentinel:
	go test -count=1 -timeout 180s ./internal/toolrun/ -run 'Sentinel|ProcessTree|ConstructGoEnv|ConstructGitEnv|Allowlist'
	go test -count=1 -timeout 180s -v ./integration/hostile/sentinel/
	go test -count=1 -timeout 180s ./integration/hostile/e2/

race:
	CGO_ENABLED=1 go test -count=1 -race ./...

integration:
	go test -count=1 -timeout 15m ./integration/generate/
	go test -count=1 -timeout 10m ./integration/dogfood/

# Product E2E (go-foundry-cli-79a): public commands, examples, generate dogfood,
# and scripted TUI PTY. Local/agent only — intentionally not wired into CI.
# See docs/dev/product-e2e-plan.md. Requires go1.26.x for generate/dogfood cells
# (FOUNDRY_GO_BIN or auto-discovery). macOS: run the same target on your MBP.
product-e2e:
	go test -count=1 -timeout 5m ./cmd/foundry -run 'TestWriteFree'
	go test -count=1 -timeout 3m ./internal/cli/ -run 'TestSixCommands|TestHelp|TestFlag|TestVersion|TestSpec|TestQuiet|TestExit'
	go test -count=1 -timeout 15m ./integration/generate/
	go test -count=1 -timeout 25m ./integration/dogfood/
	go test -count=1 -timeout 10m ./integration/tui/
	go test -count=1 -timeout 15m -tags=tui_pty ./integration/tui/ -run 'TestTUI_PTY'
	go test -count=1 -timeout 5m ./integration/fixtures/

# Non-blocking flaky-test detector: runs key suites with -count=5, uploads a
# JSON/text report, and exits non-zero only when a flake is found. CI runs this
# with continue-on-error: true; locally it is informational.
flaky:
	./scripts/flaky-detect.sh

# Verify fenced command examples in docs/recipes, docs/dev, and README.md, and
# check that documented error IDs exist in internal/diagnostic/ids.go.
docs-validate:
	python3 scripts/validate-docs.py

# Lint testscript fixtures for unused custom commands, negated custom commands,
# inconsistent ! exec usage, and missing exit-code assertions.
lint-testscripts:
	python3 scripts/lint-testscripts.py

perf:
	./scripts/perf/capture-baselines.sh
