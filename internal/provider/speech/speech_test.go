package speech

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFactory_DefaultsToSarvam(t *testing.T) {
	if got := New(Config{}).Name(); got != "sarvam" {
		t.Errorf("empty provider => %q, want sarvam", got)
	}
	if got := New(Config{Provider: "AWS"}).Name(); got != "aws" {
		t.Errorf("aws => %q", got)
	}
	if got := New(Config{Provider: "nope"}).Name(); got != "sarvam" {
		t.Errorf("unknown => %q, want sarvam fallback", got)
	}
}

func TestSarvam_NoKey_NotConfigured(t *testing.T) {
	p := NewSarvamProvider(SarvamConfig{})
	if _, err := p.TextToSpeech(context.Background(), TTSRequest{Text: "hi"}); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("tts want ErrNotConfigured, got %v", err)
	}
	if _, err := p.SpeechToText(context.Background(), STTRequest{Audio: []byte("x")}); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("stt want ErrNotConfigured, got %v", err)
	}
}

func TestSarvam_GuardsEmptyInput(t *testing.T) {
	p := NewSarvamProvider(SarvamConfig{APIKey: "k"})
	if _, err := p.TextToSpeech(context.Background(), TTSRequest{Text: "  "}); err == nil {
		t.Error("want error for empty TTS text")
	}
	if _, err := p.SpeechToText(context.Background(), STTRequest{}); err == nil {
		t.Error("want error for empty STT audio")
	}
}

func TestSarvam_TTS_SendsExpectedPayloadAndForwardsBody(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("api-subscription-key") != "k" {
			t.Errorf("missing api key header")
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		_, _ = w.Write([]byte(`{"audios":["QUJD"]}`))
	}))
	defer srv.Close()

	p := NewSarvamProvider(SarvamConfig{APIKey: "k", HTTP: srv.Client()})
	old := sarvamTTSURL
	sarvamTTSURL = srv.URL
	defer func() { sarvamTTSURL = old }()

	res, err := p.TextToSpeech(context.Background(), TTSRequest{Text: "hello"})
	if err != nil {
		t.Fatalf("tts: %v", err)
	}
	if string(res.Audio) != `{"audios":["QUJD"]}` {
		t.Errorf("body not forwarded: %s", res.Audio)
	}
	if body["model"] != "bulbul:v3" || body["speaker"] != "shubh" || body["target_language_code"] != "en-IN" {
		t.Errorf("payload drifted from inline defaults: %+v", body)
	}
}

func TestAWS_Provisioned_NotConnected(t *testing.T) {
	p := NewAWSProvider(AWSConfig{Region: "ap-south-1"})
	if p.Name() != "aws" {
		t.Errorf("name = %q", p.Name())
	}
	if _, err := p.TextToSpeech(context.Background(), TTSRequest{Text: "hi"}); !errors.Is(err, ErrNotConfigured) ||
		!strings.Contains(err.Error(), "not connected") {
		t.Errorf("tts: want 'not connected' wrapping ErrNotConfigured, got %v", err)
	}
	if _, err := p.SpeechToText(context.Background(), STTRequest{Audio: []byte("x")}); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("stt: want ErrNotConfigured, got %v", err)
	}
}
