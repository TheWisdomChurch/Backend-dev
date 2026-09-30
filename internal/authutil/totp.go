package authutil

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultTOTPPeriod = 30 * time.Second
	DefaultTOTPDigits = 6
)

func GenerateTOTPSecret(size int) (string, error) {
	if size <= 0 {
		size = 20
	}

	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}

	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf), nil
}

func BuildTOTPAuthURL(issuer, accountName, secret string) string {
	normalizedIssuer := sanitizeTOTPLabelPart(issuer)
	if normalizedIssuer == "" {
		normalizedIssuer = "Secure Account"
	}

	normalizedAccount := sanitizeTOTPLabelPart(accountName)
	label := url.PathEscape(normalizedIssuer)
	if normalizedAccount != "" {
		label += ":" + url.PathEscape(normalizedAccount)
	}

	query := url.Values{}
	query.Set("secret", strings.TrimSpace(secret))
	query.Set("issuer", normalizedIssuer)
	query.Set("algorithm", "SHA1")
	query.Set("digits", strconv.Itoa(DefaultTOTPDigits))
	query.Set("period", strconv.Itoa(int(DefaultTOTPPeriod/time.Second)))

	return "otpauth://totp/" + label + "?" + query.Encode()
}

func sanitizeTOTPLabelPart(value string) string {
	normalized := strings.TrimSpace(value)
	normalized = strings.ReplaceAll(normalized, ":", "")
	return normalized
}

func VerifyTOTP(secret, code string, now time.Time, skew int) bool {
	normalizedSecret := strings.TrimSpace(secret)
	normalizedCode := normalizeNumericCode(code)
	if normalizedSecret == "" || len(normalizedCode) != DefaultTOTPDigits {
		return false
	}

	if skew < 0 {
		skew = 0
	}

	for offset := -skew; offset <= skew; offset++ {
		candidateTime := now.Add(time.Duration(offset) * DefaultTOTPPeriod)
		candidate, err := generateTOTPCode(normalizedSecret, candidateTime)
		if err == nil && hmac.Equal([]byte(candidate), []byte(normalizedCode)) {
			return true
		}
	}

	return false
}

func generateTOTPCode(secret string, at time.Time) (string, error) {
	decoded, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil {
		return "", err
	}

	counter := uint64(at.UTC().Unix() / int64(DefaultTOTPPeriod/time.Second))
	msg := make([]byte, 8)
	binary.BigEndian.PutUint64(msg, counter)

	mac := hmac.New(sha1.New, decoded)
	mac.Write(msg)
	sum := mac.Sum(nil)

	offset := sum[len(sum)-1] & 0x0f
	binaryCode := (int(sum[offset])&0x7f)<<24 |
		(int(sum[offset+1])&0xff)<<16 |
		(int(sum[offset+2])&0xff)<<8 |
		(int(sum[offset+3]) & 0xff)

	modulo := 1
	for i := 0; i < DefaultTOTPDigits; i++ {
		modulo *= 10
	}

	return fmt.Sprintf("%0*d", DefaultTOTPDigits, binaryCode%modulo), nil
}

func normalizeNumericCode(value string) string {
	var builder strings.Builder
	for _, r := range value {
		if r >= '0' && r <= '9' {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

// GenerateRecoveryCodes generates count random alphanumeric recovery codes formatted as XXXX-XXXX.
func GenerateRecoveryCodes(count int) ([]string, error) {
	if count <= 0 {
		count = 8
	}
	// Use Crockford-like base32 characters to avoid visual ambiguity (no 0/O, 1/I).
	const charset = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"
	codes := make([]string, count)
	buf := make([]byte, 8)
	for i := 0; i < count; i++ {
		if _, err := rand.Read(buf); err != nil {
			return nil, err
		}
		var sb strings.Builder
		for j := 0; j < 8; j++ {
			if j == 4 {
				sb.WriteByte('-')
			}
			idx := int(buf[j]) % len(charset)
			sb.WriteByte(charset[idx])
		}
		codes[i] = sb.String()
	}
	return codes, nil
}

// NormalizeRecoveryCode strips hyphens, whitespace, and uppercases.
func NormalizeRecoveryCode(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	var sb strings.Builder
	for _, r := range code {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// LooksLikeRecoveryCode returns true if the input looks like a recovery code rather than a 6-digit TOTP.
func LooksLikeRecoveryCode(code string) bool {
	norm := NormalizeRecoveryCode(code)
	return len(norm) == 8 && strings.IndexFunc(norm, func(r rune) bool {
		return r < '0' || (r > '9' && (r < 'A' || r > 'Z'))
	}) == -1
}

