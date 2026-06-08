// Package guard provides anti-detection mechanisms.
package guard

import (
	"net/http"
	"strings"
)

// CanonicalizeSessionHeaders unifies all session_id header variants into a
// single lowercase "session_id" form. Conflicting variants are removed to
// prevent duplicate headers.
func CanonicalizeSessionHeaders(headers http.Header) {
	if headers == nil {
		return
	}

	var sessionID string
	for _, key := range []string{"session_id", "Session_id", "Session-Id", "Session_ID"} {
		if v := strings.TrimSpace(headers.Get(key)); v != "" {
			sessionID = v
		}
	}
	if sessionID == "" {
		return
	}

	delete(headers, "Session-Id")
	delete(headers, "Session_id")
	delete(headers, "session_id")
	delete(headers, "Session_ID")
	headers.Set("session_id", sessionID)
}

// EnsureCodexHeaders injects standard Codex headers that real ChatGPT clients
// would normally send but reverse proxies might strip.
func EnsureCodexHeaders(headers http.Header, promptCacheKey string) {
	if headers == nil {
		return
	}
	if headers.Get("X-Client-Request-Id") == "" && promptCacheKey != "" {
		headers.Set("X-Client-Request-Id", promptCacheKey)
	}
	if promptCacheKey != "" {
		headers.Set("Thread-Id", promptCacheKey)
		headers.Set("X-Codex-Window-Id", promptCacheKey+":0")
	}
}

// SyncConversationID ensures Conversation_id is present and synchronized with
// session_id when no conversation header was already provided.
func SyncConversationID(headers http.Header) {
	if headers == nil {
		return
	}
	sessionID := strings.TrimSpace(headers.Get("session_id"))
	if sessionID == "" {
		sessionID = strings.TrimSpace(headers.Get("Session_id"))
	}
	if sessionID == "" {
		return
	}
	if strings.TrimSpace(headers.Get("Conversation_id")) == "" {
		headers.Set("Conversation_id", sessionID)
	}
}

// ApplySessionGovernance runs all session header governance rules in order.
func ApplySessionGovernance(headers http.Header, promptCacheKey string) {
	CanonicalizeSessionHeaders(headers)
	EnsureCodexHeaders(headers, promptCacheKey)
	SyncConversationID(headers)
}
