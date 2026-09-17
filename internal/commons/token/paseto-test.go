package token

import (
	"time"
)

type PasetoMakerTest struct {
}

func (maker *PasetoMakerTest) CreateToken(p *Payload, duration time.Duration) (string, error) {
	return "", nil
}

func NewPasetoMakerTest() Maker {
	return &PasetoMakerTest{}
}
