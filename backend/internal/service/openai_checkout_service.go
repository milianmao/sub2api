package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var chatGPTCheckoutURL = "https://chatgpt.com/backend-api/payments/checkout"

func SetChatGPTCheckoutURLForTest(url string) string {
	oldURL := chatGPTCheckoutURL
	chatGPTCheckoutURL = url
	return oldURL
}

type chatGPTCheckoutPayload struct {
	PlanName       string                 `json:"plan_name"`
	BillingDetails map[string]string      `json:"billing_details"`
	CancelURL      string                 `json:"cancel_url"`
	PromoCampaign  map[string]any         `json:"promo_campaign"`
	CheckoutUIMode string                 `json:"checkout_ui_mode"`
}

type chatGPTCheckoutResponse struct {
	URL string `json:"url"`
}

// CreateCheckoutLink creates a hosted ChatGPT Plus checkout URL using an OpenAI OAuth access token.
func (s *OpenAIOAuthService) CreateCheckoutLink(ctx context.Context, accessToken, proxyURL string) (string, error) {
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return "", infraerrors.BadRequest("OPENAI_CHECKOUT_ACCESS_TOKEN_REQUIRED", "access token is required")
	}
	if s == nil || s.privacyClientFactory == nil {
		return "", infraerrors.InternalServer("OPENAI_CHECKOUT_CLIENT_UNAVAILABLE", "checkout client is unavailable")
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	client, err := s.privacyClientFactory(proxyURL)
	if err != nil {
		return "", infraerrors.InternalServer("OPENAI_CHECKOUT_CLIENT_FAILED", "failed to create checkout client").WithCause(err)
	}

	var result chatGPTCheckoutResponse
	payloadBytes, err := json.Marshal(chatGPTCheckoutPayload{
		PlanName: "chatgptplusplan",
		BillingDetails: map[string]string{
			"country":  "US",
			"currency": "USD",
		},
		CancelURL: "https://chatgpt.com/#pricing",
		PromoCampaign: map[string]any{
			"promo_campaign_id":          "plus-1-month-free",
			"is_coupon_from_query_param": false,
		},
		CheckoutUIMode: "hosted",
	})
	if err != nil {
		return "", infraerrors.InternalServer("OPENAI_CHECKOUT_PAYLOAD_FAILED", "failed to build checkout payload").WithCause(err)
	}

	resp, err := client.R().
		SetContext(ctx).
		SetHeader("Authorization", "Bearer "+accessToken).
		SetHeader("Origin", "https://chatgpt.com").
		SetHeader("Referer", "https://chatgpt.com/#pricing").
		SetHeader("Accept", "application/json").
		SetHeader("Content-Type", "application/json").
		SetSuccessResult(&result).
		SetBody(payloadBytes).
		Post(chatGPTCheckoutURL)
	if err != nil {
		return "", infraerrors.InternalServer("OPENAI_CHECKOUT_REQUEST_FAILED", "checkout request failed").WithCause(err)
	}
	if !resp.IsSuccessState() {
		return "", infraerrors.Newf(http.StatusBadGateway, "OPENAI_CHECKOUT_UPSTREAM_FAILED", "checkout request failed with upstream status %d", resp.StatusCode)
	}

	checkoutURL := strings.TrimSpace(result.URL)
	if !isTrustedCheckoutURL(checkoutURL) {
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
		host == "stripe.com" ||
		strings.HasSuffix(host, ".stripe.com")
}
