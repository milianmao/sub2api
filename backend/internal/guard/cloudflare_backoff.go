package guard

import (
	"math"
	"net/http"
	"strings"
	"time"
)

// CloudflareBackoffConfig defines progressive backoff parameters.
type CloudflareBackoffConfig struct {
	Enabled         bool          `yaml:"enabled" json:"enabled"`
	InitialCooldown time.Duration `yaml:"initial_cooldown" json:"initial_cooldown"`
	MaxCooldown     time.Duration `yaml:"max_cooldown" json:"max_cooldown"`
	BackoffFactor   int           `yaml:"backoff_factor" json:"backoff_factor"`
}

// DefaultCloudflareBackoffConfig returns default backoff settings.
func DefaultCloudflareBackoffConfig() CloudflareBackoffConfig {
	return CloudflareBackoffConfig{
		Enabled:         true,
		InitialCooldown: 10 * time.Second,
		MaxCooldown:     120 * time.Second,
		BackoffFactor:   3,
	}
}

// IsCloudflareChallenge checks whether an error indicates a Cloudflare page.
func IsCloudflareChallenge(err error) bool {
	if err == nil {
		return false
	}
	return isCloudflareChallengeString(err.Error())
}

// IsCloudflareChallengeResponse checks whether a response body indicates a
// Cloudflare challenge page.
func IsCloudflareChallengeResponse(resp *http.Response, body []byte) bool {
	if resp == nil {
		return false
	}
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusServiceUnavailable {
		return isCloudflareChallengeString(string(body))
	}
	return false
}

// IsCloudflareChallengeCode checks whether a status code and body indicate a
// Cloudflare challenge.
func IsCloudflareChallengeCode(statusCode int, body []byte) bool {
	if statusCode == http.StatusForbidden || statusCode == http.StatusServiceUnavailable {
		return isCloudflareChallengeString(string(body))
	}
	return false
}

func isCloudflareChallengeString(s string) bool {
	lower := strings.ToLower(strings.TrimSpace(s))
	return strings.Contains(lower, "challenge-platform") ||
		strings.Contains(lower, "cf-mitigated") ||
		strings.Contains(lower, "just a moment") ||
		(strings.Contains(lower, "cloudflare") && strings.Contains(lower, "<html"))
}

// CloudflareBackoff calculates an exponential cooldown for a backoff level.
func CloudflareBackoff(level int, cfg *CloudflareBackoffConfig) time.Duration {
	if cfg == nil {
		defaultCfg := DefaultCloudflareBackoffConfig()
		cfg = &defaultCfg
	}
	if !cfg.Enabled {
		return 0
	}
	if level < 0 {
		level = 0
	}
	factor := cfg.BackoffFactor
	if factor <= 1 {
		factor = 3
	}
	d := time.Duration(float64(cfg.InitialCooldown) * math.Pow(float64(factor), float64(level)))
	if d > cfg.MaxCooldown {
		d = cfg.MaxCooldown
	}
	if d < cfg.InitialCooldown {
		d = cfg.InitialCooldown
	}
	return d
}
