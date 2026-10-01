# Astra Platform Capabilities Reference

> Full technical snapshot of `astra-backend` (Go) and `astra-rm` (Vite/React/TS) as of 2026-10-01.
> Includes all live capabilities, the BFF endpoint plan to wire the revamped RM dashboard to real data, and the DPDP-lawful lead generation pipeline implementation plan.

---

## 1. Backend — `astra-backend` (Go / Chi / PostgreSQL)

### 1.1 Infrastructure & Cross-Cutting

| Concern | Details |
|---|---|
| **Framework** | `go-chi/chi v5` with `Recoverer`, `StripSlashes`, `Compress(5)`, `Heartbeat(/ping)` |
| **Auth (User App)** | OTP → JWT flow, `RequireAuth` middleware, `RequireAuthWS` for WebSocket routes |
| **Auth (RM App)** | Separate employee OTP → JWT (signed with `RM_JWT_SECRET`), `RequireRMAuth` middleware, admin role enforced by `RequireAdmin` |
| **CORS** | Wildcard (`*`) on default router; named origins (`astrafin.netlify.app`, `astrafinrm.netlify.app`, `localhost:*`) on the main router |
| **AI / LLM** | Provider-agnostic seam: Groq (default), AWS Bedrock (configured, activates via `LLM_PROVIDER`). Agent catalog for `app_chat`, `app_quickchat`, `rm_copilot`, `admin_copilot`, `rm_narrator`, `memory` |
| **Speech** | Sarvam (default), AWS Polly+Transcribe (activates via `SPEECH_PROVIDER`) |
| **Schedulers** | Budget month-rollover (opt-in: `BUDGET_ROLLOVER_SCHEDULER=true`), IDBI nightly spend sync (opt-in: `IDBI_SPEND_SYNC_SCHEDULER=true`) |
| **Seeding** | `RM_SEED_ON_BOOT=true` → seeds 1 admin + 2 RMs and auto-assigns unassigned users |
| **Migrations** | Auto-run on boot via `database.RunMigrations` |

### 1.2 User App APIs (`/api/...`)

**Auth** — `/api/auth/...`
- `POST /api/auth/otp/send` — send OTP
- `POST /api/auth/otp/verify` — verify OTP, receive JWT
- `POST /api/auth/refresh` — refresh JWT
- `POST /api/auth/logout` — invalidate session
- `POST /api/auth/reset` — reset user
- `GET  /api/auth/me` — get current user *(protected)*
- `PATCH /api/auth/me` — update current user *(protected)*

**Chat** — `/api/chat/...` *(protected)*
- `POST /api/chat` — send message to AI assistant
- `GET  /api/chat/history` — chat history
- `GET  /api/chat/sessions` — list sessions
- `GET  /api/chat/sessions/{sessionID}` — session messages
- `GET  /api/chat/memory` — user memory
- `POST /api/chat/memory` — add memory item
- `DELETE /api/chat/memory/{id}` — delete memory item
- `POST /api/tts` — Text-to-Speech
- `POST /api/stt` — Speech-to-Text
- `GET  /api/chat/stt/stream` *(WebSocket)* — real-time STT stream

**Financial Domain** — `/api/v1/...` *(all protected)*

| Mount | Handler | Live Endpoints |
|---|---|---|
| `/api/v1/stocks` | `StocksHandler` | Stocks search, quote, history |
| `/api/v1/catalog` | `CatalogHandler` | Product catalog |
| `/api/v1/fd` | `FDHandler` | Fixed deposits |
| `/api/v1/payments` | `PaymentsHandler` | Payments initiation |
| `/api/v1/analytics/spend` | `AnalyticsHandler` | `weekday-weekend`, `trends`, `categories`, `category-momentum`, `average`, `merchants`, `recurring`, `impulse`, `summary`, `snapshot`, `compare`, `investment-consistency`, `bnpl`, `subscriptions`, `income`, `transactions` (16 endpoints) |
| `/api/v1/analytics/budgets` | `BudgetHandler` | Budget CRUD + month rollover |
| `/api/v1/goals` | `GoalsHandler` | Financial goals |
| `/api/v1/dashboard` | `DashboardHandler` | Spend + portfolio overview |
| `/api/v1/portfolio-analysis` | `PortfolioAnalysisHandler` | Per-user portfolio analysis |
| `/api/v1/watchlist` | `WatchlistHandler` | Watchlist CRUD |
| `/api/v1/mf` | `MFHandler` | MF holdings, purchase, redeem, transactions |
| `/api/v1/aa` | `AAHandler` | Account Aggregator (scaffolded, 501) |
| `/api/v1/kyc` | `KYCHandler` | KYC (scaffolded, 501) |
| `/api/v1/idbi` | `IDbiHandler` | IDBI mirror APIs (feature-flag-gated) |
| `/webhooks/idbi-aa` | `AAHandler` | Inbound AA notifications (497/498) |

### 1.3 RM / Admin APIs (`/api/rm/...`)

**RM Auth** — `/api/rm/auth/...`
- `POST /api/rm/auth/otp/send`
- `POST /api/rm/auth/otp/verify`
- `POST /api/rm/auth/refresh`
- `POST /api/rm/auth/logout`
- `GET  /api/rm/auth/me` *(protected)*
- `PATCH /api/rm/auth/me` *(protected)*

**RM Dashboard / Book** — served by `RMHandler` *(protected)*

| Endpoint | Handler Method | What it returns |
|---|---|---|
| `GET /api/rm/dashboard/summary` | `bookSummary` | AUM, client count, NPA metrics |
| `GET /api/rm/dashboard/insights` | `bookInsights` | AI-generated book-level observations |
| `GET /api/rm/dashboard/composition` | `bookComposition` | Product/segment breakdown |
| `GET /api/rm/dashboard/followups` | `pendingFollowUps` | Pending interactions list |
| `GET /api/rm/clients` | `listClients` | Paginated client list for the RM |
| `GET /api/rm/clients/{userID}` | `getClient` | Full client profile (`?days=`) |
| `GET /api/rm/clients/{userID}/growth` | `clientGrowth` | Growth metrics (`?days=180`) |
| `GET /api/rm/clients/{userID}/portfolio-history` | `portfolioHistory` | Time-series AUM (`?days=365`) |
| `GET /api/rm/clients/{userID}/portfolio-analysis` | `portfolioAnalysis` | Asset breakdown, returns |
| `GET /api/rm/clients/{userID}/advisory` | `clientAdvisory` | Advisory notes + recommendations |
| `GET /api/rm/clients/{userID}/analytics` | `clientAnalytics` | Spend analytics for RM view |
| `GET /api/rm/clients/{userID}/analytics/narrative` | `clientNarrative` | LLM narrative (`?group=&refresh=`) |
| `GET /api/rm/clients/{userID}/spend-intelligence` | `clientSpendIntelligence` | Spend categories + inflow/outflow |
| `GET /api/rm/clients/{userID}/interactions` | `listInteractions` | Call/meeting log |
| `POST /api/rm/clients/{userID}/interactions` | `addInteraction` | Add call/meeting note |
| `POST /api/rm/clients/{userID}/interactions/{id}/complete` | `completeInteraction` | Mark interaction done |

**RM Chat (Copilot)** — served by `RMChatHandler` *(protected)*
- `POST /api/rm/chat` — send message to RM Copilot
- `GET  /api/rm/chat/history` — copilot history
- `POST /api/rm/chat/tts` — TTS
- `POST /api/rm/chat/stt` — STT
- `GET  /api/rm/chat/stt/stream` *(WebSocket)* — real-time STT
- `GET  /api/rm/events` *(WebSocket)* — SSE real-time push events

**RM Admin** — `/api/rm/admin/...` *(requires admin role)*
- `GET  /api/rm/admin/overview`
- `GET  /api/rm/admin/rms` — list all RMs
- `POST /api/rm/admin/rms` — create RM
- `GET  /api/rm/admin/rms/{rmID}`
- `PATCH /api/rm/admin/rms/{rmID}`
- `GET  /api/rm/admin/rms/{rmID}/clients`
- `POST /api/rm/admin/rms/{rmID}/offboard`
- `GET  /api/rm/admin/clients`
- `POST /api/rm/admin/assignments/assign`
- `POST /api/rm/admin/assignments/transfer`
- `POST /api/rm/admin/assignments/remove`
- `GET  /api/rm/admin/assignments/history`

**IDBI Credit Risk** (feature-flagged, registered via `IDbiRMHandler`)
- `GET /api/rm/idbi/clients/{userID}/credit-risk`

---

## 2. RM Frontend — `astra-rm` (Vite + React + TypeScript)

### 2.1 Architecture

| Concern | Implementation |
|---|---|
| **Framework** | Vite + React 18 + TypeScript |
| **State** | `AppContext` (React Context) — auth, current view, selected customer ID |
| **Routing** | Manual view switching via `currentView` string in context (no React Router) |
| **API Layer** | `src/api/bankingApi.ts` — all calls flow through here; currently mock-only |
| **Mock Flag** | `src/config/features.ts` → `USE_MOCK_DATA: boolean` |
| **Auth** | OTP-based (same backend flow), currently bypassed in mock mode |

### 2.2 Views & Components

| View Key | Component | Data Type Consumed |
|---|---|---|
| `dashboard` | `DashboardView` | `DashboardData` |
| `portfolio` | `PortfolioView` | `PortfolioData` |
| `early-warnings` | `EarlyWarningsView` | `EarlyWarningsData` |
| `customers` | `CustomerListView` | *(uses PortfolioData.customers)* |
| `customer-detail` | `Customer360View` | `Customer360Data` |
| `customer-cashflow` | `CashFlowIntelligenceView` | `CashFlowIntelligenceData` |
| `customer-credit-risk` | `CreditRiskIntelligenceView` | `CreditRiskIntelligenceData` |
| placeholder views | `ComingSoon` | N/A |

Navigation: **Sidebar** drives `setCurrentView`. Customer drill-down uses `openCustomerProfile(customerId)` → `openCashFlowIntelligence()` → `openCreditRiskIntelligence()`.

### 2.3 `DashboardData` Shape (what the UI needs from the backend)

```typescript
{
  greetingName, businessUnit, quote, quoteAttribution, asOfLabel,
  kpis: {
    totalExposure, customers, activeLoans,          // volume KPIs
    healthy, watchlist, atRisk, critical             // risk KPIs
    // each: { label, value, deltaPct?, deltaLabel?, subLabel?, tone }
  },
  aiSummary: { badge, headline, body, ctaLabel, insights[] },
  portfolioHealth: { totalCustomers, segments[] },
  loanExposure: { totalCr, products[] },
  repayment: { rangeLabel, currentEfficiencyPct, efficiencyDeltaPct, months[] },
  riskDistribution: Band[],
  priorityCustomers: PriorityCustomer[],
  workItems: WorkItem[],
  workFocusQuote
}
```

### 2.4 Other Key Data Shapes

- **`PortfolioData`**: breadcrumbs, kpis, filters/tabs, paginated `customers[]`, `loanExposure`, `aiInsight`, quote
- **`EarlyWarningsData`**: `kpis`, `featuredAlert`, `otherAlerts[]`, `aiInsight`, `riskTrend`, `recentAlerts[]`
- **`Customer360Data`**: `profile`, `healthScore`, `aiInsight`, `stats[]`, `activeLoans[]`, `creditProfile`, `recentActivity[]`, `balanceTrend`, `cashFlow`, `riskScoreTrend`, `riskInsights[]`
- **`CashFlowIntelligenceData`**: `customer`, `kpis[]`, `trend`, `breakdown`, `aiInsight`, `forecast`, `next30Days`, `transactionInsights[]`
- **`CreditRiskIntelligenceData`**: `customer`, `productChips[]`, `healthScore`, `scoreTrend`, `riskMigration`, `dpdTrend`, `utilizationTrend`, `activeAccounts`, `bureauSummary`, `riskDrivers[]`, `peerComparison`, `aiInsight`

---

## 3. Integration Gap: What's Missing

The current backend returns atomic, resource-level payloads. The new RM UI expects rich **view-model** objects with formatted labels, AI insights, trends, and risk metrics combined in a single HTTP call per view.

### Gap Summary

| UI View | Current Backend Endpoints | Missing from Backend |
|---|---|---|
| Dashboard | `summary`, `insights`, `composition`, `followups` | Aggregated KPIs (`healthy/watchlist/atRisk/critical` counts), repayment trend, `priorityCustomers` table, work items, AI summary panel |
| Portfolio | `listClients`, `composition` | Aggregate loan exposure breakdown, paginated table with risk tags, portfolio-level AI insight |
| Early Warnings | `followups` (partial) | Risk-scored alert list across full book, `riskTrend` time-series, `featuredAlert` logic |
| Customer 360 | `getClient`, `clientAnalytics` | `FinancialHealthScore` struct, `CreditProfile`, `balanceTrend` (time-series), `cashFlow` monthly chart |
| Cash Flow Intelligence | `clientSpendIntelligence` | `forecast[]`, structured `breakdown` rows with `BreakdownTone`, `transactionInsights[]` |
| Credit Risk Intelligence | `idbi/clients/{id}/credit-risk` *(feature-flagged)* | Full standalone endpoint: `riskMigration`, `dpdTrend`, `bureauSummary`, `peerComparison`, `riskDrivers[]` |
| Auth (Login Screen) | `rm/auth/otp/send`, `rm/auth/otp/verify` | ✅ Already exists — just needs wiring in `AppContext.login()` |

---

## 4. Plan: New BFF Endpoints

We will add a new **BFF (Backend-For-Frontend)** route group in `astra-backend` under `/api/rm/bff/`. These endpoints aggregate existing service calls and format the output to exactly match the TypeScript interfaces consumed by `bankingApi.ts`.

### 4.1 Route Plan

```
/api/rm/bff/
├── GET  /dashboard                       → DashboardData
├── GET  /portfolio                       → PortfolioData
├── GET  /early-warnings                  → EarlyWarningsData
└── GET  /clients/{userID}/
    ├── customer-360                      → Customer360Data
    ├── cashflow                          → CashFlowIntelligenceData
    └── credit-risk                       → CreditRiskIntelligenceData
```

All routes under `/api/rm/bff/` are protected by `RequireRMAuth`.

### 4.2 Implementation Details Per Endpoint

#### `GET /api/rm/bff/dashboard` → `DashboardData`

**Aggregation Logic:**
1. Call `svc.BookSummary(ctx, rmID)` → total AUM, customer count, active loans
2. Call `svc.BookComposition(ctx, rmID)` → segment risk distribution (healthy/watchlist/at-risk/critical)
3. Call `svc.BookInsights(ctx, rmID)` → AI summary text + bullet insights
4. Call `svc.PendingFollowUps(ctx, rmID)` → work items for `MyWorkTodayCard`
5. Query top 5 at-risk clients from `rm_repo` for `priorityCustomers` table
6. Compute repayment efficiency trend from last 6 months of transaction data

**New fields to compute server-side:**
- `greetingName` from `rm.full_name`
- `asOfLabel` = `time.Now().Format("Mon, 02 Jan 2006")`
- `kpis.healthy/watchlist/atRisk/critical` = counts from risk segment data
- `loanExposure` donut slices from `composition` data

---

#### `GET /api/rm/bff/portfolio` → `PortfolioData`

**Aggregation Logic:**
1. Call `svc.ListClients(ctx, rmID, filters)` → paginated customer list
2. For each customer, derive `riskTag` from existing risk score fields
3. Compute `loanExposure` donut from aggregate product breakdown
4. Call `svc.BookInsights(ctx, rmID)` for `aiInsight` panel
5. Support `?page=`, `?tab=`, `?search=` query params for filtering

---

#### `GET /api/rm/bff/early-warnings` → `EarlyWarningsData`

**Aggregation Logic:**
1. Query all clients sorted by risk score descending
2. Top 1 → `featuredAlert` (full detail)
3. Next N → `otherAlerts[]` with icon mapping (emi/salary/utilization/delay/bureau/borrowing/withdrawal)
4. Compute `riskTrend.points[]` from monthly snapshot data (or derive from portfolio history)
5. Generate `recentAlerts[]` from recent interactions with `risk` tone
6. Compute KPI counts: critical, high-risk, watchlist, improving

---

#### `GET /api/rm/bff/clients/{userID}/customer-360` → `Customer360Data`

**Aggregation Logic:**
1. `svc.GetClient(ctx, rmID, isAdmin, userID, 90)` → profile + basic data
2. `svc.ClientAnalytics(ctx, rmID, isAdmin, userID)` → spend breakdown
3. `svc.PortfolioHistory(ctx, rmID, isAdmin, userID, 90)` → `balanceTrend.points[]`
4. `svc.SpendIntelligence(ctx, rmID, isAdmin, userID)` → monthly `cashFlow.months[]`
5. **Compute `FinancialHealthScore`**: aggregate from CIBIL score + utilization + DPD + EMI ratio
6. **Format `CreditProfile`**: map CIBIL/DPD fields from existing client data
7. **Format `recentActivity[]`**: from interactions + spend transactions
8. **Derive `riskInsights[]`**: from risk score factors

---

#### `GET /api/rm/bff/clients/{userID}/cashflow` → `CashFlowIntelligenceData`

**Aggregation Logic:**
1. `svc.SpendIntelligence(ctx, rmID, isAdmin, userID)` → inflow/outflow data
2. Build `trend.months[]` from last 6 months of spend data
3. Build `breakdown.rows[]` with `BreakdownTone` classification
4. **Compute `forecast.months[]`**: project next 3 months using trailing average of salary + recurring EMI obligations
5. Build `transactionInsights[]` from top 4 transaction patterns (salary cadence, card spend, UPI patterns, SIP detection)
6. Build `next30Days` projection object

---

#### `GET /api/rm/bff/clients/{userID}/credit-risk` → `CreditRiskIntelligenceData`

**Aggregation Logic:**
1. `svc.GetClient(ctx, rmID, isAdmin, userID, 0)` → CIBIL score, DPD, utilization
2. `svc.PortfolioHistory(ctx, rmID, isAdmin, userID, 365)` → `scoreTrend.points[]`
3. Map CIBIL score history into `riskMigration.stages[]`
4. Derive `dpdTrend.points[]` and `utilizationTrend.points[]` from month-by-month data
5. **`bureauSummary`**: format CIBIL fields into display rows
6. **`riskDrivers[]`**: categorize risk factors (high utilization, EMI/income ratio, recent DPD, bureau enquiries)
7. **`peerComparison`**: compute against RM's portfolio median benchmarks
8. **`aiInsight`**: call `svc.ClientNarrative()` with `group=credit-risk`

---

### 4.3 File Structure Plan (in `astra-backend`)

```
internal/
├── handler/
│   └── rm_bff_handler.go          ← new: 6 BFF handlers
├── service/
│   └── rmbff/
│       ├── service.go             ← new: aggregation + formatting logic
│       └── formatters.go          ← new: label formatting helpers (₹Cr, %, dates)
└── domain/
    └── rmbff/
        └── types.go               ← new: Go structs matching TS interfaces exactly
```

### 4.4 Frontend Wiring (in `astra-rm`)

Update `src/api/bankingApi.ts` to call the real endpoints when `!USE_MOCK_DATA`:

```typescript
const BASE = import.meta.env.VITE_RM_API_BASE_URL; // e.g. http://localhost:8080

export const bankingApi = {
  async getDashboard(): Promise<DashboardData> {
    if (USE_MOCK_DATA) return delay(mockDashboardData);
    const res = await fetch(`${BASE}/api/rm/bff/dashboard`, { headers: authHeaders() });
    if (!res.ok) throw new Error(await res.text());
    return res.json();
  },
  // ... same pattern for all 6 endpoints
};
```

Auth headers will be read from `localStorage` after the OTP login flow is wired up in `AppContext.login()`.

---

## 5. DPDP-Lawful Lead Generation Pipeline

Source: [`dpdp-lead-generation-plan.md`](file:///Users/swaraj/Documents/astra-backend/docs/dpdp-lead-generation-plan.md)

This pipeline turns the spend/analytics data already flowing through the platform into RM cross-sell leads — without that becoming a **DPDP Act, 2023** violation. It feeds into `428 createLead` in the IDBI integration plan.

> [!IMPORTANT]
> **Part 1 (consent gate) MUST be built before Part 2 (scoring).** The scorer's entire input is data Part 1 decides you're allowed to touch. No exceptions.

### 5.1 Why the gate must be first

Spend data in `spend_transactions` was collected under **account-servicing** purpose. Using it to score a cross-sell lead is a **different, secondary purpose** under DPDP — it requires its own explicit opt-in consent. Reusing the account-servicing consent does not cover it.

Three invariants that shape the whole design:
- A user who has never opted into `MARKETING_CROSS_SELL` must be **fully excluded from scoring** — not scored-but-suppressed.
- Consent can be withdrawn **after** a lead was already scored and even after it was pushed to IDBI's CRM. The system needs a **withdrawal cascade**, not just a check at ingestion.
- Every decision must be **provable after the fact** — which rule allowed which data, for which user, at which time. That's a structured audit log, not a log line.

### 5.2 Part 1 — Consent & Eligibility Rules Engine

#### Rules Engine

**[Grule](https://github.com/hyperjumptech/grule-rule-engine)** — pure Go, embeds directly into `astra-backend` with no new infrastructure. Rules stored as versioned GRL text in Postgres.

Rejected alternatives: hand-rolled if/else (no audit trace, redeployment to change compliance rule), OPA/Rego (external service, overkill for v1).

#### Schema

```sql
-- What purposes exist and whether each needs explicit opt-in
CREATE TABLE consent_purposes (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code                    TEXT UNIQUE NOT NULL,  -- ACCOUNT_SERVICING | ANALYTICS_PROFILING | MARKETING_CROSS_SELL
    description             TEXT NOT NULL,
    lawful_basis            TEXT NOT NULL,         -- CONSENT | LEGITIMATE_USE (DPDP s.7)
    requires_explicit_optin BOOLEAN NOT NULL DEFAULT TRUE,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Versioned notice text — what the user actually saw when they consented (DPDP requirement)
CREATE TABLE consent_notices (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    purpose_id     UUID NOT NULL REFERENCES consent_purposes(id),
    version        INT NOT NULL,
    body           TEXT NOT NULL,
    effective_from TIMESTAMPTZ NOT NULL,
    UNIQUE (purpose_id, version)
);

CREATE TABLE user_consents (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id               UUID NOT NULL REFERENCES users(id),
    purpose_id            UUID NOT NULL REFERENCES consent_purposes(id),
    status                TEXT NOT NULL,   -- GRANTED | WITHDRAWN | EXPIRED
    notice_version        INT NOT NULL,
    channel               TEXT NOT NULL,   -- app | sms | email
    consent_artifact_hash TEXT NOT NULL,   -- hash of exactly what was shown/signed
    granted_at            TIMESTAMPTZ,
    withdrawn_at          TIMESTAMPTZ,
    expires_at            TIMESTAMPTZ,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, purpose_id, status) DEFERRABLE INITIALLY DEFERRED
);
CREATE INDEX idx_user_consents_lookup ON user_consents (user_id, purpose_id, status);

-- Rule content — versioned & approved separately from code review
CREATE TABLE dpdp_rules (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rule_id        TEXT NOT NULL,     -- stable logical id, e.g. "exclude-minors"
    version        INT NOT NULL,
    name           TEXT NOT NULL,
    category       TEXT NOT NULL,     -- purpose-match | data-minimization | exclusion | retention
    dsl            TEXT NOT NULL,     -- GRL rule text
    effective_from TIMESTAMPTZ NOT NULL,
    effective_to   TIMESTAMPTZ,
    approved_by    TEXT NOT NULL,     -- compliance/legal sign-off, not a dev username
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (rule_id, version)
);

-- Structured audit trail — every CheckEligibility call, allow or deny
CREATE TABLE consent_decision_log (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID NOT NULL REFERENCES users(id),
    purpose_code     TEXT NOT NULL,
    data_categories  TEXT[] NOT NULL,
    decision         BOOLEAN NOT NULL,
    matched_rule_ids TEXT[] NOT NULL,
    reason           TEXT NOT NULL,
    evaluated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_consent_decision_log_user ON consent_decision_log (user_id, evaluated_at DESC);
```

#### Service API — `internal/service/consent`

```go
type Purpose string

const (
    PurposeAccountServicing   Purpose = "ACCOUNT_SERVICING"
    PurposeAnalyticsProfiling Purpose = "ANALYTICS_PROFILING"
    PurposeMarketingCrossSell Purpose = "MARKETING_CROSS_SELL"
)

type Decision struct {
    Allowed      bool
    MatchedRules []string
    Reason       string
}

// CheckEligibility must be called at EVERY stage boundary — not once at ingestion.
// Consent can be withdrawn between scoring and lead creation.
func (s *Service) CheckEligibility(
    ctx context.Context,
    userID uuid.UUID,
    purpose Purpose,
    dataCategories []string,
) (Decision, error)

// RecordConsent captures a grant or withdrawal with the notice version
// and a hash of the exact consent artifact (what DPDP audits ask for).
func (s *Service) RecordConsent(
    ctx context.Context,
    userID uuid.UUID,
    purpose Purpose,
    action ConsentAction, // Grant | Withdraw
    notice NoticeRef,
    channel string,
) error

// Withdraw triggers the full cascade (see §5.4)
func (s *Service) Withdraw(ctx context.Context, userID uuid.UUID, purpose Purpose) error
```

#### Rule Catalog (v1)

| Category | Rule | Effect |
|---|---|---|
| Purpose match | Active non-expired `MARKETING_CROSS_SELL` consent | Required to enter scoring at all |
| Data minimization | Only `category`, `amount_band`, `txn_frequency` fields may enter the feature vector | Narration/merchant strings never leave `spend_transactions` for this purpose |
| Special-category exclusion | User flagged as minor, deceased, or dormant | Hard exclude — no other rule can override |
| Cross-border | Any call in the scoring path that routes data outside India | Blocked — ties to the "Bedrock VPC-only, in-region" requirement |
| Retention | Lead not converted within 90 days | Auto-expire; purge derived scoring data |
| Withdrawal | `user_consents.status` flips to `WITHDRAWN` | Immediate stop on all downstream use |

> [!WARNING]
> Rules are **additive deny, not additive allow** — a request is allowed only if every applicable rule returns allow. Any single deny wins. New rules added later keep the posture conservative by default.

### 5.3 Where the Gate Gets Called

Three mandatory call sites:
1. **Before the nightly propensity scoring job** touches `spend_transactions` for any user → `CheckEligibility(..., PurposeMarketingCrossSell, ["category","amount_band","txn_frequency"])`
2. **Again immediately before `428 createLead`** fires — even if the score was computed hours earlier. Consent may have changed in between.
3. **On every `/api/rm/bff/clients/{userID}/*` read** that would expose a lead/propensity field to an RM — same reasoning.

### 5.4 Withdrawal Cascade

Withdrawal is an **event**, not just a status flip. Must happen asynchronously (safegoroutine-wrapped) so a slow CRM retraction call doesn't block the user-facing "consent updated" response:

1. `user_consents.status` → `WITHDRAWN`, `withdrawn_at` set.
2. Any `idbi_leads` row for that user tied to `MARKETING_CROSS_SELL` that hasn't converted → marked `suppressed` (not deleted — DPDP requires evidence of lawful handling).
3. If the lead was already pushed to IDBI CRM via `428 createLead`, fire a retraction/status-update call using the stored `leadId`.
4. Any cached propensity score for that user is purged from the scoring store.

### 5.5 Part 2 — Propensity Scoring → Lead Creation *(Next Phase)*

> [!NOTE]
> Not built yet — outlined so Part 1's interface matches what Part 2 will need to call.

**Inputs (data-minimization enforced by Part 1):** transaction category distribution, amount bands, frequency patterns, existing product holdings (MF/FD/stocks). No raw narrations, no merchant strings, no bureau data unless a separate rule explicitly permits it.

**Scoring (v1):** Rules-based propensity score — no trained model yet. Example: `high recurring SIP + no FD holding + stable salary credit` → FD cross-sell signal. Keeps decisions fully explainable for the same audit reasons as the consent gate. A SageMaker-backed version is a later swap-in behind the same interface.

**Lead creation flow (on score crossing threshold):**
1. `CheckEligibility` re-check.
2. Write to `idbi_leads` — dedup on `(user_id, product_category)` within a cooldown window.
3. Call `428 createLead`, store returned `leadId` on the local row.
4. Log the `consent_decision_log` reference alongside the lead row so any lead can be traced back to the exact eligibility decision that permitted it.

### 5.6 Rollout Phases

| Phase | Scope |
|---|---|
| **1 — Consent foundation** | Schema (§5.2), `consent` service + Grule integration, `CheckEligibility`/`RecordConsent` wired, no scoring yet |
| **2 — Consent UI** | Explicit `MARKETING_CROSS_SELL` opt-in — separate screen/notice, not a bundled checkbox *(DPDP strongly favors standalone notice; bundled consent for secondary purpose is a common challenge point)* |
| **3 — Withdrawal cascade** | §5.4, including the `428` retraction call |
| **4 — Propensity scoring v1** | Rules-based scorer (§5.5) behind the eligibility gate |
| **5 — Lead creation wiring** | `428` integration, dedup/cooldown, audit linkage |
| **6 — Compliance review** | Legal/DPO sign-off on `dpdp_rules` content before Phase 4/5 go live with real customer data |

---

## 6. Master Execution Order

### Track A: BFF + Dashboard Wiring

| Priority | Task | Effort |
|---|---|---|
| 🔴 P0 | Wire `AppContext.login()` to real OTP endpoints | ~1 hr |
| 🔴 P0 | Create `rmbff/types.go` matching all 6 TS interfaces | ~2 hr |
| 🔴 P0 | Implement `GET /bff/dashboard` | ~3 hr |
| 🟠 P1 | Implement `GET /bff/portfolio` | ~2 hr |
| 🟠 P1 | Implement `GET /bff/clients/{id}/customer-360` | ~3 hr |
| 🟡 P2 | Implement `GET /bff/early-warnings` | ~3 hr |
| 🟡 P2 | Implement `GET /bff/clients/{id}/cashflow` | ~2 hr |
| 🟡 P2 | Implement `GET /bff/clients/{id}/credit-risk` | ~3 hr |
| 🟢 P3 | Update `bankingApi.ts` to call live endpoints | ~1 hr |
| 🟢 P3 | `authHeaders()` utility + JWT storage in `AppContext` | ~1 hr |

### Track B: DPDP Lead Pipeline

| Phase | Task | Effort | Dependency |
|---|---|---|---|
| Phase 1 | Postgres schema migration (§5.2 tables) | ~1 hr | — |
| Phase 1 | `internal/service/consent` package + Grule wiring | ~4 hr | Schema |
| Phase 1 | `CheckEligibility` + `RecordConsent` unit tests | ~2 hr | Service |
| Phase 2 | Consent opt-in UI screen (app-side) | ~3 hr | Service |
| Phase 2 | `POST /api/consent/marketing` endpoint | ~1 hr | Service |
| Phase 3 | Withdrawal cascade + async safegoroutine | ~3 hr | Service + IDBI `428` |
| Phase 4 | Rules-based propensity scorer | ~4 hr | Phase 1 gate ✅ |
| Phase 5 | `idbi_leads` table + `428 createLead` wiring | ~3 hr | Phase 4 scorer |
| Phase 6 | DPO/legal sign-off on `dpdp_rules` content | N/A | Phase 4/5 ready |

> [!CAUTION]
> Phase 4 (scoring) and Phase 5 (lead creation) must **not** go live with real customer data until Phase 6 compliance review is complete. Running the scorer in a dry-run mode (log decisions, don't push leads) is safe and recommended for pre-review validation.
