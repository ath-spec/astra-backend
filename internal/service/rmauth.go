package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/yourusername/astra-backend/internal/apiresponse"
	rmdomain "github.com/yourusername/astra-backend/internal/domain/rm"
	"github.com/yourusername/astra-backend/internal/repository"
)

// RMAccessTokenTTL is the lifetime of a staff access token. Kept short; the
// rotating refresh token (RefreshTokenTTL, shared with the user side) is
// what keeps a console session alive across restarts.
const RMAccessTokenTTL = 24 * time.Hour

// RMClaims is the JWT payload for the RM/Admin console. It carries the
// staff id and role, and is signed with RM_JWT_SECRET — a different key
// from the user JWT, so neither token type is ever valid on the other's
// endpoints.
type RMClaims struct {
	RMID uuid.UUID `json:"rm_id"`
	Role string    `json:"role"`
	jwt.RegisteredClaims
}

type RMAuthService struct {
	jwtSecret string
	repo      repository.RMUserRepository
}

func NewRMAuthService(jwtSecret string, repo repository.RMUserRepository) *RMAuthService {
	return &RMAuthService{jwtSecret: jwtSecret, repo: repo}
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

// HashPassword is exported so the seed command hashes seeded passwords the
// same way Login verifies them.
func HashPassword(plaintext string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(h), nil
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

// Login verifies email + password and returns a fresh token pair.
func (s *RMAuthService) Login(ctx context.Context, email, password string) (*rmdomain.TokenPair, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || password == "" {
		return nil, apiresponse.Validation("email and password are required")
	}
	staff, err := s.repo.GetByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	// Run a bcrypt comparison even when the account is missing, so response
	// timing doesn't reveal whether an email is registered.
	hash := "$2a$10$invalidinvalidinvalidinvalidinvalidinvalidinvalidinvalidinva"
	if staff != nil {
		hash = staff.PasswordHash
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil || staff == nil {
		return nil, fmt.Errorf("invalid credentials: %w", apiresponse.ErrUnauthorized)
	}
	if staff.Status != rmdomain.StatusActive {
		return nil, fmt.Errorf("account is inactive: %w", apiresponse.ErrForbidden)
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
