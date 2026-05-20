package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/imroc/req/v3"
	"github.com/stretchr/testify/require"
)

func TestOpenAIOAuthService_CreateCheckoutLink_PostsExpectedPayloadWithProxy(t *testing.T) {
	var capturedProxyURL string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "Bearer access-token-1", r.Header.Get("Authorization"))
		require.Equal(t, "https://chatgpt.com", r.Header.Get("Origin"))
		require.Equal(t, "https://chatgpt.com/#pricing", r.Header.Get("Referer"))
		require.Contains(t, r.Header.Get("Accept"), "application/json")
		require.Contains(t, r.Header.Get("Content-Type"), "application/json")
		require.NotEmpty(t, r.Header.Get("User-Agent"))
		require.Equal(t, "en-US,en;q=0.9", r.Header.Get("Accept-Language"))
		require.Equal(t, "same-origin", r.Header.Get("Sec-Fetch-Site"))
		require.Equal(t, "cors", r.Header.Get("Sec-Fetch-Mode"))
		require.Equal(t, "empty", r.Header.Get("Sec-Fetch-Dest"))
		require.Equal(t, "foo=bar; oai-did=device-123; other=value", r.Header.Get("Cookie"))
		require.Equal(t, "device-123", r.Header.Get("oai-device-id"))

		var payload map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		require.Equal(t, "chatgptplusplan", payload["plan_name"])
		require.Equal(t, "https://chatgpt.com/#pricing", payload["cancel_url"])
		require.Equal(t, "hosted", payload["checkout_ui_mode"])
		require.Equal(t, map[string]any{
			"country":  "US",
			"currency": "USD",
		}, payload["billing_details"])
		require.Equal(t, map[string]any{
			"promo_campaign_id":          "plus-1-month-free",
			"is_coupon_from_query_param": false,
		}, payload["promo_campaign"])

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"url":"https://chatgpt.com/payments/checkout/session-1"}`))
	}))
	defer server.Close()

	oldURL := SetChatGPTCheckoutURLForTest(server.URL)
	defer SetChatGPTCheckoutURLForTest(oldURL)

	svc := NewOpenAIOAuthService(nil, nil)
	svc.SetPrivacyClientFactory(func(proxyURL string) (*req.Client, error) {
		capturedProxyURL = proxyURL
		return req.C(), nil
	})

	url, err := svc.CreateCheckoutLink(context.Background(), CreateCheckoutLinkRequest{
		AccessToken: "access-token-1",
		ProxyURL:    "http://proxy.example.com:8080",
		Cookies:     "foo=bar; oai-did=device-123; other=value",
	})
	require.NoError(t, err)
	require.Equal(t, "https://chatgpt.com/payments/checkout/session-1", url)
	require.Equal(t, "http://proxy.example.com:8080", capturedProxyURL)
}

func TestOpenAIOAuthService_CreateCheckoutLink_UsesCountryCurrencyMapping(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		require.Equal(t, map[string]any{
			"country":  "GB",
			"currency": "GBP",
		}, payload["billing_details"])

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"url":"https://chatgpt.com/payments/checkout/session-1"}`))
	}))
	defer server.Close()

	oldURL := SetChatGPTCheckoutURLForTest(server.URL)
	defer SetChatGPTCheckoutURLForTest(oldURL)

	svc := NewOpenAIOAuthService(nil, nil)
	svc.SetPrivacyClientFactory(func(proxyURL string) (*req.Client, error) {
		return req.C(), nil
	})

	checkoutURL, err := svc.CreateCheckoutLink(context.Background(), CreateCheckoutLinkRequest{
		AccessToken: "access-token-1",
		Country:     "gb",
	})
	require.NoError(t, err)
	require.Equal(t, "https://chatgpt.com/payments/checkout/session-1", checkoutURL)
}

func TestOpenAIOAuthService_CreateCheckoutLink_AcceptsAlternateURLFields(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "stripe hosted url", body: `{"stripe_hosted_url":"https://chatgpt.com/payments/checkout/session-1"}`},
		{name: "checkout url", body: `{"checkout_url":"https://chatgpt.com/payments/checkout/session-1"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			oldURL := SetChatGPTCheckoutURLForTest(server.URL)
			defer SetChatGPTCheckoutURLForTest(oldURL)

			svc := NewOpenAIOAuthService(nil, nil)
			svc.SetPrivacyClientFactory(func(proxyURL string) (*req.Client, error) {
				return req.C(), nil
			})

			checkoutURL, err := svc.CreateCheckoutLink(context.Background(), CreateCheckoutLinkRequest{AccessToken: "access-token-1"})
			require.NoError(t, err)
			require.Equal(t, "https://chatgpt.com/payments/checkout/session-1", checkoutURL)
		})
	}
}

func TestOpenAIOAuthService_CreateCheckoutLink_RejectsNonHTTPURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"url":"https://evil.example/payments/checkout/session-1"}`))
	}))
	defer server.Close()

	oldURL := SetChatGPTCheckoutURLForTest(server.URL)
	defer SetChatGPTCheckoutURLForTest(oldURL)

	svc := NewOpenAIOAuthService(nil, nil)
	svc.SetPrivacyClientFactory(func(proxyURL string) (*req.Client, error) {
		return req.C(), nil
	})

	url, err := svc.CreateCheckoutLink(context.Background(), CreateCheckoutLinkRequest{AccessToken: "access-token-1"})
	require.Error(t, err)
	require.Empty(t, url)
}

func TestOpenAIOAuthService_CreateCheckoutLink_MapsUpstreamUnauthorizedToBadGateway(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()

	oldURL := SetChatGPTCheckoutURLForTest(server.URL)
	defer SetChatGPTCheckoutURLForTest(oldURL)

	svc := NewOpenAIOAuthService(nil, nil)
	svc.SetPrivacyClientFactory(func(proxyURL string) (*req.Client, error) {
		return req.C(), nil
	})

	url, err := svc.CreateCheckoutLink(context.Background(), CreateCheckoutLinkRequest{
		AccessToken: "access-token-1",
		Cookies:     "foo=bar; oai-did=device-123",
	})
	require.Error(t, err)
	require.Empty(t, url)
	require.Equal(t, http.StatusBadGateway, infraerrors.Code(err))
}
