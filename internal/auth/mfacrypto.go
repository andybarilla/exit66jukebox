package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
)

const totpEncryptionKeyLength = 32

type EncryptedSecret struct {
	Ciphertext []byte
	Nonce      []byte
	KeyVersion int
}

func EncryptTOTPSecret(key []byte, secret string, keyVersion int) (EncryptedSecret, error) {
	gcm, err := newTOTPSecretCipher(key)
	if err != nil {
		return EncryptedSecret{}, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return EncryptedSecret{}, err
	}
	return EncryptedSecret{
		Ciphertext: gcm.Seal(nil, nonce, []byte(secret), nil),
		Nonce:      nonce,
		KeyVersion: keyVersion,
	}, nil
}

func DecryptTOTPSecret(key []byte, enc EncryptedSecret) (string, error) {
	gcm, err := newTOTPSecretCipher(key)
	if err != nil {
		return "", err
	}
	plaintext, err := gcm.Open(nil, enc.Nonce, enc.Ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func newTOTPSecretCipher(key []byte) (cipher.AEAD, error) {
	if len(key) != totpEncryptionKeyLength {
		return nil, fmt.Errorf("totp encryption key must be %d bytes", totpEncryptionKeyLength)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
