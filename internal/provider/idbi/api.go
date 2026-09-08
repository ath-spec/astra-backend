package idbi

import "context"

// One method per usable IDBI operation. Each returns a typed response or an
// error: *APIError for a 4xx from the gateway, ErrUnavailable for transport /
// 5xx, ErrNotAvailable for endpoints that don't return usable data in this
// environment.
//
// data_source legend (mirrors docs/idbi-api-catalog.json):
//   idbi       — real data from the sandbox, wire directly
//   simulated  — call succeeds, shape is real, no live counterparty
//   mock       — unusable in sandbox; caller must supply its own value

// --- Accounts / identity -------------------------------------------------

// PerformAccountEnquiry (365) — data_source: idbi.
// acctId -> custId, registered name+address, all balance types.
func (c *Client) PerformAccountEnquiry(ctx context.Context, req PerformAccountEnquiryRequest) (*PerformAccountEnquiryResponse, error) {
	var out PerformAccountEnquiryResponse
	if err := c.call(ctx, "performAccountEnquiry", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetCustomerAccountsByCustID (394) — data_source: idbi.
// cifId -> all accounts with type + balance.
func (c *Client) GetCustomerAccountsByCustID(ctx context.Context, req GetCustomerAccountsByCustIDRequest) (*GetCustomerAccountsByCustIDResponse, error) {
	var out GetCustomerAccountsByCustIDResponse
	if err := c.call(ctx, "getCustomerAccountsByCustId", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetFullAccountStatement (393) — data_source: idbi.
// Page while resp.Result.HasMoreData == "Y".
func (c *Client) GetFullAccountStatement(ctx context.Context, req FullStatementRequest) (*FullStatementResponse, error) {
	var out FullStatementResponse
	if err := c.call(ctx, "getFullAccountStatementWithPagination", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AccountLienEnquiry (362) — data_source: idbi.
// Address-validated: req.Input.BankInfo.PostAddr must be the account's
// registered address (from a prior PerformAccountEnquiry).
func (c *Client) AccountLienEnquiry(ctx context.Context, req LienEnquiryRequest) (*LienEnquiryResponse, error) {
	var out LienEnquiryResponse
	if err := c.call(ctx, "accountLienEnquiry", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// --- Account Aggregator (all simulated) --------------------------------

// RequestConsent (590) — data_source: simulated. Returns a fixed handle.
func (c *Client) RequestConsent(ctx context.Context, req RequestConsentRequest) (*RequestConsentResponse, error) {
	var out RequestConsentResponse
	if err := c.call(ctx, "requestConsentFromFinPro", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetConsentList (591) — data_source: idbi (the list itself is real).
func (c *Client) GetConsentList(ctx context.Context, req GetConsentListRequest) (*GetConsentListResponse, error) {
	var out GetConsentListResponse
	if err := c.call(ctx, "getConsentListFromFinPro", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetWebRedirectionURL (592) — data_source: simulated. The returned URL is a
// dummy in sandbox; the caller decides whether to open it (live) or
// stub-advance (sandbox) based on IDBI_AA_REDIRECT_MODE.
func (c *Client) GetWebRedirectionURL(ctx context.Context, req GetWebRedirectionURLRequest) (*GetWebRedirectionURLResponse, error) {
	var out GetWebRedirectionURLResponse
	if err := c.call(ctx, "getWebRedirectionEncryptedURL", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GenerateDecryptedResponse (593) — data_source: simulated. Canned decode.
// data.Errorcode == "0" means approved.
func (c *Client) GenerateDecryptedResponse(ctx context.Context, req GenerateDecryptedResponseRequest) (*GenerateDecryptedResponse, error) {
	var out GenerateDecryptedResponse
	if err := c.call(ctx, "generateDecryptedResponseFromFinPro", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetAAStatement (595) — data_source: idbi (usable fixture shape).
func (c *Client) GetAAStatement(ctx context.Context, req GetAAStatementRequest) (*GetAAStatementResponse, error) {
	var out GetAAStatementResponse
	if err := c.call(ctx, "getAccountStatement", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetAAStatementFromFinPro (739) — data_source: idbi. Same shape as 595,
// slightly richer profile block.
func (c *Client) GetAAStatementFromFinPro(ctx context.Context, req GetAAStatementRequest) (*GetAAStatementResponse, error) {
	var out GetAAStatementResponse
	if err := c.call(ctx, "getAccountStatementFromFinPro", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// --- Loans (all real) --------------------------------------------------

// GetLoanAccountDetails (391) — data_source: idbi. Address-validated like 362.
func (c *Client) GetLoanAccountDetails(ctx context.Context, req GetLoanAccountDetailsRequest) (*GetLoanAccountDetailsResponse, error) {
	var out GetLoanAccountDetailsResponse
	if err := c.call(ctx, "getLoanAccountDetails", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetLoanOverdueDetails (402) — data_source: idbi.
func (c *Client) GetLoanOverdueDetails(ctx context.Context, req GetLoanOverdueDetailsRequest) (*GetLoanOverdueDetailsResponse, error) {
	var out GetLoanOverdueDetailsResponse
	if err := c.call(ctx, "getLoanOverdueDetails", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetLoanOverduePosition (404) — data_source: idbi. Paginated
// (resp.RecCtrlOut.IsLastSet == "Y" on the final page).
func (c *Client) GetLoanOverduePosition(ctx context.Context, req GetLoanOverduePositionRequest) (*GetLoanOverduePositionResponse, error) {
	var out GetLoanOverduePositionResponse
	if err := c.call(ctx, "getLoanOverduePositionEnquiry", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// InquireHPPayoff (538) — data_source: idbi. NOTE the gateway path is
// "InquireHPAyoff" (IDBI's typo) — handled here.
func (c *Client) InquireHPPayoff(ctx context.Context, req InquireHPPayoffRequest) (*InquireHPPayoffResponse, error) {
	var out InquireHPPayoffResponse
	if err := c.call(ctx, "InquireHPAyoff", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// FetchLoanAccountLimits (441) — data_source: idbi.
func (c *Client) FetchLoanAccountLimits(ctx context.Context, req FetchLoanAccountLimitsRequest) (*FetchLoanAccountLimitsResponse, error) {
	var out FetchLoanAccountLimitsResponse
	if err := c.call(ctx, "fetchLoanAccountLimits", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GenerateRepaymentSchedule (473) — data_source: idbi.
func (c *Client) GenerateRepaymentSchedule(ctx context.Context, req GenerateRepaymentScheduleRequest) (*GenerateRepaymentScheduleResponse, error) {
	var out GenerateRepaymentScheduleResponse
	if err := c.call(ctx, "generateLoanRepaymentSchedule", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// --- Corporate / MSME ------------------------------------------------

// FetchCustomerLimitDetails (442) — data_source: idbi.
func (c *Client) FetchCustomerLimitDetails(ctx context.Context, req FetchCustomerLimitDetailsRequest) (*FetchCustomerLimitDetailsResponse, error) {
	var out FetchCustomerLimitDetailsResponse
	if err := c.call(ctx, "fetchCustomerLimitDetails", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// --- KYC / staff / CRM ---------------------------------------------

// SearchCkycDetails (415) — data_source: idbi. JSON in, JSON out.
func (c *Client) SearchCkycDetails(ctx context.Context, req SearchCkycRequest) (*SearchCkycResponse, error) {
	var out SearchCkycResponse
	if err := c.call(ctx, "searchCkycDetails", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// FetchHRMSEmployeeDetails (508) — data_source: idbi (OTP simulated).
func (c *Client) FetchHRMSEmployeeDetails(ctx context.Context, req FetchHRMSEmployeeRequest) (*FetchHRMSEmployeeResponse, error) {
	var out FetchHRMSEmployeeResponse
	if err := c.call(ctx, "fetchHRMSEmployeeDetails", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateLead (428) — data_source: idbi. 400 (*APIError) on malformed PAN.
func (c *Client) CreateLead(ctx context.Context, req CreateLeadRequest) (*CreateLeadResponse, error) {
	var out CreateLeadResponse
	if err := c.call(ctx, "createLead", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// --- Unusable in sandbox ------------------------------------------

// FetchCibilScore (408) — data_source: mock. The sandbox rejects the bureau
// credentials and returns no score. Always returns ErrNotAvailable so the
// service layer falls back to a mock credit score. Wire the real parse when
// ACC provides working credentials.
func (c *Client) FetchCibilScore(ctx context.Context, _ FetchCibilScoreRequest) (any, error) {
	return nil, ErrNotAvailable
}

// FetchLoanInterestRates (433) — data_source: mock. Sandbox returns an
// unrelated object, not a rate table. Returns ErrNotAvailable.
func (c *Client) FetchLoanInterestRates(ctx context.Context, _ any) (any, error) {
	return nil, ErrNotAvailable
}

// PerformCustomerMasterDedupeCheck (456) — data_source: mock. No OpenAPI spec
// published; endpoint path unknown. Returns ErrNotAvailable until ACC
// provides the spec.
func (c *Client) PerformCustomerMasterDedupeCheck(ctx context.Context, _ any) (any, error) {
	return nil, ErrNotAvailable
}
