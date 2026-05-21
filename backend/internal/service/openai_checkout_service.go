package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var chatGPTCheckoutURL = "https://chatgpt.com/backend-api/payments/checkout"

var chatGPTTeamCheckoutBaseURL = "https://chatgpt.com/checkout/openai_llc/"

var chatGPTCountryCurrency = map[string]string{
	"SG": "SGD",
	"US": "USD",
	"TR": "TRY",
	"JP": "JPY",
	"HK": "HKD",
	"GB": "GBP",
	"EU": "EUR",
	"AU": "AUD",
	"CA": "CAD",
	"IN": "INR",
	"BR": "BRL",
	"MX": "MXN",
}

func SetChatGPTCheckoutURLForTest(url string) string {
	oldURL := chatGPTCheckoutURL
	chatGPTCheckoutURL = url
	return oldURL
}

type chatGPTCheckoutPayload struct {
	PlanName       string            `json:"plan_name"`
	BillingDetails map[string]string `json:"billing_details"`
	CancelURL      string            `json:"cancel_url"`
	PromoCampaign  map[string]any    `json:"promo_campaign"`
	CheckoutUIMode string            `json:"checkout_ui_mode"`
}

type chatGPTCheckoutResponse struct {
	URL               string `json:"url"`
	StripeHostedURL   string `json:"stripe_hosted_url"`
	CheckoutURL       string `json:"checkout_url"`
	CheckoutSessionID string `json:"checkout_session_id"`
}

type CreateCheckoutLinkRequest struct {
	AccessToken string
	ProxyURL    string
	Cookies     string
	Country     string
	Currency    string
}

// CreateCheckoutLink creates a hosted ChatGPT Plus checkout URL using an OpenAI OAuth access token.
func (s *OpenAIOAuthService) CreateCheckoutLink(ctx context.Context, checkoutReq CreateCheckoutLinkRequest) (string, error) {
	accessToken := strings.TrimSpace(checkoutReq.AccessToken)
	if accessToken == "" {
		return "", infraerrors.BadRequest("OPENAI_CHECKOUT_ACCESS_TOKEN_REQUIRED", "access token is required")
	}
	if s == nil || s.privacyClientFactory == nil {
		return "", infraerrors.InternalServer("OPENAI_CHECKOUT_CLIENT_UNAVAILABLE", "checkout client is unavailable")
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	client, err := s.privacyClientFactory(checkoutReq.ProxyURL)
	if err != nil {
		return "", infraerrors.InternalServer("OPENAI_CHECKOUT_CLIENT_FAILED", "failed to create checkout client").WithCause(err)
	}

	billingDetails := resolveCheckoutBillingDetails(checkoutReq.Country, checkoutReq.Currency)
	payloadBytes, err := json.Marshal(chatGPTCheckoutPayload{
		PlanName:       "chatgptplusplan",
		BillingDetails: billingDetails,
		CancelURL:      "https://chatgpt.com/#pricing",
		PromoCampaign: map[string]any{
			"promo_campaign_id":          "plus-1-month-free",
			"is_coupon_from_query_param": false,
		},
		CheckoutUIMode: "hosted",
	})
	if err != nil {
		return "", infraerrors.InternalServer("OPENAI_CHECKOUT_PAYLOAD_FAILED", "failed to build checkout payload").WithCause(err)
	}

	cookies := strings.TrimSpace(checkoutReq.Cookies)
	deviceID := extractOAIDeviceIDFromCookies(cookies)
	request := client.R().
		SetContext(ctx).
		SetHeader("Authorization", "Bearer "+accessToken).
		SetHeader("Origin", "https://chatgpt.com").
		SetHeader("Referer", "https://chatgpt.com/#pricing").
		SetHeader("Accept", "application/json").
		SetHeader("Content-Type", "application/json").
		SetHeader("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36").
		SetHeader("Accept-Language", "en-US,en;q=0.9").
		SetHeader("Sec-Ch-Ua", `"Chromium";v="125", "Google Chrome";v="125", "Not=A?Brand";v="8"`).
		SetHeader("Sec-Ch-Ua-Mobile", "?0").
		SetHeader("Sec-Ch-Ua-Platform", `"Windows"`).
		SetHeader("Sec-Fetch-Site", "same-origin").
		SetHeader("Sec-Fetch-Mode", "cors").
		SetHeader("Sec-Fetch-Dest", "empty").
		SetBody(payloadBytes)
	if cookies != "" {
		request.SetHeader("Cookie", cookies)
	}
	if deviceID != "" {
		request.SetHeader("oai-device-id", deviceID)
	}

	resp, err := request.Post(chatGPTCheckoutURL)
	if err != nil {
		if resp != nil && resp.IsSuccessState() {
			checkoutURL := checkoutURLFromRawResponse(resp.String())
			if isTrustedCheckoutURL(checkoutURL) {
				return checkoutURL, nil
			}
		}
		slog.Warn("openai_checkout_request_failed",
			"proxy_configured", strings.TrimSpace(checkoutReq.ProxyURL) != "",
			"cookies_configured", cookies != "",
			"device_id_configured", deviceID != "",
			"country", billingDetails["country"],
			"currency", billingDetails["currency"],
			"error", err.Error(),
		)
		return "", infraerrors.InternalServer("OPENAI_CHECKOUT_REQUEST_FAILED", "checkout request failed").WithCause(err)
	}
	if !resp.IsSuccessState() {
		slog.Warn("openai_checkout_upstream_failed",
			"status", resp.StatusCode,
			"body", truncateCheckoutDiagnostic(resp.String(), 300),
			"proxy_configured", strings.TrimSpace(checkoutReq.ProxyURL) != "",
			"cookies_configured", cookies != "",
			"device_id_configured", deviceID != "",
			"country", billingDetails["country"],
			"currency", billingDetails["currency"],
		)
		return "", infraerrors.Newf(http.StatusBadGateway, "OPENAI_CHECKOUT_UPSTREAM_FAILED", "checkout request failed with upstream status %d", resp.StatusCode)
	}

	checkoutURL := checkoutURLFromRawResponse(resp.String())
	if !isTrustedCheckoutURL(checkoutURL) {
		rawCheckoutSessionID := extractCheckoutSessionID(resp.String())
		parsedCheckoutURL, parseErr := url.Parse(checkoutURL)
		checkoutURLHost := ""
		checkoutURLScheme := ""
		if parseErr == nil {
			checkoutURLHost = parsedCheckoutURL.Hostname()
			checkoutURLScheme = parsedCheckoutURL.Scheme
		}
		slog.Warn("openai_checkout_invalid_response",
			"body", truncateCheckoutDiagnostic(resp.String(), 300),
			"checkout_url_configured", strings.TrimSpace(checkoutURL) != "",
			"checkout_url_len", len(checkoutURL),
			"checkout_url_parse_error", parseErr != nil,
			"checkout_url_scheme", checkoutURLScheme,
			"checkout_url_host", checkoutURLHost,
			"raw_session_id_configured", rawCheckoutSessionID != "",
			"raw_session_id_len", len(rawCheckoutSessionID),
			"proxy_configured", strings.TrimSpace(checkoutReq.ProxyURL) != "",
			"cookies_configured", cookies != "",
			"device_id_configured", deviceID != "",
			"country", billingDetails["country"],
			"currency", billingDetails["currency"],
		)
		return "", infraerrors.BadRequest("OPENAI_CHECKOUT_INVALID_URL", "checkout response did not include a valid url")
	}
	return checkoutURL, nil
}

// ResolveAccountProxyURL returns the account proxy URL when one is configured.
func (s *OpenAIOAuthService) ResolveAccountProxyURL(ctx context.Context, account *Account) string {
	if s == nil || s.proxyRepo == nil || account == nil || account.ProxyID == nil {
		return ""
	}
	proxy, err := s.proxyRepo.GetByID(ctx, *account.ProxyID)
	if err != nil || proxy == nil {
		return ""
	}
	return proxy.URL()
}

func extractOAIDeviceIDFromCookies(cookies string) string {
	for _, part := range strings.Split(cookies, ";") {
		name, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		if strings.TrimSpace(name) == "oai-did" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func resolveCheckoutBillingDetails(country, currency string) map[string]string {
	normalizedCountry := strings.ToUpper(strings.TrimSpace(country))
	if normalizedCountry == "" {
		normalizedCountry = "US"
	}
	normalizedCurrency := strings.ToUpper(strings.TrimSpace(currency))
	if normalizedCurrency == "" {
		normalizedCurrency = chatGPTCountryCurrency[normalizedCountry]
	}
	if normalizedCurrency == "" {
		normalizedCurrency = "USD"
	}
	return map[string]string{
		"country":  normalizedCountry,
		"currency": normalizedCurrency,
	}
}

func firstCheckoutURL(result chatGPTCheckoutResponse) string {
	for _, value := range []string{result.URL, result.StripeHostedURL, result.CheckoutURL} {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	if checkoutSessionID := strings.TrimSpace(result.CheckoutSessionID); checkoutSessionID != "" {
		return checkoutURLFromSessionID(checkoutSessionID)
	}
	return ""
}

func decodeCheckoutResponse(value string, result *chatGPTCheckoutResponse) error {
	return json.Unmarshal([]byte(value), result)
}

func checkoutURLFromRawResponse(value string) string {
	var result chatGPTCheckoutResponse
	if decodeCheckoutResponse(value, &result) == nil {
		if checkoutURL := firstCheckoutURL(result); checkoutURL != "" {
			return checkoutURL
		}
	}
	return checkoutURLFromSessionID(extractCheckoutSessionID(value))
}

func extractCheckoutSessionID(value string) string {
	_, after, ok := strings.Cut(value, `"checkout_session_id"`)
	if !ok {
		return ""
	}
	_, after, ok = strings.Cut(after, `:`)
	if !ok {
		return ""
	}
	after = strings.TrimLeft(after, " \t\r\n")
	if !strings.HasPrefix(after, `"`) {
		return ""
	}
	after = strings.TrimPrefix(after, `"`)
	end := strings.Index(after, `"`)
	if end < 0 {
		return ""
	}
	candidate := after[:end]
	for _, r := range candidate {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_') {
			return ""
		}
	}
	return candidate
}

func checkoutURLFromSessionID(checkoutSessionID string) string {
	if checkoutSessionID == "" {
		return ""
	}
	return chatGPTTeamCheckoutBaseURL + checkoutSessionID
}

func truncateCheckoutDiagnostic(value string, maxLen int) string {
	value = strings.TrimSpace(value)
	if maxLen <= 0 || len(value) <= maxLen {
		return value
	}
	return value[:maxLen]
}

func isTrustedCheckoutURL(value string) bool {
	parsed, err := url.Parse(value)
	if err != nil {
		return false
	}
	if parsed.Scheme != "https" || parsed.Host == "" {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == "chatgpt.com" ||
		host == "openai.com" ||
		strings.HasSuffix(host, ".openai.com") ||
		host == "stripe.com" ||
		strings.HasSuffix(host, ".stripe.com")
}
