# DPDP-Lawful Lead Generation — Implementation Plan

How astra-backend turns consumer insight (spend/analytics data already flowing
through the platform) into an RM/CRM lead, without that pipeline becoming a
DPDP Act, 2023 violation. Read alongside:

- [`idbi-integration-plan.md`](./idbi-integration-plan.md) — `428 createLead`
  is the CRM hand-off this pipeline feeds (Phase 4, RM lead pipeline)
- [`idbi-api-catalog.json`](./idbi-api-catalog.json) — `428` request/response
  shape

Built in two parts, in this order: **Part 1 is the consent/eligibility gate
and rules engine — build this first.** Nothing in Part 2 (the scoring
algorithm) is allowed to run before Part 1 exists, because Part 2's whole
input is data Part 1 decides you're allowed to touch.

---

## 1. Why this has to be gate-first, not model-first

Spend/transaction data in `spend_transactions` is collected under an
**account-servicing** purpose (the user agreed to it so the app can show
them their own spend). Using that same data to score a customer for a loan
or investment cross-sell lead is a **different, secondary purpose** under
DPDP. That requires its own specific, informed, opt-in consent — reusing the
account-servicing consent does not cover it, no matter how useful the data
would be for scoring.

Consequences that shape the whole design:

- A customer who never opts into `MARKETING_CROSS_SELL` must be **fully
  excluded** from scoring — not scored-but-not-contacted, not scored-and-
  suppressed. Excluded from the computation entirely.
- Consent can be withdrawn **after** a lead was already scored and even
  after it was already pushed to IDBI's CRM via `428`. The system needs a
  withdrawal cascade, not just a consent check at ingestion.
- Every decision needs to be **provable after the fact** — which rule
  allowed which use of which data, for which customer, at what time. That's
  an audit log, not a log line.

---

## 2. Part 1 — Consent & Eligibility Rules Engine

### 2.1 Rules engine choice

**[Grule](https://github.com/hyperjumptech/grule-rule-engine)** — pure Go,
embeds directly into astra-backend with no new infrastructure, rules are
readable GRL text stored and versioned in Postgres.

Considered and rejected for v1:
- **Hand-rolled if/else** — works until the first rule change, then you're
  redeploying the binary to adjust a compliance rule. Also gives no
  structured "which rule fired" trace for an audit.
- **OPA/Rego** — stronger policy tooling and widely used in regulated
  environments, but it's a separate service to run and operate. Revisit if
  the rule surface grows beyond consent/eligibility into general
  authorization — don't adopt it just for this.

### 2.2 Schema

```sql
-- Reference table: what purposes exist, and whether each needs explicit opt-in
CREATE TABLE consent_purposes (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code                   TEXT UNIQUE NOT NULL,   -- ACCOUNT_SERVICING, ANALYTICS_PROFILING, MARKETING_CROSS_SELL
    description            TEXT NOT NULL,
    lawful_basis           TEXT NOT NULL,          -- CONSENT | LEGITIMATE_USE (DPDP s.7)
    requires_explicit_optin BOOLEAN NOT NULL DEFAULT TRUE,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Versioned notice text — what the user actually saw when they consented.
-- DPDP requires you be able to reproduce this, not just that consent = true.
CREATE TABLE consent_notices (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    purpose_id  UUID NOT NULL REFERENCES consent_purposes(id),
    version     INT NOT NULL,
    body        TEXT NOT NULL,
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

-- Rule content — versioned and approved separately from code review.
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

-- The audit trail. Every CheckEligibility call writes one row here,
-- allow or deny, successful or not.
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

### 2.3 Service API

New package: `internal/service/consent`.

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

// CheckEligibility loads the user's active consents, evaluates them against
// the current dpdp_rules set via Grule, and logs the outcome to
// consent_decision_log regardless of result. Every caller that touches
// personal data for a non-account-servicing purpose must call this —
// not once at ingestion, but at every stage boundary, because consent can
// be withdrawn in the gap between scoring and lead creation.
func (s *Service) CheckEligibility(
	ctx context.Context,
	userID uuid.UUID,
	purpose Purpose,
	dataCategories []string,
) (Decision, error)

// RecordConsent captures a new grant or withdrawal, including the notice
// version shown and a hash of the exact consent artifact — the record a
// DPDP audit actually asks for.
func (s *Service) RecordConsent(
	ctx context.Context,
	userID uuid.UUID,
	purpose Purpose,
	action ConsentAction, // Grant | Withdraw
	notice NoticeRef,
	channel string,
) error

// Withdraw triggers the cascade described in §2.5.
func (s *Service) Withdraw(ctx context.Context, userID uuid.UUID, purpose Purpose) error
```

### 2.4 Rule catalog (v1)

| Category | Rule | Effect |
|---|---|---|
| Purpose match | Active, non-expired `MARKETING_CROSS_SELL` consent exists | Required to allow scoring at all |
| Data minimization | Only `category`, `amount_band`, `txn_frequency` fields may enter the feature vector | Narration/merchant strings never leave `spend_transactions` for this purpose, even with consent |
| Special-category exclusion | User flagged as minor, or marked deceased/dormant | Hard exclude — no rule can override this one |
| Cross-border | Any call in the scoring path that would route data outside India (e.g. a non-India Bedrock region) | Blocked — ties to the existing "Bedrock, VPC-only, in-region" requirement from the AWS provisioning plan |
| Retention | Lead not converted within 90 days | Auto-expire, purge derived scoring data |
| Withdrawal | Consent status flips to `WITHDRAWN` | Immediate stop on all downstream use — see §2.5 |

Rules are **additive deny, not additive allow** — a request is allowed only
if every applicable rule returns allow; any single deny wins. This keeps the
default posture conservative as new rules get added later.

### 2.5 Withdrawal cascade

Withdrawal is an event, not just a status flip:

1. `user_consents.status` → `WITHDRAWN`, `withdrawn_at` set.
2. Any `idbi_leads` row for that user tied to `MARKETING_CROSS_SELL` that
   hasn't converted gets marked `suppressed`, not deleted (DPDP requires
   you can show what happened and when, not erase the evidence of lawful
   handling).
3. If the lead was already pushed to IDBI's CRM via `428 createLead`,
   fire a retraction/status-update call using the stored `leadId` — don't
   leave a live lead in a third-party CRM after consent withdrawal.
4. Any cached/precomputed propensity score for that user is purged from
   wherever Part 2 stores it.

This needs to run as its own handler, not inline in the HTTP request that
records the withdrawal — treat it as a `safegoroutine`-wrapped async step so
a slow CRM retraction call doesn't block the user-facing "consent updated"
response.

### 2.6 Where the gate gets called

- Before the nightly/on-demand propensity scoring job touches
  `spend_transactions` for a given user → `CheckEligibility(..., PurposeMarketingCrossSell, ["category","amount_band","txn_frequency"])`
- Again immediately before `createLead` (428) fires, even if the score was
  computed hours earlier — consent may have changed in between.
- On every `/api/rm/bff/clients/{userID}/*` read that would expose a
  lead/propensity field to an RM — same reasoning, state can change between
  scoring and viewing.

---

## 3. Part 2 — Propensity Scoring → Lead Creation (next phase, builds on Part 1)

Not built yet — outlined here so Part 1's shape matches what Part 2 will
actually need to call.

### 3.1 Inputs

Only the fields Part 1's data-minimization rule allows: transaction
category distribution, amount bands, frequency patterns, existing
product holdings (MF/FD/stocks — already-owned, not inferred). No raw
narration, no merchant strings, no bureau data unless a separate rule
explicitly permits it.

### 3.2 Scoring

v1: a rules-based propensity score (e.g. "high recurring SIP + no FD
holding + stable salary credit" → FD cross-sell signal), not a trained
model — keeps the decision explainable, which matters for the same audit
reasons as the consent gate. A ML/SageMaker-backed version (per the AWS
infra plan) is a later swap-in behind the same interface, not a v1
requirement.

### 3.3 Lead creation

On a score crossing threshold:
1. `CheckEligibility` re-check (see §2.6).
2. Write to `idbi_leads` (dedup on `(user_id, product_category)` within a
   cooldown window — don't re-lead the same signal daily).
3. Call `428 createLead` per the IDBI integration plan, store the returned
   `leadId` against the local row.
4. Log the `consent_decision_log` reference alongside the lead row, so any
   lead can be traced back to the exact eligibility decision that permitted
   it.

---

## 4. Rollout phases

| Phase | Scope |
|---|---|
| **1 — Consent foundation** | Schema (§2.2), `consent` service + Grule integration, `CheckEligibility`/`RecordConsent` wired, no scoring yet |
| **2 — Consent UI** | Explicit `MARKETING_CROSS_SELL` opt-in flow — separate screen/notice, not a bundled checkbox (see open question below) |
| **3 — Withdrawal cascade** | §2.5, including the `428` retraction call |
| **4 — Propensity scoring v1** | Rules-based scorer (§3.2) behind the eligibility gate |
| **5 — Lead creation wiring** | `428` integration, dedup/cooldown, audit linkage |
| **6 — Compliance review** | Legal/DPO sign-off on `dpdp_rules` content before Phase 4/5 go live with real customer data |

### Open question before Phase 2 starts

Explicit opt-in flow (own screen, own notice) vs. bundled into onboarding as
one more checkbox — DPDP favors the former strongly (bundled consent for a
secondary purpose is a common point of challenge), but it's a product
decision that changes the UI scope, not just the backend.

---

## 5. Prior art & validation

Three tiers, most load-bearing first. The regulatory reference architecture
matters more here than any academic paper, because it's what an actual
DPDP Board audit will be checked against — the academic work explains *why*
the pattern is sound, the regulatory precedent proves *this exact shape* is
what India's own framework expects.

### 5.1 Regulatory reference architecture (India-specific, directly on point)

- **MeitY's Business Requirement Document for Consent Management under the
  DPDP Act, 2023** (June 2025) — the official reference architecture for a
  consent management system in India: lifecycle management (collect,
  validate, modify, renew, withdraw), a user-facing dashboard, and
  mandatory activity logging for auditability. §2.2–2.3 of this plan
  (`user_consents` lifecycle states, `consent_decision_log`) maps directly
  onto this, not coincidentally.
- **DPDP Rules, 2025** (notified 13 Nov 2025) — formalizes the Consent
  Manager role, with a **7-year audit-trail retention requirement**. This
  plan's `consent_decision_log` currently has no retention policy specified
  — add one now, set to at least 7 years, before this ships.
  [DPDP Rules 2025](https://www.dpdpa.com/dpdparules.html) ·
  [Draft rules analysis](https://www.freshfields.com/en/our-thinking/blogs/technology-quotient/draft-implementation-rules-issued-for-indian-digital-personal-data-protection-act)
- **India's Account Aggregator consent artifact framework** (ReBIT/Sahamati)
  — already live in production across the Indian banking system, and
  already part of this codebase's own AA integration (`590`–`593` in the
  IDBI plan). Its consent artifact follows the **ORGANS** principle — Open,
  Revocable, Granular, Auditable, Notice, Security-by-design — which is the
  same shape as §2.2's schema (purpose-scoped, explicit notice version,
  revocable, logged). Worth treating ORGANS as a checklist against the
  schema before Phase 1 is considered done.
  [Sahamati — ReBIT AA Client Standards](https://sahamati.org.in/rebit-aa-client-standards/)

### 5.2 Academic foundation

- **Byun & Li, "Purpose-based access control for privacy protection in
  relational database systems,"** *The VLDB Journal*, 2008 — the
  foundational paper for exactly this pattern: tagging data with intended
  purpose, enforcing access via purpose-conditional rules, and supporting
  explicit prohibitions (a privacy officer can mark certain data as
  unusable for certain purposes regardless of consent — the basis for the
  "special-category exclusion" rule in §2.4, which overrides even a valid
  consent).
  [VLDB Journal](https://link.springer.com/article/10.1007/s00778-006-0023-0) ·
  [Semantic Scholar](https://www.semanticscholar.org/paper/Purpose-based-access-control-for-privacy-protection-Byun-Li/3e7468c1fe7c776613e6fe604a35f134c266758b)
- **The SPECIAL-K Personal Data Processing Transparency and Compliance
  Platform** (EU H2020 SPECIAL project) — an engineered, peer-reviewed
  system that does what §2.3's `CheckEligibility` + `consent_decision_log`
  does: encode consent and purpose as machine-checkable policy, log every
  data-processing event against it, and automatically verify compliance at
  scale. Their published benchmark demonstrates this scales with event and
  user volume — direct evidence the architecture pattern (not just the
  concept) holds up under load.
  [arXiv:2001.09461](https://arxiv.org/pdf/2001.09461)
- **"A combined rule-based and machine learning approach for automated
  GDPR compliance checking"** (ACM, 2021) — relevant mainly for Part 2:
  validates pairing a rule-based compliance layer with a
  learned/statistical component (the propensity scorer), rather than
  treating compliance and scoring as one entangled system.
  [ACM DL](https://dl.acm.org/doi/abs/10.1145/3462757.3466081)

### 5.3 Industry precedent for the engineering choices

- **OneTrust's Consent Management Platform** — the dominant enterprise CMP,
  validates the core pattern (consent receipts in an audit-ready store,
  purpose enforcement, change history) at commercial scale. One deliberate
  divergence worth noting: OneTrust's *default* architecture stores consent
  browser-side (cookies/client scripts), with server-side as an opt-in
  configuration. This plan is server-side by design from the start —
  correctly, since a regulated financial entity needs the consent record to
  survive independently of any client, and the MeitY BRD's audit
  expectations assume durable server-side records, not a browser cookie.
  [OneTrust CMP](https://www.onetrust.com/products/consent-management/)
- **Grule in production financial decisioning** — used in practice for
  customer scoring, credit eligibility, and risk classification in
  financial services contexts, which is the same problem shape as this
  plan's eligibility gate and Part 2's propensity scoring. GoRules is a
  comparable commercial option if Grule's GRL syntax turns out to be too
  limited for compliance-team authoring later.
  [grule-rule-engine](https://github.com/hyperjumptech/grule-rule-engine) ·
  [GoRules — financial rules engine](https://gorules.io/industries/financial)

### 5.4 One correction this research surfaces

Add an explicit retention/purge policy to `consent_decision_log` and
`user_consents` — **7 years**, per the DPDP Rules' Consent Manager
retention requirement — rather than leaving it unbounded. This wasn't
specified in §2.2 and should be before migrations are written.
