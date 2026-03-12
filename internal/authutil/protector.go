package authutil

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
)

type Protector struct {
	key []byte
}

func NewProtector(secret string) (*Protector, error) {
	trimmed := strings.TrimSpace(secret)
	if trimmed == "" {
		return nil, errors.New("secret is required")
	}

	sum := sha256.Sum256([]byte(trimmed))
	return &Protector{key: sum[:]}, nil
}

func (p *Protector) EncryptString(plaintext string) (string, error) {
	if p == nil || len(p.key) == 0 {
		return "", errors.New("protector is not configured")
	}

	block, err := aes.NewCipher(p.key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	payload := append(nonce, ciphertext...)
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func (p *Protector) DecryptString(ciphertext string) (string, error) {
	if p == nil || len(p.key) == 0 {
		return "", errors.New("protector is not configured")
	}

	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(ciphertext))
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(p.key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(raw) <= nonceSize {
		return "", errors.New("ciphertext is invalid")
	}

	plaintext, err := gcm.Open(nil, raw[:nonceSize], raw[nonceSize:], nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

func (p *Protector) SignPayload(payload []byte) string {
	if p == nil || len(p.key) == 0 {
		return ""
	}

	mac := hmac.New(sha256.New, p.key)
	mac.Write(payload)

	return base64.RawURLEncoding.EncodeToString(payload) + "." + hex.EncodeToString(mac.Sum(nil))
}

func (p *Protector) VerifySignedPayload(value string) ([]byte, error) {
	if p == nil || len(p.key) == 0 {
		return nil, errors.New("protector is not configured")
	}

	parts := strings.Split(strings.TrimSpace(value), ".")
	if len(parts) != 2 {
		return nil, errors.New("signed payload is invalid")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, err
	}

	expectedHex := p.SignPayload(payload)
	expectedParts := strings.Split(expectedHex, ".")
	if len(expectedParts) != 2 {
		return nil, errors.New("failed to verify payload")
	}

	expected, err := hex.DecodeString(expectedParts[1])
	if err != nil {
		return nil, err
	}

	actual, err := hex.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}

	if !hmac.Equal(actual, expected) {
		return nil, fmt.Errorf("signed payload verification failed")
	}

	return payload, nil
}
