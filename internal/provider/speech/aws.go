package speech

import (
	"context"
	"fmt"
)

// AWSProvider is the provisioned Amazon Polly (TTS) + Transcribe (STT)
// backend. Interface, config and wiring are in place; the SDK calls are
// switched on after the AWS review by setting SPEECH_PROVIDER=aws and
// filling AWS_POLLY_* / AWS_TRANSCRIBE_*.
//
// When implementing:
//   - TTS: github.com/aws/aws-sdk-go-v2/service/polly SynthesizeSpeech,
//     VoiceId from Voice/config, OutputFormat "mp3";
//   - STT: github.com/aws/aws-sdk-go-v2/service/transcribestreaming for
//     low-latency, or StartTranscriptionJob (S3) for batch;
//   - credentials from the default AWS chain — never hard-coded.
type AWSProvider struct {
	region     string
	pollyVoice string
	sttLang    string
}

// AWSConfig configures the AWS speech backend.
type AWSConfig struct {
	Region     string // AWS_REGION / SPEECH_AWS_REGION
	PollyVoice string // AWS_POLLY_VOICE, e.g. "Kajal" (en-IN neural)
	STTLang    string // AWS_TRANSCRIBE_LANGUAGE, e.g. "en-IN"
}

func NewAWSProvider(cfg AWSConfig) *AWSProvider {
	return &AWSProvider{region: cfg.Region, pollyVoice: cfg.PollyVoice, sttLang: cfg.STTLang}
}

func (p *AWSProvider) Name() string { return "aws" }

func (p *AWSProvider) TextToSpeech(ctx context.Context, req TTSRequest) (*TTSResult, error) {
	return nil, fmt.Errorf("aws polly backend not connected yet (region=%q voice=%q): %w",
		p.region, orDefault(req.Voice, p.pollyVoice), ErrNotConfigured)
}

func (p *AWSProvider) SpeechToText(ctx context.Context, req STTRequest) (*STTResult, error) {
	return nil, fmt.Errorf("aws transcribe backend not connected yet (region=%q): %w", p.region, ErrNotConfigured)
}
