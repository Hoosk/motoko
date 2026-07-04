package sessiontitle

import (
	"testing"
)

func TestSanitizePrefersCleanFinalTitle(t *testing.T) {
	raw := `(The user wants a title for the session. The session is just starting, so it's a general programming session.)

* *Option 1:* Sesion de programacion con Motoko
* *Option 2:* Asistencia experta en desarrollo de software
* *Option 3:* Tu asistente personal de programacion

* *Constraint Check:* "4 a 8 palabras".

Asistencia experta en desarrollo de software`
	got := Sanitize(raw)
	if got != "Asistencia experta en desarrollo de software" {
		t.Fatalf("Sanitize() = %q", got)
	}
}

func TestSanitizeKeepsSingleLineTitle(t *testing.T) {
	got := Sanitize("Depuracion de tools en Gemini")
	if got != "Depuracion de tools en Gemini" {
		t.Fatalf("Sanitize() = %q", got)
	}
}

func TestSanitizeEmpty(t *testing.T) {
	got := Sanitize("")
	if got != "" {
		t.Fatalf("Sanitize() = %q", got)
	}
}

func TestSanitizeTrimsWhitespace(t *testing.T) {
	got := Sanitize("  My Title  \n")
	if got != "My Title" {
		t.Fatalf("Sanitize() = %q", got)
	}
}

func TestExtractStructuredMessageAcceptsFencedJSON(t *testing.T) {
	raw := "```json\n{\"message\":\"Asistencia experta en desarrollo de software\"}\n```"
	got := ExtractStructuredMessage(raw)
	if got != "Asistencia experta en desarrollo de software" {
		t.Fatalf("ExtractStructuredMessage() = %q", got)
	}
}

func TestExtractStructuredMessageNoJSON(t *testing.T) {
	got := ExtractStructuredMessage("plain text")
	if got != "" {
		t.Fatalf("ExtractStructuredMessage() = %q", got)
	}
}
