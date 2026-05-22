package service

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"github.com/tidwall/gjson"
)

type openAIChatGPTWebImageSSEState struct {
	ConversationID string
	FileIDs        []string
	SedimentIDs    []string
	AssistantText  string
	Blocked        bool
}

var (
	openAIWebImageFilePointerPattern     = regexp.MustCompile(`file-service://([A-Za-z0-9_-]+)`)
	openAIWebImageSedimentPointerPattern = regexp.MustCompile(`sediment://([A-Za-z0-9_-]+)`)
)

func parseOpenAIChatGPTWebImageSSE(body []byte) (openAIChatGPTWebImageSSEState, error) {
	state := openAIChatGPTWebImageSSEState{}
	forEachOpenAISSEDataPayload(string(body), func(payload []byte) {
		if !gjson.ValidBytes(payload) {
			return
		}
		if cid := strings.TrimSpace(gjson.GetBytes(payload, "conversation_id").String()); cid != "" {
			state.ConversationID = cid
		}
		if cid := strings.TrimSpace(gjson.GetBytes(payload, "conversation.id").String()); cid != "" {
			state.ConversationID = cid
		}
		if gjson.GetBytes(payload, "blocked").Bool() || strings.EqualFold(gjson.GetBytes(payload, "type").String(), "moderation_blocked") {
			state.Blocked = true
		}
		message := gjson.GetBytes(payload, "message")
		if message.Exists() && message.Get("author.role").String() == "assistant" {
			parts := message.Get("content.parts")
			parts.ForEach(func(_, part gjson.Result) bool {
				if text := strings.TrimSpace(part.String()); text != "" {
					if state.AssistantText != "" {
						state.AssistantText += "\n"
					}
					state.AssistantText += text
				}
				return true
			})
		}
		extractOpenAIChatGPTWebImagePointers(payload, &state)
	})
	if len(bytes.TrimSpace(body)) > 0 && state.ConversationID == "" && len(state.FileIDs) == 0 && len(state.SedimentIDs) == 0 && state.AssistantText == "" {
		return state, fmt.Errorf("malformed chatgpt web image SSE")
	}
	return state, nil
}

func extractOpenAIChatGPTWebImagePointers(payload []byte, state *openAIChatGPTWebImageSSEState) {
	if state == nil {
		return
	}
	text := string(payload)
	for _, match := range openAIWebImageFilePointerPattern.FindAllStringSubmatch(text, -1) {
		if len(match) == 2 {
			appendUniqueString(&state.FileIDs, match[1])
		}
	}
	for _, match := range openAIWebImageSedimentPointerPattern.FindAllStringSubmatch(text, -1) {
		if len(match) == 2 {
			appendUniqueString(&state.SedimentIDs, match[1])
		}
	}
}

func appendUniqueString(values *[]string, value string) {
	value = strings.TrimSpace(value)
	if value == "" || values == nil {
		return
	}
	for _, existing := range *values {
		if existing == value {
			return
		}
	}
	*values = append(*values, value)
}
