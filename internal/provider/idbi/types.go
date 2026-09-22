package idbi

import "encoding/json"

// Request/response types for the IDBI Atlas APIs, shaped from the live sandbox
// capture in testdata/. Notes that apply everywhere:
//
//   - Every numeric value arrives as a JSON string ("56780.25", "0").
//   - Money is usually {"amountValue","currencyCode"}; a few APIs use
//     {"amount","currency"} (see Money vs Amount below). Normalise on ingest.
//   - Absent values appear as "", "NULL" (literal), or null — treat all three
//     as empty. Use NullableDate for date fields.
//   - There are two IDBI id spaces: cifId (customer information file) and
//     custId (customer id). 365 maps acctId -> custId+name; 394 maps
//     cifId -> accounts. You need both to bootstrap identity.

// Money is the {amountValue,currencyCode} shape used by most APIs.
type Money struct {
	AmountValue  string `json:"amountValue"`
	CurrencyCode string `json:"currencyCode"`
}

// Amount is the {amount,currency} shape used by 442 fetchCustomerLimitDetails.
type Amount struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

// NullableDate is a date field that may be "", "NULL", or an ISO string.
type NullableDate string

// IsZero reports whether the date is absent under any of IDBI's conventions.
func (d NullableDate) IsZero() bool {
	switch string(d) {
	case "", "NULL", "null":
		return true
	}
	return false
}

// PersonName recurs across account/loan responses.
type PersonName struct {
	TitlePrefix string `json:"titlePrefix"`
	FirstName   string `json:"firstName"`
	MiddleName  string `json:"middleName"`
	LastName    string `json:"lastName"`
	Name        string `json:"name"` // pre-joined, no spaces: "PRIYAPATIL"
}

// PostAddr is the registered address block. 362 and 391 cross-validate the
// address you SEND against the account's registered address — populate this
// from a prior 365 call, do not hand-craft it.
type PostAddr struct {
	Addr1      string `json:"addr1"`
	Addr2      string `json:"addr2"`
	Addr3      string `json:"addr3"`
	City       string `json:"city"`
	StateProv  string `json:"stateProv"`
	PostalCode string `json:"postalCode"`
	Country    string `json:"country"`
	AddrType   string `json:"addrType"`
}

type BankInfo struct {
	BankID     string   `json:"bankId"`
	Name       string   `json:"name"`
	BranchID   string   `json:"branchId"`
	BranchName string   `json:"branchName"`
	PostAddr   PostAddr `json:"postAddr"`
}

type AcctType struct {
	SchmCode string `json:"schmCode"`
	SchmType string `json:"schmType"` // SAVINGS, CURRENT, TERM_DEPOSIT, SALARY, ...
}

// ---------------------------------------------------------------------------
// 365 performAccountEnquiry  — flat envelope
// acctId -> custId, name, registered address, and all balance types.
// This is the identity-resolution entry point (account no -> custId).
// ---------------------------------------------------------------------------

type PerformAccountEnquiryRequest struct {
	AcctID string `json:"acctId"`
}

// BalTypes seen: LEDGER, AVAIL, EFFAVL, LIEN, FLOAT, DRWPWR, ACCBAL.
type AccountBalance struct {
	BalType string `json:"balType"`
	BalAmt  Money  `json:"balAmt"`
}

type PerformAccountEnquiryResponse struct {
	AcctID             string           `json:"acctId"`
	AcctType           AcctType         `json:"acctType"`
	AcctCurr           string           `json:"acctCurr"`
	CustID             string           `json:"custId"`
	PersonName         PersonName       `json:"personName"`
	AcctOpenDt         string           `json:"acctOpenDt"`
	BankAcctStatusCode string           `json:"bankAcctStatusCode"` // "A" = active
	BankInfo           BankInfo         `json:"bankInfo"`
	AcctBal            []AccountBalance `json:"acctBal"`
	CustStat           struct {
		RefCode    string `json:"refCode"`
		RefRecType string `json:"refRecType"`
		RefDesc    string `json:"refDesc"`
	} `json:"custStat"`
}

// Balance returns the amount for a balType (e.g. "AVAIL"), or "" if absent.
func (r *PerformAccountEnquiryResponse) Balance(balType string) string {
	for _, b := range r.AcctBal {
		if b.BalType == balType {
			return b.BalAmt.AmountValue
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// 394 getCustomerAccountsByCustId  — flat envelope
// cifId -> list of accounts with type + balance. NOTE: request carries cifId,
// the response is keyed by cifId, but per-account ownership resolves to custId
// via 365. acctType here is a coarse label (SBA / SAVINGS / CURRENT / ...).
// ---------------------------------------------------------------------------

type GetCustomerAccountsByCustIDRequest struct {
	Input struct {
		AcctType string `json:"acctType"` // "SBA"
		BranchID string `json:"branchId"`
		CifID    string `json:"cifId"`
	} `json:"input"`
	Txn string `json:"txn"` // "E"
}

type CustomerAccountInfo struct {
	AcctCurrCode string `json:"acctCurrCode"`
	AcctNumber   string `json:"acctNumber"`
	AcctBalance  Money  `json:"acctBalance"`
	AcctType     string `json:"acctType"`
}

type GetCustomerAccountsByCustIDResponse struct {
	NumOfAccounts       string                `json:"numOfAccounts"`
	CustomerAccountInfo []CustomerAccountInfo `json:"customerAccountInfo"`
	AcctTypeRequested   string                `json:"acctTypeRequested"`
	CifID               string                `json:"cifId"`
}

// ---------------------------------------------------------------------------
// 393 getFullAccountStatementWithPagination  — "result" envelope
// The core statement feed for spend analytics. Per-txn fields are nested under
// transactionSummary. txnType is "D"/"C". Paginate while hasMoreData == "Y"
// using the last row's txnId/pstdDate/txnSrlNo.
// ---------------------------------------------------------------------------

type FullStatementRequest struct {
	Input struct {
		Acid              string `json:"acid"`
		BranchID          string `json:"branchId"`
		FromDate          string `json:"fromDate"` // "2025-05-01T00:00:00.000"
		ToDate            string `json:"toDate"`
		SortIn            string `json:"sortIn"` // "D" descending
		PaginationDetails struct {
			LastBalance  Money  `json:"lastBalance"`
			LastPstdDate string `json:"lastPstdDate"`
			LastTxnDate  string `json:"lastTxnDate"`
			LastTxnID    string `json:"lastTxnId"`
			LastTxnSrlNo string `json:"lastTxnSrlNo"`
		} `json:"paginationDetails"`
	} `json:"input"`
}

type StatementTxn struct {
	PstdDate           string `json:"pstdDate"`
	TransactionSummary struct {
		InstrumentID string `json:"instrumentId"`
		TxnAmt       Money  `json:"txnAmt"`
		TxnDate      string `json:"txnDate"`
		TxnDesc      string `json:"txnDesc"` // generic in sandbox ("S1 TXN 1")
		TxnType      string `json:"txnType"` // "D" | "C"
	} `json:"transactionSummary"`
	TxnBalance Money  `json:"txnBalance"`
	TxnCat     string `json:"txnCat"`
	TxnID      string `json:"txnId"`
	TxnSrlNo   string `json:"txnSrlNo"`
	ValueDate  string `json:"valueDate"`
}

type FullStatementResponse struct {
	Result struct {
		AccountBalances struct {
			Acid               string `json:"acid"`
			AvailableBalance   Money  `json:"availableBalance"`
			BranchID           string `json:"branchId"`
			CurrencyCode       string `json:"currencyCode"`
			FFDBalance         Money  `json:"fFDBalance"`
			FloatingBalance    Money  `json:"floatingBalance"`
			LedgerBalance      Money  `json:"ledgerBalance"`
			UserDefinedBalance Money  `json:"userDefinedBalance"`
		} `json:"accountBalances"`
		HasMoreData        string         `json:"hasMoreData"` // "Y" | "N"
		TransactionDetails []StatementTxn `json:"transactionDetails"`
	} `json:"result"`
}

// ---------------------------------------------------------------------------
// 362 accountLienEnquiry  — "result" envelope, errors:[]
// lienDetails is nested INSIDE bankInfo. Usable balance = balance - sum of
// active (isDeleted=="N") newLienAmt. Address-validated: send the account's
// registered PostAddr from a prior 365.
// ---------------------------------------------------------------------------

type LienEnquiryRequest struct {
	Input struct {
		AcctID     string   `json:"acctId"`
		ModuleType string   `json:"moduleType"` // "DEPOSIT"
		AcctCurr   string   `json:"acctCurr"`
		AcctType   AcctType `json:"acctType"`
		BankInfo   BankInfo `json:"bankInfo"`
	} `json:"input"`
}

type LienDetails struct {
	NewLienAmt Money `json:"newLienAmt"`
	OldLienAmt Money `json:"oldLienAmt"`
	LienDate   struct {
		StartDate NullableDate `json:"startDate"`
		EndDate   NullableDate `json:"endDate"`
	} `json:"lienDate"`
	ReasonCode string `json:"reasonCode"`
	Remarks    string `json:"remarks"`
	IsDeleted  string `json:"isDeleted"` // "N" = active
	LienID     string `json:"lienId"`
}

type LienEnquiryResponse struct {
	Result struct {
		AcctID     string   `json:"acctId"`
		ModuleType string   `json:"moduleType"`
		AcctCurr   string   `json:"acctCurr"`
		AcctType   AcctType `json:"acctType"`
		BankInfo   struct {
			BankInfo
			LienDetails LienDetails `json:"lienDetails"`
		} `json:"bankInfo"`
	} `json:"result"`
	Errors []APIErrorEnvelopeItem `json:"errors"`
}

// APIErrorEnvelopeItem is one entry of the {"errors":[...]} success/failure array.
type APIErrorEnvelopeItem struct {
	Code        string `json:"code"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// ---------------------------------------------------------------------------
// Account Aggregator chain (590 -> 592 -> 593 -> 595/739, + 497/498 webhooks).
// SIMULATED end-to-end in the sandbox: 590 returns a fixed handle, 592's
// webRedirectionUrl is a dummy that goes nowhere, 593 returns a canned decode.
// Build the flow to this shape; gate the redirect step behind a stub/live
// switch (IDBI_AA_REDIRECT_MODE). 595/739 statement payloads ARE usable as
// realistic fixture data.
// ---------------------------------------------------------------------------

// 590 requestConsentFromFinPro
type RequestConsentRequest struct {
	PartyIdentifierType  string `json:"partyIdentifierType"` // "MOBILE"
	PartyIdentifierValue string `json:"partyIdentifierValue"`
	ProductID            string `json:"productID"` // "TEST"
	AccountID            string `json:"accountID"`
	Vua                  string `json:"vua"` // "<mobile>@onemoney"
	TransactionID        string `json:"transactionID"`
}

type RequestConsentResponse struct {
	Ver  string `json:"ver"`
	Data struct {
		Status        string `json:"status"` // "PENDING"
		ConsentHandle string `json:"consent_handle"`
	} `json:"data"`
	Status string `json:"status"` // "success"
}

// 591 getConsentListFromFinPro
type GetConsentListRequest struct {
	PartyIdentifierType  string `json:"partyIdentifierType"`
	PartyIdentifierValue string `json:"partyIdentifierValue"`
	ProductID            string `json:"productID"`
	AccountID            string `json:"accountID"`
	Vua                  string `json:"vua"`
}

type ConsentLinkedAccount struct {
	FipName             string `json:"fipName"`
	FipID               string `json:"fipId"`
	AccountType         string `json:"accountType"`
	LinkReferenceNumber string `json:"linkReferenceNumber"`
	MaskedAccountNumber string `json:"maskedAccountNumber"`
	FiType              string `json:"fiType"`
}

type Consent struct {
	ConsentID           string                 `json:"consentID"`
	Status              string                 `json:"status"` // "ACTIVE"
	ConsentHandle       string                 `json:"consent_handle"`
	ProductID           string                 `json:"productID"`
	AccountID           string                 `json:"accountID"` // comma-joined
	AaID                string                 `json:"aaId"`
	Vua                 string                 `json:"vua"`
	ConsentCreationData string                 `json:"consentCreationData"`
	Accounts            []ConsentLinkedAccount `json:"accounts"`
}

type GetConsentListResponse struct {
	Status    string                 `json:"status"` // "SUCCESS"
	Ver       string                 `json:"ver"`
	Data      []Consent              `json:"data"`
	ErrorCode *string                `json:"errorCode"`
	ErrorMsg  *string                `json:"errorMsg"`
	Errors    []APIErrorEnvelopeItem `json:"errors"`
}

// 592 getWebRedirectionEncryptedURL — dummy URL in sandbox.
type GetWebRedirectionURLRequest struct {
	ConsentHandle string `json:"consentHandle"`
	RedirectURL   string `json:"redirectUrl"`
}

type GetWebRedirectionURLResponse struct {
	Status string `json:"status"`
	Ver    string `json:"ver"`
	Data   []struct {
		WebRedirectionURL string `json:"webRedirectionUrl"`
	} `json:"data"`
}

// 593 generateDecryptedResponseFromFinPro — canned decode in sandbox.
type GenerateDecryptedResponseRequest struct {
	WebRedirectionURL struct {
		Ecres   string `json:"ecres"`
		Resdate string `json:"resdate"`
		Fi      string `json:"fi"`
	} `json:"webRedirectionURL"`
}

type GenerateDecryptedResponse struct {
	Ver  string `json:"ver"`
	Data struct {
		Redirect  string  `json:"redirect"`
		SessionID string  `json:"sessionid"`
		Pan       *string `json:"pan"`
		Srcref    string  `json:"srcref"`
		Userid    string  `json:"userid"`
		Errorcode string  `json:"errorcode"` // "0" = approved
		Email     *string `json:"email"`
		Status    string  `json:"status"` // "S"
		Txnid     string  `json:"txnid"`
	} `json:"data"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

// 595 / 739 getAccountStatement(FromFinPro) — AA statement, "data[]" envelope.
// The richest statement shape: holder KYC profile + IFSC/MICR + transactions.
// Some sandbox samples use "transactionalBalance" instead of "currentBalance";
// AAStatementTxn accepts both.
type GetAAStatementRequest struct {
	ConsentID     string   `json:"consentId"`
	LinkRefNumber []string `json:"linkRefNumber"`
}

type AAHolder struct {
	Name           string `json:"name"`
	Dob            string `json:"dob"`
	Pan            string `json:"pan"`
	Email          string `json:"email"`
	Mobile         string `json:"mobile"`
	Address        string `json:"address"`
	Nominee        string `json:"nominee"`
	Landline       string `json:"landline"`
	CkycRegistered string `json:"ckycRegistered"`
}

type AAStatementTxn struct {
	Mode                 string `json:"mode"` // UPI, NEFT, NACH, ECS, CHEQUE, CARD, OTHERS
	Type                 string `json:"type"` // "DEBIT" | "CREDIT"
	TxnID                string `json:"txnId"`
	Amount               string `json:"amount"`
	Narration            string `json:"narration"`
	Reference            string `json:"reference"`
	TransactionTimestamp string `json:"transactionTimestamp"`
	ValueDate            string `json:"valueDate"`
	CurrentBalance       string `json:"currentBalance"`
	TransactionalBalance string `json:"transactionalBalance"` // alt key in some samples
}

// Balance returns whichever running-balance key the sample used.
func (t AAStatementTxn) Balance() string {
	if t.CurrentBalance != "" {
		return t.CurrentBalance
	}
	return t.TransactionalBalance
}

type AAStatementAccount struct {
	LinkReferenceNumber string `json:"linkReferenceNumber"`
	MaskedAccountNumber string `json:"maskedAccountNumber"`
	FiType              string `json:"fiType"`
	Bank                string `json:"bank"`
	Profile             struct {
		Holders struct {
			Type   string     `json:"type"`
			Holder []AAHolder `json:"Holder"`
		} `json:"Holders"`
	} `json:"Profile"`
	Summary struct {
		Ifsc            string `json:"ifsc"`
		Branch          string `json:"branch"`
		Status          string `json:"status"`
		Currency        string `json:"currency"`
		MicrCode        string `json:"micrCode"`
		AccountType     string `json:"accountType"`
		OpeningDate     string `json:"openingDate"`
		CurrentBalance  string `json:"currentBalance"`
		BalanceDateTime string `json:"balanceDateTime"`
	} `json:"Summary"`
	Transactions struct {
		StartDate   string           `json:"startDate"`
		EndDate     string           `json:"endDate"`
		Transaction []AAStatementTxn `json:"Transaction"`
	} `json:"Transactions"`
}

type GetAAStatementResponse struct {
	Ver    string               `json:"ver"`
	Status string               `json:"status"`
	Data   []AAStatementAccount `json:"data"`
}

// 497 pushConsentNotification / 498 pushDataNotification — INBOUND webhooks we
// host. IDBI POSTs these to us; we ack with HTTP 200 (empty body). These
// structs are for decoding the inbound body, not for calling out.
type PushConsentNotification struct {
	TransactionID string `json:"transactionID"`
	Timestamp     string `json:"timestamp"`
	ConsentHandle string `json:"consentHandle"`
	EventType     string `json:"eventType"`   // "CONSENT"
	EventStatus   string `json:"eventStatus"` // "CONSENT_APPROVED" | ...
	ConsentID     string `json:"consentId"`
	EventMessage  string `json:"eventMessage"`
	Vua           string `json:"vua"`
	ProductID     string `json:"productID"`
	AccountID     string `json:"accountID"`
	FetchType     string `json:"fetchType"`
	ConsentExpiry string `json:"consentExpiry"`
}

type PushDataNotification struct {
	TransactionID  string `json:"transactionID"`
	Timestamp      string `json:"timestamp"`
	ConsentHandle  string `json:"consentHandle"`
	EventType      string `json:"eventType"`   // "DATA"
	EventStatus    string `json:"eventStatus"` // "DATA_READY"
	ConsentID      string `json:"consentId"`
	Vua            string `json:"vua"`
	SessionID      string `json:"sessionId"`
	FirstTimeFetch string `json:"firstTimeFetch"`
	LinkRefNumbers []struct {
		LinkRefNumber       string `json:"linkRefNumber"`
		FiStatus            string `json:"fiStatus"`
		FipName             string `json:"fipName"`
		FipID               string `json:"fipId"`
		MaskedAccountNumber string `json:"maskedAccountNumber"`
	} `json:"linkRefNumbers"`
}

// ---------------------------------------------------------------------------
// Loans (391, 402, 404, 538, 441, 473). All real in the sandbox.
// ---------------------------------------------------------------------------

// 391 getLoanAccountDetails — "result" envelope. ~4KB, mostly boilerplate.
type GetLoanAccountDetailsRequest struct {
	Input struct {
		LoanAcctID LoanAcctID `json:"loanAcctId"`
		CustID     struct {
			CustID string `json:"custId"`
		} `json:"custId"`
		Channel   string `json:"channel"` // "API"
		RequestID string `json:"requestId"`
		ReqDate   string `json:"reqDate"` // "2026-07-16"
	} `json:"input"`
}

type LoanAcctID struct {
	AcctID   string   `json:"acctId"`
	AcctType AcctType `json:"acctType"`
	AcctCurr string   `json:"acctCurr"`
	BankInfo BankInfo `json:"bankInfo"`
}

type GetLoanAccountDetailsResponse struct {
	Result struct {
		LoanAcctID LoanAcctID `json:"loanAcctId"`
		NetIntRate struct {
			Value string `json:"value"` // "8.75"
		} `json:"netIntRate"`
		AcctOpenDt string `json:"acctOpenDt"`
		ModeOfOper string `json:"modeOfOper"`
		CustID     struct {
			CustID     string     `json:"custId"`
			PersonName PersonName `json:"personName"`
		} `json:"custId"`
		AmtAlreadyDisb  Money `json:"amtAlreadyDisb"`
		AmtAvailForDisb Money `json:"amtAvailForDisb"`
		DisbAmt         Money `json:"disbAmt"`
		LoanGenDetails  struct {
			LoanAmt          Money  `json:"loanAmt"`
			LoanPeriodDays   string `json:"loanPeriodDays"`
			LoanPeriodMonths string `json:"loanPeriodMonths"`
			RePmtMethod      string `json:"rePmtMethod"` // "EMI"
		} `json:"loanGenDetails"`
	} `json:"result"`
}

// 402 getLoanOverdueDetails — flat, {"overdueDetails":[...]}
type GetLoanOverdueDetailsRequest struct {
	CustomerID string `json:"customerId"`
	AccountNo  string `json:"accountNo"` // "" = all
}

type LoanOverdueDetail struct {
	CustomerID      string       `json:"customerId"`
	AccountID       string       `json:"accountId"`
	OutstandingBal  string       `json:"outstandingBal"`
	OverdueDate     NullableDate `json:"overdueDate"`
	Dpd             string       `json:"dpd"`       // days past due
	NpaStatus       string       `json:"npaStatus"` // "SA" | SMA-0/1/2 | NPA
	TotalOverdueAmt string       `json:"totalOverdueAmt"`
	NpaDate         NullableDate `json:"npaDate"`
}

type GetLoanOverdueDetailsResponse struct {
	OverdueDetails []LoanOverdueDetail `json:"overdueDetails"`
}

// 404 getLoanOverduePositionEnquiry — flat, {"loanOvduRec":[...]}, paginated.
type GetLoanOverduePositionRequest struct {
	Input struct {
		LoanOvduPosInqCustomData struct {
			SetID string `json:"setId"`
		} `json:"loanOvduPosInqCustomData"`
		AsOnDate  string `json:"asOnDate"`
		RecCtrlIn struct {
			MaxRec string `json:"maxRec"`
			SetNum string `json:"setNum"`
		} `json:"recCtrlIn"`
		CustID struct {
			PersonName PersonName `json:"personName"`
			CustID     string     `json:"custId"`
		} `json:"custId"`
		CurCode            string `json:"curCode"`
		SelRangeLoanAcctID struct {
			LowAcctID  LoanAcctID `json:"lowAcctId"`
			HighAcctID LoanAcctID `json:"highAcctId"`
		} `json:"selRangeLoanAcctId"`
	} `json:"input"`
}

type LoanOverdueRec struct {
	TotalIntColl Money      `json:"totalIntColl"`
	TotalIntDmd  Money      `json:"totalIntDmd"`
	TotalIntOvdu Money      `json:"totalIntOvdu"`
	PTotalColl   Money      `json:"pTotalColl"`
	PTotalDmd    Money      `json:"pTotalDmd"`
	PTotalOvdu   Money      `json:"pTotalOvdu"`
	AcctID       LoanAcctID `json:"acctId"`
}

type GetLoanOverduePositionResponse struct {
	LoanOvduRec []LoanOverdueRec `json:"loanOvduRec"`
	RecCtrlOut  struct {
		IsLastSet string `json:"isLastSet"` // "Y"
		SetNum    string `json:"setNum"`
	} `json:"recCtrlOut"`
}

// 538 inquireHPPayoff — flat, {"executeFinacleScriptCustomData":{...}}
// NOTE: request op name is "InquireHPAyoff" (typo in IDBI's path). netPayofamt
// is spelled with one "f".
type InquireHPPayoffRequest struct {
	HPayOffInq struct {
		Foracid string `json:"foracid"`
	} `json:"hPayOffInq"`
}

type InquireHPPayoffResponse struct {
	ExecuteFinacleScriptCustomData struct {
		NetPayofamt                  string `json:"netPayofamt"`
		AccountLiab                  string `json:"accountLiab"`
		PendingPrincipal             string `json:"pendingPrincipal"`
		PendingNormalInterest        string `json:"pendingNormalInterest"`
		PendingPenalInterest         string `json:"pendingPenalInterest"`
		PendingOverdueInterest       string `json:"pendingOverdueInterest"`
		InterestSinceLastApplication string `json:"interestSinceLastApplication"`
		InterestRate                 string `json:"interestRate"`
	} `json:"executeFinacleScriptCustomData"`
}

// 441 fetchLoanAccountLimits — flat, drawing-power + sanction history.
type FetchLoanAccountLimitsRequest struct {
	Foracid string `json:"foracid"`
}

type LimitHistItem struct {
	ApplicableDate string `json:"applicableDate"`
	ExpiryDate     string `json:"expiryDate"`
	DrwngPower     Money  `json:"drwngPower"`
	DrwngPowerPcnt struct {
		Value string `json:"value"`
	} `json:"drwngPowerPcnt"`
	SanctLimit Money `json:"sanctLimit"`
}

type FetchLoanAccountLimitsResponse struct {
	AccountLimitDetails struct {
		AcctDrwngPowerLimitHistMsgInq struct {
			OlimitLL []LimitHistItem `json:"olimitLL"`
		} `json:"acctDrwngPowerLimitHistMsgInq"`
		AcctSanctLimitHistMsg struct {
			OlimitLL []LimitHistItem `json:"olimitLL"`
		} `json:"acctSanctLimitHistMsg"`
	} `json:"accountLimitDetails"`
}

// 473 generateLoanRepaymentSchedule — flat, {"loanModellingSchOutputVO":{...}}
type GenerateRepaymentScheduleRequest struct {
	LoanModellingMsgInputVO struct {
		MandatoryParameters struct {
			CrncyCode       string `json:"crncyCode"`
			OriginationDate string `json:"originationDate"`
			SchmCode        struct {
				SchmCode string `json:"schmCode"`
			} `json:"schmCode"`
		} `json:"mandatoryParameters"`
		AdvanceParameters struct {
			EIFormula string `json:"eIFormula"`
		} `json:"advanceParameters"`
		Variables struct {
			LoanAmount Money `json:"loanAmount"`
			IntRate    struct {
				Value string `json:"Value"`
			} `json:"intRate"`
			NoOfInstalmnts string `json:"noOfInstalmnts"`
		} `json:"Variables"`
	} `json:"loanModellingMsgInputVO"`
}

type AmortRow struct {
	AmortStruct struct {
		CummPrincAmt     Money  `json:"cummPrincAmt"`
		CummIntAmt       Money  `json:"cummIntAmt"`
		PrincOutStanding Money  `json:"princOutStanding"`
		InstlAmt         Money  `json:"instlAmt"`
		IntAmt           Money  `json:"intAmt"`
		FlowDate         string `json:"flowDate"`
		PrincAmt         Money  `json:"princAmt"`
		FlowDesc         string `json:"flowDesc"`
	} `json:"amortStruct"`
	Key struct {
		SerialNum string `json:"serial_num"`
	} `json:"Key"`
}

type GenerateRepaymentScheduleResponse struct {
	LoanModellingSchOutputVO struct {
		LamodRepaymentLL []struct {
			FlowAmt         Money  `json:"flowAmt"`
			NoOfInstalments string `json:"noOfInstalments"`
			Freq            string `json:"Freq"` // "M"
			FlowDesc        string `json:"flowDesc"`
		} `json:"lamodRepaymentLL"`
		OamortLL []AmortRow `json:"oamortLL"`
	} `json:"loanModellingSchOutputVO"`
}

// ---------------------------------------------------------------------------
// 442 fetchCustomerLimitDetails — flat. Uses {amount,currency} (Amount), not
// {amountValue,currencyCode}. Real MSME/corporate exposure data.
// ---------------------------------------------------------------------------

type FetchCustomerLimitDetailsRequest struct {
	CustCifID string `json:"custCifId"`
}

type FetchCustomerLimitDetailsResponse struct {
	CustomerSummary struct {
		CustomerName   string `json:"customerName"`
		CustCifID      string `json:"custCifId"`
		AccountManager string `json:"accountManager"`
		CustomerID     string `json:"customerID"`
		CustRating     string `json:"custRating"`
	} `json:"customerSummary"`
	ExposureSummary struct {
		TotalLimit           Amount `json:"totalLimit"`
		FundedLimit          Amount `json:"fundedLimit"`
		NonFundedLimit       Amount `json:"nonFundedLimit"`
		TotalOutstanding     Amount `json:"totalOutstanding"`
		TotalNodeLimit       Amount `json:"totalNodeLimit"`
		TotalNodeOutstanding Amount `json:"totalNodeOutstanding"`
	} `json:"exposureSummary"`
	CustomerLimits []struct {
		Currency     string `json:"currency"`
		SanctionDate string `json:"sanctionDate"` // "dd-mm-yyyy"
		ExpiryDate   string `json:"expiryDate"`
	} `json:"customerLimits"`
	EmptyLimitRecords string `json:"emptyLimitRecords"`
}

// ---------------------------------------------------------------------------
// 415 searchCkycDetails — "result" envelope. Gateway takes JSON (not the XML
// the xlsx implied). Response carries a base64 photo — ignore those bytes.
// ---------------------------------------------------------------------------

type CkycRequestDetail struct {
	QueryID             string `json:"queryId"`
	RecordIdentifier    string `json:"recordIdentifier"`
	ApplicationFormNo   string `json:"applicationFormNo"`
	BranchCode          string `json:"branchCode"`
	InputIDType         string `json:"inputIdType"` // "C" = PAN
	InputIDNo           string `json:"inputIdNo"`
	APITags             string `json:"apiTags"`
	SourceSystem        string `json:"sourceSystem"`
	SourceSystemSegment string `json:"sourceSystemSegment"`
	Remarks             string `json:"remarks"`
}

type SearchCkycRequest struct {
	Input struct {
		APIToken                   string              `json:"apiToken"`
		ParentCompany              string              `json:"parentCompany"`
		SearchInCkycRequestDetails []CkycRequestDetail `json:"searchInCkycRequestDetails"`
	} `json:"input"`
}

type SearchCkycResponse struct {
	Result struct {
		ParentCompany string `json:"parentCompany"`
		RequestStatus string `json:"requestStatus"`
		Details       struct {
			SearchInCkycResponseDetails struct {
				QueryID           string `json:"queryId"`
				TransactionStatus string `json:"transactionStatus"` // "CkycSuccess"
				BranchCode        string `json:"branchCode"`
				CkycAvailable     string `json:"ckycAvailable"` // "Yes"
				CkycAccType       string `json:"ckycAccType"`
				CkycID            string `json:"ckycId"`
				CkycAge           string `json:"ckycAge"`
				CkycFatherName    string `json:"ckycFatherName"`
				CkycGenDate       string `json:"ckycGenDate"`
				CkycName          string `json:"ckycName"`
				CkycRequestID     string `json:"ckycRequestId"`
				CkycRequestDate   string `json:"ckycRequestDate"`
				CkycUpdatedDate   string `json:"ckycUpdatedDate"`
				CkycIDDetails     struct {
					ID []struct {
						CkycAvailableIDType       string `json:"ckycAvailableIdType"`
						CkycAvailableIDTypeStatus string `json:"ckycAvailableIdTypeStatus"`
						CkycIDremarks             string `json:"ckycIdremarks"`
					} `json:"id"`
				} `json:"ckycIdDetails"`
				// ckycPhoto / ckycPhotoBytes deliberately omitted.
			} `json:"searchInCkycResponseDetails"`
		} `json:"details"`
	} `json:"result"`
}

// ---------------------------------------------------------------------------
// 508 fetchHRMSEmployeeDetails — flat. Backs RM/staff login: verify the EIN is
// a real active employee; otpRequired:"Y" makes HRMS send an OTP (otpId comes
// back). Simulated OTP in sandbox (ein 137075).
// ---------------------------------------------------------------------------

type FetchHRMSEmployeeRequest struct {
	Ein         string `json:"ein"`
	OtpRequired string `json:"otpRequired"` // "Y" | "N"
	ChannelName string `json:"channelName"` // "MOBILE_APP"
}

type FetchHRMSEmployeeResponse struct {
	FailureStage           *string `json:"failureStage"`
	OtpID                  int64   `json:"otpId"`
	GetHRMSEmployeeDetails struct {
		Ein              string `json:"ein"`
		FullNameTitle    string `json:"fullNameTitle"`
		Grade            string `json:"grade"`
		Gender           string `json:"gender"`
		EmpClass         string `json:"empClass"`
		Vertical         string `json:"vertical"`
		Organization     string `json:"organization"`
		Position         string `json:"position"`
		Location         string `json:"location"`
		Region           string `json:"region"`
		Email            string `json:"email"`
		Sol              string `json:"sol"`
		Zone             string `json:"zone"`
		SupEin           string `json:"supEin"`
		SupFullNameTitle string `json:"supFullNameTitle"`
		SupGrade         string `json:"supGrade"`
		SupEmail         string `json:"supEmail"`
		SupLocation      string `json:"supLocation"`
		SupSol           string `json:"supSol"`
		SupZone          string `json:"supZone"`
		PosID            string `json:"posId"`
	} `json:"getHRMSEmployeeDetails"`
	Errors []APIErrorEnvelopeItem `json:"errors"`
}

// ---------------------------------------------------------------------------
// 428 createLead — "result" envelope. Real. 400 on malformed PAN.
// ---------------------------------------------------------------------------

type CreateLeadRequest struct {
	Input struct {
		LeadType        string `json:"leadType"` // "NEW"
		CustomerType    string `json:"customerType"`
		FirstName       string `json:"firstName"`
		LastName        string `json:"lastName"`
		MobileNo        string `json:"mobileNo"`
		EmailID         string `json:"emailId"`
		Pancard         string `json:"pancard"` // must match AAAAA9999A
		AddressLine1    string `json:"addressLine1"`
		Pincode         string `json:"pincode"`
		State           string `json:"state"`
		Product         string `json:"product"`
		ProdCategory    string `json:"prodCategory"`
		ProdSubCategory string `json:"prodSubCategory"`
		EstimatedAmount string `json:"estimatedAmount"`
		Solid           string `json:"solid"`
		LeadChannel     string `json:"leadChannel"`
		LeadSource      string `json:"leadSource"`
		LeadID          string `json:"leadId"`
	} `json:"input"`
}

type CreateLeadResponse struct {
	Result struct {
		Message string                 `json:"message"` // "Lead already created" | ...
		LeadID  string                 `json:"leadId"`
		Errors  []APIErrorEnvelopeItem `json:"errors"`
	} `json:"result"`
}

// ---------------------------------------------------------------------------
// 408 fetchCibilScore — request kept for completeness; the sandbox rejects the
// embedded bureau credentials and returns no score. FetchCibilScore returns
// ErrNotAvailable until ACC supplies working credentials.
// ---------------------------------------------------------------------------

type FetchCibilScoreRequest struct {
	Raw json.RawMessage `json:"-"` // pass the spec sample through unchanged
}
