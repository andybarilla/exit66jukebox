package auth

import "testing"

func TestGenerateRecoveryCodesReturnsDistinctDisplayCodes(t *testing.T) {
	codes, err := GenerateRecoveryCodes(10)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, code := range codes {
		if seen[code] {
			t.Fatalf("duplicate code %q", code)
		}
		seen[code] = true
		if len(code) != 19 {
			t.Fatalf("code %q length = %d, want 19", code, len(code))
		}
	}
}

func TestRecoveryCodeHashNormalizesInput(t *testing.T) {
	hash := HashRecoveryCode("ABCD-EFGH-IJKL-MNOP")
	if !VerifyRecoveryCode("abcd efgh ijkl mnop", hash) {
		t.Fatal("normalized code should verify")
	}
	if VerifyRecoveryCode("abcd efgh ijkl mnoq", hash) {
		t.Fatal("different code should not verify")
	}
}
