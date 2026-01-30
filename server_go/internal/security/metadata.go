package security

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// GetGoogleIDToken fetches an OIDC ID token from the Google Metadata Server
func GetGoogleIDToken(audience string) (string, error) {
	// Only attempt fetch if in production or K_SERVICE is set (Cloud Run environment)
	if os.Getenv("K_SERVICE") == "" && os.Getenv("APP_ENV") != "production" {
		return "", nil
	}

	url := fmt.Sprintf("http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/identity?audience=%s", audience)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Add("Metadata-Flavor", "Google")

	client := &http.Client{
		Timeout: 5 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("metadata request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("metadata server returned status %d", resp.StatusCode)
	}

	token, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(token), nil
}
