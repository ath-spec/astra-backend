package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	rmdomain "github.com/yourusername/astra-backend/internal/domain/rm"
	"github.com/yourusername/astra-backend/internal/repository"
)

// stubRMRepo satisfies repository.RMUserRepository; only the three methods
// SendOTP touches are implemented, the rest panic if unexpectedly called.
type stubRMRepo struct {
	repository.RMUserRepository
	staff       *rmdomain.StaffUser
	createdOTPs int
}

func (r *stubRMRepo) GetByIdentifier(context.Context, string) (*rmdomain.StaffUser, error) {
	return r.staff, nil
}
func (r *stubRMRepo) InvalidateOTPs(context.Context, uuid.UUID) error { return nil }
func (r *stubRMRepo) CreateOTP(context.Context, uuid.UUID, string, time.Time) error {
	r.createdOTPs++
	return nil
}

type stubHRMS struct {
	ok  bool
	err error
}

func (s stubHRMS) VerifyActiveEmployee(context.Context, string) (bool, error) { return s.ok, s.err }

func activeStaff() *rmdomain.StaffUser {
	phone := "9876500000"
	return &rmdomain.StaffUser{
		ID:           uuid.New(),
		EmployeeCode: "137075",
		Email:        "rm@bank.com",
		Role:         rmdomain.RoleRM,
		Status:       rmdomain.StatusActive,
		PhoneNumber:  &phone,
	}
}

func TestSendOTP_NoHRMSVerifier_Unchanged(t *testing.T) {
	repo := &stubRMRepo{staff: activeStaff()}
	s := NewRMAuthService("secret", "123456", repo)

	res, err := s.SendOTP(context.Background(), "137075")
	if err != nil {
		t.Fatalf("SendOTP: %v", err)
	}
	if !res.Sent || res.MaskedPhone == "" {
		t.Errorf("expected a code to be sent, got %+v", res)
	}
	if repo.createdOTPs != 1 {
		t.Errorf("expected 1 OTP created, got %d", repo.createdOTPs)
	}
}

func TestSendOTP_HRMSApproves_CodeSent(t *testing.T) {
	repo := &stubRMRepo{staff: activeStaff()}
	s := NewRMAuthService("secret", "123456", repo)
	s.UseHRMSVerifier(stubHRMS{ok: true})

	res, err := s.SendOTP(context.Background(), "137075")
	if err != nil {
		t.Fatalf("SendOTP: %v", err)
	}
	if !res.Sent || res.MaskedPhone == "" || repo.createdOTPs != 1 {
		t.Errorf("HRMS-approved staff should get a code: %+v, otps=%d", res, repo.createdOTPs)
	}
}

func TestSendOTP_HRMSRejects_NoCode(t *testing.T) {
	repo := &stubRMRepo{staff: activeStaff()}
	s := NewRMAuthService("secret", "123456", repo)
	s.UseHRMSVerifier(stubHRMS{ok: false})

	res, err := s.SendOTP(context.Background(), "137075")
	if err != nil {
		t.Fatalf("SendOTP must stay uniform, not error: %v", err)
	}
	if !res.Sent {
		t.Error("response must stay uniform (Sent=true) to avoid enumeration")
	}
	if res.MaskedPhone != "" {
		t.Errorf("no masked phone when no code went out, got %q", res.MaskedPhone)
	}
	if repo.createdOTPs != 0 {
		t.Errorf("HRMS-rejected staff must not get a code, got %d", repo.createdOTPs)
	}
}

func TestSendOTP_HRMSRejects_AdminBypasses(t *testing.T) {
	staff := activeStaff()
	staff.Role = rmdomain.RoleAdmin
	repo := &stubRMRepo{staff: staff}
	s := NewRMAuthService("secret", "123456", repo)
	s.UseHRMSVerifier(stubHRMS{ok: false}) // HRMS would reject

	if _, err := s.SendOTP(context.Background(), "AD001"); err != nil {
		t.Fatalf("SendOTP: %v", err)
	}
	if repo.createdOTPs != 1 {
		t.Errorf("admin login must skip the HRMS check, got %d otps", repo.createdOTPs)
	}
}

func TestSendOTP_HRMSErrors_FailsOpen(t *testing.T) {
	repo := &stubRMRepo{staff: activeStaff()}
	s := NewRMAuthService("secret", "123456", repo)
	s.UseHRMSVerifier(stubHRMS{err: errors.New("hrms 503")})

	res, err := s.SendOTP(context.Background(), "137075")
	if err != nil {
		t.Fatalf("SendOTP: %v", err)
	}
	if repo.createdOTPs != 1 {
		t.Errorf("an HRMS outage must not lock staff out — code should still send, got %d", repo.createdOTPs)
	}
	_ = res
}
