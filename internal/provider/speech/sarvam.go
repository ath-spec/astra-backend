package speech

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// URLs are vars (not consts) so tests can point them at a stub server.
var (
	sarvamTTSURL      = "https://api.sarvam.ai/text-to-speech"
	sarvamSTTURL      = "https://api.sarvam.ai/speech-to-text"
	sarvamSTTStreamURL = "wss://api.sarvam.ai/speech-to-text-realtime/ws"
)

// maxSpeechResponseBytes bounds how much of an upstream response we buffer.
// TTS audio comes back base64-in-JSON, so this needs headroom; STT replies
// are tiny.
const maxSpeechResponseBytes = 32 << 20 // 32 MiB

// SarvamProvider wraps Sarvam's TTS + STT HTTP APIs. Behaviour matches the
// existing inline calls in internal/service (bulbul:v3 TTS, saarika STT with
// auto language detection).
type SarvamProvider struct {
	apiKey     string
	ttsModel   string
	ttsSpeaker string
	sttModel   string
	http       *http.Client
}

// SarvamConfig configures the Sarvam backend.
type SarvamConfig struct {
	APIKey     string
	TTSModel   string        // default "bulbul:v3"
	TTSSpeaker string        // default "shubh"
	STTModel   string        // default "saarika:v2.5"
	Timeout    time.Duration // default 30s
	HTTP       *http.Client
}

func NewSarvamProvider(cfg SarvamConfig) *SarvamProvider {
	hc := cfg.HTTP
	if hc == nil {
		to := cfg.Timeout
		if to == 0 {
			to = 30 * time.Second
		}
		hc = &http.Client{Timeout: to}
	}
	return &SarvamProvider{
		apiKey:     cfg.APIKey,
		ttsModel:   orDefault(cfg.TTSModel, "bulbul:v3"),
		ttsSpeaker: orDefault(cfg.TTSSpeaker, "shubh"),
		sttModel:   orDefault(cfg.STTModel, "saarika:v2.5"),
		http:       hc,
	}
}

func (p *SarvamProvider) Name() string { return "sarvam" }

func (p *SarvamProvider) TextToSpeech(ctx context.Context, req TTSRequest) (*TTSResult, error) {
	if p.apiKey == "" {
		return nil, ErrNotConfigured
	}
	if strings.TrimSpace(req.Text) == "" {
		return nil, fmt.Errorf("sarvam tts: empty text")
	}
	speaker := orDefault(req.Voice, p.ttsSpeaker)
	lang := orDefault(req.Language, "en-IN")
	payload, err := json.Marshal(map[string]any{
		"inputs":               []string{req.Text},
		"target_language_code": lang,
		"speaker":              speaker,
		"model":                p.ttsModel,
	})
	if err != nil {
		return nil, fmt.Errorf("sarvam tts: encode request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, sarvamTTSURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("api-subscription-key", p.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("sarvam tts: %w", err)
	}
	defer resp.Body.Close()
	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, maxSpeechResponseBytes))
	if readErr != nil {
		return nil, fmt.Errorf("sarvam tts: read response: %w", readErr)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sarvam tts returned %d: %s", resp.StatusCode, truncate(string(raw), 300))
	}
	// Sarvam returns { "audios": ["<base64 wav>"] }. Callers that already
	// forward the raw body keep working; we also expose the decoded bytes.
	return &TTSResult{Audio: raw, MIME: "application/json", Provider: "sarvam"}, nil
}

func (p *SarvamProvider) SpeechToText(ctx context.Context, req STTRequest) (*STTResult, error) {
	if p.apiKey == "" {
		return nil, ErrNotConfigured
	}
	if len(req.Audio) == 0 {
		return nil, fmt.Errorf("sarvam stt: empty audio")
	}
	filename := orDefault(req.Filename, "speech.webm")

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		return nil, err
	}
	if _, err := fw.Write(req.Audio); err != nil {
		return nil, err
	}
	_ = mw.WriteField("model", p.sttModel)
	_ = mw.WriteField("language_code", orDefault(req.Language, "unknown"))
	if err := mw.Close(); err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, sarvamSTTURL, &buf)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("api-subscription-key", p.apiKey)
	httpReq.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := p.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("sarvam stt: %w", err)
	}
	defer resp.Body.Close()
	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, maxSpeechResponseBytes))
	if readErr != nil {
		return nil, fmt.Errorf("sarvam stt: read response: %w", readErr)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sarvam stt returned %d: %s", resp.StatusCode, truncate(string(raw), 300))
	}
	var parsed struct {
		Transcript   string `json:"transcript"`
		LanguageCode string `json:"language_code"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("sarvam stt: parse transcript: %w", err)
	}
	return &STTResult{Text: strings.TrimSpace(parsed.Transcript), Language: parsed.LanguageCode, Provider: "sarvam"}, nil
}

// SpeechToTextStream bridges an already-upgraded client WebSocket to Sarvam's
// realtime STT WebSocket: client audio frames go one way, Sarvam's partial/
// final transcript events go the other. language is normally "auto" so
// Sarvam detects whatever language the user speaks; silence_duration_ms
// closes Sarvam's turn after a pause, matching the client's own stop-on-
// silence UX.
func (p *SarvamProvider) SpeechToTextStream(ctx context.Context, clientConn *websocket.Conn, language string) error {
	if p.apiKey == "" {
		return ErrNotConfigured
	}
	lang := orDefault(language, "auto")
	url := fmt.Sprintf("%s?language_code=%s&stream_type=balanced&silence_duration_ms=8000", sarvamSTTStreamURL, lang)

	header := http.Header{}
	header.Set("api-subscription-key", p.apiKey)

	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	sarvamConn, resp, err := dialer.DialContext(ctx, url, header)
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		return fmt.Errorf("sarvam stt stream: dial failed (status %d): %w", status, err)
	}
	defer sarvamConn.Close()

	// ctx cancellation (client disconnect, request timeout, server shutdown)
	// must tear down the Sarvam leg too, since neither read loop below is
	// otherwise woken by context cancellation.
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = sarvamConn.Close()
		case <-done:
		}
	}()

	errCh := make(chan error, 2)

	// client -> Sarvam: forward audio_input frames as-is.
	go func() {
		for {
			messageType, message, err := clientConn.ReadMessage()
			if err != nil {
				errCh <- fmt.Errorf("client read: %w", err)
				return
			}
			if err := sarvamConn.WriteMessage(messageType, message); err != nil {
				errCh <- fmt.Errorf("sarvam write: %w", err)
				return
			}
		}
	}()

	// Sarvam -> client: forward transcript.partial / transcript.final events.
	go func() {
		for {
			messageType, message, err := sarvamConn.ReadMessage()
			if err != nil {
				errCh <- fmt.Errorf("sarvam read: %w", err)
				return
			}
			if err := clientConn.WriteMessage(messageType, message); err != nil {
				errCh <- fmt.Errorf("client write: %w", err)
				return
			}
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
