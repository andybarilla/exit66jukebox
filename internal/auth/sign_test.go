package auth

import "testing"

func TestSignVerifyMedia(t *testing.T) {
	secret := []byte("server-secret")
	tok := SignMedia(secret, 42, 1_000_000)
	if id, ok := VerifyMedia(secret, tok, 999_000); !ok || id != 42 {
		t.Fatalf("valid token rejected: id=%d ok=%v", id, ok)
	}
}

func TestVerifyMediaExpired(t *testing.T) {
	secret := []byte("s")
	tok := SignMedia(secret, 1, 100)
	if _, ok := VerifyMedia(secret, tok, 101); ok {
		t.Fatal("expired token accepted")
	}
}

func TestVerifyMediaTampered(t *testing.T) {
	secret := []byte("s")
	tok := SignMedia(secret, 1, 1_000_000)
	if _, ok := VerifyMedia([]byte("other"), tok, 1); ok {
		t.Fatal("forged token accepted under wrong secret")
	}
	if _, ok := VerifyMedia(secret, tok+"x", 1); ok {
		t.Fatal("mutated token accepted")
	}
}
