package authutil

import (
	"bufio"
	"crypto/sha1" //nolint:gosec // SHA-1 required by HIBP k-anonymity API spec
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// HIBPClient checks passwords against the HaveIBeenPwned k-anonymity API.
// Only the first 5 hex characters of the SHA-1 hash are ever sent to the API —
// the full hash (and therefore the plaintext) never leaves the process.
type HIBPClient struct {
	httpClient *http.Client
	baseURL    string
}

// NewHIBPClient creates a client with a sensible default timeout.
func NewHIBPClient() *HIBPClient {
	return &HIBPClient{
		httpClient: &http.Client{Timeout: 5 * time.Second},
		baseURL:    "https://api.pwnedpasswords.com/range/",
	}
}

// IsBreached returns true when the password appears in a known data breach.
// Returns (false, nil) on any network error — fail open so users are never
// locked out by HIBP downtime.
func (c *HIBPClient) IsBreached(password string) (bool, error) {
	//nolint:gosec // SHA-1 is mandated by the HIBP k-anonymity range API
	sum := sha1.Sum([]byte(password))
	hashHex := strings.ToUpper(hex.EncodeToString(sum[:]))

	prefix := hashHex[:5]
	suffix := hashHex[5:]

	resp, err := c.httpClient.Get(c.baseURL + prefix)
	if err != nil {
		// Fail open: network error is not a reason to block registration.
		return false, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("HIBP API returned status %d", resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if len(line) < 5 {
			continue
		}
		// Each line: "SUFFIX:COUNT"
		colonIdx := strings.Index(line, ":")
		if colonIdx < 0 {
			continue
		}
		if strings.EqualFold(line[:colonIdx], suffix) {
			return true, nil
		}
	}

	return false, scanner.Err()
}
