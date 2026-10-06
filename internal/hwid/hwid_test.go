package hwid

import (
	"regexp"
	"testing"
)

var uuidRe = regexp.MustCompile(`^[0-9A-F]{8}-[0-9A-F]{4}-4[0-9A-F]{3}-[89AB][0-9A-F]{3}-[0-9A-F]{12}$`)

func TestGenerate(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		id, err := Generate()
		if err != nil {
			t.Fatalf("Generate() error: %v", err)
		}
		if !uuidRe.MatchString(id) {
			t.Fatalf("Generate() = %q, want uppercase UUID v4", id)
		}
		if seen[id] {
			t.Fatalf("Generate() returned duplicate %q", id)
		}
		seen[id] = true
	}
}
