package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// RefreshTokenTTL is how long an issued refresh token stays valid. The
// access token (JWT) is short-lived (24h, see GenerateToken); the refresh
// token is what lets a client stay "logged in" across app restarts beyond
// that without re-doing OTP verification.
const RefreshTokenTTL = 30 * 24 * time.Hour

type Claims struct {
	UserID uuid.UUID `json:"user_id"`
	jwt.RegisteredClaims
}

type AuthService struct {
	jwtSecret string
}

func NewAuthService(jwtSecret string) *AuthService {
	return &AuthService{
		jwtSecret: jwtSecret,
	}
}

func (s *AuthService) GenerateToken(userID uuid.UUID) (string, error) {
	// Set expiration time to 24 hours
	expirationTime := time.Now().Add(24 * time.Hour)

	claims := &Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(s.jwtSecret))

	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}

	return tokenString, nil
}

// GenerateRefreshToken returns a fresh cryptographically random refresh
// token (the plaintext value to hand back to the client) alongside its
// SHA-256 hash (the value actually stored in the database, so a stolen DB
// backup alone can't be replayed as a valid refresh token).
func (s *AuthService) GenerateRefreshToken() (plaintext string, hash string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("generate refresh token: %w", err)
	}
	plaintext = base64.RawURLEncoding.EncodeToString(buf)
	return plaintext, HashRefreshToken(plaintext), nil
}

// HashRefreshToken is exported so callers verifying a client-supplied
// refresh token hash it the same way before looking it up.
func HashRefreshToken(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

func (s *AuthService) ValidateToken(tokenString string) (*Claims, error) {
	claims := &Claims{}

	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.jwtSecret), nil
	})

	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	if !token.Valid {
		return nil, fmt.Errorf("token is not valid")
	}

	return claims, nil
}
