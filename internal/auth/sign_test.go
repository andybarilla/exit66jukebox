package auth

import "testing"

func TestSignVerifyPath(t *testing.T) {
	secret := []byte("server-secret")
	tok := SignPath(secret, "/stream/house.mp3", 1_000_000)
	if !VerifyPath(secret, tok, "/stream/house.mp3", 999_000) {
		t.Fatal("valid token rejected")
	}
}

func TestVerifyPathExpired(t *testing.T) {
	secret := []byte("s")
	tok := SignPath(secret, "/stream/house.mp3", 100)
	if VerifyPath(secret, tok, "/stream/house.mp3", 101) {
		t.Fatal("expired token accepted")
	}
}

func TestVerifyPathTampered(t *testing.T) {
	secret := []byte("s")
	tok := SignPath(secret, "/stream/house.mp3", 1_000_000)
	if VerifyPath([]byte("other"), tok, "/stream/house.mp3", 1) {
		t.Fatal("forged token accepted under wrong secret")
	}
	if VerifyPath(secret, tok, "/stream/evil.mp3", 1) {
		t.Fatal("token valid for a different path")
	}
	if VerifyPath(secret, tok+"x", "/stream/house.mp3", 1) {
		t.Fatal("mutated token accepted")
	}
}
