# IDBI API Usage Map

**Source:** `docs/api_req_response_hackathon.xlsx` (25 APIs)
**Plan:** [`idbi-integration-plan.md`](./idbi-integration-plan.md) · **Payloads + real endpoints:** [`idbi-api-payloads.md`](./idbi-api-payloads.md) · **Machine-readable:** [`idbi-api-catalog.json`](./idbi-api-catalog.json) · **Raw specs:** `idbireposne/*.yaml` (31 OpenAPI exports)

Two questions this doc answers:
1. **Where** does each API belong — the customer **App**, the **RM portal**, both, or backend plumbing?
2. **What's the work** — is there already a feature to wire it into, or is it a new feature to build?

---

## 1. Build status at a glance

| Status | Meaning | APIs |
|---|---|---|
| 🟢 **wire** | Feature exists in astra today; just swap the mock/placeholder data for this API | **393, 365, 394** |
| 🟡 **extend** | Partial scaffold exists (a route/handler); finish the integration | **590, 739, 595, 415** |
| 🔴 **build** | Net-new — astra has nothing for this today | **362, 591, 592, 593, 497, 498, 408, 433, 473, 391, 538, 404, 402, 441, 442, 428, 456, 508** |

So **3 of 25** are a data-source swap. **4** finish existing scaffolds. The other **18 are new features** IDBI exposes that we don't have — the loans area, credit score, liens, MSME credit health, default/early-warning, the lead pipeline, dedupe, CKYC, HRMS.

---

## 2. Surface split

| Surface | APIs |
|---|---|
| **App** (customer) primary | 393, 365, 394, 362, 739, 595, 590, 591, 592, 593, 408, 433, 473, 538 |
| **RM portal** (staff) primary | 428, 442, 441, 404, 402, 456, 415, 508, 391 |
| **Both** (same call, App = own data, RM = across customers) | 393, 365, 394, 362, 739, 595, 408, 433, 473, 391, 538 |
| **Backend plumbing** (no UI) | 497, 498 |

---

## 3. By feature area

Legend: 🟢 wire · 🟡 extend · 🔴 build · fields = the useful bits of the response (full schema in the payloads doc).

### Statements & balances — App core, RM shares it

| API | Status | Feature / work | Key response fields |
|---|:--:|---|---|
| **393** getFullAccountStatementWithPagination | 🟢 | Spend analytics + budget run on `analyticsprovider.NewMockSource` today. **Work:** sync worker pages this into `spend_transactions`. | `transactionDetails[].transactionSummary.{txnAmt,txnDate,txnDesc,txnType}`, `.{txnCat,txnBalance,valueDate}`, 5 balances in `accountBalances`, `hasMoreData` |
| **365** performAccountEnquiry | 🟢 | `/dashboard/summary` exists; real balance not wired. **Work:** call on dashboard open, short TTL. | `acctBal[].{balType,balAmt}`, `bankAcctStatusCode`, `acctOpenDt` |
| **394** getCustomerAccountsByCustId | 🟢 | Investment portfolio view exists; bank accounts not listed. **Work:** sync into an `accounts` table; Savings/Current/FD/Loan list. | `customerAccountInfo[].{acctNumber,acctType,acctCurrCode,acctBalance}`, `numOfAccounts` |
| **362** accountLienEnquiry | 🔴 | **New:** "investible vs locked" balance — astra shows a flat balance only. | `bankInfo.lienDetails.{newLienAmt,reasonCode,remarks,isDeleted}`, `lienDate.{startDate,endDate}` |

### Account Aggregator (multi-bank) — App, with backend plumbing

| API | Status | Feature / work | Key response fields |
|---|:--:|---|---|
| **590** requestConsentFromFinPro | 🟡 | `/aa` has `POST /consent`. **Work:** point it at this API, store the handle. | `data[].consent_handle`, `data[].status` |
| **591** getConsentListFromFinPro | 🔴 | **New:** "Manage connected accounts" screen (list + revoke). | `data[].{consentID,status}`, `accounts[].{fipName,accountType,maskedAccountNumber,linkReferenceNumber}` |
| **592** getWebRedirectionEncryptedURL | 🔴 | **New:** redirect step — open the OneMoney webview. | `data[].webRedirectionUrl` |
| **593** generateDecryptedResponseFromFinPro | 🔴 | **New:** read approve/reject on return. | `data[].{status,errorCode,sessionId}` (errorCode 0=approved, 1=rejected) |
| **739** getAccountStatementFromFinPro | 🟡 | `/aa/accounts/{id}/transactions` exists (mock). **Work:** wire the real AA fetch. | `Summary.{currentBalance,accountType,drawingLimit}`, `Transactions.Transaction[].{amount,type,narration,transactionDateTime}`, `Profile.Holders.Holder.name` |
| **595** MoneyOne FIU – Get Account Statement | 🟡 | Ingestion-side twin of 739. **Work:** backend calls it after the 498 webhook; upsert into `spend_transactions`. | `data[].Summary.*`, `data[].Transactions.Transaction[].*`, `data[].Profile.Holders.Holder[].{name,pan}` |
| **497** pushConsentNotification | 🔴 | **New** hosted webhook: consent approved/rejected → update status, return 200 empty. | `consentHandle`, `consentId`, `eventType`, `eventStatus` |
| **498** pushDataNotification | 🔴 | **New** hosted webhook: data ready → backend calls 595. Return 200 empty. | `sessionId`, `consentId`, `linkRefNumbers[].{linkRefNumber,fipName,maskedAccountNumber}` |

### Loans — new area (App: "My Loans"; RM: credit view)

| API | Status | Feature / work | Key response fields |
|---|:--:|---|---|
| **391** getLoanAccountDetails | 🔴 | **New:** loan detail screen (App) + restructuring-history risk signal (RM). | `netIntRate.value`, `loanGenDetails.loanAmt`, `amtAlreadyDisb/amtAvailForDisb`, `loanGenDetails.reschedParams.*`, `pmtPlan.repmtRec` |
| **433** fetchLoanInterestRates | 🔴 | **New:** loan calculator / "check your rate". RM: quoting. | `baseIntRate.value`, `effctIntRate.value`, `penalIntRate.value`, `oslabRateLL[].{beginSlabAmt,endSlabAmt,normalIntPcnt}` |
| **473** generateLoanRepaymentSchedule | 🔴 | **New:** EMI / amortization table. | `oamortLL[].amortStruct.{flowDate,instlAmt,intAmt,princAmt,princOutStanding,cummIntAmt}` |
| **538** Inquire HP Payoff | 🔴 | **New:** foreclosure quote. | `executeFinacleScriptCustomData.{netPayofamt,pendingPrincipal,pendingNormalInterest,pendingPenalInterest,pendingOverdueInterest}` |

### Credit risk & MSME health — new, RM portal (PS3 / PS4)

| API | Status | Feature / work | Key response fields |
|---|:--:|---|---|
| **404** getLoanOverduePositionEnquiry | 🔴 | **New RM module:** default prediction — interest-vs-principal overdue split, batched across the book. App: EMI nudge only. | `loanOvduRec[].{totalIntDmd,totalIntColl,totalIntOvdu,pTotalDmd,pTotalColl,pTotalOvdu}`, `recCtrlOut.isLastSet` |
| **402** getLoanOverdueDetails | 🔴 | **New RM module:** DPD + NPA/SMA classification — primary model feed. | `overdueDetails[].{dpd,npaStatus,npaDate,outstandingBal,totalOverdueAmt}` |
| **441** fetchLoanAccountLimits | 🔴 | **New RM module:** drawing-power erosion vs sanction limit over time. | `acctDrwngPowerLimitHistMsgInq.olimitLL[].{drwngPower,drwngPowerPcnt,applicableDate}`, `acctSanctLimitHistMsg.olimitLL[].{sanctLimit,expiryDate}` |
| **442** Fetch Customer Limit Details | 🔴 | **New RM module:** CIF-level exposure, funded/non-funded mix, rating drift. | `{totalLimit,totalOutstanding,fundedLimit,nonFundedLimit}`, `limitHeaderDetails.custRating`, `ltCustomerDetails.accountManager` |
| **362** accountLienEnquiry | 🔴 | (also RM) new liens often precede default — stress signal. | see Statements & balances |

### Credit score — new, App + RM

| API | Status | Feature / work | Key response fields |
|---|:--:|---|---|
| **408** fetch CIBIL Score | 🔴 | **New:** "check credit health" screen. Bureau rules: user/RM-initiated, consent-logged, cache ~30 days — never a background pull. | `body.dcResponse.decision`, `bureauResponse.{status,isSuccess}`, `applicant.name`, `header.{statusCode,statusMessage}` |

> ⚠️ The sample response has **no raw numeric score field** — you get `decision` + `bureauResponse`. The score, if returned, is inside `document` (the `formattedReport`). Verify on a live call before promising a number in the UI.

### Lead pipeline & onboarding — new, RM portal (PS2)

| API | Status | Feature / work | Key response fields |
|---|:--:|---|---|
| **428** createLead | 🔴 | RM has `/clients/{id}/interactions` (follow-ups) but not CRM lead creation. **New:** AI insight → POST a lead, track `leadId`. | `result.leadId`, `result.errors` |
| **456** performCustomerMasterDedupeCheck | 🔴 | **New:** on lead entry, search PAN/mobile/GSTIN → existing relationship. | `allMasterRecords[].{customerCount,custId,custName,dateOfBirth,panGirNum,ckycNo,customerType,kycDueDate}` |
| **415** searchCkycDetails | 🟡 | `/kyc` handler exists but `POST /pan/verify` is a `notConfigured` stub. **Work:** implement — gateway takes **JSON** (`input.searchInCkycRequestDetails[]`), by ID or name+DOB, batch-capable. | `searchInCkycResponseDetails.{ckycAvailable,ckycId,ckycName,ckycAccType}`, `ckycIdDetails.id[].{ckycAvailableIdType,ckycAvailableIdTypeStatus}` |

### Staff — new, RM portal

| API | Status | Feature / work | Key response fields |
|---|:--:|---|---|
| **508** Fetch HRMS Employee Details | 🔴 | RM auth exists (OTP) but no HRMS lookup. **New:** EIN → name/grade/branch/region + reporting chain for lead routing. | `getHRMSEmployeeDetails.{ein,fullNameTitle,grade,position,location,region,email}`, `.{supEin,supFullNameTitle,supEmail}` |

> `otpRequired` in the request / `otpId` in the response — this call can be OTP-gated.

---

## 4. How each API is called (integration pattern)

**IDBI APIs → sync/ingestion layer → our Postgres → App & RM read our DB.** The App almost never calls IDBI in the request path; only the consent flow is live.

| Pattern | APIs | Notes |
|---|---|---|
| **Scheduled sync** → Postgres, UI reads DB | 393, 394, 362, 391, 441, 442, 404, 402 | Nightly or on pull-to-refresh. Needs a worker — same shape as the `BUDGET_ROLLOVER_SCHEDULER` ticker in `main.go`; on AWS an ECS scheduled task / EventBridge cron. |
| **On-demand + TTL cache** | 365 (5 min), 408 (~30 days), 433, 456, 415, 538, 508 | 408 CIBIL: bureau rules forbid speculative/bulk pulls — must be user/RM-triggered and consent-logged. |
| **Interactive** (live path) | 590, 592, 593, 739, 591 | The AA linking flow, driven by the user tapping through OneMoney. |
| **Event-driven** | 428, 595 | 428 fired by an AI-insight worker; 595 fired by the 498 webhook. |
| **Inbound webhook** (we host) | 497, 498 | Return HTTP 200 with an empty body; non-2xx makes AA retry. |

---

## 5. Endpoints & gotchas

**Gateway** (from the OpenAPI specs in `idbireposne/`):
- Sandbox base URL: `https://sandboxpocgatewayprod.idbi.bank.in` — `innobox.idbi.bank.in` is only the doc portal.
- Every API is `POST /Development/<name>test` — **dev/sandbox stage**. Production path prefix and the `test` suffix will differ; get prod paths from the portal.
- **24 of 25 have a spec**; **456** (dedupe) has none — endpoint unknown, only the xlsx skeleton.
- Error envelope is `{ "errors": [ { "code", "title", "description" } ] }` — `title`/`description`, not `type`/`message`.
- Full endpoint + real sample request per API in [`idbi-api-payloads.md`](./idbi-api-payloads.md); structured in [`idbi-api-catalog.json`](./idbi-api-catalog.json) under each API's `endpoint`.

### Notes

- **393 vs 739/595:** 393 is IDBI-internal (their own CASA account, no consent). 739/595 are Account-Aggregator (any consented bank). Build on 393 first; the AA chain only matters once "link other banks" exists.
- **393 `txnCat`:** if IDBI already returns a spend category per transaction, it seeds our categoriser instead of inferring everything. Check early.
- **404 vs 402:** overlapping. 404 = demanded/collected/overdue split across all loans; 402 adds `dpd` + `npaStatus`. If only one, take 402.
- **441 vs 442:** 441 = per-account drawing power; 442 = CIF-level rollup with rating + account manager. 442 for the portfolio view, 441 to drill in.
- **Spec vs xlsx discrepancies** (spec wins — see payloads doc): **415** request is **JSON** at the gateway, not XML. **433** and **441** have no `input` wrapper (flat body). **593** wraps args in `webRedirectionURL`. **428** keys are clean (`mobileNo`, no trailing space). **404** nests `loanOvduPosInqCustomData` inside `input`. **591** request also carries `vua`. **394** takes a top-level `txn: "E"`.
- **Source-data quirks** (kept verbatim in the payloads doc): `"consent_handle: "` (590/591) and `" webRedirectionUrl"` / `" status"` (592/593) have stray spaces/colons in keys — trim on parse. 739's response JSON in the sheet is missing commas. 538's `netPayofamt` is spelled with one `f`. 408 has no numeric score field in the sample.
- **Not covered here:** payments, transfers, trading execution — those stay on the existing astra providers. These 25 are read-only bank data plus lead creation.
