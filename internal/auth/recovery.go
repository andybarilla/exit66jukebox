package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/hex"
	"strings"
)

func GenerateRecoveryCodes(count int) ([]string, error) {
	if count <= 0 {
		return nil, nil
	}
	codes := make([]string, 0, count)
	for range count {
		raw := make([]byte, 10)
		if _, err := rand.Read(raw); err != nil {
			return nil, err
		}
		text := strings.TrimRight(base32.StdEncoding.EncodeToString(raw), "=")
		codes = append(codes, text[0:4]+"-"+text[4:8]+"-"+text[8:12]+"-"+text[12:16])
	}
	return codes, nil
}

func HashRecoveryCode(code string) string {
	sum := sha256.Sum256([]byte(normalizeRecoveryCode(code)))
	return hex.EncodeToString(sum[:])
}

func VerifyRecoveryCode(code, hash string) bool {
	decodedHash, err := hex.DecodeString(hash)
	if err != nil {
		return false
	}
	expectedHash := sha256.Sum256([]byte(normalizeRecoveryCode(code)))
	if len(decodedHash) != len(expectedHash) {
		return false
	}
	return subtle.ConstantTimeCompare(expectedHash[:], decodedHash) == 1
}

func normalizeRecoveryCode(code string) string {
	replacer := strings.NewReplacer("-", "", " ", "")
	return strings.ToUpper(replacer.Replace(strings.TrimSpace(code)))
}
