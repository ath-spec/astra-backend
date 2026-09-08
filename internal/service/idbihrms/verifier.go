// Package idbihrms is IDBI integration feature 7: an active-employee check
// for the RM/staff login flow, backed by 508 fetchHRMSEmployeeDetails.
//
// It implements service.HRMSVerifier. It is deliberately additive — it can
// only *withhold* a login code for an EIN that HRMS positively reports as
// not an active employee; it never issues tokens and never touches the
// local OTP store. Gated by IDBI_HRMS_LOGIN_ENABLED.
package idbihrms

import (
	"context"
	"strings"

	"github.com/yourusername/astra-backend/internal/provider/idbi"
)

// Provider is the subset of *idbi.Client this check needs.
type Provider interface {
	FetchHRMSEmployeeDetails(ctx context.Context, req idbi.FetchHRMSEmployeeRequest) (*idbi.FetchHRMSEmployeeResponse, error)
}

// Verifier checks an employee code against HRMS.
type Verifier struct {
	prov        Provider
	channelName string
}

// New builds a Verifier. channelName defaults to "MOBILE_APP".
func New(prov Provider, channelName string) *Verifier {
	if channelName == "" {
		channelName = "MOBILE_APP"
	}
	return &Verifier{prov: prov, channelName: channelName}
}

// VerifyActiveEmployee returns (true, nil) when HRMS recognises the code as
// an active employee, (false, nil) when it positively does not, and a
// non-nil error when the lookup itself failed (caller treats that as
// "cannot say" and proceeds).
//
// otpRequired is "N": we only want the identity check here. The RM login
// code is still the app's own, sent through the existing local OTP path —
// HRMS OTP delivery is simulated in the sandbox and not relied on.
func (v *Verifier) VerifyActiveEmployee(ctx context.Context, employeeCode string) (bool, error) {
	ein := strings.TrimSpace(employeeCode)
	if ein == "" {
		return false, nil
	}

	resp, err := v.prov.FetchHRMSEmployeeDetails(ctx, idbi.FetchHRMSEmployeeRequest{
		Ein:         ein,
		OtpRequired: "N",
		ChannelName: v.channelName,
	})
	if err != nil {
		return false, err
	}
	if resp == nil {
		return false, nil
	}
	if resp.FailureStage != nil && strings.TrimSpace(*resp.FailureStage) != "" {
		return false, nil
	}

	d := resp.GetHRMSEmployeeDetails
	if strings.TrimSpace(d.Ein) == "" {
		return false, nil
	}
	// empClass values seen: "Permanent", "Contract", "Retired", "Separated".
	switch strings.ToUpper(strings.TrimSpace(d.EmpClass)) {
	case "RETIRED", "SEPARATED", "TERMINATED", "INACTIVE", "SUSPENDED":
		return false, nil
	}
	return true, nil
}
