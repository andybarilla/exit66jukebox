package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const totpPeriodSeconds int64 = 30

func GenerateTOTPSecret() (string, error) {
	raw := make([]byte, 20)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return strings.TrimRight(base32.StdEncoding.EncodeToString(raw), "="), nil
}

func TOTPCode(secret string, at time.Time) (string, error) {
	key, err := decodeTOTPSecret(secret)
	if err != nil {
		return "", err
	}
	step := at.Unix() / totpPeriodSeconds
	return codeForStep(key, step), nil
}

func VerifyTOTP(secret, code string, at time.Time, skew int64) bool {
	_, ok := VerifyTOTPAfterStep(secret, code, at, skew, -1)
	return ok
}

func VerifyTOTPAfterStep(secret, code string, at time.Time, skew, lastAcceptedStep int64) (int64, bool) {
	if len(code) != 6 {
		return 0, false
	}
	key, err := decodeTOTPSecret(secret)
	if err != nil {
		return 0, false
	}
	currentStep := at.Unix() / totpPeriodSeconds
	for step := currentStep - skew; step <= currentStep+skew; step++ {
		if step <= lastAcceptedStep {
			continue
		}
		if hmac.Equal([]byte(code), []byte(codeForStep(key, step))) {
			return step, true
		}
	}
	return 0, false
}

func TOTPURI(issuer, account, secret string) string {
	label := escapeTOTPLabelPart(issuer) + ":" + escapeTOTPLabelPart(account)
	values := url.Values{}
	values.Set("secret", secret)
	values.Set("issuer", issuer)
	values.Set("algorithm", "SHA1")
	values.Set("digits", "6")
	values.Set("period", "30")
	return "otpauth://totp/" + label + "?" + values.Encode()
}

func escapeTOTPLabelPart(part string) string {
	return strings.ReplaceAll(url.QueryEscape(part), "+", "%20")
}

func decodeTOTPSecret(secret string) ([]byte, error) {
	clean := strings.ToUpper(strings.TrimSpace(secret))
	if clean == "" {
		return nil, fmt.Errorf("totp secret is required")
	}
	for len(clean)%8 != 0 {
		clean += "="
	}
	return base32.StdEncoding.DecodeString(clean)
}

func codeForStep(key []byte, step int64) string {
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(step))
	mac := hmac.New(sha1.New, key)
	mac.Write(counter[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := (uint32(sum[offset])&0x7f)<<24 | (uint32(sum[offset+1])&0xff)<<16 | (uint32(sum[offset+2])&0xff)<<8 | (uint32(sum[offset+3]) & 0xff)
	return fmt.Sprintf("%06d", value%1_000_000)
}
