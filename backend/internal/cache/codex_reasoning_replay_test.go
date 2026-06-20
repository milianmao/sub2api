package cache

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestInsertCodexReasoningReplayItems_InsertsBeforeFirstNonReplayItem(t *testing.T) {
	body := []byte(`{"input":[{"type":"reasoning"},{"type":"input_text","text":"hello"},{"type":"message","content":[{"type":"input_text","text":"world"}]}]}`)
	replayItems := [][]byte{
		[]byte(`{"type":"function_call","call_id":"call_1","name":"noop","arguments":"{}"}`),
	}

	updated, ok := InsertCodexReasoningReplayItems(body, replayItems)

	require.True(t, ok)
	require.Equal(t, "reasoning", gjson.GetBytes(updated, "input.0.type").String())
	require.Equal(t, "function_call", gjson.GetBytes(updated, "input.1.type").String())
	require.Equal(t, "call_1", gjson.GetBytes(updated, "input.1.call_id").String())
	require.Equal(t, "input_text", gjson.GetBytes(updated, "input.2.type").String())
	require.Equal(t, "message", gjson.GetBytes(updated, "input.3.type").String())
}
