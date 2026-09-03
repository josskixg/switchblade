# Contributing to Switchblade

Thank you for taking the time to contribute! This document covers everything you need to get from zero to merged PR.

---

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Ways to Contribute](#ways-to-contribute)
- [Getting Started](#getting-started)
- [Development Workflow](#development-workflow)
- [Project Architecture](#project-architecture)
- [Adding a Provider](#adding-a-provider)
- [Code Style](#code-style)
- [Testing](#testing)
- [Commit Messages](#commit-messages)
- [Pull Request Process](#pull-request-process)

---

## Code of Conduct

Be respectful, be constructive, be collaborative. We're all here to build something great. Harassment or abuse of any kind will not be tolerated.

---

## Ways to Contribute

| Type | Description |
|---|---|
| 🐛 **Bug reports** | Open an issue with a minimal reproduction |
| 💡 **Feature requests** | Open an issue describing the use-case |
| 📝 **Documentation** | Improve README, add inline comments, fix typos |
| 🔌 **New provider** | Add a new AI provider (see guide below) |
| 🧪 **Tests** | Unit tests, integration tests, edge cases |
| 🔧 **Fixes** | Bug fixes, performance improvements, refactors |

---

## Getting Started

### Prerequisites

- **Go 1.25+** — [install](https://golang.org/dl/) (see `go.mod`)
- **Python 3.10+** — only needed if working on browser auth bots (`scripts/auth/`)
- **make** — for convenience commands

### Setup

```bash
# 1. Fork the repo on GitHub, then clone your fork
git clone https://github.com/YOUR_USERNAME/switchblade.git
cd switchblade

# 2. Add the upstream remote
git remote add upstream https://github.com/yourorg/switchblade.git

# 3. Copy env and configure
cp .env.example .env
# Edit .env — at minimum set API_KEY and ENCRYPTION_KEY

# 4. Build and run
make build
make run
```

---

## Development Workflow

```bash
# Create a feature branch from main
git checkout -b feat/my-feature main

# Make changes, write tests
# ...

# Run tests
make test

# Build and smoke-test
make build
./switchblade

# Commit (see commit message guide below)
git commit -m "feat(providers): add xyzai provider"

# Push and open a PR
git push origin feat/my-feature
```

---

## Project Architecture

```
cmd/switchblade/       ← Entry point only. No business logic here.
internal/
  api/                 ← HTTP handlers. Keep handlers thin — call into proxy/db/auth.
  auth/                ← Account lifecycle (login, warmup, scheduler).
  config/              ← Config struct. All env vars go through here.
  crypto/              ← Encryption utilities. Don't roll your own elsewhere.
  db/                  ← SQLite layer. All DB queries go here, not in handlers.
  metrics/             ← Prometheus metrics. Add new metrics here.
  providers/           ← One file per provider. See adding-a-provider guide.
  proxy/               ← Core routing logic, account pool, fallback, compression.
  relay/               ← Relay tunnel client/server.
  storage/             ← Image/file storage utilities.
  ws/                  ← SSE broadcast hub.
```

**Key rules:**
- Handlers (`api/`) call into `proxy/`, `db/`, `auth/` — never the other way
- Providers (`providers/`) must be stateless — all state lives in the DB
- Config is only read from `internal/config/config.go` — never `os.Getenv` in other packages
- Errors bubble up — don't swallow errors with empty `if err != nil { return }`

---

## Adding a Provider

> This is the most common contribution. Here's the exact steps.

### 1. Create the provider file

```bash
touch internal/providers/myprovider.go
```

Use this template:

```go
package providers

import (
    "context"
    "net/http"

    "switchblade/internal/config"
    "switchblade/internal/db"
)

type MyProvider struct{}

func (p *MyProvider) Name() string { return "myprovider" }

func (p *MyProvider) OwnsModel(model string) bool {
    return strings.HasPrefix(model, "mp-")
}

func (p *MyProvider) Execute(ctx context.Context, account db.Account, req ChatRequest) (*http.Response, error) {
    cfg := config.Get()
    // build request to upstream API...
    // use account.Tokens for credentials...
    // return raw *http.Response
}

func (p *MyProvider) SupportsStream() bool { return true }

func (p *MyProvider) EstimateTokens(text string) int {
    return len(text) / 4 // rough estimate
}
```

### 2. Register the provider

Open `internal/providers/registry.go` and add to `PROVIDER_ORDER`:

```go
var PROVIDER_ORDER = []Provider{
    // ... existing providers ...
    &MyProvider{},
    // ...
}
```

Place it at the appropriate priority position (higher in the list = tried first).

### 3. Add model prefix to the matrix

Update the provider table in `README.md` with your prefix, auth method, and free-tier info.

### 4. Write a test

```bash
touch test/providers/myprovider_test.go
```

At minimum test `OwnsModel`:

```go
func TestMyProviderOwnsModel(t *testing.T) {
    p := &providers.MyProvider{}
    assert.True(t, p.OwnsModel("mp-some-model"))
    assert.False(t, p.OwnsModel("gr-llama-3"))
}
```

### 5. Open a PR

Include in your PR description:
- Provider name + website
- Free tier details (quota, rate limits)
- Auth method
- Which models are available

---

## Code Style

We follow standard Go conventions:

- **`gofmt`** — always run before committing (`gofmt -w .`)
- **`golint`** / **`staticcheck`** — avoid common pitfalls
- **Error handling** — always handle errors explicitly, don't use `_` for errors
- **Context** — always pass `ctx context.Context` as the first argument
- **Package names** — short, lowercase, no underscores
- **Comments** — exported functions must have a doc comment

```bash
# Format all Go files
gofmt -w .

# Vet for common issues
go vet ./...
```

---

## Testing

```bash
# Run all tests
make test

# Run specific package tests
go test ./internal/proxy/compression/... -v

# Run with race detector
go test -race ./...

# Coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

**Coverage targets:**
- New providers: test `OwnsModel` at minimum
- New API handlers: test happy path + auth failure
- New proxy logic: test with mock provider
- Compression stages: test compression ratio correctness

---

## Commit Messages

We use [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <short description>

[optional body]

[optional footer]
```

**Types:**

| Type | When to use |
|---|---|
| `feat` | New feature or provider |
| `fix` | Bug fix |
| `docs` | Documentation only |
| `test` | Adding/updating tests |
| `refactor` | Code change with no feature or fix |
| `perf` | Performance improvement |
| `chore` | Build, CI, dependency update |

**Examples:**

```bash
git commit -m "feat(providers): add cerebras provider with cb- prefix"
git commit -m "fix(proxy): handle nil response in fallback chain"
git commit -m "docs(readme): add relay system config examples"
git commit -m "perf(compression): short-circuit TSC stage when no tools present"
```

---

## Pull Request Process

1. **Fork → branch → commit → push → open PR**
2. Link any related issues in the PR description (`Closes #123`)
3. Fill out the PR template (description, test plan, screenshots if UI)
4. Ensure CI passes (build + tests)
5. Request review from a maintainer
6. Address review feedback — push new commits, don't force-push (makes review easier)
7. Maintainer squash-merges after approval

**PR title** should follow the same Conventional Commits format:
```
feat(providers): add myprovider with free-tier account pooling
```

---

## Questions?

Open an issue with the `question` label, or start a Discussion on GitHub. We're happy to help.

---

<div align="center">

*Every contribution, no matter how small, makes Switchblade better. Thank you.*

</div>
