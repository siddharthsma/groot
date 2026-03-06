package apikey

import (
	"strings"
	"testing"
)

func TestParsePrefixAllowsUnderscoreInSecret(t *testing.T) {
	prefix, err := ParsePrefix("groot_ab12cd34_secret_with_underscores")
	if err != nil {
		t.Fatalf("ParsePrefix() error = %v", err)
	}
	if prefix != "ab12cd34" {
		t.Fatalf("prefix = %q, want ab12cd34", prefix)
	}
}

func TestGenerateKeyUsesEightCharPrefix(t *testing.T) {
	fullKey, prefix, err := generateKey()
	if err != nil {
		t.Fatalf("generateKey() error = %v", err)
	}
	if len(prefix) != 8 {
		t.Fatalf("prefix len = %d, want 8", len(prefix))
	}
	if strings.Contains(prefix, "_") {
		t.Fatalf("prefix = %q, want no underscore", prefix)
	}
	if !strings.HasPrefix(fullKey, "groot_"+prefix+"_") {
		t.Fatalf("fullKey = %q, prefix = %q", fullKey, prefix)
	}
}
