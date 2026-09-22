package token

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/yourusername/astra-backend/internal/commons/util"

	"aidanwoods.dev/go-paseto"
)

type PasetoMaker struct {
	secretKey paseto.V4AsymmetricSecretKey
}

func (maker *PasetoMaker) CreateToken(p *Payload, duration time.Duration) (string, error) {
	payload := newPayload(p, duration)

	token := paseto.NewToken()
	token.SetIssuedAt(payload.IssuedAt)
	token.SetNotBefore(time.Now())
	token.SetExpiration(payload.ExpiresAt)

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", util.ErrInternal
	}

	token.SetString("payload", string(jsonData))

	return token.V4Sign(maker.secretKey, nil), nil
}

// NewPasetoMaker builds a maker from PASETO_SECRET_KEY (a hex-encoded
// Ed25519 seed for a PASETO v4 asymmetric key pair). The key must never be
// hardcoded in source — it's injected as a Secret (Secrets Manager via
// External Secrets Operator in EKS, plain env var in local dev).
func NewPasetoMaker() (Maker, error) {
	secretKeyHex := os.Getenv("PASETO_SECRET_KEY")
	if secretKeyHex == "" {
		return nil, fmt.Errorf("PASETO_SECRET_KEY is not set")
	}

	secretKey, err := paseto.NewV4AsymmetricSecretKeyFromHex(secretKeyHex)
	if err != nil {
		return nil, err
	}

	maker := &PasetoMaker{
		secretKey: secretKey,
	}
	return maker, nil
}
