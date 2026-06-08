package guard

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// ConfuseKey returns a deterministic obfuscated key for a given account, kind,
// and value. Same inputs produce the same output; different account IDs produce
// isolated outputs.
func ConfuseKey(accountID int64, kind string, value string) string {
	name := fmt.Sprintf("sub2api:identity-confuse:%s:account_%d:%s",
		strings.TrimSpace(kind),
		accountID,
		strings.TrimSpace(value),
	)
	h := sha256.Sum256([]byte(name))
	return fmt.Sprintf("%x", h[:16])
}

// ConfuseState tracks original-to-obfuscated mappings for a single request so
// response payloads can be restored transparently.
type ConfuseState struct {
	mu sync.Mutex

	AccountID int64

	PromptCacheKey string
	origCacheKey   string

	origTurnIDs map[string]string
}

func newConfuseState(accountID int64) *ConfuseState {
	return &ConfuseState{
		AccountID:   accountID,
		origTurnIDs: make(map[string]string),
	}
}

func (s *ConfuseState) recordTurnID(obfuscated, original string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.origTurnIDs[obfuscated] = original
}

// ConfuseBody obfuscates session identity fields in the JSON request body.
func ConfuseBody(body []byte, accountID int64) ([]byte, *ConfuseState) {
	if len(body) == 0 || accountID <= 0 {
		return body, nil
	}

	state := newConfuseState(accountID)
	updated := body

	if pcKey := strings.TrimSpace(gjson.GetBytes(updated, "prompt_cache_key").String()); pcKey != "" {
		state.origCacheKey = pcKey
		state.PromptCacheKey = ConfuseKey(accountID, "prompt-cache", pcKey)
		updated, _ = sjson.SetBytes(updated, "prompt_cache_key", state.PromptCacheKey)
	}

	if instID := strings.TrimSpace(gjson.GetBytes(updated, "client_metadata.x-codex-installation-id").String()); instID != "" {
		updated, _ = sjson.SetBytes(updated, "client_metadata.x-codex-installation-id", ConfuseKey(accountID, "installation", instID))
	}

	if turnMeta := strings.TrimSpace(gjson.GetBytes(updated, "client_metadata.x-codex-turn-metadata").String()); turnMeta != "" {
		updated = confuseTurnMetadata(updated, turnMeta, state)
	}

	if state.PromptCacheKey != "" {
		if winID := strings.TrimSpace(gjson.GetBytes(updated, "client_metadata.x-codex-window-id").String()); winID != "" {
			_ = winID
			updated, _ = sjson.SetBytes(updated, "client_metadata.x-codex-window-id", state.PromptCacheKey+":0")
		}
	}

	return updated, state
}

func confuseTurnMetadata(body []byte, rawMeta string, state *ConfuseState) []byte {
	updatedMeta := rawMeta

	if state.PromptCacheKey != "" && state.origCacheKey != "" {
		if gjson.Get(rawMeta, "prompt_cache_key").Exists() {
			updatedMeta, _ = sjson.Set(updatedMeta, "prompt_cache_key", state.PromptCacheKey)
		} else {
			updatedMeta = strings.ReplaceAll(updatedMeta, state.origCacheKey, state.PromptCacheKey)
		}
	}

	if turnID := strings.TrimSpace(gjson.Get(rawMeta, "turn_id").String()); turnID != "" {
		obfuscated := ConfuseKey(state.AccountID, "turn", turnID)
		updatedMeta, _ = sjson.Set(updatedMeta, "turn_id", obfuscated)
		state.recordTurnID(obfuscated, turnID)
	}

	if state.PromptCacheKey != "" && gjson.Get(rawMeta, "window_id").Exists() {
		updatedMeta, _ = sjson.Set(updatedMeta, "window_id", state.PromptCacheKey+":0")
	}

	body, _ = sjson.SetBytes(body, "client_metadata.x-codex-turn-metadata", updatedMeta)
	return body
}

// ConfuseHeaders obfuscates session identity fields in the request headers.
func ConfuseHeaders(headers http.Header, accountID int64, state *ConfuseState, confusedPromptCacheKey string) {
	if headers == nil || accountID <= 0 {
		return
	}

	for _, key := range []string{"Session-Id", "Session_id", "session_id", "Session_ID"} {
		if v := strings.TrimSpace(headers.Get(key)); v != "" {
			obfuscated := ConfuseKey(accountID, "session", v)
			delete(headers, "Session-Id")
			delete(headers, "Session_id")
			delete(headers, "session_id")
			delete(headers, "Session_ID")
			headers.Set("session_id", obfuscated)
			break
		}
	}

	if convID := strings.TrimSpace(headers.Get("Conversation_id")); convID != "" {
		headers.Set("Conversation_id", ConfuseKey(accountID, "conversation", convID))
	} else if confusedPromptCacheKey != "" {
		headers.Set("Conversation_id", confusedPromptCacheKey)
	}

	if headers.Get("X-Client-Request-Id") == "" {
		headers.Set("X-Client-Request-Id", confusedPromptCacheKey)
	}
	if confusedPromptCacheKey != "" {
		headers.Set("Thread-Id", confusedPromptCacheKey)
		headers.Set("X-Codex-Window-Id", confusedPromptCacheKey+":0")
	}

	_ = state
}

// RestoreResponseRestorer returns a response payload restorer function.
func RestoreResponseRestorer(state *ConfuseState) func([]byte) []byte {
	if state == nil {
		return func(b []byte) []byte { return b }
	}
	return func(payload []byte) []byte {
		return restoreResponse(payload, state)
	}
}

func restoreResponse(payload []byte, state *ConfuseState) []byte {
	if len(payload) == 0 || state == nil {
		return payload
	}

	restored := payload
	if state.PromptCacheKey != "" && state.origCacheKey != "" {
		restored = bytesReplaceAll(restored, []byte(state.PromptCacheKey), []byte(state.origCacheKey))
	}
	for obfuscated, original := range state.origTurnIDs {
		if obfuscated != original {
			restored = bytesReplaceAll(restored, []byte(obfuscated), []byte(original))
		}
	}
	return restored
}

func bytesReplaceAll(src, old, new []byte) []byte {
	if len(src) == 0 || len(old) == 0 || len(new) == 0 {
		return src
	}
	result := make([]byte, 0, len(src))
	for {
		i := indexOf(src, old)
		if i < 0 {
			result = append(result, src...)
			break
		}
		result = append(result, src[:i]...)
		result = append(result, new...)
		src = src[i+len(old):]
	}
	return result
}

func indexOf(s, sep []byte) int {
	if len(sep) == 0 {
		return 0
	}
	for i := 0; i <= len(s)-len(sep); i++ {
		if string(s[i:i+len(sep)]) == string(sep) {
			return i
		}
	}
	return -1
}
