package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"math/big"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/yourusername/astra-backend/internal/apiresponse"
	rmdomain "github.com/yourusername/astra-backend/internal/domain/rm"
	"github.com/yourusername/astra-backend/internal/repository"
)

const (
	// RMAccessTokenTTL is the lifetime of a staff access token. Kept short;
	// the rotating refresh token (RefreshTokenTTL, shared with the user
	// side) keeps a console session alive across restarts.
	RMAccessTokenTTL = 24 * time.Hour

	// rmOTPTTL is how long a login code stays valid once sent.
	rmOTPTTL = 10 * time.Minute

	// rmOTPMaxAttempts caps wrong guesses per issued code before it must be
	// re-requested.
	rmOTPMaxAttempts = 5
)

// RMClaims is the JWT payload for the RM/Admin console. It carries the staff
// id and role, and is signed with RM_JWT_SECRET — a different key from the
// user JWT, so neither token type is ever valid on the other's endpoints.
type RMClaims struct {
	RMID uuid.UUID `json:"rm_id"`
	Role string    `json:"role"`
	jwt.RegisteredClaims
}

type RMAuthService struct {
	jwtSecret  string
	otpDevCode string // when non-empty, always accepted (dev/testing only)
	repo       repository.RMUserRepository
}

func NewRMAuthService(jwtSecret, otpDevCode string, repo repository.RMUserRepository) *RMAuthService {
	return &RMAuthService{jwtSecret: jwtSecret, otpDevCode: otpDevCode, repo: repo}
}

// generateOpaqueToken returns a random refresh-token plaintext plus its
// SHA-256 hash (the value stored server-side), mirroring
// AuthService.GenerateRefreshToken on the user side.
func generateOpaqueToken() (plaintext string, hash string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("generate refresh token: %w", err)
	}
	plaintext = base64.RawURLEncoding.EncodeToString(buf)
	return plaintext, HashRefreshToken(plaintext), nil
}

// generateNumericOTP returns a cryptographically random 6-digit code.
func generateNumericOTP() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", fmt.Errorf("generate otp: %w", err)
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

func maskPhone(phone string) string {
	p := strings.TrimSpace(phone)
	if len(p) <= 4 {
		return "••••"
	}
	return "••••••" + p[len(p)-4:]
}

func (s *RMAuthService) generateAccessToken(rmID uuid.UUID, role string) (string, error) {
	claims := &RMClaims{
		RMID: rmID,
		Role: role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(RMAccessTokenTTL)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Audience:  jwt.ClaimStrings{"rm-console"},
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(s.jwtSecret))
	if err != nil {
		return "", fmt.Errorf("sign rm token: %w", err)
	}
	return signed, nil
}

// ValidateToken parses and verifies a staff access token.
func (s *RMAuthService) ValidateToken(tokenString string) (*RMClaims, error) {
	claims := &RMClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(s.jwtSecret), nil
	})
	if err != nil {
		return nil, fmt.Errorf("invalid rm token: %w", err)
	}
	if !token.Valid {
		return nil, errors.New("rm token is not valid")
	}
	return claims, nil
}

// issueTokenPair mints an access token and a fresh rotating refresh token,
// persisting the refresh token's hash.
func (s *RMAuthService) issueTokenPair(ctx context.Context, staff *rmdomain.StaffUser) (*rmdomain.TokenPair, error) {
	access, err := s.generateAccessToken(staff.ID, staff.Role)
	if err != nil {
		return nil, err
	}
	refreshPlain, refreshHash, err := generateOpaqueToken()
	if err != nil {
		return nil, err
	}
	if err := s.repo.CreateRefreshToken(ctx, staff.ID, refreshHash, time.Now().Add(RefreshTokenTTL)); err != nil {
		return nil, err
	}
	return &rmdomain.TokenPair{
		AccessToken:  access,
		RefreshToken: refreshPlain,
		Role:         staff.Role,
		RM:           staff.Public(),
	}, nil
}

// SendOTP starts a login: it resolves the staff member by employee code or
// email and dispatches a one-time code to their registered phone. The
// response never reveals whether the identifier matched an account (only
// that "if it exists, a code was sent") to avoid account enumeration — but
// it does return a masked phone when a code really went out, so the client
// can show "sent to ••••1234".
func (s *RMAuthService) SendOTP(ctx context.Context, identifier string) (*rmdomain.OTPSendResponse, error) {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return nil, apiresponse.Validation("identifier (employee code or email) is required")
	}
	resp := &rmdomain.OTPSendResponse{Sent: true}

	staff, err := s.repo.GetByIdentifier(ctx, identifier)
	if err != nil {
		return nil, err
	}
	if staff == nil || staff.Status != rmdomain.StatusActive || staff.PhoneNumber == nil || strings.TrimSpace(*staff.PhoneNumber) == "" {
		// Uniform response — don't disclose which of these was the reason.
		return resp, nil
	}

	code := s.otpDevCode
	if code == "" {
		code, err = generateNumericOTP()
		if err != nil {
			return nil, err
		}
	}

	// Only the newest code for a staff member is valid at a time.
	if err := s.repo.InvalidateOTPs(ctx, staff.ID); err != nil {
		return nil, err
	}
	if err := s.repo.CreateOTP(ctx, staff.ID, HashRefreshToken(code), time.Now().Add(rmOTPTTL)); err != nil {
		return nil, err
	}

	// TODO: no SMS provider is wired in yet (mirrors the user-side OTP in
	// handler/auth.go). The code is logged so it can be read from server
	// logs during development; set RM_OTP_DEV_CODE for a fixed testable
	// code. Replace with a real SMS send once a provider is chosen.
	log.Printf("MOCK RM OTP: code %s for %s (%s) -> %s", code, staff.EmployeeCode, staff.Email, *staff.PhoneNumber)

	resp.MaskedPhone = maskPhone(*staff.PhoneNumber)
	return resp, nil
}

// VerifyOTP completes a login: it checks the code against the newest active
// one for that staff member and, on success, returns a fresh token pair.
func (s *RMAuthService) VerifyOTP(ctx context.Context, identifier, otp string) (*rmdomain.TokenPair, error) {
	identifier = strings.TrimSpace(identifier)
	otp = strings.TrimSpace(otp)
	if identifier == "" || otp == "" {
		return nil, apiresponse.Validation("identifier and otp are required")
	}

	staff, err := s.repo.GetByIdentifier(ctx, identifier)
	if err != nil {
		return nil, err
	}
	if staff == nil {
		return nil, fmt.Errorf("invalid code: %w", apiresponse.ErrUnauthorized)
	}
	if staff.Status != rmdomain.StatusActive {
		return nil, fmt.Errorf("account is inactive: %w", apiresponse.ErrForbidden)
	}

	code, err := s.repo.LatestActiveOTP(ctx, staff.ID)
	if err != nil {
		return nil, err
	}
	if code == nil {
		return nil, fmt.Errorf("no active code — request a new one: %w", apiresponse.ErrUnauthorized)
	}
	if code.Attempts >= rmOTPMaxAttempts {
		return nil, fmt.Errorf("too many attempts — request a new code: %w", apiresponse.ErrUnauthorized)
	}
	if HashRefreshToken(otp) != code.CodeHash {
		if err := s.repo.IncrementOTPAttempts(ctx, code.ID); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("invalid code: %w", apiresponse.ErrUnauthorized)
	}

	if err := s.repo.ConsumeOTP(ctx, code.ID); err != nil {
		return nil, err
	}
	return s.issueTokenPair(ctx, staff)
}

// Refresh rotates a still-valid refresh token for a new token pair.
func (s *RMAuthService) Refresh(ctx context.Context, refreshToken string) (*rmdomain.TokenPair, error) {
	if refreshToken == "" {
		return nil, apiresponse.Validation("refresh_token is required")
	}
	hash := HashRefreshToken(refreshToken)
	rt, err := s.repo.GetRefreshToken(ctx, hash)
	if err != nil {
		return nil, err
	}
	if rt == nil || rt.RevokedAt != nil || time.Now().After(rt.ExpiresAt) {
		return nil, fmt.Errorf("invalid or expired refresh token: %w", apiresponse.ErrUnauthorized)
	}
	if err := s.repo.RevokeRefreshToken(ctx, hash); err != nil {
		return nil, err
	}
	staff, err := s.repo.GetByID(ctx, rt.RMID)
	if err != nil {
		return nil, err
	}
	if staff == nil {
		return nil, fmt.Errorf("staff account no longer exists: %w", apiresponse.ErrUnauthorized)
	}
	if staff.Status != rmdomain.StatusActive {
		return nil, fmt.Errorf("account is inactive: %w", apiresponse.ErrForbidden)
	}
	return s.issueTokenPair(ctx, staff)
}

// Logout revokes the given refresh token.
func (s *RMAuthService) Logout(ctx context.Context, refreshToken string) error {
	if refreshToken == "" {
		return nil
	}
	return s.repo.RevokeRefreshToken(ctx, HashRefreshToken(refreshToken))
}

// UpdateProfile lets an authenticated staff member edit their own name,
// login email and OTP phone number.
func (s *RMAuthService) UpdateProfile(ctx context.Context, rmID uuid.UUID, req rmdomain.UpdateProfileRequest) (*rmdomain.Public, error) {
	upd := rmdomain.UpdateRMRequest{}

	if req.Name != nil {
		n := strings.TrimSpace(*req.Name)
		if n == "" {
			return nil, apiresponse.Validation("name cannot be empty")
		}
		upd.Name = &n
	}
	if req.Email != nil {
		e := strings.ToLower(strings.TrimSpace(*req.Email))
		if !strings.Contains(e, "@") {
			return nil, apiresponse.Validation("a valid email is required")
		}
		if existing, err := s.repo.GetByEmail(ctx, e); err != nil {
			return nil, err
		} else if existing != nil && existing.ID != rmID {
			return nil, fmt.Errorf("that email is already in use: %w", apiresponse.ErrConflict)
		}
		upd.Email = &e
	}
	if req.Phone != nil {
		p := strings.TrimSpace(*req.Phone)
		if p == "" {
			return nil, apiresponse.Validation("phone cannot be empty — login codes are sent there")
		}
		upd.Phone = &p
	}

	staff, err := s.repo.Update(ctx, rmID, upd)
	if err != nil {
		return nil, err
	}
	if staff == nil {
		return nil, fmt.Errorf("staff account not found: %w", apiresponse.ErrUnauthorized)
	}
	pub := staff.Public()
	return &pub, nil
}

// Me returns the staff profile for an authenticated request.
func (s *RMAuthService) Me(ctx context.Context, rmID uuid.UUID) (*rmdomain.Public, error) {
	staff, err := s.repo.GetByID(ctx, rmID)
	if err != nil {
		return nil, err
	}
	if staff == nil {
		return nil, fmt.Errorf("staff account not found: %w", apiresponse.ErrUnauthorized)
	}
	pub := staff.Public()
	return &pub, nil
}
