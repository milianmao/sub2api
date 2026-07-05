package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	microsoftOAuthTokenEndpoint = "https://login.microsoftonline.com/common/oauth2/v2.0/token"
	microsoftGraphMessagesURL   = "https://graph.microsoft.com/v1.0/me/mailFolders/inbox/messages"
	microsoftGraphHTTPTimeout   = 15 * time.Second
)

type MicrosoftGraphHTTPClient struct {
	httpClient  *http.Client
	tokenURL    string
	messagesURL string
}

func NewMicrosoftGraphHTTPClient() *MicrosoftGraphHTTPClient {
	httpClient := &http.Client{Timeout: microsoftGraphHTTPTimeout}
	return &MicrosoftGraphHTTPClient{httpClient: httpClient, tokenURL: microsoftOAuthTokenEndpoint, messagesURL: microsoftGraphMessagesURL}
}

func (c *MicrosoftGraphHTTPClient) RefreshAccessToken(ctx context.Context, clientID, refreshToken string) (string, error) {
	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("refresh_token", refreshToken)
	form.Set("grant_type", "refresh_token")
	form.Set("scope", "https://graph.microsoft.com/.default offline_access")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("microsoft token refresh failed: status %d: %s", resp.StatusCode, sanitizeMicrosoftGraphErrorBody(body))
	}

	var payload struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", err
	}
	if payload.AccessToken == "" {
		if payload.Error != "" || payload.ErrorDesc != "" {
			return "", fmt.Errorf("microsoft token refresh failed: %s %s", payload.Error, payload.ErrorDesc)
		}
		return "", fmt.Errorf("microsoft token refresh failed: missing access token")
	}
	return payload.AccessToken, nil
}

func (c *MicrosoftGraphHTTPClient) ListRecentMessages(ctx context.Context, accessToken string, limit int) ([]MicrosoftGraphMessage, error) {
	if limit <= 0 {
		limit = microsoftVerificationMessageLimit
	}
	u, err := url.Parse(c.messagesURL)
	if err != nil {
		return nil, err
	}
	query := u.Query()
	query.Set("$top", fmt.Sprintf("%d", limit))
	query.Set("$orderby", "receivedDateTime desc")
	query.Set("$select", "subject,from,receivedDateTime,bodyPreview,body")
	u.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Prefer", `outlook.body-content-type="text"`)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("microsoft messages fetch failed: status %d: %s", resp.StatusCode, sanitizeMicrosoftGraphErrorBody(body))
	}
	return parseMicrosoftGraphMessages(body)
}

func parseMicrosoftGraphMessages(body []byte) ([]MicrosoftGraphMessage, error) {
	var payload struct {
		Value []struct {
			Subject          string `json:"subject"`
			ReceivedDateTime string `json:"receivedDateTime"`
			BodyPreview      string `json:"bodyPreview"`
			From             struct {
				EmailAddress struct {
					Name    string `json:"name"`
					Address string `json:"address"`
				} `json:"emailAddress"`
			} `json:"from"`
			Body struct {
				Content string `json:"content"`
			} `json:"body"`
		} `json:"value"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	messages := make([]MicrosoftGraphMessage, 0, len(payload.Value))
	for _, item := range payload.Value {
		receivedAt, _ := time.Parse(time.RFC3339, item.ReceivedDateTime)
		from := item.From.EmailAddress.Address
		if from == "" {
			from = item.From.EmailAddress.Name
		}
		messages = append(messages, MicrosoftGraphMessage{
			Subject:     item.Subject,
			From:        from,
			ReceivedAt:  receivedAt,
			BodyPreview: item.BodyPreview,
			BodyText:    item.Body.Content,
		})
	}
	return messages, nil
}

func sanitizeMicrosoftGraphErrorBody(body []byte) string {
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return "empty response"
	}
	var payload struct {
		Error any `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err == nil && payload.Error != nil {
		sanitized, err := json.Marshal(payload.Error)
		if err == nil {
			return string(sanitized)
		}
	}
	if len(body) > 2048 {
		body = body[:2048]
	}
	return string(body)
}
