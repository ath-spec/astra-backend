package speech

import "strings"

// Config selects and configures the active speech provider.
type Config struct {
	Provider string // "sarvam" (default) | "aws"
	Sarvam   SarvamConfig
	AWS      AWSConfig
}

// New builds the provider named by cfg.Provider. Unknown / empty => sarvam.
func New(cfg Config) Provider {
	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case "aws":
		return NewAWSProvider(cfg.AWS)
	default:
		return NewSarvamProvider(cfg.Sarvam)
	}
}
