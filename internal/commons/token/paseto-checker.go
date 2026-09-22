package token

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/yourusername/astra-backend/internal/commons/util"

	"aidanwoods.dev/go-paseto"
)

type PasetoChecker struct {
	parser    paseto.Parser
	publicKey paseto.V4AsymmetricPublicKey
}

var checker *PasetoChecker

func VerifyToken(token string) (*Payload, error) {
	parsedToken, err := checker.parser.ParseV4Public(checker.publicKey, token, nil)
	if err != nil {
		if err.Error() == "this token has expired" {
			return nil, util.ErrExpiredToken
		}
		return nil, util.ErrInvalidToken
	}

	p, err := parsedToken.GetString("payload")
	if err != nil {
		return nil, util.ErrInvalidToken
	}

	// Local, not package-level: a shared package var here would let
	// concurrent requests race on and overwrite each other's payload.
	var payload Payload
	if err := json.Unmarshal([]byte(p), &payload); err != nil {
		return nil, util.ErrInvalidToken
	}

	return &payload, nil
}

// InitPasetoChecker builds the verifier from PASETO_PUBLIC_KEY (a
// hex-encoded Ed25519 public key matching the PASETO_SECRET_KEY used by
// NewPasetoMaker). Must never be hardcoded in source.
func InitPasetoChecker() error {
	publicKeyHex := os.Getenv("PASETO_PUBLIC_KEY")
	if publicKeyHex == "" {
		return fmt.Errorf("PASETO_PUBLIC_KEY is not set")
	}

	publicKey, err := paseto.NewV4AsymmetricPublicKeyFromHex(publicKeyHex)
	if err != nil {
		return err
	}

	checker = &PasetoChecker{
		parser:    paseto.NewParser(),
		publicKey: publicKey,
	}

	return nil
}
