package openai

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultModelsIncludesCodexGPTImage2(t *testing.T) {
	ids := DefaultModelIDs()

	require.Contains(t, ids, "codex-gpt-image-2")
}

func TestDefaultModelsIncludeBareGPT56Alias(t *testing.T) {
	require.Contains(t, DefaultModelIDs(), "gpt-5.6")
}
