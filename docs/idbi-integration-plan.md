# IDBI API Integration Plan

How the 25 IDBI/Atlas APIs get wired into `astra-backend` (and surfaced in the
app + RM portal). Read alongside:
- [`idbi-api-usage-map.md`](./idbi-api-usage-map.md) — which API goes where, build status
- [`idbi-api-payloads.md`](./idbi-api-payloads.md) — endpoints + request/response
- [`idbi-api-catalog.json`](./idbi-api-catalog.json) — the same, machine-readable
- [`idbi-portal-access.md`](./idbi-portal-access.md) — how to reach the IP-whitelisted gateway for dev/testing

---

## 1. Constraints

- **~15 working days** once the AWS account lands (≈ Sep 7–9). Deploy setup eats the first ~2.
- Gateway is **IP-allow-listed** to the EC2 (`43.205.52.148`). All dev calls go through the SOCKS proxy (`scripts/idbi-proxy.*`); the deployed backend runs inside the whitelisted network.
- Sandbox only for now: `https://sandboxpocgatewayprod.idbi.bank.in`, paths `POST /Development/<name>test`. Prod base URL, path prefix, and auth scheme are **still unknown** — see §9.
- `456` (dedupe) has **no spec**; treat as blocked until we get one.

## 2. Target architecture

```
                    ┌─────────────────────────────────────────┐
IDBI Atlas gateway  │  internal/provider/idbi   (one client)   │
  (24 endpoints) ───┤   - auth header, base URL, timeouts      │
                    │   - error-envelope -> typed errors       │
                    │   - one method per API, typed DTOs       │
                    └───────────────┬─────────────────────────┘
                                    │
        ┌───────────────────────────┼───────────────────────────┐
        │ sync workers              │ on-demand services        │ webhook receivers
        │ (scheduled, -> Postgres)  │ (call + short TTL cache)   │ (we host; AA calls us)
        │ 393 394 362 391 441 442   │ 365 408 433 473 538 415    │ 497 498
        │ 404 402                   │ 508                        │
        └───────────┬───────────────┴───────────┬───────────────┴──────┬────────┘
                    │                           │                      │
                    ▼                           ▼                      ▼
             ┌──────────────┐          ┌──────────────┐        (498 -> call 595 -> upsert)
             │  Postgres    │◄─────────┤ existing     │
             │ new tables + │          │ handlers /   │
             │ spend_txns   │          │ services     │
             └──────┬───────┘          └──────┬───────┘
                    │                         │
                    └──────────► App + RM portal read our API only
```

Rule: **the app and RM portal never call IDBI directly.** They call astra-backend,
which reads Postgres (synced data) or a cached on-demand result. The only live
outbound-in-request-path calls are the AA consent steps (590/592/593).

## 3. Foundation (do first)

### 3.1 `internal/provider/idbi` package
Mirror the existing `internal/provider/budget` (`budgetprovider.Client`) pattern:

- `Client{ baseURL, apiKey string; http *http.Client{Timeout: 30s} }`, `NewClient(baseURL, apiKey)`.
- `do(ctx, path, body, dst)` — sets auth header, `Content-Type: application/json`, marshals, unmarshals, maps the error envelope:
  - `{"errors":[{"code","title","description"}]}` → `*IDBIError{Code, Title, Description}`
  - HTTP ≥ 500 or transport failure → `ErrUnavailable` (callers degrade, don't 500)
  - HTTP 403 → `ErrForbidden` (usually IP / auth, surface loudly in logs)
- One method + request/response struct per API, names matching the catalogue (`GetFullAccountStatement`, `PerformAccountEnquiry`, …). Structs only carry the fields in `key_fields` from the catalogue — ignore the boilerplate branches.
- Package doc comment lists the endpoints, same as `budget/mlclient.go`.

### 3.2 Config
Add to `internal/config/config.go` (same `decryptOrFatal` treatment as `BUDGET_ML_TOKEN`):
- `IDBI_BASE_URL` (default the sandbox URL)
- `IDBI_API_KEY` (encrypted at rest via `MASTER_INTERNAL_KEY`)
- CIBIL bureau creds if separate: `IDBI_CIBIL_MEMBER_CODE`, `IDBI_CIBIL_MEMBER_PASSWORD`, `IDBI_CIBIL_DC_USER`, `IDBI_CIBIL_DC_PASSWORD` (408's request embeds these) — Secrets Manager entries, see the AWS questionnaire doc.
- `IDBI_SYNC_ENABLED` (bool) to gate the sync scheduler, like `BUDGET_ROLLOVER_SCHEDULER`.

### 3.3 Sync scheduler
Extend the in-process ticker in `cmd/api/main.go` (the `BUDGET_ROLLOVER_SCHEDULER` block) or add a sibling: on boot + every N hours, run the category-1 syncs for every user with a linked IDBI account. On AWS this can stay in-process for the hackathon; note as a follow-up to move to an ECS scheduled task / EventBridge cron.

### 3.4 Dev loop
`scripts/idbi-proxy.ps1` open → run integration calls with `HTTP_PROXY=socks5h://localhost:1080` (or a per-client `http.Transport` dialer) → capture responses as fixtures under `internal/provider/idbi/testdata/` for offline contract tests.

## 4. Phase 1 — wire the features that already exist

Highest demo value, lowest risk. These 3 back features that already work on mock data.

| API | Change |
|---|---|
| **393** getFullAccountStatement | New `IDBIStatementSource` implementing the same interface as `analyticsprovider.MockSource`; a sync worker pages 393 (`hasMoreData` loop) into `spend_transactions` for the user's IDBI `acid`. Swap `analyticsprovider.NewMockSource` in `main.go` for a source that reads the synced rows. Budget + `/analytics/spend` light up unchanged. |
| **365** performAccountEnquiry | On-demand + 5-min cache; feed the real available/ledger balance into `/dashboard/summary`. |
| **394** getCustomerAccountsByCustId | Sync into a new `idbi_accounts` table; new `/accounts` list endpoint (or fold into dashboard) showing Savings/Current/FD/Loan. |

**Check early:** does 393 return a usable `txnCat` per transaction? If yes, seed our
categoriser from it instead of inferring everything.

**New table:** `idbi_accounts(user_id, acid, acct_type, currency, balance_available, balance_ledger, status, synced_at)`.

## 5. Phase 2 — Account Aggregator chain

Finishes the `/aa` scaffold (`aa_handler.go` already has `/accounts`, `/consent`, `/accounts/{id}/transactions`).

1. **590** `POST /consent` → call `requestConsentFromFinPro`, store `consent_handle`.
2. **592** new step → `getWebRedirectionEncryptedURL(consentHandle, redirectUrl)`; return the URL to the client to open OneMoney.
3. **593** new callback endpoint → `generateDecryptedResponseFromFinPro({webRedirectionURL:{ecres,resdate,fi}})`; branch on `errorCode` (0 approved / 1 rejected).
4. **497** webhook receiver → update consent status.
5. **498** webhook receiver → on `DATA_READY`, enqueue a job that calls **595** with `sessionId` + `linkRefNumbers`, upsert transactions into `spend_transactions` tagged by source bank (`fipName`).
6. **591** new `GET /aa/consents` → "manage connected accounts" list + revoke.
7. **739** wire behind `/aa/accounts/{id}/transactions` for on-read fetch of a linked account.

**New tables:** `aa_consents(user_id, consent_id, consent_handle, status, vua, created_at, expiry)`,
`aa_linked_accounts(user_id, consent_id, link_ref_number, fip_id, fip_name, masked_acct, fi_type, status)`.

**Webhook endpoints** must be registered with OneMoney and reachable from their network — coordinate hosting path with ACC. Return `200` + empty body; make handlers idempotent on `transactionID`.

## 6. Phase 3 — new customer-facing read features (pick by value)

| Feature | APIs | Notes |
|---|---|---|
| Investible vs locked balance | **362** | Small. Sync liens per account; `usable = balance − Σ active liens`. Show reason (`reasonCode`). |
| "My Loans" | **391** (detail), **473** (EMI schedule), **433** (rate), **538** (foreclosure quote) | New feature area. `391` on sync; `473/433/538` on-demand. `441` (drawing power) only if a business-banking user. |
| Credit health | **408** | New screen. User-initiated only, consent-logged, cache ~30 days. Bureau creds server-side. Confirm where the actual score number is before promising it in UI (the sample only has `decision`). |

**New tables:** `idbi_liens(...)`, `idbi_loans(...)`, `idbi_loan_schedule(...)`, `cibil_reports(user_id, pulled_at, decision, raw jsonb, expires_at)`.

## 7. Phase 4 — RM portal risk & pipeline (as time allows)

RM console (`rm_handler.go`) already has clients / interactions / dashboards. Add:

| Module | APIs | Work |
|---|---|---|
| Early-warning / default risk | **404**, **402** | Batch across the book on sync; store `dpd`, `npaStatus`, overdue split in `idbi_risk_metrics`; surface on `/rm/clients/{id}` and a new dashboard tile. |
| MSME credit health | **441**, **442** | Sync drawing-power history + CIF exposure; compute utilisation, funded/non-funded mix, rating drift. |
| Lead pipeline | **428** | When an AI insight flags propensity → `createLead`; track `leadId` alongside the existing interaction model. |
| Prospect checks | **456**, **415** | Dedupe + CKYC on lead entry. `456` blocked until we get its spec; `415` fills the `/kyc` `notConfigured` stub (JSON body per the OpenAPI spec, not XML). |
| Staff routing | **508** | Resolve EIN → branch/region + reporting chain for lead assignment. Handle the `otpRequired` path. |

**New tables:** `idbi_risk_metrics(...)`, `idbi_credit_exposure(...)`, `idbi_leads(...)`.

## 8. Cross-cutting

- **Caching:** on-demand results in Postgres or the existing `ttlCache` pattern from `service/budget/service.go`. TTLs: 365 → 5 min, 408 → 30 days, 433/441/442 → 1 day, 508 → 1 day.
- **Retries:** exponential backoff on `ErrUnavailable`, max 3, jittered. Never retry a 4xx.
- **Idempotency:** webhook handlers dedupe on `transactionID`; `createLead` dedupe on our own generated `leadId`.
- **Secrets:** `IDBI_API_KEY` + CIBIL creds via AWS Secrets Manager (already listed in the AWS questionnaire response). Never in `.env` on a deployed box.
- **Rate limits:** unknown — see §9. Until known, cap sync concurrency low (e.g. 2 in-flight) and spread the nightly run.
- **Observability:** log every IDBI call with API id, latency, HTTP status, error code; alarm on 403 spikes (IP/auth) and `ErrUnavailable` rate (via the SNS topic from the AWS setup).
- **PII:** statement + CIBIL + CKYC payloads are heavy PII. Store only the `key_fields`; don't persist raw `formattedReport` / photo blobs. Reuse the existing `MASTER_INTERNAL_KEY` encryption for any at-rest sensitive columns.

## 9. Open questions / blockers

1. **Production gateway** — base URL, path prefix (drop `/Development/` + `test`?), and stage. Sandbox is all we have.
2. **Auth scheme** — API key header name? OAuth client-credentials? mTLS? Not in the specs.
3. **456 dedupe** — no OpenAPI spec. Request one from ACC or find it in the portal catalogue.
4. **408 CIBIL** — where is the numeric score? Sample response only has `decision` + `bureauResponse`. Confirm on a live sandbox call.
5. **Rate limits / quotas** per API on the sandbox and prod.
6. **Webhook hosting** — what URL do we give OneMoney for 497/498, and is inbound from their network allowed to our ALB?
7. **Which customer identifiers** we actually have to key these calls: `acid` / `cifId` / `custId` / `foracid` / mobile. Drives whether a user can be linked at all.
8. **Does the RM portal get its own gateway credentials / scope**, or share the app's?
9. **`415`** — is the gateway response JSON or XML? Spec shows a JSON request; response schema in the xlsx is XML-shaped.

## 10. Data model — what fills, what stays empty, what needs computing

### 10.1 The identity gap (blocks everything)

`users` today is `id, astra_user_id, name, phone_number` — **no CIF, PAN, DOB, or
account number.** Every IDBI call keys off one of: `acid`/`foracid` (365, 393,
441, 538, 362), `cifId` (394, 442), `custId` (391, 404, 402, 456),
`panCardNo`+`dateOfBirth` (408, 456, 415), `ein` (508, staff only).

**New table `idbi_customer_link`** — must exist before any call works:
`user_id, primary_acid, cif_id, pan, dob, linked_at, link_method`.
**Linking step:** user enters an account number → **365** returns `custId` →
**394** returns the CIF + every account → store. RM's `ClientProfile.PAN` is
already `*string` (nullable, empty today) — populate it from the same link.

### 10.2 `spend_transactions` — needs remodelling

Current columns: `amount, type(DEBIT/CREDIT), category, merchant, occurred_at`.
393 gives `transactionSummary.{txnAmt.amountValue, txnDate, txnDesc, txnType}`,
`txnCat`, `txnId`, `txnSrlNo`, `txnBalance`, `pstdDate`, `valueDate`.

| Column | Source | Work |
|---|---|---|
| `amount` | `txnAmt.amountValue` | **string → numeric.** Every IDBI amount is `{amountValue:"string", currencyCode}`, sometimes with a leading space. Assume INR. |
| `type` | `txnType` | **normalise** — values unknown (likely `D`/`C` or `DR`/`CR`) → `DEBIT`/`CREDIT`. |
| `category` | `txnCat` | **map, don't trust.** IDBI's taxonomy ≠ our 11 canonical budget categories. Extend the budget `CategoryIndex`/`canon` with an IDBI-cat alias set; fall back to our own categoriser when `txnCat` is blank/unknown. |
| `merchant` | — | **derived.** No merchant field — parse it out of `txnDesc` (narration). Add a `narration` column, keep `merchant` as best-effort computed. Mock data had clean merchant strings; real data won't. |
| `occurred_at` | `txnDate` / `pstdDate` | tolerant date parser (see §10.6). |
| *(new)* `txn_ref` | `txnId` | dedup key with `user_id`. |
| *(new)* `txn_srl_no` | `txnSrlNo` | pagination cursor + tiebreak. |
| *(new)* `balance_after` | `txnBalance.amountValue` | enables running-balance / min-balance analytics. |
| *(new)* `value_date` | `valueDate` | |
| *(new)* `source` | — | `idbi_393` / `aa_595` / `mock` — so mock and real can coexist during migration. |

The analytics engine's internal `Transaction` struct (`ID, Amount, Type,
Category, Merchant, OccurredAt`) doesn't change — it reads whatever the source
provides. Only the ingestion + the source implementation change.

### 10.3 Balances — one field vs five

`aa_bank_accounts.current_balance` is a single number. 365 and 393 return **5
balance types** (`available`, `ledger`, `fFD`, `floating`, `userDefined`).
Either keep `current_balance = availableBalance` and drop the rest, or add
`balance_ledger`, `balance_ffd`, `balance_floating`, `balance_userdefined`.
Recommend: store `available` + `ledger`, ignore the other three unless a feature
needs them.

### 10.4 Tables that don't exist yet (create per phase)

| Table | Fills from | Phase |
|---|---|---|
| `idbi_customer_link` | 365 + 394 | Foundation |
| `idbi_accounts` (acct list) | 394 | 1 |
| `aa_consents`, `aa_linked_accounts` | 590 / 591 / 498 | 2 |
| *(reuse `aa_transactions`)* | 595 / 739 | 2 — already close; add `value_date`, `fip_name` |
| `idbi_liens` | 362 | 3 |
| `idbi_loans`, `idbi_loan_schedule` | 391 / 473 | 3 |
| `cibil_reports` | 408 | 3 |
| `idbi_overdue` / `idbi_risk_metrics` | 404 / 402 | 4 |
| `idbi_credit_limits` (per-acct DP) | 441 | 4 |
| `idbi_credit_exposure` (CIF rollup) | 442 | 4 |
| `idbi_leads` | 428 | 4 |
| `ckyc_lookups` | 415 | 4 |
| `idbi_staff` (HRMS cache) | 508 | 4 |

`kyc_verifications` (from 000006) was built for a PAN-verify vendor — 415 CKYC
doesn't map onto it cleanly (`aadhaar_seeding_status`, `pan_status`,
`name_match` have no CKYC source). Use a separate `ckyc_lookups` table rather
than forcing it in.

### 10.5 Computed / derived — not a raw field from any API

| Value | Inputs | Note |
|---|---|---|
| `usable_balance` | 365 available − Σ active liens (362) | the "investible vs locked" headline (PS1) |
| `merchant` | 393 `txnDesc` | narration parsing / lookup table |
| canonical `category` | 393 `txnCat` + our categoriser | mapping layer |
| income / salary detection | existing analytics engine, now on real txns | already built — just runs on real data |
| `next_emi_date`, `emis_remaining`, `principal_paid_pct` | 473 `oamortLL[]` + today | schedule math |
| default-risk score (RM) | 404 overdue split + 402 `dpd`/`npaStatus` + 441 DP erosion + 442 utilisation/rating | **new model** |
| `dp_utilisation = outstanding / drawingPower`, `dp_erosion_pct` over time | 441 history | new |
| `total_utilisation = totalOutstanding / totalLimit`, `funded_nonfunded_ratio` | 442 | new |
| CIBIL numeric score | 408 `document` / `formattedReport` | **may need extraction** — not a direct field in the sample (§9.4) |
| amount normalisation | every `{amountValue, currencyCode}` | shared helper |
| date normalisation | IDBI's 3+ date formats | shared tolerant parser (§10.6) |

### 10.6 Format normalisation helpers (write once, use everywhere)

- **Amounts:** `{"amountValue":" 1234.50","currencyCode":"INR"}` → `float64` / `decimal`. Trim spaces; handle `""`; reject non-INR or convert.
- **Dates:** seen so far — `2025-05-01T00:00:00.000`, `2026-07-16`, `2027-01-17 09:47:07` (space, no `T`), literal `"date"` placeholders in schemas. One parser that tries each layout.
- **DR/CR:** map whatever `txnType` actually returns to `DEBIT`/`CREDIT`.
- **Keys with stray whitespace:** `" webRedirectionUrl"`, `"consent_handle: "` — trim on unmarshal (custom `UnmarshalJSON` or a post-process pass).

### 10.7 What IDBI does NOT fill — and how each gets filled instead

Every gap has a disposition; nothing is left "just empty" by accident.

| Field / table | Why IDBI won't fill it | How it gets filled |
|---|---|---|
| `mf_folios`, `mf_transactions` | no IDBI mutual-fund API | **existing MF provider** (already wired) — unchanged |
| stocks / equity holdings | not covered | **existing stocks provider** — unchanged |
| portfolio DNA / allocation snapshots | derived data, not a bank feed | **computed** from holdings — already implemented |
| `goals` | app-native concept | **user-entered** in-app — unchanged |
| FD product catalogue / rates | 394 returns FD *accounts*, not the catalogue | **existing FD provider** — unchanged |
| dashboard `MutualFunds` / `Stocks` / `FixedDeposits` buckets | not bank-balance data | **their providers**; only `BankBalance` switches to 365/393 |
| legacy `bank_accounts` (000001_init) | superseded | **leave empty** — `idbi_accounts` / `aa_bank_accounts` replace it; don't extend the old table |
| `spend_transactions.merchant` | 393 has no merchant field | **computed** — parse `txnDesc`; fall back to raw narration if the parser can't resolve one |
| `spend_transactions.category` | 393 `txnCat` taxonomy ≠ ours (or blank) | **computed** — map `txnCat` → canonical, else run our own categoriser |
| `usable_balance` / "locked" | no single field | **computed** — 365 available − Σ active liens (362) |
| CIBIL numeric score | sample response only has `decision` | **extract** from `document` / `formattedReport` if present (§9.4); if genuinely absent, **derive a band** from `decision` and label it as such — do not fabricate a precise number |
| `kyc_verifications.name_match` | CKYC gives a name, not a match verdict | **computed** — compare `ckycName` (415) against our stored name |
| `kyc_verifications.pan_status` | CKYC ≠ PAN verification | **needs a separate PAN-verify vendor**; for the hackathon demo, **mock** a "VALID" status and flag it as stubbed |
| `kyc_verifications.aadhaar_seeding_status` | not in CKYC | same — **separate vendor or mocked stub** for the demo |
| `spend_transactions` for users with **no linked IDBI account** (incl. the 4 seeded archetypes / demo logins) | they have no `acid` | **keep the mock `MockSource`** as a fallback; the `source` column lets mock and real rows coexist. Real linked user → IDBI data; demo user → mock data. |

**Rule of thumb:** investment data (MF / stocks / FD / goals) stays on the
providers it's already on; bank data (balances, statements, loans, liens,
credit) comes from IDBI; anything in between (merchant, category, usable
balance, risk score, name-match) is **computed** by us; the two KYC status
fields and (worst case) the CIBIL number are the only things that fall back to
a **mocked stub** for the demo, and each must be visibly labelled as such.
