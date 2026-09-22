# IDBI API Endpoint Map

> **Base URL (Sandbox):** `https://sandboxpocgatewayprod.idbi.bank.in`  
> **Auth:** None at app layer — gateway allow-lists egress IP `43.205.52.148`  
> **Method:** All endpoints are `POST`  
> **Error envelope:** `{ "errors": [{ "code": "", "title": "", "description": "" }] }`

---

## Legend

| Symbol | Meaning |
|--------|---------|
| ✅ `wired` | Feature exists in astra; swap mock for this API |
| 🔧 `extend` | Partial scaffold exists; finish integration |
| 🆕 `build` | Net-new feature |
| 💀 `mock` | Sandbox dead; must mock |
| 🏦 `idbi` | Real data from sandbox |
| 🎭 `simulated` | Call succeeds but no live counterparty |
| 📥 `webhook-in` | We host the endpoint; AA calls us |

---

## Account APIs

### 1. getCustomerAccountsByCustId `#394`
| Field | Value |
|-------|-------|
| **Path** | `POST /Development/getCustomerAccountsByCustIdtest` |
| **Surface** | `both` (app + RM) |
| **Status** | ✅ wired |
| **Call Pattern** | `sync` |
| **Data Source** | 🏦 idbi |
| **Feature** | Consolidated IDBI holdings — savings / current / FD / loan list |
| **Wired As** | `idbiaccounts.Refresh` → `GET /api/v1/idbi/accounts` |

**Key Response Fields**
```json
{
  "customerAccountInfo": [
    {
      "acctNumber": "...",
      "acctType": "SAVINGS | CURRENT | TERM_DEPOSIT | SALARY",
      "acctCurrCode": "INR",
      "acctBalance": { "amountValue": 12345.00 }
    }
  ],
  "numOfAccounts": 4
}
```

---

### 2. performAccountEnquiry `#365`
| Field | Value |
|-------|-------|
| **Path** | `POST /Development/performAccountEnquirytest` |
| **Surface** | `both` |
| **Status** | ✅ wired |
| **Call Pattern** | `on-demand-cache` |
| **Data Source** | 🏦 idbi |
| **Feature** | Balance on home dashboard |
| **Wired As** | `idbiaccounts.Refresh` → `GET /api/v1/idbi/accounts` |

**Key Response Fields**
```json
{
  "acctBal": [
    { "balType": "...", "balAmt": { "amountValue": 50000.00 } }
  ],
  "bankAcctStatusCode": "A",
  "acctOpenDt": "2020-01-15",
  "custId": "...",
  "personName": "..."
}
```
> ⚠️ `acctBal[]` has 7 balType rows — select by `balType`, don't index.

---

### 3. accountLienEnquiry `#362`
| Field | Value |
|-------|-------|
| **Path** | `POST /Development/accountLienEnquirytest` |
| **Surface** | `both` |
| **Status** | ✅ wired |
| **Call Pattern** | `sync` |
| **Data Source** | 🏦 idbi |
| **Feature** | Investible-vs-locked balance |
| **Wired As** | `idbiaccounts.AccountLien` → `GET /api/v1/idbi/accounts/{n}/lien` |

**Key Response Fields**
```json
{
  "result": {
    "bankInfo": {
      "lienDetails": [
        {
          "lienId": "...",
          "newLienAmt": 10000.00,
          "reasonCode": "...",
          "remarks": "...",
          "isDeleted": "N",
          "lienDate": { "startDate": "2026-01-01", "endDate": "2026-12-31" }
        }
      ]
    }
  },
  "errors": []
}
```
> ⚠️ `lienDetails` nested inside `result.bankInfo`. Usable balance = total balance − sum(active lienAmt).

---

## Statement & Transaction APIs

### 4. getFullAccountStatementWithPagination `#393`
| Field | Value |
|-------|-------|
| **Path** | `POST /Development/getFullAccountStatementWithPaginationtest` |
| **Surface** | `both` |
| **Status** | ✅ wired |
| **Call Pattern** | `sync` |
| **Data Source** | 🏦 idbi |
| **Feature** | Spend analytics + budget diagnosis |
| **Wired As** | `statementsync → spend_transactions` → `POST /api/v1/idbi/spend/refresh` |

**Key Response Fields**
```json
{
  "result": {
    "transactionDetails": [
      {
        "transactionSummary": {
          "txnAmt": 500.00,
          "txnDate": "2026-09-01",
          "txnDesc": "UPI Transfer",
          "txnType": "D"
        },
        "txnCat": "FOOD",
        "txnBalance": 49500.00,
        "pstdDate": "2026-09-01",
        "valueDate": "2026-09-01"
      }
    ],
    "accountBalances": {
      "availableBalance": 49500.00,
      "ledgerBalance": 49500.00,
      "fFDBalance": 0.00,
      "floatingBalance": 0.00,
      "userDefinedBalance": 0.00
    },
    "hasMoreData": "Y"
  }
}
```
> ⚠️ Per-txn fields nested under `transactionSummary`. Page while `hasMoreData == "Y"`. `txnType`: `D` = Debit, `C` = Credit.

---

### 5. getAccountStatementFromFinPro `#739`
| Field | Value |
|-------|-------|
| **Path** | `POST /Development/getAccountStatementFromFinProtest` |
| **Surface** | `both` |
| **Status** | ✅ wired |
| **Call Pattern** | `interactive` |
| **Data Source** | 🏦 idbi |
| **Feature** | Multi-bank statements via Account Aggregator |
| **Wired As** | `idbiaa.FetchStatements` when `IDBI_AA_STATEMENT_SOURCE=finpro` |

**Key Response Fields**
```json
{
  "data": [
    {
      "Profile": {
        "Holders": {
          "Holder": [{ "name": "John Doe", "dob": "1990-01-01", "pan": "XXXXX9999X", "mobile": "9999999999" }]
        }
      },
      "Summary": {
        "currentBalance": 50000.00,
        "accountType": "SAVINGS",
        "drawingLimit": 0,
        "ifsc": "IDBI0001234"
      },
      "Transactions": {
        "Transaction": [
          {
            "amount": 500.00,
            "type": "DEBIT",
            "narration": "UPI",
            "transactionDateTime": "2026-09-01T10:00:00",
            "balance": 49500.00,
            "mode": "UPI",
            "reference": "ref123"
          }
        ]
      },
      "pageDetails": { "pageNumber": 1, "totalPages": 3 }
    }
  ]
}
```

---

### 6. MoneyOneFIU_GetAccountStatement `#595`
| Field | Value |
|-------|-------|
| **Path** | `POST /Development/getAccountStatementtest` |
| **Surface** | `both` |
| **Status** | ✅ wired |
| **Call Pattern** | `event` (fired after data-ready webhook #498) |
| **Data Source** | 🏦 idbi |
| **Feature** | AA statement ingestion side |
| **Wired As** | `idbiaa.FetchStatements → spend_transactions` source=`aa` |

**Key Response Fields**
```json
{
  "data": [
    {
      "Summary": {
        "currentBalance": 50000.00,
        "accountType": "SAVINGS",
        "drawingLimit": 0,
        "currentODLimit": 0
      },
      "Transactions": {
        "Transaction": [
          {
            "amount": 200.00,
            "type": "CREDIT",
            "narration": "SALARY",
            "valueDate": "2026-09-01",
            "transactionTimestamp": "2026-09-01T09:00:00"
          }
        ]
      },
      "Profile": {
        "Holders": {
          "Holder": [{ "name": "John Doe", "pan": "XXXXX9999X" }]
        }
      }
    }
  ],
  "pageDetails": { "pageNumber": 1, "totalPages": 2 }
}
```
> ⚠️ Some samples use `transactionalBalance` instead of `currentBalance` — accept both.

---

## Account Aggregator (AA) Flow APIs

### 7. requestConsentFromFinPro `#590`
| Field | Value |
|-------|-------|
| **Path** | `POST /Development/requestConsentFromFinProtest` |
| **Surface** | `app` |
| **Status** | ✅ wired |
| **Call Pattern** | `interactive` |
| **Data Source** | 🎭 simulated |
| **Feature** | "Link other bank accounts" — create AA consent |
| **Wired As** | `idbiaa.RequestConsent` → `POST /api/v1/aa/consent` |

**Key Response Fields**
```json
{
  "data": [
    {
      "consent_handle": "ch-abc-123",
      "status": "PENDING"
    }
  ]
}
```
> ⚠️ Sandbox returns a fixed/canned handle — no live counterparty.

---

### 8. getConsentListFromFinPro `#591`
| Field | Value |
|-------|-------|
| **Path** | `POST /Development/getConsentListFromFinProtest` |
| **Surface** | `both` |
| **Status** | ✅ wired |
| **Call Pattern** | `on-demand-cache` |
| **Data Source** | 🏦 idbi |
| **Feature** | "Manage connected accounts" — list active/expired consents |
| **Wired As** | `idbiaa.Refresh` → `POST /api/v1/aa/consents/{h}/refresh` |

**Key Response Fields**
```json
{
  "data": [
    {
      "consentID": "consent-xyz",
      "status": "ACTIVE",
      "accounts": [
        {
          "fipName": "HDFC Bank",
          "fipId": "HDFC-FIP",
          "accountType": "SAVINGS",
          "maskedAccountNumber": "XXXXXXXX1234",
          "linkReferenceNumber": "ref-001",
          "fiType": "DEPOSIT"
        }
      ]
    }
  ]
}
```

---

### 9. getWebRedirectionEncryptedURL `#592`
| Field | Value |
|-------|-------|
| **Path** | `POST /Development/getWebRedirectionEncryptedURLtest` |
| **Surface** | `app` |
| **Status** | ✅ wired |
| **Call Pattern** | `interactive` |
| **Data Source** | 🎭 simulated |
| **Feature** | AA redirect — exchange consent handle for webview URL |
| **Wired As** | `idbiaa.RequestConsent` (live mode) |

**Key Response Fields**
```json
{
  "data": [
    { "webRedirectionUrl": "https://webrd.onemoney.in/uat?token=..." }
  ]
}
```
> ⚠️ Sandbox URL is dummy and renders nothing. Gate behind `IDBI_AA_REDIRECT_MODE=stub|live`.

---

### 10. generateDecryptedResponseFromFinPro `#593`
| Field | Value |
|-------|-------|
| **Path** | `POST /Development/generateDecryptedResponseFromFinProtest` |
| **Surface** | `app` |
| **Status** | ✅ wired |
| **Call Pattern** | `interactive` |
| **Data Source** | 🎭 simulated |
| **Feature** | Decrypt AA redirect callback — read approve/reject result |
| **Wired As** | `idbiaa.CompleteRedirect` → `POST /api/v1/aa/consents/callback` |

**Request (spec shape)**
```json
{
  "args": {
    "webRedirectionURL": {
      "ecres": "<long-encrypted-blob-from-redirect>",
      "resdate": "2026-09-01",
      "fi": "IDBI"
    }
  }
}
```

**Key Response Fields**
```json
{
  "data": [
    {
      "status": "S",
      "errorcode": "0",
      "sessionid": "sess-abc",
      "txnid": "txn-xyz",
      "userid": "user-001"
    }
  ]
}
```
> ⚠️ `errorcode == "0"` means approved. Every key inside `data[0]` has leading spaces in source — trim.

---

### 11. pushConsentNotification `#497` — Inbound Webhook
| Field | Value |
|-------|-------|
| **Path** | `POST /webhooks/idbi-aa/consent-notification` *(we host this)* |
| **Surface** | `plumbing` |
| **Status** | ✅ wired |
| **Call Pattern** | `webhook-in` |
| **Data Source** | 🎭 simulated |
| **Feature** | AA calls us when consent is approved/rejected |
| **Wired As** | `aa_handler webhook POST /webhooks/idbi-aa/consent-notification` |

**Inbound Payload (AA → us)**
```json
{
  "consentHandle": "ch-abc-123",
  "consentId": "consent-xyz",
  "eventType": "CONSENT",
  "eventStatus": "ACTIVE"
}
```

**Our Response:** `HTTP 200 (empty body)`

> ⚠️ Non-2xx triggers AA retry.

---

### 12. pushDataNotification `#498` — Inbound Webhook
| Field | Value |
|-------|-------|
| **Path** | `POST /webhooks/idbi-aa/data-notification` *(we host this)* |
| **Surface** | `plumbing` |
| **Status** | ✅ wired |
| **Call Pattern** | `webhook-in` |
| **Data Source** | 🎭 simulated |
| **Feature** | AA calls us when financial data is ready to fetch |
| **Wired As** | `aa_handler webhook POST /webhooks/idbi-aa/data-notification` |

**Inbound Payload (AA → us)**
```json
{
  "sessionId": "sess-abc",
  "consentId": "consent-xyz",
  "linkRefNumbers": [
    {
      "linkRefNumber": "ref-001",
      "fipName": "HDFC Bank",
      "maskedAccountNumber": "XXXXXXXX1234",
      "fiStatus": "READY"
    }
  ]
}
```

**Our Response:** `HTTP 200 (empty body)`

> On receipt — triggers call to API #595 to pull statements.

---

## Loan APIs

### 13. getLoanAccountDetails `#391`
| Field | Value |
|-------|-------|
| **Path** | `POST /Development/getLoanAccountDetailstest` |
| **Surface** | `both` |
| **Status** | ✅ wired |
| **Call Pattern** | `sync` |
| **Data Source** | 🏦 idbi |
| **Feature** | "My Loans" detail screen + RM credit-risk view |
| **Wired As** | `idbiloans.Refresh` master enrich → `GET /api/v1/idbi/loans` |

**Key Response Fields**
```json
{
  "result": {
    "netIntRate": { "value": 8.5 },
    "loanGenDetails": {
      "loanAmt": 500000.00,
      "loanPeriodMonths": 60,
      "rePmtMethod": "EMI",
      "reschedParams": {},
      "pmtPlan": { "repmtRec": [] }
    },
    "amtAlreadyDisb": 500000.00,
    "amtAvailForDisb": 0.00,
    "disbAmt": 500000.00
  }
}
```

---

### 14. generateLoanRepaymentSchedule `#473`
| Field | Value |
|-------|-------|
| **Path** | `POST /Development/generateLoanRepaymentScheduletest` |
| **Surface** | `both` |
| **Status** | ✅ wired |
| **Call Pattern** | `on-demand-cache` |
| **Data Source** | 🏦 idbi |
| **Feature** | EMI schedule / amortization visualizer |
| **Wired As** | `idbiloans.RepaymentSchedule` → `GET /api/v1/idbi/loans/{id}/schedule` |

**Key Response Fields**
```json
{
  "result": {
    "loanModellingSchOutputVO": {
      "lamodRepaymentLL": [{ "...": "EMI summary" }],
      "oamortLL": [
        {
          "amortStruct": {
            "flowDate": "2026-10-01",
            "instlAmt": 10000.00,
            "intAmt": 3541.67,
            "princAmt": 6458.33,
            "princOutStanding": 493541.67,
            "cummIntAmt": 3541.67,
            "cummPrincAmt": 6458.33
          }
        }
      ]
    }
  }
}
```

---

### 15. inquireHPPayoff `#538`
| Field | Value |
|-------|-------|
| **Path** | `POST /Development/InquireHPAyofftest` *(IDBI gateway typo)* |
| **Surface** | `both` |
| **Status** | ✅ wired |
| **Call Pattern** | `on-demand-cache` |
| **Data Source** | 🏦 idbi |
| **Feature** | "Foreclose my loan" — payoff quote |
| **Wired As** | `idbiloans.PayoffQuote` → `GET /api/v1/idbi/loans/{id}/payoff` |

**Key Response Fields**
```json
{
  "executeFinacleScriptCustomData": {
    "netPayofamt": 450000.00,
    "pendingPrincipal": 430000.00,
    "pendingNormalInterest": 15000.00,
    "pendingPenalInterest": 0.00,
    "pendingOverdueInterest": 5000.00,
    "interestRate": 8.5
  }
}
```
> ⚠️ `netPayofamt` — one 'f' — IDBI typo. No errors array in response.

---

### 16. getLoanOverduePositionEnquiry `#404`
| Field | Value |
|-------|-------|
| **Path** | `POST /Development/getLoanOverduePositionEnquirytest` |
| **Surface** | `rm` |
| **Status** | ✅ wired |
| **Call Pattern** | `sync` |
| **Data Source** | 🏦 idbi |
| **Feature** | Default-prediction / Early Warning — RM portal |
| **Wired As** | `idbiloans.OverduePosition` → `GET /api/v1/idbi/loans/{id}/overdue-position` |

**Key Response Fields**
```json
{
  "result": {
    "loanOvduRec": [
      {
        "totalIntDmd": 5000.00,
        "totalIntColl": 3000.00,
        "totalIntOvdu": 2000.00,
        "pTotalDmd": 10000.00,
        "pTotalColl": 8000.00,
        "pTotalOvdu": 2000.00
      }
    ],
    "recCtrlOut": { "isLastSet": "Y", "setNum": 1 }
  }
}
```
> ⚠️ Paginated. First page: send `recCtrlIn.maxRec` and `recCtrlIn.setNum` as empty strings.

---

### 17. getLoanOverdueDetails `#402`
| Field | Value |
|-------|-------|
| **Path** | `POST /Development/getLoanOverdueDetailstest` |
| **Surface** | `rm` |
| **Status** | ✅ wired |
| **Call Pattern** | `sync` |
| **Data Source** | 🏦 idbi |
| **Feature** | NPA / DPD tracking — RM portal |
| **Wired As** | `idbiloans.Refresh` + `rmcreditrisk.Refresh` |

**Key Response Fields**
```json
{
  "result": {
    "overdueDetails": [
      {
        "accountId": "ACC001",
        "dpd": 30,
        "npaStatus": "SMA-1",
        "npaDate": "NULL",
        "outstandingBal": 490000.00,
        "totalOverdueAmt": 10000.00,
        "overdueDate": "2026-08-01"
      }
    ]
  }
}
```
> ⚠️ `npaStatus`: `SA` (standard), `SMA-0/1/2`, `NPA`. Empty dates are literal string `"NULL"`.

---

### 18. fetchLoanAccountLimits `#441`
| Field | Value |
|-------|-------|
| **Path** | `POST /Development/fetchLoanAccountLimitstest` |
| **Surface** | `rm` |
| **Status** | ✅ wired |
| **Call Pattern** | `sync` |
| **Data Source** | 🏦 idbi |
| **Feature** | MSME working-capital health / drawing-power erosion |
| **Wired As** | `idbiloans.LoanLimits` → `GET /api/v1/idbi/loans/{id}/limits` |

**Key Response Fields**
```json
{
  "result": {
    "accountLimitDetails": {
      "acctDrwngPowerLimitHistMsgInq": {
        "olimitLL": [
          { "drwngPower": 800000.00, "drwngPowerPcnt": 80.0, "applicableDate": "2026-09-01" }
        ]
      },
      "acctSanctLimitHistMsg": {
        "olimitLL": [
          { "sanctLimit": 1000000.00, "expiryDate": "2027-01-01", "applicableDate": "2026-01-01" }
        ]
      }
    }
  }
}
```
> Sort both arrays by `applicableDate` to get current drawing power vs. sanction limit.

---

### 19. fetchLoanInterestRates `#433` — Dead in Sandbox
| Field | Value |
|-------|-------|
| **Path** | `POST /Development/fetchLoanInterestRatestest` |
| **Surface** | `both` |
| **Status** | 💀 mock |
| **Call Pattern** | `on-demand-cache` |
| **Data Source** | 🎭 mock (broken in sandbox) |
| **Feature** | Loan calculator / "check your rate" |
| **Wired As** | Dead in sandbox — falls back to mock / `ErrNotAvailable` |

**Expected Response Fields (when live)**
```json
{
  "result": {
    "baseIntRate": { "value": 7.5 },
    "effctIntRate": { "value": 8.0 },
    "penalIntRate": { "value": 2.0 },
    "oslabRateLL": [
      { "beginSlabAmt": 0, "endSlabAmt": 500000, "normalIntPcnt": 8.0 }
    ]
  }
}
```
> ⚠️ Sandbox returns an unrelated object — mock or hardcode until fixed by ACC.

---

## Credit / Bureau APIs

### 20. fetchCibilScore `#408` — Dead in Sandbox
| Field | Value |
|-------|-------|
| **Path** | `POST /Development/fetchCibilScoretest` |
| **Surface** | `both` |
| **Status** | 💀 mock |
| **Call Pattern** | `on-demand-cache` |
| **Data Source** | 🎭 mock (bureau credentials invalid) |
| **Feature** | "Check credit health" — credit score |
| **Wired As** | Dead in sandbox — falls back to mock / `ErrNotAvailable` |

**Expected Response Fields (when live)**
```json
{
  "header": { "statusCode": "0", "statusMessage": "SUCCESS" },
  "body": {
    "dcResponse": {
      "decision": "ACCEPT",
      "applicant": { "name": "John Doe" },
      "bureauResponse": { "status": "success", "isSuccess": true }
    }
  }
}
```
> ⚠️ No raw numeric score field — you get `decision` + `bureauResponse`. Score (if any) is inside `document.formattedReport`.

---

## Customer / KYC APIs

### 21. searchCkycDetails `#415`
| Field | Value |
|-------|-------|
| **Path** | `POST /Development/searchCkycDetailstest` |
| **Surface** | `rm` |
| **Status** | ✅ wired |
| **Call Pattern** | `on-demand-cache` |
| **Data Source** | 🏦 idbi |
| **Feature** | CKYC verification |
| **Wired As** | `idbikyc.VerifyPAN` → `POST /api/v1/kyc/pan/verify` |

**Key Response Fields**
```json
{
  "result": {
    "details": {
      "searchInCkycResponseDetails": {
        "ckycAvailable": "Yes",
        "ckycId": "12345678",
        "ckycName": "JOHN DOE",
        "ckycAccType": "NORMAL",
        "ckycFatherName": "JAMES DOE",
        "ckycGenDate": "2020-01-01",
        "ckycIdDetails": {
          "id": [
            { "ckycAvailableIdType": "PAN", "ckycAvailableIdTypeStatus": "VERIFIED" }
          ]
        }
      }
    }
  }
}
```
> ⚠️ Response includes `ckycPhoto` / `ckycPhotoBytes` (base64 image blob) — ignore these.

---

### 22. performCustomerMasterDedupeCheck `#456` — No Spec
| Field | Value |
|-------|-------|
| **Path** | `POST` — **UNKNOWN** (no OpenAPI spec available) |
| **Surface** | `rm` |
| **Status** | 💀 mock |
| **Call Pattern** | `on-demand-cache` |
| **Data Source** | 🎭 mock (no spec / endpoint unknown) |
| **Feature** | Prospecting / duplicate-CIF check by PAN/mobile/GSTIN |
| **Wired As** | Dead in sandbox — falls back to mock / `ErrNotAvailable` |

**Expected Response Fields (when spec received)**
```json
{
  "result": {
    "allMasterRecords": [
      {
        "customerCount": 1,
        "custId": "CIF001",
        "custName": "John Doe",
        "dateOfBirth": "1990-01-01",
        "panGirNum": "XXXXX9999X",
        "ckycNo": "12345678",
        "customerType": "INDIVIDUAL",
        "kycDueDate": "2027-01-01"
      }
    ]
  }
}
```
> ⚠️ `customerCount` on record 0 indicates match / no-match / multi-match. Request spec from ACC/IDBI portal.

---

## RM / CRM APIs

### 23. createLead `#428`
| Field | Value |
|-------|-------|
| **Path** | `POST /Development/createLeadtest` |
| **Surface** | `rm` |
| **Status** | ✅ wired |
| **Call Pattern** | `event` |
| **Data Source** | 🏦 idbi |
| **Feature** | Lead pipeline — AI propensity → CRM lead creation |
| **Wired As** | `idbileads.Submit` → `POST /api/v1/idbi/leads` |

**Key Request Fields (spec)**
```json
{
  "leadType": "...",
  "customerType": "INDIVIDUAL",
  "firstName": "John",
  "lastName": "Doe",
  "mobileNo": "9999999999",
  "emailId": "john@example.com",
  "pancard": "AAAAA9999A",
  "addressLine1": "...",
  "pincode": "400001",
  "state": "MH",
  "product": "HOME_LOAN",
  "estimatedAmount": 5000000,
  "leadChannel": "...",
  "leadSource": "ASTRA"
}
```

**Key Response Fields**
```json
{
  "result": {
    "leadId": "LEAD12345",
    "message": "Lead already created",
    "errors": []
  }
}
```

---

### 24. fetchCustomerLimitDetails `#442`
| Field | Value |
|-------|-------|
| **Path** | `POST /Development/fetchCustomerLimitDetailstest` |
| **Surface** | `rm` |
| **Status** | ✅ wired |
| **Call Pattern** | `sync` |
| **Data Source** | 🏦 idbi |
| **Feature** | CIF-level credit exposure / leverage scoring |
| **Wired As** | `rmcreditrisk.Refresh` → `GET /api/rm/idbi/clients/{id}/credit-risk` |

**Key Response Fields**
```json
{
  "result": {
    "customerLimitDetailsResponse": {
      "totalLimit": { "amount": 5000000, "currency": "INR" },
      "totalOutstanding": { "amount": 3000000, "currency": "INR" },
      "fundedLimit": { "amount": 4000000, "currency": "INR" },
      "nonFundedLimit": { "amount": 1000000, "currency": "INR" },
      "limitHeaderDetails": { "custRating": "A1" },
      "ltCustomerDetails": { "accountManager": "RM001" }
    }
  }
}
```
> ⚠️ Uses `{amount, currency}` format — NOT `{amountValue, currencyCode}`. Dates are `dd-mm-yyyy`.

---

## HR / Staff APIs

### 25. fetchHRMSEmployeeDetails `#508`
| Field | Value |
|-------|-------|
| **Path** | `POST /Development/fetchHRMSEmployeeDetailstest` |
| **Surface** | `rm` |
| **Status** | ✅ wired |
| **Call Pattern** | `on-demand-cache` |
| **Data Source** | 🏦 idbi |
| **Feature** | Staff directory / lead routing — resolve EIN to name/grade/org |
| **Wired As** | `idbihrms.VerifyActiveEmployee` (RM login pre-check) |

**Key Response Fields**
```json
{
  "otpId": "otp-123",
  "getHRMSEmployeeDetails": {
    "ein": "EIN001",
    "fullNameTitle": "Mr. John Doe",
    "grade": "M3",
    "position": "Relationship Manager",
    "location": "Mumbai",
    "region": "West",
    "email": "john.doe@idbi.co.in",
    "sol": "SOL001",
    "zone": "Zone-A",
    "supEin": "EIN000",
    "supFullNameTitle": "Mr. Jane Smith",
    "supEmail": "jane.smith@idbi.co.in",
    "supLocation": "Mumbai HO"
  },
  "errors": []
}
```
> ⚠️ Call can be OTP-gated: `otpRequired` in request / `otpId` in response.

---

## Summary Table

| # | API Name | ID | Path | Surface | Status | Pattern | Data |
|---|----------|-----|------|---------|--------|---------|------|
| 1 | getCustomerAccountsByCustId | 394 | `/Development/getCustomerAccountsByCustIdtest` | both | ✅ | sync | 🏦 |
| 2 | performAccountEnquiry | 365 | `/Development/performAccountEnquirytest` | both | ✅ | on-demand-cache | 🏦 |
| 3 | accountLienEnquiry | 362 | `/Development/accountLienEnquirytest` | both | ✅ | sync | 🏦 |
| 4 | getFullAccountStatementWithPagination | 393 | `/Development/getFullAccountStatementWithPaginationtest` | both | ✅ | sync | 🏦 |
| 5 | getAccountStatementFromFinPro | 739 | `/Development/getAccountStatementFromFinProtest` | both | ✅ | interactive | 🏦 |
| 6 | MoneyOneFIU_GetAccountStatement | 595 | `/Development/getAccountStatementtest` | both | ✅ | event | 🏦 |
| 7 | requestConsentFromFinPro | 590 | `/Development/requestConsentFromFinProtest` | app | ✅ | interactive | 🎭 |
| 8 | getConsentListFromFinPro | 591 | `/Development/getConsentListFromFinProtest` | both | ✅ | on-demand-cache | 🏦 |
| 9 | getWebRedirectionEncryptedURL | 592 | `/Development/getWebRedirectionEncryptedURLtest` | app | ✅ | interactive | 🎭 |
| 10 | generateDecryptedResponseFromFinPro | 593 | `/Development/generateDecryptedResponseFromFinProtest` | app | ✅ | interactive | 🎭 |
| 11 | pushConsentNotification | 497 | `/webhooks/idbi-aa/consent-notification` | plumbing | ✅ | webhook-in | 🎭 |
| 12 | pushDataNotification | 498 | `/webhooks/idbi-aa/data-notification` | plumbing | ✅ | webhook-in | 🎭 |
| 13 | getLoanAccountDetails | 391 | `/Development/getLoanAccountDetailstest` | both | ✅ | sync | 🏦 |
| 14 | generateLoanRepaymentSchedule | 473 | `/Development/generateLoanRepaymentScheduletest` | both | ✅ | on-demand-cache | 🏦 |
| 15 | inquireHPPayoff | 538 | `/Development/InquireHPAyofftest` | both | ✅ | on-demand-cache | 🏦 |
| 16 | getLoanOverduePositionEnquiry | 404 | `/Development/getLoanOverduePositionEnquirytest` | rm | ✅ | sync | 🏦 |
| 17 | getLoanOverdueDetails | 402 | `/Development/getLoanOverdueDetailstest` | rm | ✅ | sync | 🏦 |
| 18 | fetchLoanAccountLimits | 441 | `/Development/fetchLoanAccountLimitstest` | rm | ✅ | sync | 🏦 |
| 19 | fetchLoanInterestRates | 433 | `/Development/fetchLoanInterestRatestest` | both | 💀 | on-demand-cache | 🎭 |
| 20 | fetchCibilScore | 408 | `/Development/fetchCibilScoretest` | both | 💀 | on-demand-cache | 🎭 |
| 21 | searchCkycDetails | 415 | `/Development/searchCkycDetailstest` | rm | ✅ | on-demand-cache | 🏦 |
| 22 | performCustomerMasterDedupeCheck | 456 | ❓ unknown | rm | 💀 | on-demand-cache | 🎭 |
| 23 | createLead | 428 | `/Development/createLeadtest` | rm | ✅ | event | 🏦 |
| 24 | fetchCustomerLimitDetails | 442 | `/Development/fetchCustomerLimitDetailstest` | rm | ✅ | sync | 🏦 |
| 25 | fetchHRMSEmployeeDetails | 508 | `/Development/fetchHRMSEmployeeDetailstest` | rm | ✅ | on-demand-cache | 🏦 |

---

*Generated from [`idbi-api-catalog.json`](file:///Users/swaraj/Documents/astra-backend/docs/idbi-api-catalog.json) — captured 2026-09-08, status refreshed 2026-09-09.*
