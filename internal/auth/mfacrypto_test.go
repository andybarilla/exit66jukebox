package auth

import (
	"bytes"
	"testing"
)

func TestEncryptDecryptTOTPSecretRoundTripPreservesKeyVersion(t *testing.T) {
	key := bytes.Repeat([]byte{1}, 32)
	secret := "JBSWY3DPEHPK3PXP"

	enc, err := EncryptTOTPSecret(key, secret, 7)
	if err != nil {
		t.Fatal(err)
	}
	if enc.KeyVersion != 7 {
		t.Fatalf("key version = %d, want 7", enc.KeyVersion)
	}

	got, err := DecryptTOTPSecret(key, enc)
	if err != nil {
		t.Fatal(err)
	}
	if got != secret {
		t.Fatalf("decrypted secret = %q, want %q", got, secret)
	}
}

func TestEncryptTOTPSecretUsesUniqueNonceAndCiphertext(t *testing.T) {
	key := bytes.Repeat([]byte{2}, 32)
	secret := "JBSWY3DPEHPK3PXP"

	first, err := EncryptTOTPSecret(key, secret, 1)
	if err != nil {
		t.Fatal(err)
	}
	second, err := EncryptTOTPSecret(key, secret, 1)
	if err != nil {
		t.Fatal(err)
	}

	if bytes.Equal(first.Nonce, second.Nonce) {
		t.Fatal("nonce should differ across encryptions")
	}
	if bytes.Equal(first.Ciphertext, second.Ciphertext) {
		t.Fatal("ciphertext should differ across encryptions")
	}
}

func TestDecryptTOTPSecretRejectsDifferentKey(t *testing.T) {
	key := bytes.Repeat([]byte{3}, 32)
	wrongKey := bytes.Repeat([]byte{4}, 32)

	enc, err := EncryptTOTPSecret(key, "JBSWY3DPEHPK3PXP", 1)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := DecryptTOTPSecret(wrongKey, enc); err == nil {
		t.Fatal("decrypt with different key should fail")
	}
}

func TestEncryptDecryptTOTPSecretRejectInvalidKeyLengths(t *testing.T) {
	if _, err := EncryptTOTPSecret([]byte("short"), "JBSWY3DPEHPK3PXP", 1); err == nil {
		t.Fatal("encrypt should reject invalid key length")
	}

	enc := EncryptedSecret{Ciphertext: []byte("ciphertext"), Nonce: []byte("nonce"), KeyVersion: 1}
	if _, err := DecryptTOTPSecret([]byte("short"), enc); err == nil {
		t.Fatal("decrypt should reject invalid key length")
	}
}
