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

	url, err := svc.CreateCheckoutLink(context.Background(), "access-token-1", "http://proxy.example.com:8080")
	require.NoError(t, err)
	require.Equal(t, "https://chatgpt.com/payments/checkout/session-1", url)
	require.Equal(t, "http://proxy.example.com:8080", capturedProxyURL)
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

	url, err := svc.CreateCheckoutLink(context.Background(), "access-token-1", "")
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

	url, err := svc.CreateCheckoutLink(context.Background(), "access-token-1", "")
	require.Error(t, err)
	require.Empty(t, url)
	require.Equal(t, http.StatusBadGateway, infraerrors.Code(err))
}
