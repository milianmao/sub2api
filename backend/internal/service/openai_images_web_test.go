package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseOpenAIChatGPTWebImageSSE(t *testing.T) {
	body := []byte("data: {\"type\":\"conversation_detail_metadata\",\"conversation_id\":\"conv_123\"}\n\n" +
		"data: {\"message\":{\"author\":{\"role\":\"assistant\"},\"content\":{\"parts\":[\"done\"]}}}\n\n" +
		"data: {\"message\":{\"author\":{\"role\":\"tool\"},\"metadata\":{\"async_task_type\":\"image_gen\"},\"content\":{\"content_type\":\"multimodal_text\",\"parts\":[{\"asset_pointer\":\"file-service://file_abc\"},\"sediment://sed_xyz\"]}}}\n\n")

	got, err := parseOpenAIChatGPTWebImageSSE(body)

	require.NoError(t, err)
	require.Equal(t, "conv_123", got.ConversationID)
	require.Equal(t, []string{"file_abc"}, got.FileIDs)
	require.Equal(t, []string{"sed_xyz"}, got.SedimentIDs)
	require.Equal(t, "done", got.AssistantText)
	require.False(t, got.Blocked)
}
