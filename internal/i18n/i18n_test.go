package i18n

import "testing"

func TestTReplacesPlaceholders(t *testing.T) {
	got := T(LangRU, "sub.test_origin_ok", "example.com", 200, 42)
	want := "✅ example.com: HTTP 200, 42 байт"
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
