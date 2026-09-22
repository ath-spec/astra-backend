package idbihrms

import (
	"context"
	"errors"
	"testing"

	"github.com/yourusername/astra-backend/internal/provider/idbi"
)

type fakeProv struct {
	resp *idbi.FetchHRMSEmployeeResponse
	err  error
	gots []idbi.FetchHRMSEmployeeRequest
}

func (f *fakeProv) FetchHRMSEmployeeDetails(_ context.Context, req idbi.FetchHRMSEmployeeRequest) (*idbi.FetchHRMSEmployeeResponse, error) {
	f.gots = append(f.gots, req)
	return f.resp, f.err
}

func active(ein string) *idbi.FetchHRMSEmployeeResponse {
	r := &idbi.FetchHRMSEmployeeResponse{OtpID: 982374}
	r.GetHRMSEmployeeDetails.Ein = ein
	r.GetHRMSEmployeeDetails.EmpClass = "Permanent"
	return r
}

func TestVerifier_ActiveEmployee(t *testing.T) {
	fp := &fakeProv{resp: active("137075")}
	v := New(fp, "")
	ok, err := v.VerifyActiveEmployee(context.Background(), "137075")
	if err != nil || !ok {
		t.Fatalf("want (true,nil), got (%v,%v)", ok, err)
	}
	if len(fp.gots) != 1 || fp.gots[0].OtpRequired != "N" {
		t.Errorf("expected otpRequired=N identity check, got %+v", fp.gots)
	}
}

func TestVerifier_NotAnEmployee(t *testing.T) {
	fp := &fakeProv{resp: &idbi.FetchHRMSEmployeeResponse{}} // empty details
	v := New(fp, "")
	ok, err := v.VerifyActiveEmployee(context.Background(), "999999")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if ok {
		t.Error("empty HRMS details must not count as active")
	}
}

func TestVerifier_RetiredEmployee(t *testing.T) {
	r := active("137075")
	r.GetHRMSEmployeeDetails.EmpClass = "Retired"
	v := New(&fakeProv{resp: r}, "")
	ok, _ := v.VerifyActiveEmployee(context.Background(), "137075")
	if ok {
		t.Error("retired employee must not count as active")
	}
}

func TestVerifier_FailureStage(t *testing.T) {
	msg := "IDENTITY_MISMATCH"
	r := active("137075")
	r.FailureStage = &msg
	v := New(&fakeProv{resp: r}, "")
	ok, _ := v.VerifyActiveEmployee(context.Background(), "137075")
	if ok {
		t.Error("failureStage set must not count as active")
	}
}

func TestVerifier_TransportErrorPropagates(t *testing.T) {
	v := New(&fakeProv{err: errors.New("gateway 503")}, "")
	ok, err := v.VerifyActiveEmployee(context.Background(), "137075")
	if err == nil {
		t.Fatal("expected the transport error to propagate so the caller can fall through")
	}
	if ok {
		t.Error("ok must be false on error")
	}
}

func TestVerifier_EmptyCode(t *testing.T) {
	fp := &fakeProv{resp: active("137075")}
	v := New(fp, "")
	ok, err := v.VerifyActiveEmployee(context.Background(), "  ")
	if err != nil || ok {
		t.Errorf("empty code => (false,nil), got (%v,%v)", ok, err)
	}
	if len(fp.gots) != 0 {
		t.Error("empty code must not call HRMS")
	}
}
