package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseMicrosoftGraphMessages(t *testing.T) {
	body := []byte(`{
		"value": [
			{
				"subject": "Your code is 112233",
				"receivedDateTime": "2026-05-22T10:30:00Z",
				"bodyPreview": "Use 112233 to continue",
				"from": {"emailAddress": {"name": "Microsoft", "address": "security@example.com"}},
				"body": {"contentType": "text", "content": "Full body 112233"}
			}
		]
	}`)

	messages, err := parseMicrosoftGraphMessages(body)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, "Your code is 112233", messages[0].Subject)
	require.Equal(t, "security@example.com", messages[0].From)
	require.Equal(t, time.Date(2026, 5, 22, 10, 30, 0, 0, time.UTC), messages[0].ReceivedAt)
	require.Equal(t, "Use 112233 to continue", messages[0].BodyPreview)
	require.Equal(t, "Full body 112233", messages[0].BodyText)
}
