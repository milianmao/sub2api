package openai

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultModelsIncludesCodexGPTImage2(t *testing.T) {
	ids := DefaultModelIDs()

	require.Contains(t, ids, "codex-gpt-image-2")
}
