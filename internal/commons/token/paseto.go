package token

import (
	"time"
)

type Maker interface {
	CreateToken(p *Payload, duration time.Duration) (string, error)
}
