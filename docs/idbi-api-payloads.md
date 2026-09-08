# IDBI API — Endpoints, Request / Response payloads

Companion to [`idbi-api-usage-map.md`](./idbi-api-usage-map.md) ·
Machine-readable: [`idbi-api-catalog.json`](./idbi-api-catalog.json)

**Two sources merged here:**
- `docs/api_req_response_hackathon.xlsx` — skeleton request/response bodies (field lists; empty strings are placeholders).
- `docs/idbireposne/*.yaml` — 31 OpenAPI specs exported from the Atlas portal: real **endpoints**, real **sample request values**, and per-API doc links. Responses in these specs are mostly empty (`responses: {}`); only 362 carries an error-envelope sample.

## Gateway

| | |
|---|---|
| Sandbox base URL | `https://sandboxpocgatewayprod.idbi.bank.in` |
| Doc portal (browse only) | `https://innobox.idbi.bank.in` |
| Path shape | `POST /Development/<name>test` — **dev/sandbox stage**. Production path prefix and the `test` suffix will differ; get prod paths from the portal. |
| Method | `POST` for all 25 |
| Auth | gateway API key / token from the Atlas portal (exact scheme TBD) |
| Error envelope | `{ "errors": [ { "code": "", "title": "", "description": "" } ] }` — note `title`/`description`, **not** `type`/`message` as some xlsx cells show. Sample codes: `EH003` "Mandatory fields missing", `400`, `403`, `500`. |
| Missing | **456** `performCustomerMasterDedupeCheck` — no OpenAPI spec in `idbireposne/`; only the xlsx skeleton below. |

---

## 393 — getFullAccountStatementWithPagination  (ESB FI_717)
`POST https://sandboxpocgatewayprod.idbi.bank.in/Development/getFullAccountStatementWithPaginationtest`
[spec](../docs/idbireposne/Open%20API%20Specifications%20%2814%29.yaml) · [portal doc](https://innobox.idbi.bank.in/api-specification/e7c58fee-dc58-4759-ac9a-159cb26f7a42/d2d078a6-e887-44b9-a648-6dc77590ec61)

<details><summary>Request — OpenAPI sample (real values)</summary>

```json
{
  "input": {
    "acid": "660100100003",
    "branchId": "105",
    "fromDate": "2025-05-01T00:00:00.000",
    "toDate": "2025-05-27T00:00:00.000",
    "sortIn": "D",
    "paginationDetails": {
      "lastBalance": { "amountValue": "88955.73", "currencyCode": "INR" },
      "lastPstdDate": "2022-12-30T22:56:49.000",
      "lastTxnDate": "2022-12-30T00:00:00.000",
      "lastTxnId": "S41732195",
      "lastTxnSrlNo": "1"
    }
  }
}
```
First page: send `paginationDetails` empty / omitted; subsequent pages: feed the last row back.
</details>

<details><summary>Response — xlsx skeleton</summary>

```json
{
  "result": {
    "accountBalances": {
      "acid": "", "branchId": "", "currencyCode": "",
      "availableBalance":   { "amountValue": "", "currencyCode": "" },
      "ledgerBalance":      { "amountValue": "", "currencyCode": "" },
      "fFDBalance":         { "amountValue": "", "currencyCode": "" },
      "floatingBalance":    { "amountValue": "", "currencyCode": "" },
      "userDefinedBalance": { "amountValue": "", "currencyCode": "" }
    },
    "hasMoreData": "",
    "field125": "", "field126": "", "field127": "",
    "transactionDetails": [
      {
        "pstdDate": "date", "valueDate": "date",
        "txnId": "", "txnSrlNo": "", "txnCat": "",
        "txnBalance": { "amountValue": "", "currencyCode": "" },
        "transactionSummary": {
          "instrumentId": "",
          "txnAmt": { "amountValue": "", "currencyCode": "" },
          "txnDate": "date", "txnDesc": "", "txnType": ""
        }
      }
    ]
  },
  "customData": { "THB": "" }
}
```
Per-txn amount/date/desc/type live under `transactionSummary`. **5 balance types.**
Paginate on `pstdDate`/`txnDate`/`txnId`/`txnSrlNo` + running balance; stop when `hasMoreData` is false. Ignore `field125/126/127`, `customData.THB`.
</details>

---

## 365 — performAccountEnquiry
`POST https://sandboxpocgatewayprod.idbi.bank.in/Development/performAccountEnquirytest`
[spec](../docs/idbireposne/Open%20API%20Specifications%20%2811%29.yaml) · [portal doc](https://innobox.idbi.bank.in/api-specification/c36c574a-8f10-4271-8d06-cdaea4f4fd40/d1d8a2f4-85a4-4624-b367-9d51811ecd55)
_(multi-account clone: `/Development/performAccountEnquirytest01`)_

<details><summary>Request — OpenAPI sample</summary>

```json
{ "acctId": "660100100003" }
```
Flat body, no `input` wrapper.
</details>

<details><summary>Response — xlsx skeleton (smart-quotes normalised)</summary>

```json
{
  "result": {
    "acctId": "",
    "acctType": { "schmCode": "", "schmType": "" },
    "acctCurr": "",
    "custId": "",
    "personName": { "lastName": "", "firstName": "", "middleName": "", "name": "", "titlePrefix": "" },
    "acctOpenDt": "date",
    "bankAcctStatusCode": "",
    "acctBal": [ { "balType": "", "balAmt": { "amountValue": "", "currencyCode": "" } } ],
    "custStat": { "refCode": "", "refRecType": "", "refDesc": "" },
    "acctInqCustomData": { "acctName": "", "status": "" },
    "errors": [ { "code": "", "type": "", "message": "" } ]
  }
}
```
`acctBal[]` carries multiple `balType` rows (available / ledger / shadow / unclear) — pick by `balType`.
</details>

---

## 394 — getCustomerAccountsByCustId
`POST https://sandboxpocgatewayprod.idbi.bank.in/Development/getCustomerAccountsByCustIdtest`
[spec](../docs/idbireposne/Open%20API%20Specifications%20%281%29.yaml) · [portal doc](https://innobox.idbi.bank.in/api-specification/370ef1b0-a38f-4ca1-8ab9-b50f74c13a9a/a4ff4177-f6ac-48c8-b543-ac64d0d8f6a3)
_(multi-account clone: `/Development/getCustomerAccountsByCustIdtest01`)_

<details><summary>Request — OpenAPI sample</summary>

```json
{
  "input": { "acctType": "SBA", "branchId": "105", "cifId": "98655854" },
  "txn": "E"
}
```
Spec adds a top-level `"txn": "E"` not shown in the xlsx.
</details>

<details><summary>Response — xlsx skeleton</summary>

```json
{
  "result": {
    "cifId": "",
    "acctTypeRequested": "",
    "customerAccountInfo": [
      { "acctNumber": "", "acctType": "", "acctCurrCode": "", "acctBalance": { "amountValue": "", "currencyCode": "" } }
    ],
    "numOfAccounts": "integer",
    "customData": { "thb": "" },
    "errors": [ { "code": "", "type": "", "message": "" } ]
  }
}
```
</details>

---

## 362 — accountLienEnquiry
`POST https://sandboxpocgatewayprod.idbi.bank.in/Development/accountLienEnquirytest`
[spec](../docs/idbireposne/Open%20API%20Specifications%20%2813%29.yaml) · [portal doc](https://innobox.idbi.bank.in/api-specification/d19a7df9-7a80-4d2f-b74d-6401b3cc1cb7/7b38bf35-b991-49e8-b71a-646edfa27ad3)

Spec context: liens are maintained per Worldline/FastTag DSA account; look up by `acctId` + lien `moduleType`.

<details><summary>Request — OpenAPI sample (real values)</summary>

```json
{
  "input": {
    "acctId": "660100100003",
    "moduleType": "DEPOSIT",
    "acctCurr": "INR",
    "acctType": { "schmCode": "SB002", "schmType": "SAVINGS" },
    "bankInfo": {
      "bankId": "IDBI001", "name": "IDBIBANK", "branchId": "105", "branchName": "PUNE",
      "postAddr": { "addr1": "142, Lake View", "addr2": "Near City Mall", "addr3": "MH", "city": "PUNE", "stateProv": "MH", "postalCode": "411001", "country": "India", "addrType": "REGISTERED" }
    }
  }
}
```
</details>

<details><summary>Response — xlsx skeleton + spec error samples</summary>

```json
{
  "result": {
    "acctId": "", "moduleType": "", "acctCurr": "",
    "acctType": { "schmCode": "", "schmType": "" },
    "bankInfo": {
      "bankId": "", "name": "", "branchId": "", "branchName": "", "postAddr": { },
      "lienDetails": {
        "newLienAmt": { "amountValue": "", "currencyCode": "" },
        "oldLienAmt": { "amountValue": "", "currencyCode": "" },
        "lienDate": { "startDate": "", "endDate": "" },
        "reasonCode": "", "remarks": "", "isDeleted": "", "lienId": ""
      }
    }
  }
}
```
Error envelope (from the 362 spec — this is the canonical shape for all APIs):
```json
{ "errors": [ { "code": "EH003", "title": "Mandatory fields missing", "description": "string" } ] }
```
`lienDetails` nested inside `bankInfo`. Usable balance = balance − sum(active, i.e. `isDeleted`=false, lien amounts).
</details>

---

## 739 — getAccountStatementFromFinPro  (Account Aggregator)
`POST https://sandboxpocgatewayprod.idbi.bank.in/Development/getAccountStatementFromFinProtest`
[spec](../docs/idbireposne/Open%20API%20Specifications%20%2826%29.yaml) · [portal doc](https://innobox.idbi.bank.in/api-specification/4d0aa45f-e5bb-4ab4-a336-fe80500ecd55/59dedfad-1b1a-4b77-b847-d796a93717ec)

<details><summary>Request — OpenAPI sample</summary>

```json
{
  "consentId": "CONSENT-0001",
  "linkRefNumber": [ "76ae28bd-eebf-4a49-8701-68a14346d996" ]
}
```
</details>

<details><summary>Response — xlsx skeleton (source JSON was missing commas; cleaned)</summary>

```json
{
  "status": "", "ver": "",
  "pageDetails": { "totalRecords": "", "currentPageNumber": "", "totalPages": "" },
  "data": {
    "linkReferenceNumber": "", "maskedAccountNumber": "", "fiType": "", "bank": "",
    "Summary": {
      "currentBalance": "", "currency": "", "balanceDateTime": "", "type": "", "accountType": "",
      "branch": "", "facility": "", "ifsc": "", "micrCode": "", "openingDate": "",
      "currentODLimit": "", "drawingLimit": "", "status": "",
      "maturityDate": "", "maturityAmount": "", "interestRate": "", "principalAmount": "",
      "Pending": { "amount": "", "transactionType": "" }
    },
    "Profile": { "Holders": { "type": "", "Holder": { "name": "", "dob": "", "mobile": "", "nominee": "", "landline": "", "address": "", "email": "", "pan": "", "ckycCompliance": "" } } },
    "Transactions": {
      "startDate": "", "endDate": "",
      "Transaction": { "txnId": "", "type": "", "mode": "", "amount": "", "currentBalance": "", "balance": "", "transactionTimeStamp": "", "transactionDateTime": "", "valueDate": "", "narration": "", "reference": "" }
    }
  },
  "errors": [ { "code": "", "type": "", "message": "" } ]
}
```
`Holder` and `Transaction` are arrays in practice. Paginate via `pageDetails`.
</details>

---

## 595 — MoneyOne FIU – Get Account Statement
`POST https://sandboxpocgatewayprod.idbi.bank.in/Development/getAccountStatementtest`
[spec](../docs/idbireposne/Open%20API%20Specifications%20%2824%29.yaml) · [portal doc](https://innobox.idbi.bank.in/api-specification/ed2cc79e-6cd2-4d84-af0f-d608ce320618/b774f008-27b1-4123-8162-3b7f7a2e3b43)
_(multi-account clone: `/Development/getAccountStatementtest01`)_

<details><summary>Request — OpenAPI sample</summary>

```json
{
  "consentId": "CONSENT-0001",
  "linkRefNumber": [ "19818fc6-d5ee-429b-9d14-4dfd5d92fc8e" ]
}
```
Same request as 739. (The xlsx's "request" cell for 595 was actually the statement schema — shown below as the response.)
</details>

<details><summary>Response schema — xlsx</summary>

```json
{
  "ver": "", "status": "",
  "data": [
    {
      "fiType": "", "bank": "", "linkReferenceNumber": "", "maskedAccountNumber": "",
      "Summary": {
        "accountType": "", "accountSubType": "", "currentBalance": "", "balanceDateTime": "",
        "branch": "", "drawingLimit": "", "currentODLimit": "", "currency": "", "ifsc": "",
        "micrCode": "", "facility": "", "openingDate": "", "status": "",
        "PendingTxns": { "PendingTxn": { "transactionType": "", "amount": "" } }
      },
      "Profile": { "Holders": { "type": "", "Holder": [ { "name": "", "dob": "", "mobile": "", "email": "", "pan": "", "address": "", "nominee": "", "landline": "", "ckycRegistered": "" } ] } },
      "Transactions": {
        "startDate": "", "endDate": "",
        "Transaction": [ { "txnId": "", "type": "", "mode": "", "amount": "", "narration": "", "reference": "", "transactionalBalance": "", "valueDate": "", "transactionTimestamp": "" } ]
      }
    }
  ]
}
```
</details>

---

## 590 — requestConsentFromFinPro
`POST https://sandboxpocgatewayprod.idbi.bank.in/Development/requestConsentFromFinProtest`
[spec](../docs/idbireposne/Open%20API%20Specifications%20%2827%29.yaml) · [portal doc](https://innobox.idbi.bank.in/api-specification/6804b0f7-7034-488a-bbf8-b90525128e34/57edafc1-4e70-47b6-9326-8a5c6356263a)

<details><summary>Request — OpenAPI sample</summary>

```json
{
  "partyIdentifierType": "MOBILE",
  "partyIdentifierValue": "9988776655",
  "productID": "TEST",
  "accountID": "660100100003",
  "vua": "9988776655@onemoney",
  "transactionID": "9080"
}
```
</details>

<details><summary>Response — xlsx skeleton</summary>

```json
{
  "status": "", "ver": "",
  "data": [ { "status": "", "consent_handle: ": "" } ],
  "timestamp": "", "errorCode": "", "errorMsg": "",
  "errors": [ { "code": "", "type": "", "message": "" } ]
}
```
⚠️ Source key is literally `"consent_handle: "` (trailing colon + space) — trim on parse.
</details>

---

## 591 — getConsentListFromFinPro
`POST https://sandboxpocgatewayprod.idbi.bank.in/Development/getConsentListFromFinProtest`
[spec](../docs/idbireposne/Open%20API%20Specifications%20%2821%29.yaml) · [portal doc](https://innobox.idbi.bank.in/api-specification/ba5ab412-3bd6-43c7-8044-68a777e5a177/645c953d-aa03-4513-96c7-63798fccecad)
_(multi-account clone: `/Development/getConsentListFromFinProtest01`)_

<details><summary>Request — OpenAPI sample</summary>

```json
{
  "partyIdentifierType": "MOBILE",
  "partyIdentifierValue": "9988776655",
  "productID": "TEST",
  "accountID": "660100100003",
  "vua": "9988776655@onemoney"
}
```
Spec adds `vua` vs the xlsx.
</details>

<details><summary>Response — xlsx skeleton</summary>

```json
{
  "status": "", "ver": "",
  "data": [
    {
      "consentID": "", "status": "", "consent_handle: ": "",
      "productID": "", "accountID": "", "aaId": "", "vua": "", "consentCreationData": "",
      "accounts": [
        { "fipName": "", "fipId": "", "accountType": "", "linkReferenceNumber": "", "maskedAccountNumber": "", "fiType": "" }
      ]
    }
  ],
  "timestamp": "", "errorCode": "", "errorMsg": "",
  "errors": [ { "code": "", "type": "", "message": "" } ]
}
```
</details>

---

## 592 — getWebRedirectionEncryptedURL
`POST https://sandboxpocgatewayprod.idbi.bank.in/Development/getWebRedirectionEncryptedURLtest`
[spec](../docs/idbireposne/Open%20API%20Specifications%20%2822%29.yaml) · [portal doc](https://innobox.idbi.bank.in/api-specification/be2b4350-2341-4bbc-a0b5-f15922b8e582/9e816d01-63cf-4d0d-9165-79295e9fede1)

<details><summary>Request — OpenAPI sample</summary>

```json
{
  "consentHandle": "6afdd734-be3b-475f-a8fc-3fcd18c04714",
  "redirectUrl": "https://myapp.com/consent/callback"
}
```
</details>

<details><summary>Response — xlsx skeleton</summary>

```json
{
  "status": "", "ver": "", "message": "",
  "data": [ { " webRedirectionUrl": "" } ],
  "timestamp": "", "errorCode": "", "errorMsg": "",
  "errors": [ { "code": "", "type": "", "message": "" } ]
}
```
⚠️ Source key has a leading space: `" webRedirectionUrl"`.
</details>

---

## 593 — generateDecryptedResponseFromFinPro
`POST https://sandboxpocgatewayprod.idbi.bank.in/Development/generateDecryptedResponseFromFinProtest`
[spec](../docs/idbireposne/Open%20API%20Specifications%20%2825%29.yaml) · [portal doc](https://innobox.idbi.bank.in/api-specification/f8734776-9591-44e0-abf6-e3f09a39d08d/f460b7ba-41d5-4b67-874d-0d3c5432f497)

<details><summary>Request — OpenAPI sample (note the wrapper — differs from the xlsx)</summary>

```json
{
  "webRedirectionURL": {
    "ecres": "0cxcJYX2p0TUjgqwGtPr3_C7uABITDhYa-ObDOmO1Weo...<long encrypted blob>",
    "resdate": "100720230858407",
    "fi": "Xl5VWl1eV0odWVQ"
  }
}
```
The xlsx showed `{ecres, resdate, fi}` flat — the real body wraps them in `webRedirectionURL`. `ecres` is the encrypted payload from the redirect return.
</details>

<details><summary>Response — xlsx skeleton</summary>

```json
{
  "status": "", "ver": "", "message": "",
  "data": [ { " status": "", " errorCode": "", " txnId": "", " sessionId": "", " srcref": "", " userid": "", " redirect": "" } ],
  "timestamp": "", "errorCode": "", "errorMsg": "",
  "errors": [ { "code": "", "type": "", "message": "" } ]
}
```
⚠️ Every key inside `data[0]` has a leading space. `errorCode` `0` = approved, `1` = rejected.
</details>

---

## 497 — pushConsentNotification  (inbound webhook, AA → us)
Reference endpoint (their test harness): `POST .../Development/pushConsentNotificationtest`
[spec](../docs/idbireposne/Open%20API%20Specifications%20%285%29.yaml) · [portal doc](https://innobox.idbi.bank.in/api-specification/7514fc60-af7a-4bf5-b78a-8cffa61d56eb/f3178fdd-df1f-4474-a687-81493684b256)
**We host our own receiver** — path TBD, register it with OneMoney.

<details><summary>Body we receive — OpenAPI sample (real values)</summary>

```json
{
  "transactionID": "1990927",
  "timestamp": "2023-01-18T12:15:16.215Z",
  "consentHandle": "6afdd734-be3b-475f-a8fc-3fcd18c04714",
  "eventType": "CONSENT",
  "eventStatus": "CONSENT_APPROVED",
  "consentId": "CONSENT-0001",
  "eventMessage": "Consent approved",
  "vua": "9988776655@onemoney",
  "productID": "TEST",
  "accountID": "660100100003",
  "fetchType": "PERIODIC",
  "consentExpiry": "2027-01-17 09:47:07"
}
```
</details>

**Response:** blank body, HTTP `200`. Non-2xx makes AA retry.

---

## 498 — pushDataNotification  (inbound webhook, AA → us)
Reference endpoint: `POST .../Development/pushDataNotificationtest`
[spec](../docs/idbireposne/Open%20API%20Specifications%20%2812%29.yaml) · [portal doc](https://innobox.idbi.bank.in/api-specification/6c194acc-e25e-4f63-a7d0-a7203e7dfdb5/977b2f55-c805-420f-beba-c4b53f440bd4)
**We host our own receiver** — path TBD.

<details><summary>Body we receive — OpenAPI sample (real values)</summary>

```json
{
  "transactionID": "1990927",
  "timestamp": "2023-01-18T12:15:16.215Z",
  "consentHandle": "6afdd734-be3b-475f-a8fc-3fcd18c04714",
  "eventType": "DATA",
  "eventStatus": "DATA_READY",
  "consentId": "CONSENT-0001",
  "vua": "9988776655@onemoney",
  "eventMessage": "Data ready for consent id CONSENT-0001",
  "productID": "TEST",
  "accountID": "660100100003",
  "fetchType": "PERIODIC",
  "consentExpiry": "2027-01-17 09:47:07",
  "dataExpiry": "2026-01-10T06:15:48.000Z",
  "sessionId": "21caa19d-f69f-4954-b965-bdc1d6fc5f8a",
  "firstTimeFetch": "false",
  "linkRefNumbers": [
    { "linkRefNumber": "cf885ed4-3085-45db-bd65-f9472d0073a4", "fiStatus": "READY", "fipName": "IDBI Bank", "fipId": "IDBI001", "maskedAccountNumber": "XXXXXXXX0003" }
  ]
}
```
</details>

**Response:** blank body, HTTP `200`. On receipt, backend calls **595** with `sessionId` / `linkRefNumbers` to pull the statement.

---

## 408 — fetch CIBIL Score
`POST https://sandboxpocgatewayprod.idbi.bank.in/Development/fetchCibilScoretest`
[spec](../docs/idbireposne/Open%20API%20Specifications%20%286%29.yaml) · [portal doc](https://innobox.idbi.bank.in/api-specification/ef7b3d77-5970-4712-99e0-be87a0aab0fb/d4cc63df-56a3-4dc1-86f5-987898e9f7b3)

<details><summary>Request — OpenAPI sample (real values; ~3 KB decisioning-engine wrapper)</summary>

```json
{
  "fetchCibilScoreRequest": {
    "header": { "transactionDate": "2022-10-18T18:00:05.005", "transactionId": "cibil0101", "sourceSystem": "iAstra" },
    "body": {
      "dcRequest": {
        "authentication": { "type": "OnDemand", "userId": "<gateway user>", "password": "<gateway pass>" },
        "requestInfo": { "solutionSetId": 368, "executeLatestVersion": true, "executionMode": "NewWithContext" },
        "fields": {
          "Applicants": { "applicants": [ {
            "applicantType": "Individual", "applicantFirstName": "PRIYA", "applicantLastName": "PATIL",
            "dateOfBirth": "20061995", "gender": "Female",
            "identifiers": [ { "idNumber": "FGHPP4567T", "idType": "02" } ],
            "telephones": [ { "telephoneNumber": "9988776655", "telephoneType": "01" } ],
            "addresses": [ { "addressLine1": "142, Lake View", "addressType": "01", "city": "PUNE", "pinCode": "411001", "residenceType": "01", "stateCode": "27" } ],
            "accounts": [ { "accountNumber": "" } ]
          } ] },
          "ApplicationData": {
            "purpose": "00", "amount": 10000, "scoreType": "10", "GSTStateCode": "27",
            "memberCode": "<member code>", "password": "<member pass>",
            "cibilBureauFlag": false, "DSTuNtcFlag": true, "idVerificationFlag": false,
            "mfiBureauFlag": true, "NTCProductType": true, "consumerConsentForUIDAIAuthentication": "N", "formattedReport": true
          }
        }
      }
    }
  }
}
```
`idType "02"` = PAN. `authentication` + `memberCode`/`password` are bureau creds — keep them server-side (Secrets Manager). `formattedReport: true` asks for the full report in the response `document`.
</details>

<details><summary>Response — xlsx skeleton</summary>

```json
{
  "fetchCibilScoreResponse": {
    "header": { "success": "", "statusCode": "number", "statusMessage": "", "transactionId": "" },
    "body": {
      "dcResponse": {
        "status": "", "decision": "", "applicationId": "",
        "applicant": { "name": { "firstName": "", "middleName": "", "lastName": "" }, "dateOfBirth": "", "gender": "", "identifiers": [ { "type": "", "value": "" } ], "address": { "line1": "", "line2": "", "city": "", "stateCode": "", "pinCode": "" } },
        "bureauResponse": { "status": "", "errorCode": "", "errorDescription": "", "isSuccess": "" },
        "idVision": { "isSuccess": "", "errorCode": "", "errorMessage": "" },
        "tuVerification": { "isSuccess": "" },
        "document": { "id": "", "name": "" },
        "applicationData": { "amount": "number", "purpose": "", "scoreType": "", "flags": { "cibilBureau": "", "idVerification": "", "mfiBureau": "" } }
      }
    }
  }
}
```
⚠️ No raw numeric score in this schema — you get `dcResponse.decision` + `bureauResponse`. The score number, if returned, is in `document` (the `formattedReport`). Confirm on a live call.
</details>

---

## 433 — fetchLoanInterestRates
`POST https://sandboxpocgatewayprod.idbi.bank.in/Development/fetchLoanInterestRatestest`
[spec](../docs/idbireposne/Open%20API%20Specifications%20%2815%29.yaml) · [portal doc](https://innobox.idbi.bank.in/api-specification/f8b2ce08-deff-4d06-8f98-a25bb129dc41/20aa3c86-06d8-4a47-86e1-9b4b9d1f8aa5)

<details><summary>Request — OpenAPI sample (flat — NO <code>input</code> wrapper, unlike the xlsx)</summary>

```json
{
  "crncyCode": "INR",
  "intTblCode": { "tblCode": "LY001" },
  "loanAmt": { "amountValue": "10000", "currencyCode": "INR" },
  "loanPerdDays": "0",
  "loanPerdMnths": "12",
  "originationDate": "2022-01-03T17:13:06.751"
}
```
</details>

<details><summary>Response — xlsx skeleton</summary>

```json
{
  "result": {
    "baseIntRate": { "value": "" },
    "effctIntRate": { "value": "" },
    "penalIntRate": { "value": "" },
    "crncyCode": "", "diffTypeFlg": "",
    "loanAmt": { "amountValue": "", "currencyCode": "" },
    "loanPerdDays": "", "loanPerdMnths": "", "originationDate": "date",
    "oslabRateLL": [
      { "beginSlabAmt": { "amountValue": "", "currencyCode": "" }, "endSlabAmt": { "amountValue": "", "currencyCode": "" },
        "normalIntPcnt": { "value": "" }, "penalIntPcnt": { "value": "" },
        "loanTenorInd": "", "maxTenorDays": "", "maxTenorMths": "", "delFlg": "", "key": { "serial_num": "" }, "laVerSlabSrlNum": "" }
    ],
    "errors": [ { "code": "", "type": "", "message": "" } ]
  }
}
```
</details>

---

## 473 — generateLoanRepaymentSchedule
`POST https://sandboxpocgatewayprod.idbi.bank.in/Development/generateLoanRepaymentScheduletest`
[spec](../docs/idbireposne/Open%20API%20Specifications%20%2823%29.yaml) · [portal doc](https://innobox.idbi.bank.in/api-specification/c7587225-fee5-4577-bb41-3923cf850a66/7ac814d1-5c6c-4c31-a390-faa58f21f1a4)

<details><summary>Request — OpenAPI sample (real values; no <code>input</code> wrapper in the spec sample)</summary>

```json
{
  "loanModellingMsgInputVO": {
    "mandatoryParameters": { "crncyCode": "INR", "originationDate": "2020-08-31T00:00:00.000", "schmCode": { "schmCode": "EIDEM" } },
    "advanceParameters": { "eIFormula": "" },
    "Variables": { "loanAmount": { "amountValue": "100000", "currencyCode": "INR" }, "intRate": { "Value": "1.2" }, "noOfInstalmnts": "24" }
  }
}
```
</details>

<details><summary>Response — xlsx skeleton</summary>

```json
{
  "result": {
    "loanModellingSchOutputVO": {
      "lamodRepaymentLL": [ { "flowAmt": { "amountValue": " ", "currencyCode": "" }, "flowDesc": "", "flowStartDate": "", "Freq": "", "noOfInstalments": "" } ],
      "oamortLL": [
        { "amortStruct": {
            "flowDate": "", "flowDesc": "",
            "instlAmt": { "amountValue": " ", "currencyCode": "" },
            "intAmt": { "amountValue": " ", "currencyCode": "" },
            "princAmt": { "amountValue": " ", "currencyCode": "" },
            "princOutStanding": { "amountValue": " ", "currencyCode": "" },
            "cummIntAmt": { "amountValue": " ", "currencyCode": "" },
            "cummPrincAmt": { "amountValue": " ", "currencyCode": "" }
        } }
      ]
    },
    "doScheduleCustomData": {},
    "errors": [ { "code": "", "type": "", "message": "" } ]
  }
}
```
`oamortLL[]` = the amortization table (one row per EMI).
</details>

---

## 391 — getLoanAccountDetails
`POST https://sandboxpocgatewayprod.idbi.bank.in/Development/getLoanAccountDetailstest`
[spec](../docs/idbireposne/Open%20API%20Specifications%20%2820%29.yaml) · [portal doc](https://innobox.idbi.bank.in/api-specification/82814bf5-f546-4afb-a147-92ad799d77cb/4c4e4152-844e-4523-b25f-777eedecc310)

<details><summary>Request — OpenAPI sample (much simpler than the xlsx skeleton)</summary>

```json
{
  "input": {
    "loanAcctId": {
      "acctId": "660100100003",
      "acctType": { "schmCode": "SB002", "schmType": "SAVINGS" },
      "acctCurr": "INR",
      "bankInfo": { "bankId": "IDBI001", "name": "IDBIBANK", "branchId": "105", "branchName": "PUNE", "postAddr": { "addr1": "142, Lake View", "city": "PUNE", "stateProv": "MH", "postalCode": "411001", "country": "India", "addrType": "Office" } }
    },
    "custId": { "custId": "68453002" },
    "channel": "API",
    "requestId": "REQ202607160001",
    "reqDate": "2026-07-16"
  }
}
```
Spec adds `custId`, `channel`, `requestId`, `reqDate` inside `input`.
</details>

<details><summary>Response — xlsx skeleton (trimmed to useful branches; full payload ~7.8 KB)</summary>

```json
{
  "result": {
    "netIntRate": { "value": "" },
    "acctOpenDt": "date", "modeOfOper": "",
    "custId": { "custId": "", "personName": { "lastName": "", "firstName": "", "middleName": "", "name": "", "titlePrefix": "" } },
    "amtAlreadyDisb": { "amountValue": "", "currencyCode": "" },
    "amtAvailForDisb": { "amountValue": "", "currencyCode": "" },
    "disbAmt": { "amountValue": "", "currencyCode": "" },
    "loanGenDetails": {
      "loanAmt": { "amountValue": "", "currencyCode": "" },
      "loanPeriodDays": "", "loanPeriodMonths": "", "rePmtMethod": "",
      "reschedParams": { "reschedAdjTenorAmtFlg": "", "autoReschedForIntRateChange": "", "reschedAmtFlg": "", "autoReschedPrepaymentFlg": "", "installmentGracePeriodDays": "", "InstallmentGracePeriodMonths": "" },
      "pmtPlan": { "repmtRec": [ { "installmentId": "", "installStartDt": "date", "noOfInstall": "", "flowAmt": { "amountValue": "", "currencyCode": "" }, "installRate": { "value": "" } } ], "negotiatedRate": { "value": "" }, "defApplIntRate": { "value": "" }, "capEMIFlg": "" }
    },
    "relPartyRec": [ { "relPartyType": "", "relPartyCode": "", "custId": "", "relPartyContactInfo": { "phoneNum": { "telephoneNum": "" }, "emailAddr": "" } } ],
    "tranId": "", "tranDt": "date"
  },
  "errors": [ { "code": "", "type": "", "message": "" } ]
}
```
Ignored in the real payload: `loanAcctGenInfo`, `ACHDetails`, `multiSrcInstructionRec`, `postDtChkRec`, `leaseDetailsAdd/leaseFinanceChargesAdd`, dealer codes, branch `postAddr` blocks.
</details>

---

## 538 — Inquire HP Payoff
`POST https://sandboxpocgatewayprod.idbi.bank.in/Development/InquireHPAyofftest`
[spec](../docs/idbireposne/Open%20API%20Specifications%20%2818%29.yaml) · [portal doc](https://innobox.idbi.bank.in/api-specification/2d61e77c-2c20-453c-b985-950891c08eb4/89b70ca4-9a66-4940-8759-c6b16fbda90d)

<details><summary>Request — OpenAPI sample</summary>

```json
{ "hPayOffInq": { "foracid": "660100100003" } }
```
</details>

<details><summary>Response — xlsx skeleton</summary>

```json
{
  "executeFinacleScriptCustomData": {
    "netPayofamt": "", "accountLiab": "",
    "pendingPrincipal": "", "pendingNormalInterest": "", "pendingPenalInterest": "", "pendingOverdueInterest": "",
    "interestSinceLastApplication": "", "interestRate": ""
  }
}
```
`netPayofamt` (one `f` in the source) is the amount to close the loan; `pending*` = its breakup. No `errors` array.
</details>

---

## 404 — getLoanOverduePositionEnquiry
`POST https://sandboxpocgatewayprod.idbi.bank.in/Development/getLoanOverduePositionEnquirytest`
[spec](../docs/idbireposne/Open%20API%20Specifications%20%288%29.yaml) · [portal doc](https://innobox.idbi.bank.in/api-specification/4abca8cd-a62c-4c0d-97f2-3e381e3d0eb6/b3c70029-5315-4237-a31a-a9975423ddb3)

<details><summary>Request — OpenAPI sample (note <code>loanOvduPosInqCustomData</code> is INSIDE <code>input</code> here)</summary>

```json
{
  "input": {
    "loanOvduPosInqCustomData": { "setId": "1234" },
    "asOnDate": "2026-05-27T07:08:38.194",
    "recCtrlIn": { "maxRec": "", "setNum": "" },
    "custId": { "custId": "68453002", "personName": { "lastName": "", "firstName": "", "name": "", "middleName": "", "titlePrefix": "" } },
    "curCode": "INR",
    "selRangeLoanAcctId": {
      "lowAcctId":  { "acctId": "660100100003", "acctCurr": "", "acctType": { "schmCode": "", "schmType": "" }, "bankInfo": { "branchId": "" } },
      "highAcctId": { "acctId": "660100100003", "acctCurr": "", "acctType": { "schmCode": "", "schmType": "" }, "bankInfo": { "branchId": "105", "bankId": "", "name": "", "branchName": "", "postAddr": { } } }
    }
  }
}
```
</details>

<details><summary>Response — xlsx skeleton</summary>

```json
{
  "result": {
    "loanOvduRec": [
      {
        "totalIntColl": { "amountValue": "number", "currencyCode": "" },
        "totalIntDmd":  { "amountValue": "number", "currencyCode": "" },
        "totalIntOvdu": { "amountValue": "number", "currencyCode": "" },
        "pTotalColl":   { "amountValue": "number", "currencyCode": "" },
        "pTotalDmd":    { "amountValue": "number", "currencyCode": "" },
        "pTotalOvdu":   { "amountValue": "number", "currencyCode": "" },
        "acctId": { "acctId": "", "acctType": { "schmCode": "", "schmType": "" }, "acctCurr": "", "bankInfo": { "branchId": "" } }
      }
    ],
    "recCtrlOut": { "isLastSet": "", "setNum": "int" }
  },
  "errors": [ { "code": "", "type": "", "message": "" } ]
}
```
Paginate with `recCtrlIn.setNum`; stop when `recCtrlOut.isLastSet`. Demands are cumulative across instalments.
</details>

---

## 402 — getLoanOverdueDetails
`POST https://sandboxpocgatewayprod.idbi.bank.in/Development/getLoanOverdueDetailstest`
[spec](../docs/idbireposne/Open%20API%20Specifications%20%287%29.yaml) · [portal doc](https://innobox.idbi.bank.in/api-specification/3925b39f-62de-409b-bac4-6f9067a983d7/fc3639ba-3f64-4f04-9c2e-e3fdfa00e320)
_(multi-account clone: `/Development/getLoanOverdueDetailstest01`)_

<details><summary>Request — OpenAPI sample</summary>

```json
{ "customerId": "98655854", "accountNo": "" }
```
`accountNo` empty = all loans for the customer.
</details>

<details><summary>Response — xlsx skeleton</summary>

```json
{
  "result": {
    "overdueDetails": [
      { "customerId": "", "accountId": "", "outstandingBal": "", "totalOverdueAmt": "", "overdueDate": "", "dpd": "", "npaStatus": "", "npaDate": "" }
    ],
    "errors": [ { "code": "", "type": "", "message": "" } ]
  }
}
```
`dpd` = days past due; `npaStatus` = SMA/NPA classification.
</details>

---

## 441 — fetchLoanAccountLimits
`POST https://sandboxpocgatewayprod.idbi.bank.in/Development/fetchLoanAccountLimitstest`
[spec](../docs/idbireposne/Open%20API%20Specifications%20%2828%29.yaml) · [portal doc](https://innobox.idbi.bank.in/api-specification/9ca0ce35-fb37-4cd7-a484-c5a93a9c717b/867b6626-f4a4-473b-a846-5127dbab9fdc)

<details><summary>Request — OpenAPI sample (flat — no <code>input</code> wrapper in the spec sample)</summary>

```json
{ "foracid": "660100100003" }
```
The xlsx showed `{ "input": { "foracid": "" } }`; the spec sample is flat. Try flat first.
</details>

<details><summary>Response — xlsx skeleton</summary>

```json
{
  "result": {
    "accountLimitDetails": {
      "acctDrwngPowerLimitHistMsgInq": { "olimitLL": [ { "applicableDate": "date", "drwngPower": { "amountValue": "", "currencyCode": "" }, "drwngPowerPcnt": { "value": "" } } ] },
      "acctSanctLimitHistMsg": { "olimitLL": [ { "applicableDate": "date", "expiryDate": "date", "sanctLimit": { "amountValue": "", "currencyCode": "" } } ] },
      "errors": [ { "code": "", "type": "", "message": "" } ]
    }
  }
}
```
Both `olimitLL[]` are history — sort by `applicableDate`.
</details>

---

## 442 — Fetch Customer Limit Details
`POST https://sandboxpocgatewayprod.idbi.bank.in/Development/fetchCustomerLimitDetailstest`
[spec](../docs/idbireposne/Open%20API%20Specifications%20%284%29.yaml) · [portal doc](https://innobox.idbi.bank.in/api-specification/ba793222-d48e-4b84-9ebc-6895f9abf33e/26292680-408a-40ac-a9d8-a54539b13d8b)

<details><summary>Request — OpenAPI sample (flat)</summary>

```json
{ "custCifId": "98655854" }
```
</details>

<details><summary>Response — xlsx skeleton</summary>

```json
{
  "result": {
    "customerLimitDetailsResponse": {
      "customerLimitDetails": [ { "crncyCode": "", "entityId": "", "limit": "", "limitEffDate": "", "sanctionDate": "Date", "expiryDate": "Date", "statusCode": "" } ],
      "limitHeaderDetails": { "custRating": "" },
      "totalLimit":       { "amountValue": " ", "currencyCode": "" },
      "totalOutstanding": { "amountValue": " ", "currencyCode": "" },
      "fundedLimit":      { "amountValue": " ", "currencyCode": "" },
      "nonFundedLimit":   { "amountValue": " ", "currencyCode": "" },
      "totalNodeLimit":       { "amountValue": " ", "currencyCode": "" },
      "totalNodeOutstanding": { "amountValue": " ", "currencyCode": "" },
      "ltCustomerDetails": { "accountManager": "", "custCifId": "", "customerID": "", "customerName": "" }
    },
    "errors": [ { "code": "", "type": "", "message": "" } ]
  }
}
```
</details>

---

## 428 — createLead
`POST https://sandboxpocgatewayprod.idbi.bank.in/Development/createLeadtest`
[spec](../docs/idbireposne/Open%20API%20Specifications%20%283%29.yaml) · [portal doc](https://innobox.idbi.bank.in/api-specification/d6c2980d-a6cd-434c-900b-bc53936a5ed9/5c611dd3-bb88-46e4-8944-dcabdabd5b96)

<details><summary>Request — OpenAPI sample (clean keys — the xlsx's trailing spaces were an artifact)</summary>

```json
{
  "input": {
    "leadType": "NEW",
    "customerType": "INDIVIDUAL",
    "firstName": "PRIYA",
    "lastName": "PATIL",
    "mobileNo": "9988776655",
    "emailId": "priya@gmail",
    "pancard": "FGHPP4567T",
    "addressLine1": "142 Lake View",
    "pincode": "411001",
    "state": "MH",
    "product": "Home Loan",
    "prodCategory": "Loans",
    "prodSubCategory": "Housing Loan",
    "estimatedAmount": "5000000",
    "solid": "0183",
    "leadChannel": "Online",
    "leadSource": "Website",
    "leadId": "LD20260622001"
  }
}
```
Full field list (from xlsx) also allows: `customerId, accountNo, title, middleName, gender, dob, telephoneNo, extension, addressLine2, landmark, country, district, city, area, branch, preferedDate, preferedTime, leadCreatedBy, image`.
</details>

<details><summary>Response — xlsx skeleton</summary>

```json
{ "result": { "leadId": "", "errors": [ { "code": "", "type": "", "message": "" } ] } }
```
</details>

---

## 508 — Fetch HRMS Employee Details
`POST https://sandboxpocgatewayprod.idbi.bank.in/Development/fetchHRMSEmployeeDetailstest`
[spec](../docs/idbireposne/Open%20API%20Specifications%20%2817%29.yaml) · [portal doc](https://innobox.idbi.bank.in/api-specification/d293668b-1aaf-4930-8df1-bf119c7967c0/6098f184-3bd6-42ac-87af-88f42c661d90)

<details><summary>Request — OpenAPI sample</summary>

```json
{ "ein": "137075", "otpRequired": "Y", "channelName": "MOBILE_APP" }
```
`otpRequired: "Y"` → the response carries an `otpId` and a second OTP-verify step is needed. Set `"N"` where allowed.
</details>

<details><summary>Response — xlsx skeleton</summary>

```json
{
  "failureStage": "",
  "otpId": "Number",
  "getHRMSEmployeeDetails": {
    "ein": "", "fullNameTitle": "", "grade": "", "gender": "", "empClass": "", "vertical": "",
    "organization": "", "position": "", "location": "", "region": "", "email": "", "sol": "", "zone": "",
    "supEin": "", "supFullNameTitle": "", "supGrade": "", "supEmail": "", "supposition": "", "supLocation": "", "supSol": "", "supZone": "", "posId": ""
  },
  "errors": [ { "code": "", "type": "", "message": "" } ]
}
```
`sup*` = reporting manager (lead routing / escalation).
</details>

---

## 415 — searchCkycDetails
`POST https://sandboxpocgatewayprod.idbi.bank.in/Development/searchCkycDetailstest`
[spec](../docs/idbireposne/Open%20API%20Specifications%20%2816%29.yaml) · [portal doc](https://innobox.idbi.bank.in/api-specification/0a85e451-deae-4e1c-bad2-b56de9accd23/3775f2c5-004b-4104-8961-f570bfaf1347)

**The gateway takes JSON** (the xlsx showed XML — use the JSON shape below). The underlying CKYC registry is XML; the Atlas gateway wraps it.

<details><summary>Request — OpenAPI sample (JSON, real values)</summary>

```json
{
  "input": {
    "apiToken": "3420c172-fba9-44ac-ba95-2a9bb66f788f",
    "parentCompany": "AAAAA8597P",
    "searchInCkycRequestDetails": [
      {
        "queryId": "1702508",
        "recordIdentifier": "1702508",
        "applicationFormNo": "FF01",
        "branchCode": "HOBRANCH",
        "inputIdType": "C",
        "inputIdNo": "FGHPP4567T",
        "apiTags": "",
        "sourceSystem": "Finacle",
        "sourceSystemSegment": "",
        "remarks": ""
      }
    ]
  }
}
```
Multiple objects in `searchInCkycRequestDetails[]` = batch lookup. Search by ID (`inputIdType`/`inputIdNo`) or by name + DOB (`firstName`/`lastName`/`dateOfBirth`/`gender`).
</details>

<details><summary>Response — xlsx skeleton (XML-structured; may come back as JSON with these keys)</summary>

```
result.requestStatus / requestRejectionCode / requestRejectionDescription
result.details.searchInCkycResponseDetails[]:
  queryId, transactionStatus, branchCode, recordIdentifier, applicationFormNo,
  ckycAvailable, ckycAccType, ckycId, ckycAge, ckycName, ckycFatherName,
  ckycGenDate, ckycRequestId, ckycRequestDate, ckycUpdatedDate,
  ckycIdDetails.id[]: { ckycAvailableIdType, ckycAvailableIdTypeStatus, ckycIdremarks }
result.errors: { code, type, message }
```
Ignore `ckycPhoto` / `ckycPhotoBytes` (image blob).
</details>

---

## 456 — performCustomerMasterDedupeCheck
**No OpenAPI spec in `idbireposne/`** — endpoint unknown. Only the xlsx skeleton is available; ask ACC/IDBI for the spec or find it in the portal catalogue.

<details><summary>Request — xlsx skeleton</summary>

```json
{
  "input": {
    "custId": "", "custName": "", "dateOfBirth": "", "panCardNo": "", "natIdCardNo": "",
    "emailId": "", "custMobileNo": "", "psprtNo": "", "ckycNo": "", "drivingLicence": "",
    "gstRegistration": "", "corporateIdentityNumber": "", "leiCode": "", "udyogAadharNo": "", "voterIdCard": ""
  }
}
```
</details>

<details><summary>Response — xlsx skeleton</summary>

```json
{
  "result": {
    "allMasterRecords": [
      {
        "customerCount": "", "custId": "", "custName": "", "dateOfBirth": "",
        "emailId": "", "panGirNum": "", "ckycNo": "", "natIdCardNum": "",
        "custSex": "", "customerType": "", "nriFLag": "", "minorFlg": "",
        "custComuAddr1": "", "custComuCityCode": "", "custComuStateCode": "", "custComuPinCode": "",
        "custPermAddr1": "", "custPermCityCode": "", "custPermStateCode": "", "custPermPinCode": "",
        "createdDate": "", "kycDueDate": "", "delFlg": "", "nonCustomerFlag": ""
      }
    ],
    "return": "",
    "errors": [ { "code": "", "type": "", "message": "" } ]
  }
}
```
`customerCount` on record 0 → match / no-match / multi-match. Ignore obsolete ID tags (NREGA/PIO/OIC/workPermit), `freeText15`.
</details>
