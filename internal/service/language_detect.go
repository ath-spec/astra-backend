package service

// detectLanguageCode picks a Sarvam TTS target_language_code from the script
// the text is actually written in. TTS (unlike STT) has no "auto" option —
// Sarvam's docs confirm target_language_code must be one of a fixed set, so
// this is a real fallback, not a duplicate of STT's native auto-detection.
// Used only when a caller has no explicit language (e.g. not yet threaded
// through from the client) — synthesizing Devanagari text as "en-IN"
// mispronounces it, so scanning for the first non-Latin script block and
// mapping it gets far closer than always defaulting to English.
func detectLanguageCode(text string) string {
	for _, r := range text {
		switch {
		case r >= 0x0900 && r <= 0x097F:
			return "hi-IN" // Devanagari (Hindi/Marathi)
		case r >= 0x0A80 && r <= 0x0AFF:
			return "gu-IN" // Gujarati
		case r >= 0x0980 && r <= 0x09FF:
			return "bn-IN" // Bengali
		case r >= 0x0B80 && r <= 0x0BFF:
			return "ta-IN" // Tamil
		case r >= 0x0C00 && r <= 0x0C7F:
			return "te-IN" // Telugu
		case r >= 0x0C80 && r <= 0x0CFF:
			return "kn-IN" // Kannada
		case r >= 0x0D00 && r <= 0x0D7F:
			return "ml-IN" // Malayalam
		case r >= 0x0A00 && r <= 0x0A7F:
			return "pa-IN" // Punjabi (Gurmukhi)
		case r >= 0x0B00 && r <= 0x0B7F:
			return "od-IN" // Odia — TTS uses "od-IN", unlike the STT-realtime endpoint's "or-IN"
		}
	}
	return "en-IN"
}
