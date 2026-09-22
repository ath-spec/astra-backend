# Astra Backend

The API server behind **Astra**, a modern wealth-management app for Indian retail
investors. It's a single Go service that powers two clients:

- **The Astra app** — portfolio tracking, spend analytics, budgets, goals, mutual
  funds, stocks, fixed deposits, payments, and an AI wealth-advisor chat.
- **The RM portal** — a relationship-manager console for staff, with its own
  auth, an AI copilot, client credit-risk views, and live event streaming.

It talks to Postgres for storage, Groq (or AWS Bedrock) for LLM chat, Sarvam
(or AWS Polly/Transcribe) for speech, and IDBI Bank's Atlas sandbox/gateway for
real banking data — every IDBI integration is off by default and gated behind
its own env flag, so the app runs on mock data out of the box.

---

## Contents

- [What it does](#what-it-does)
- [Architecture at a glance](#architecture-at-a-glance)
- [Prerequisites](#prerequisites)
- [Quick start (local, mock data)](#quick-start-local-mock-data)
- [Running with IDBI sandbox data](#running-with-idbi-sandbox-data)
- [Configuration reference](#configuration-reference)
- [Database & migrations](#database--migrations)
- [API surface](#api-surface)
- [Testing](#testing)
- [Deploying to AWS](#deploying-to-aws)
- [Project layout](#project-layout)

---

## What it does

Astra's backend is organized around two personas hitting the same service:

| Persona | Entry point | Highlights |
|---|---|---|
| **End user** (the app) | `/api/*` | AI chat advisor, portfolio & fund data, spend/budget analytics, goals, watchlist, payments, KYC, Account Aggregator consent flow |
| **Relationship Manager / Admin** (the portal) | `/api/rm/*` | Separate OTP auth, RM AI copilot, client narrative & composition views, credit-risk exposure, live SSE event stream, admin HRMS-gated login |

The AI advisor (`internal/handler/chat.go`, `internal/service/ai.go`) is a
system-prompted LLM chat that:
- Grounds every answer in the user's **real, live financial data** (never invents holdings or balances).
- Never names a specific fund/stock to buy — only strategy ("increase equity allocation").
- Detects and replies in the user's own language/script.
- Switches into a **lead-generation flow** when a user shows purchase intent for a
  product like insurance (including recommendation-style asks, e.g. "which car
  insurance is best for me") — asks qualifying questions one at a time, then
  hands off to an RM via a structured `rm_lead` JSON payload instead of
  hallucinating advice.
- Keeps cross-session memory (dated facts/preferences) and a PII redaction pass
  on every turn in and out.

## Architecture at a glance

```
                     ┌─────────────────────┐
   Astra app  ─────▶ │                     │
                     │   cmd/api (chi      │──▶ Postgres (sqlc-generated repo layer)
   RM portal  ─────▶ │   HTTP router)      │
                     │                     │──▶ Groq / AWS Bedrock  (LLM)
                     └─────────┬───────────┘
                               │                ──▶ Sarvam / AWS Polly+Transcribe (speech)
                     internal/handler (HTTP)
                               │
                     internal/service (business logic)
                               │
                     internal/provider (external integrations)
                               │
                     IDBI Atlas gateway (accounts, loans, AA, KYC, credit
                     score, leads, RM credit-risk — sandbox by default)
```

- **Router:** [`go-chi`](https://github.com/go-chi/chi), wired up in
  [`cmd/api/main.go`](cmd/api/main.go).
- **Data access:** [`sqlc`](https://sqlc.dev/) generated queries against
  Postgres (`internal/db`, `internal/repository`, migrations in
  `internal/database/migrations`).
- **AI providers:** pluggable per-surface — `LLM_PROVIDER=groq|bedrock`,
  `SPEECH_PROVIDER=sarvam|aws`. Individual Bedrock *agents* can be swapped in
  per chat surface without touching code.
- **IDBI integration:** every live-banking feature (`internal/provider/idbi`)
  is a **flag flip** — with everything unset the app behaves byte-for-byte
  like it does on mock/seeded data.

## Prerequisites

- **Go** 1.25+
- **Docker** (for local Postgres via `docker-compose.yml`) — or your own Postgres instance
- **[golang-migrate](https://github.com/golang-migrate/migrate)** CLI, for `make migrate-up` / `migrate-down`
- **make** (Git Bash on Windows doesn't ship one — use WSL, Git Bash + a `make` binary, or run the underlying commands directly; see [Makefile](Makefile))

## Quick start (local, mock data)

```bash
# 1. Start Postgres in Docker
make db-up

# 2. Apply migrations
DATABASE_URL="postgres://postgres:password@localhost:5432/astra?sslmode=disable" make migrate-up

# 3. Run the API (mock providers, no IDBI features, no cloud creds needed)
make run
```

That's it — `make run` exports sane local defaults (`JWT_SECRET`,
`RM_JWT_SECRET`, `PORT=8080`, `LLM_PROVIDER=groq`, `RM_OTP_DEV_CODE=123456`)
and starts the server on `http://localhost:8080`. Hit `GET /` for a liveness check.

> You still need a real `GROQ_API_KEY` for chat to work — copy `.env.example`
> to `.env` and fill it in, or export it before `make run`.

To wipe the local DB and start fresh:

```bash
make db-clean   # docker-compose down -v — drops the volume
make db-up
make migrate-up
```

## Running with IDBI sandbox data

```bash
make run-idbi
```

This flips on **every** IDBI feature flag (accounts, spend, loans, KYC,
credit score, leads, Account Aggregator, RM credit-risk) pointed at
`IDBI_BASE_URL=mock://sandbox`, using the sandbox CKYC credentials baked into
the Makefile and the fixtures in `internal/provider/idbi/testdata`. No real
IDBI gateway access or IP allow-listing is needed for this mode — it's a
canned local sandbox, not the live bank.

To point at the **real** IDBI sandbox/gateway instead, set `IDBI_BASE_URL` to
the real host and flip individual feature flags in `.env` — see
[Configuration reference](#configuration-reference). Note the live gateway
authorizes purely by the caller's **allow-listed egress IP**; nothing else
will get you in without an SSM/SOCKS proxy tunnel (see `scripts/idbi-proxy.sh`
/ `scripts/idbi-proxy.ps1`).

To (re)seed IDBI-linked demo customers:

```bash
make seed
```

## Configuration reference

All configuration is environment variables, documented end-to-end in
[`.env.example`](.env.example). Copy it to `.env` (git-ignored) and fill in
secrets — never commit real values.

**Core**
| Var | Purpose |
|---|---|
| `PORT` | HTTP port (default `8080`) |
| `DATABASE_URL` | Postgres connection string |
| `REDIS_URL` | Optional cache/session store |
| `FRONTEND_URL` | CORS origin for the app |
| `JWT_SECRET` / `RM_JWT_SECRET` | Signing secrets for app vs. RM-portal auth (kept separate) |
| `MASTER_INTERNAL_KEY` | 32-char key that, when set, makes the server decrypt marked secrets (like `GROQ_API_KEY`) on boot instead of reading them plaintext |

**AI providers** — each surface can be switched independently:
| Var | Purpose |
|---|---|
| `LLM_PROVIDER` | `groq` (default, wired) or `bedrock` (provisioned) |
| `GROQ_API_KEY` | Required when `LLM_PROVIDER=groq` |
| `LLM_GROQ_MODELS` | Optional CSV override of the fallback model chain |
| `BEDROCK_REGION`, `BEDROCK_MODEL_ID`, `BEDROCK_AGENT_ID`, `BEDROCK_AGENT_ALIAS_ID` | Base Bedrock config |
| `BEDROCK_AGENT_*_ID` | Per-surface agent overrides (portfolio tabs, app chat, RM copilot, admin copilot, RM narrator, memory) — move one chat surface to Bedrock at a time with zero capability change |
| `SPEECH_PROVIDER` | `sarvam` (default) or `aws` (Polly + Transcribe) |
| `SARVAM_API_KEY` | Required when `SPEECH_PROVIDER=sarvam` |
| `AI_TIPS_ENABLED` | Toggles the portfolio-analysis AI tip endpoints |

**IDBI Atlas integration** — every row below is `false`/blank by default,
meaning the app runs entirely on mock data until you opt in:

| Flag | Unlocks |
|---|---|
| `IDBI_ACCOUNTS_ENABLED` | Real account balances on the dashboard |
| `IDBI_SPEND_ENABLED` | Real transactions feeding spend analytics & budgets |
| `IDBI_LOANS_ENABLED` | "My Loans" |
| `IDBI_AA_ENABLED` (+ `IDBI_AA_REDIRECT_MODE`, `IDBI_AA_CALLBACK_URL`, etc.) | Account Aggregator consent flow |
| `IDBI_RM_RISK_ENABLED` | RM-portal per-client credit-risk snapshot |
| `IDBI_KYC_ENABLED` (+ `IDBI_CKYC_*`) | CKYC / PAN verification |
| `IDBI_HRMS_LOGIN_ENABLED` (+ `RM_SEED_RM{1,2}_EMPLOYEE_CODE`) | Verifies staff EIN against HRMS before issuing an RM login code (fails open on HRMS outage) |
| `IDBI_CREDIT_SCORE_ENABLED` | `GET /api/v1/idbi/credit-score` (dead upstream in sandbox — runs on mock source) |
| `IDBI_LEADS_ENABLED` (+ `IDBI_LEAD_*`) | `POST /api/v1/idbi/leads` — thin passthrough to IDBI's CRM |

Full inline documentation for every flag (including sandbox test values) lives
in [`.env.example`](.env.example) — treat it as the source of truth.

## Database & migrations

Migrations live in `internal/database/migrations` and run via
[golang-migrate](https://github.com/golang-migrate/migrate):

```bash
DATABASE_URL="postgres://postgres:password@localhost:5432/astra?sslmode=disable" make migrate-up
DATABASE_URL="postgres://postgres:password@localhost:5432/astra?sslmode=disable" make migrate-down
```

Query code is generated with [`sqlc`](https://sqlc.dev/) from `sqlc.yaml`:

```bash
make generate
```

## API surface

Everything is mounted in [`cmd/api/main.go`](cmd/api/main.go).

**App-facing (`/api/...`)**
- `POST /api/auth/otp/send` / `verify` / `refresh` / `logout` / `reset`
- `POST /api/chat`, `GET /api/chat/history`, `/sessions`, `/sessions/{id}`, `/memory`
- `GET /api/chat/stt/stream` (WebSocket), `POST /api/tts`, `POST /api/stt`
- `/api/v1/stocks`, `/catalog`, `/fd`, `/payments`, `/analytics/spend`,
  `/analytics/budgets`, `/goals`, `/dashboard`, `/portfolio-analysis`,
  `/watchlist`, `/mf`, `/aa`, `/kyc` — each a self-contained route group
  (see the matching `internal/handler/*_handler.go`)
- `/api/v1/idbi/*` — only mounted when at least one IDBI flag is enabled

**RM portal (`/api/rm/...`)**
- `POST /api/rm/auth/otp/send` / `verify` / `refresh` / `logout`
- `GET /api/rm/auth/me`
- `/api/rm/admin/*` — admin-only routes
- `GET /api/rm/chat/stt/stream` (WebSocket)
- `GET /api/rm/events` — Server-Sent Events stream for live RM notifications

Auth is JWT-based with **separate secrets** for app users vs. RM/admin staff,
so a leaked app token can't be replayed against the portal or vice versa.

## Testing

```bash
make test        # go test -v ./...
```

Notable test files: `internal/service/ai_test.go`,
`internal/service/memory_test.go`, `internal/service/pii_redactor_test.go`,
`internal/service/portfolio_analysis_test.go`,
`internal/service/rmauth_hrms_test.go`. There's also a smoke-test script for
the RM surfaces: `scripts/rm_smoketest.sh`.

## Deploying to AWS

Production runs on a single EC2 box behind Docker Compose
([`docker-compose.prod.yml`](docker-compose.prod.yml) +
[`Dockerfile.prod`](Dockerfile.prod)):

```bash
make deploy               # rsync + remote docker compose build/up
make deploy-ssm            # same, but tunneled through AWS SSM instead of direct SSH
```

Both targets cross-compile a Linux binary locally
(`CGO_ENABLED=0 GOOS=linux GOARCH=amd64`), sync the repo to the box, and run
`docker compose -f docker-compose.prod.yml up -d --build` remotely. Defaults
point at `EC2_IP=43.205.52.148`, `EC2_USER=ec2-user`,
`PEM_FILE=./zeyro-idbi.pem` — override any of them:

```bash
make deploy PEM_FILE=/path/to/key.pem
```

**Disk hygiene:** the EC2 box has a small (8 GB) root volume. After every
deploy, prune old images and build cache so it doesn't fill up:

```bash
ssh -i <pem> ec2-user@<ip> "docker image prune -af && docker builder prune -af"
```

**Gotcha:** if you ever hand-roll the sync step (e.g. `rsync`/`make` aren't
available in your shell) with a `tar`/`scp` pipeline instead, remember to
preserve/`chmod +x` the compiled binaries before rebuilding — a tarball built
on a non-Unix filesystem can silently drop the executable bit, which makes
the container crash-loop with `permission denied` on `/app/main`.

## Project layout

```
cmd/
  api/          — main entrypoint, router wiring, DI/bootstrapping
  encrypt/      — CLI to encrypt secrets for MASTER_INTERNAL_KEY mode
  rmseed/       — RM staff seeding CLI
internal/
  handler/      — HTTP handlers (one file per feature area)
  service/      — business logic (AI chat, memory, PII redaction, RM copilot, analytics, ...)
  provider/     — external integrations (IDBI, LLM, speech, and per-domain data sources)
  domain/       — domain models per feature area
  repository/   — data-access layer over generated sqlc queries
  db/           — sqlc-generated code
  database/     — migrations
  middleware/   — auth, CORS, logging, etc.
  config/       — env loading & validation
  crypto/       — secret encryption helpers
  discoverypool, events, idbimap, apiresponse, apitime, commons, httpx, rmseed
scripts/        — IDBI fixture capture, SSM/SOCKS proxy helpers, smoke tests, seeding
docs/           — IDBI API catalog, integration plan, payload references
```

---

Questions about a specific feature or flag are usually answered fastest by
grepping its name straight in `.env.example` or its `*_handler.go` file — this
README covers the map, not every leaf.
