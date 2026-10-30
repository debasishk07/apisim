# apisim Implementation Plan

This document records key design decisions, interfaces, and risks before implementation begins.

## Design Decisions

1. **License: MIT.** Permissive, widely adopted, simple.

2. **Config format: YAML v3 (gopkg.in/yaml.v3), strict.** Unknown fields are rejected. Errors include line numbers via yaml.v3's node structure and `sigs.k8s.io/yaml`-style field paths built manually (keep deps minimal; implement line-number reporting using yaml.Node).

3. **HTTP stack: net/http + httputil.ReverseProxy.** Standard library only. Custom `Director` and `ModifyResponse`/`ErrorHandler` hooks run injectors. Streaming bodies pass through unbuffered.

4. **Injector pipeline.** One `Injector` interface with `BeforeRequest` and `AfterResponse` hooks plus a short-circuit return. Injectors run in scenario order per route. Each route keeps its own counter; a global counter is also maintained.

5. **Trigger engine: seeded RNG.** `math/rand/v2` seeded from `seed` field. All randomness flows through an injectable `RNG` interface. Triggers: probability, every_nth, after_requests, first_n_requests, between_requests, time_window. Conditions ANDed within one `when`.

6. **Clock: injectable `clock.Clock` interface.** `time.Now` and `time.Since` abstractions for deterministic tests.

7. **Storage: SQLite via modernc.org/sqlite (pure Go, no CGO).** Schema migrations built-in (embed migrations as SQL files or inline). Tables: runs, requests, applied_failures.

8. **Templating: Go text/template.** Custom function map: now, uuid, randomInt, path/query/header accessors. No `os`/`exec` access.

9. **Binding: 127.0.0.1 by default** unless `--bind 0.0.0.0` explicitly passed.

10. **CLI: cobra.** Commands: start, validate, record, replay, logs, report, init, version.

11. **Dependencies (justified in DECISIONS.md):**
    - github.com/spf13/cobra — CLI framework (standard in Go ecosystem)
    - gopkg.in/yaml.v3 — YAML parsing with line numbers
    - modernc.org/sqlite — pure-Go SQLite (no CGO, cross-compile friendly)
    - github.com/stretchr/testify — test assertions (standard)
    - github.com/PaesslerAG/jsonpath — JSONPath for body matching/mutation (considered; keep minimal)

## Interfaces

```go
// internal/inject
type Injector interface {
    Name() string
    BeforeRequest(req *http.Request) (*http.Response, error) // non-nil resp short-circuits
    AfterResponse(resp *http.Response) (*http.Response, error)
}

type Trigger interface {
    Fire(ctx context.Context, counters Counters) bool
}

// internal/match
type Matcher interface {
    Match(req *http.Request) (bool, int /* specificity score */)
}

// internal/store
type Store interface {
    RecordRequest(...)
    QueryLogs(...)
    Close()
}

// internal/clock
type Clock interface {
    Now() time.Time
    Since(time.Time) time.Duration
}

// internal/config
type Scenario struct {
    Version int
    Name string
    Listen string
    Seed int64
    Defaults Defaults
    Routes []Route
    Webhooks []Webhook
}
```

## Route Precedence (documented)

1. Exact method + exact path match wins.
2. Method match + path with params (`{id}`) wins over wildcard.
3. Wildcard (`/*`) lowest.
4. Among equal specificity, more match constraints (headers/query/body) win.
5. First matching route in file breaks remaining ties.

## Risks

- JSONPath implementation: keep a minimal subset (field access, `$`) rather than a heavy library to avoid dependency bloat; document supported subset.
- Seeded reproducibility: every injector must use the shared RNG, not `math/rand` globals.
- Streaming + injectors that buffer body (mutate_body) conflict; document that mutate_body buffers the full response.
- SQLite concurrency: use WAL mode and a single connection pool with busy timeout.

## Phase Acceptance Summary

- Phase 0: `make build` binary, CI exists, `apisim version` works.
- Phase 1: config loader + matcher + reverse proxy + stdout logging; integration tests with httptest.
- Phase 2: injectors + triggers, unit + integration tests, seeded determinism test.
- Phase 3: mock mode + templating + SQLite + `apisim logs`.
- Phase 4: record/replay + resilience report.
- Phase 5: webhook simulator.
- Phase 6: packs, dashboard, GitHub Action.
- Phase 7: docs, examples, GoReleaser, release.
