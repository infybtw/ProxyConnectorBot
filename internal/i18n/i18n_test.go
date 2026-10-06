package i18n

import "testing"

func TestTReplacesPlaceholders(t *testing.T) {
	got := T(LangRU, "sub.test_ok", 200, 42, "text/plain")
	want := "✅ Origin ответил: HTTP 200, 42 байт.\nContent-Type: <code>text/plain</code>"
	if got != want {
		t.Fatalf("T() = %q, want %q", got, want)
	}
}

func TestTFallsBackToRussian(t *testing.T) {
	if got := T("de", "btn.add", nil); got != T(LangRU, "btn.add") {
		t.Fatalf("fallback mismatch: %q", got)
	}
}

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"ru": LangRU, "ru-RU": LangRU, "en": LangEN, "en-US": LangEN, "de": LangRU,
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Fatalf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}
