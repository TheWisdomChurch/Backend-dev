package authutil

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"strings"
)

// RSAKeyPair holds a matched private/public key pair for JWT RS256 signing.
type RSAKeyPair struct {
	Private *rsa.PrivateKey
	Public  *rsa.PublicKey
}

// LoadRSAKeyPair loads an RSA key pair from PEM files.
// Returns (nil, nil) when both paths are empty — caller falls back to HS256.
func LoadRSAKeyPair(privatePath, publicPath string) (*RSAKeyPair, error) {
	privatePath = strings.TrimSpace(privatePath)
	publicPath = strings.TrimSpace(publicPath)

	if privatePath == "" && publicPath == "" {
		return nil, nil
	}
	if privatePath == "" || publicPath == "" {
		return nil, errors.New("JWT_PRIVATE_KEY_PATH and JWT_PUBLIC_KEY_PATH must both be set")
	}

	privPEM, err := os.ReadFile(privatePath)
	if err != nil {
		return nil, fmt.Errorf("reading JWT private key: %w", err)
	}

	pubPEM, err := os.ReadFile(publicPath)
	if err != nil {
		return nil, fmt.Errorf("reading JWT public key: %w", err)
	}

	privKey, err := parseRSAPrivateKey(privPEM)
	if err != nil {
		return nil, fmt.Errorf("parsing JWT private key: %w", err)
	}

	pubKey, err := parseRSAPublicKey(pubPEM)
	if err != nil {
		return nil, fmt.Errorf("parsing JWT public key: %w", err)
	}

	return &RSAKeyPair{Private: privKey, Public: pubKey}, nil
}

// GenerateEphemeralRSAKeyPair creates a temporary 2048-bit key pair for
// development use when no key files are configured.
func GenerateEphemeralRSAKeyPair() (*RSAKeyPair, error) {
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generating ephemeral RSA key: %w", err)
	}
	return &RSAKeyPair{Private: privKey, Public: &privKey.PublicKey}, nil
}

func parseRSAPrivateKey(pemBytes []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("no PEM block found in private key file")
	}

	switch block.Type {
	case "RSA PRIVATE KEY":
		key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		return key, nil
	case "PRIVATE KEY":
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("PKCS8 key is not RSA")
		}
		return rsaKey, nil
	default:
		return nil, fmt.Errorf("unsupported PEM block type: %s", block.Type)
	}
}

func parseRSAPublicKey(pemBytes []byte) (*rsa.PublicKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("no PEM block found in public key file")
	}

	switch block.Type {
	case "RSA PUBLIC KEY":
		key, err := x509.ParsePKCS1PublicKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		return key, nil
	case "PUBLIC KEY":
		key, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		rsaKey, ok := key.(*rsa.PublicKey)
		if !ok {
			return nil, errors.New("PKIX key is not RSA")
		}
		return rsaKey, nil
	default:
		return nil, fmt.Errorf("unsupported PEM block type: %s", block.Type)
	}
}
