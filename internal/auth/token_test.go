package auth

import "testing"

func TestGenerateTokenUnique(t *testing.T) {
	a, err := GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := GenerateToken()
	if a == b || len(a) < 32 {
		t.Fatalf("weak/duplicate token: %q %q", a, b)
	}
}

func TestHashTokenStable(t *testing.T) {
	if HashToken("abc") != HashToken("abc") {
		t.Fatal("hash not stable")
	}
	if HashToken("abc") == HashToken("abd") {
		t.Fatal("hash collision")
	}
}
