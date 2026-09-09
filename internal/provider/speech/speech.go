// Package speech is the provider-agnostic seam for text-to-speech and
// speech-to-text.
//
// Today the only wired backend is Sarvam. An AWS backend (Polly for TTS,
// Transcribe for STT) is provisioned behind the same interface and becomes
// active by setting SPEECH_PROVIDER=aws once the AWS side is ready — no
// call-site changes.
package speech

import (
	"context"
	"errors"
)

// TTSRequest is a text-to-speech call.
type TTSRequest struct {
	Text     string
	Language string // BCP-47-ish, e.g. "en-IN"; provider maps as needed
	Voice    string // provider-specific voice / speaker id; "" => provider default
}

// TTSResult carries the synthesized audio.
type TTSResult struct {
	Audio    []byte
	MIME     string // e.g. "audio/wav", "audio/mpeg"
	Provider string
}

// STTRequest is a speech-to-text call.
type STTRequest struct {
	Audio    []byte
	Filename string // original filename, helps the provider sniff the format
	Language string // "" => auto-detect where supported
}

// STTResult carries the transcript.
type STTResult struct {
	Text     string
	Language string // detected / used language, when reported
	Provider string
}

// Provider is implemented by each speech backend.
type Provider interface {
	TextToSpeech(ctx context.Context, req TTSRequest) (*TTSResult, error)
	SpeechToText(ctx context.Context, req STTRequest) (*STTResult, error)
	Name() string
}

// ErrNotConfigured means the selected backend has no usable wiring yet.
var ErrNotConfigured = errors.New("speech: provider not configured")
