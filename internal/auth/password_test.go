package auth

import (
	"strings"
	"testing"
)

func TestHashVerifyRoundTrip(t *testing.T) {
	h, err := HashPassword("hunter2")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !strings.HasPrefix(h, "pbkdf2-sha256$") {
		t.Fatalf("unexpected format: %s", h)
	}
	if !VerifyPassword("hunter2", h) {
		t.Fatal("correct password rejected")
	}
	if VerifyPassword("wrong", h) {
		t.Fatal("wrong password accepted")
	}
}

func TestHashSaltsDiffer(t *testing.T) {
	a, _ := HashPassword("same")
	b, _ := HashPassword("same")
	if a == b {
		t.Fatal("identical hashes — salt not random")
	}
}

func TestVerifyMalformed(t *testing.T) {
	if VerifyPassword("x", "garbage") {
		t.Fatal("malformed hash accepted")
	}
}
