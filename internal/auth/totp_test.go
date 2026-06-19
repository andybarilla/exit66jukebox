package auth

import (
	"strings"
	"testing"
	"time"
)

func TestGenerateTOTPSecretReturnsRFCCompatibleBase32Secret(t *testing.T) {
	secret, err := GenerateTOTPSecret()
	if err != nil {
		t.Fatal(err)
	}
	if len(secret) != 32 {
		t.Fatalf("secret length = %d, want 32", len(secret))
	}
	if strings.Trim(secret, "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567") != "" {
		t.Fatalf("secret is not unpadded base32: %q", secret)
	}
}

func TestTOTPCodeAcceptsCurrentAndAdjacentStepOnly(t *testing.T) {
	secret := "JBSWY3DPEHPK3PXP"
	now := time.Unix(1_700_000_000, 0)
	code, err := TOTPCode(secret, now)
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyTOTP(secret, code, now.Add(30*time.Second), 1) {
		t.Fatal("adjacent step should verify")
	}
	if VerifyTOTP(secret, code, now.Add(60*time.Second), 1) {
		t.Fatal("outside skew should not verify")
	}
}

func TestTOTPReplayRejectsSameOrOlderAcceptedStep(t *testing.T) {
	secret := "JBSWY3DPEHPK3PXP"
	now := time.Unix(1_700_000_000, 0)
	code, err := TOTPCode(secret, now)
	if err != nil {
		t.Fatal(err)
	}
	step, ok := VerifyTOTPAfterStep(secret, code, now, 1, -1)
	if !ok {
		t.Fatal("first code should verify")
	}
	if _, ok := VerifyTOTPAfterStep(secret, code, now, 1, step); ok {
		t.Fatal("replayed step should fail")
	}
}

func TestTOTPURIContainsIssuerAccountAndSecret(t *testing.T) {
	uri := TOTPURI("Exit 66", "user@example.com", "JBSWY3DPEHPK3PXP")
	if !strings.HasPrefix(uri, "otpauth://totp/Exit%2066:user%40example.com?") {
		t.Fatalf("unexpected uri prefix: %s", uri)
	}
	for _, want := range []string{"secret=JBSWY3DPEHPK3PXP", "issuer=Exit+66", "algorithm=SHA1", "digits=6", "period=30"} {
		if !strings.Contains(uri, want) {
			t.Fatalf("uri missing %s: %s", want, uri)
		}
	}
}
