# apisim

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Report Card](https://goreportcard.com/badge/github.com/apisim/apisim)](https://goreportcard.com/report/github.com/apisim/apisim)

## One-liner

apisim is a chaos-testing proxy for third-party APIs. It sits between your app and external APIs (Stripe, Twilio, OpenAI, etc.) and injects realistic failures so developers can verify their app handles outages, slowness, bad data, and webhook problems.

## Quickstart (5 minutes)

1. **Install**: `go install github.com/apisim/apisim@latest`
2. **Create a scenario file** (`scenario.yaml`):

```yaml
version: 1
name: "test-proxy"
listen: ":8080"
seed: 42
defaults:
  upstream: https://httpbin.org
routes:
  - name: mock-endpoint
    match:
      method: GET
      path: /get
    mode: mock
    response:
      status: 200
      body: '{"mocked": true}'
```

3. **Run apisim**:

```bash
apisim start scenario.yaml
```

4. **Test it**:

```bash
# The proxy is now running at http://localhost:8080
# Requests to /v1/get will be forwarded to httpbin.org
# Unmatched requests get a 404 JSON error

curl -s http://localhost:8080/v1/get?foo=bar
```

## Installation

### Using Go

```bash
go install github.com/apisim/apisim@latest
```

### From binary

Download the pre-built binary from the [releases page](https://github.com/debasishk07/apisim/releases).

### From source

```bash
git clone https://github.com/debasishk07/apisim.git
cd apisim
```

## CLI Commands

```
apisim start <scenario.yaml>      Run the proxy/mock server
apisim validate <scenario.yaml>   Validate config, print friendly errors
apisim record <scenario.yaml>     Proxy to real upstreams and save request/response pairs
apisim replay <scenario.yaml>     Serve recorded responses (no network needed)
apisim logs [--follow] [--route X] [--failed]   Query the request log
apisim report <run-id>            Print resilience report
apisim init [--template stripe|openai|twilio]   Generate a starter scenario
apisim version                      Print the apisim version
```

## Flags for `start`

| Flag | Default | Description |
|------|---------|-------------|
| `--port` | (overrides scenario) | Override listen port |
| `--log-level` | `info` | Log level (debug, info, warn, error) |
| `--db` | `""` | Path to SQLite database |
| `--seed` | `0` | Seed for reproducible randomness (overrides scenario) |
| `--dashboard` | `false` | Enable embedded dashboard (Phase 6) |
| `--fail-fast` | `false` | Exit on first config error |
| `--ci` | `false` | Machine-readable output; nonzero exit on config error |

## Git Commands

```bash
# Clone the repository
git clone https://github.com/debasishk07/apisim.git
cd apisim

# Install dependencies and build
go install github.com/apisim/apisim@latest
# Or build from source
go build -o apisim ./cmd/apisim

# Start apisim with a scenario
apisim start scenario.yaml

# Start apisim with dashboard (Phase 6)
apisim start scenario.yaml --dashboard

# Validate a scenario file
apisim validate scenario.yaml

# Generate a new starter scenario
apisim init --template stripe

# Check the version
apisim version
```

## Dashboard (Phase 6)

Run apisim with the dashboard flag:

```bash
apisim start scenario.yaml --dashboard
```

The dashboard is embedded at `http://localhost:8080/__apisim/`. It provides 5 beautifully designed screens with the Anthropic orange & baige theme:

- **Home** - Quick start with curated packs or blank scenario. Features professional card layouts, pack descriptions, and a "Start blank" call-to-action.
- **Live Traffic** - Real-time request log with filtering, search, pause/resume toggle, and kill switch for emergency stops.
- **Chaos Controls** - Intuitive slider-based controls for latency (ms), error probability, status codes, rate limiting (req/second), and timeouts. All controls persist in real-time.
- **Report** - Generate resilience reports in Text, JSON, or HTML format. Export button saves the report locally.
- **Recordings** - View captured request/response interactions. Promote favorite recordings to mock routes for replay without the upstream.

**Design highlights:**
- Anthropic-inspired orange & baige color palette with high contrast
- Card-based layouts with subtle shadows and rounded corners
- Focus-visible states for keyboard navigation
- Reduced-motion support for accessibility
- Tailwind CSS 3.x with custom theme variables
- All assets served from `/dashboard.css` and `/dashboard.js` routes

**Note:** The dashboard CSS and JS are served from `/dashboard.css` and `/dashboard.js` routes, registered alongside the `/__apisim/` proxy paths.

## Scenario File Format

See the [scenario reference](docs/scenario.schema.json) for every field and failure type with examples.

### Minimal scenario

```yaml
version: 1
name: "minimal"
listen: ":8080"
defaults:
  upstream: https://httpbin.org
routes:
  - name: get-endpoint
    match:
      method: GET
      path: /get
    mode: proxy
```

### With failure injectors

```yaml
version: 1
name: "checkout-resilience"
listen: ":8080"
seed: 42
defaults:
  upstream: https://api.stripe.com
routes:
  - name: create-charge
    match:
      method: POST
      path: /v1/charges
      headers: { "X-Env": "test" }
      query: { currency: usd }
      body_json: { "$.amount": 5000 }
    mode: proxy
    upstream: https://api.stripe.com
    failures:
      - type: latency
        delay_ms: 3000
        jitter_ms: 500
        when: { probability: 0.5 }
      - type: error
        status: 503
        body: '{"error":"service unavailable"}'
        headers: { "Retry-After": "5" }
        when: { every_nth: 3 }
```

### Mock mode

```yaml
version: 1
name: "mock-endpoint"
listen: ":8080"
routes:
  - name: get-customer
    match: { method: GET, path: /v1/customers/{id} }
    mode: mock
    response:
      status: 200
      headers: { "Content-Type": "application/json" }
      body_file: ./fixtures/customer.json
      template: true
```

Template helpers: `{{ .Path.id }}`, `{{ .Query.x }}`, `{{ .Header.Authorization }}`, `{{ now }}`, `{{ uuid }}`, `{{ randomInt 100 }}`

### Record mode

```yaml
version: 1
name: "record-stripe"
listen: ":8080"
defaults:
  upstream: https://api.stripe.com
routes:
  - name: all-charges
    match: { method: GET, path: /v1/charges }
    mode: record
```

Run `apisim record scenario.yaml` to capture interactions, then `apisim replay scenario.yaml` to serve them without the upstream.

## Integration test example

```bash
# Start apisim with a test upstream
apisim start scenario.yaml &

# In another terminal, send a request
curl -s http://localhost:8080/v1/test

# View the log
apisim logs

# Generate a resilience report
apisim report <run-id>
```

## Directory structure

```
apisim/
  cmd/apisim/main.go          # CLI entry point
  internal/config/           # Config parsing & validation
  internal/match/            # Route matcher
  internal/proxy/            # Reverse proxy engine
  internal/inject/           # Injectors + trigger engine
  internal/mock/             # Mock responses + templating
  internal/store/            # SQLite persistence + migrations
  internal/clock/            # Injectable clock
  packs/                     # Stripe, OpenAI, Twilio scenarios
  examples/                  # Sample app + scenario
  docs/                      # Scenario reference, CI guide, recipes
  docs/DECISIONS.md          # Design decisions & trade-offs
  docs/scenario.schema.json  # JSON Schema for editor autocomplete
  .github/workflows/         # CI workflow
  Makefile                   # build, test, lint, ci, run
  go.mod                     # Go module
  .golangci.yml              # Linter config
  .gitignore
  LICENSE                    # MIT
  CONTRIBUTING.md
```

## Development

```bash
# Build
make build

# Run tests with race detection
make test

# Run lint
make lint

# Run all checks
make ci

# Run apisim
./apisim start scenario.yaml
```

## Learn more

- [Scenario reference](docs/scenario.schema.json) - Every field and failure type
- [DECISIONS.md](docs/DECISIONS.md) - Design decisions and trade-offs
- [CI guide](docs/CI_GUIDE.md) - Continuous integration setup
- [Recipes](docs/RECIPES.md) - Common use cases ("test retry with backoff", "test circuit breaker", "test webhook idempotency")
- [Roadmap](docs/ROADMAP.md) - Future ideas and planned features