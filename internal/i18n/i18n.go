// Package i18n provides simple JSON-based localization of bot texts.
package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
)

//go:embed locales/*.json
var localesFS embed.FS

// Supported interface languages.
const (
	LangRU = "ru"
	LangEN = "en"
)

var bundles = map[string]map[string]string{}

func init() {
	for _, lang := range []string{LangRU, LangEN} {
		raw, err := localesFS.ReadFile("locales/" + lang + ".json")
		if err != nil {
			panic(fmt.Errorf("i18n: read locale %s: %w", lang, err))
		}
		var m map[string]string
		if err := json.Unmarshal(raw, &m); err != nil {
			panic(fmt.Errorf("i18n: parse locale %s: %w", lang, err))
		}
		bundles[lang] = m
	}
}

// Normalize maps a Telegram language code to a supported locale.
func Normalize(langCode string) string {
	code := strings.ToLower(langCode)
	switch {
	case strings.HasPrefix(code, LangRU):
		return LangRU
	case strings.HasPrefix(code, LangEN):
		return LangEN
	default:
		return LangRU
	}
}

// T returns the localized string for key with {0}, {1}, ... placeholders
// replaced by args. Unknown keys return the key itself.
func T(lang, key string, args ...any) string {
	bundle, ok := bundles[lang]
	if !ok {
		bundle = bundles[LangRU]
	}
	text, ok := bundle[key]
	if !ok {
		if fallback, ok := bundles[LangRU][key]; ok {
			text = fallback
		} else {
			slog.Warn("i18n: missing key", "key", key, "lang", lang)
			return key
		}
	}
	if len(args) == 0 {
		return text
	}
	repl := make([]string, 0, len(args)*2)
	for i, arg := range args {
		repl = append(repl, "{"+strconv.Itoa(i)+"}", fmt.Sprint(arg))
	}
	return strings.NewReplacer(repl...).Replace(text)
}
