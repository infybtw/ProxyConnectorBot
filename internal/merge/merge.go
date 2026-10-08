// Package merge combines several subscription bodies into one. Subscriptions
// are lists of share links, either plain text or base64 encoded.
package merge

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ErrEmpty is returned when there is nothing to merge.
var ErrEmpty = errors.New("bundle: no subscription parts")

// Part is one successful origin response.
type Part struct {
	Body   []byte
	Header http.Header
}

// Result is the merged subscription.
type Result struct {
	Body   []byte
	Header http.Header
}

// userinfoKeys are the traffic counters of subscription-userinfo that are
// summed across parts. The earliest non-zero expire wins.
var userinfoKeys = []string{"upload", "download", "total"}

// Merge concatenates the share links of all parts, dropping duplicates while
// keeping order. The output is base64 when any part was base64 encoded.
// Headers come from the first part, with traffic counters summed.
func Merge(parts []Part) (Result, error) {
	if len(parts) == 0 {
		return Result{}, ErrEmpty
	}

	var (
		out     []string
		seen    = make(map[string]bool)
		encoded bool
		info    []string
	)
	for _, p := range parts {
		lines, wasEncoded := splitLines(p.Body)
		encoded = encoded || wasEncoded
		for _, l := range lines {
			if !seen[l] {
				seen[l] = true
				out = append(out, l)
			}
		}
		if v := p.Header.Get("Subscription-Userinfo"); v != "" {
			info = append(info, v)
		}
	}

	body := strings.Join(out, "\n")
	if encoded {
		body = base64.StdEncoding.EncodeToString([]byte(body))
	}

	header := parts[0].Header.Clone()
	if header == nil {
		header = http.Header{}
	}
	if len(info) > 0 {
		header.Set("Subscription-Userinfo", mergeUserinfo(info))
	}
	return Result{Body: []byte(body), Header: header}, nil
}

// splitLines returns the non-empty trimmed lines of a subscription body and
// reports whether the body was base64 encoded.
func splitLines(body []byte) ([]string, bool) {
	text := strings.TrimSpace(string(body))
	if decoded, ok := decodeBase64(text); ok {
		return toLines(decoded), true
	}
	return toLines(text), false
}

// decodeBase64 decodes a base64 subscription body. Anything that does not
// decode to text with share links is treated as plain text.
func decodeBase64(text string) (string, bool) {
	compact := strings.Join(strings.Fields(text), "")
	if compact == "" {
		return "", false
	}
	encodings := []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	}
	for _, enc := range encodings {
		raw, err := enc.DecodeString(compact)
		if err != nil || !utf8.Valid(raw) {
			continue
		}
		if strings.Contains(string(raw), "://") {
			return string(raw), true
		}
	}
	return "", false
}

func toLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// mergeUserinfo sums traffic counters and keeps the earliest expiry from
// subscription-userinfo header values such as "upload=1; download=2; total=3; expire=4".
func mergeUserinfo(values []string) string {
	sums := make(map[string]int64)
	present := make(map[string]bool)
	var expire int64

	for _, v := range values {
		for _, field := range strings.Split(v, ";") {
			k, val, ok := strings.Cut(strings.TrimSpace(field), "=")
			if !ok {
				continue
			}
			n, err := strconv.ParseInt(strings.TrimSpace(val), 10, 64)
			if err != nil {
				continue
			}
			k = strings.ToLower(strings.TrimSpace(k))
			switch {
			case k == "expire":
				if n > 0 && (expire == 0 || n < expire) {
					expire = n
				}
			case contains(userinfoKeys, k):
				sums[k] += n
				present[k] = true
			}
		}
	}

	var parts []string
	for _, k := range userinfoKeys {
		if present[k] {
			parts = append(parts, k+"="+strconv.FormatInt(sums[k], 10))
		}
	}
	if expire > 0 {
		parts = append(parts, "expire="+strconv.FormatInt(expire, 10))
	}
	return strings.Join(parts, "; ")
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
