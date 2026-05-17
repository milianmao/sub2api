package apicompat

// ChatCompletionsResponseToResponses converts a Chat Completions response into
// a Responses response. Non-standard Chat reasoning_content is kept out of
// output_text and represented as a reasoning summary when present.
func ChatCompletionsResponseToResponses(resp *ChatCompletionsResponse) *ResponsesResponse {
	if resp == nil {
		return &ResponsesResponse{Object: "response", Status: "completed"}
	}

	out := &ResponsesResponse{
		ID:     resp.ID,
		Object: "response",
		Model:  resp.Model,
		Status: chatFinishReasonToResponsesStatus(firstChatFinishReason(resp)),
	}

	if len(resp.Choices) == 0 {
		return out
	}

	choice := resp.Choices[0]
	msg := choice.Message

	if msg.ReasoningContent != "" {
		out.Output = append(out.Output, ResponsesOutput{
			Type: "reasoning",
			Summary: []ResponsesSummary{{
				Type: "summary_text",
				Text: msg.ReasoningContent,
			}},
		})
	}

	if text, err := parseChatContent(msg.Content); err == nil && text != "" {
		out.Output = append(out.Output, ResponsesOutput{
			Type: "message",
			Role: "assistant",
			Content: []ResponsesContentPart{{
				Type: "output_text",
				Text: text,
			}},
		})
	}

	for _, tc := range msg.ToolCalls {
		out.Output = append(out.Output, ResponsesOutput{
			Type:      "function_call",
			CallID:    tc.ID,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		})
	}

	if resp.Usage != nil {
		out.Usage = &ResponsesUsage{
			InputTokens:  resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
			TotalTokens:  resp.Usage.PromptTokens + resp.Usage.CompletionTokens,
		}
		if resp.Usage.PromptTokensDetails != nil && resp.Usage.PromptTokensDetails.CachedTokens > 0 {
			out.Usage.InputTokensDetails = &ResponsesInputTokensDetails{
				CachedTokens: resp.Usage.PromptTokensDetails.CachedTokens,
			}
		}
	}

	return out
}

// ChatCompletionChunkToResponsesEvents converts one Chat Completions streaming
// chunk into Responses stream events. reasoning_content is emitted only as a
// reasoning summary delta, never as an output_text delta.
func ChatCompletionChunkToResponsesEvents(chunk *ChatCompletionsChunk) []ResponsesStreamEvent {
	if chunk == nil || len(chunk.Choices) == 0 {
		return nil
	}

	var events []ResponsesStreamEvent
	for _, choice := range chunk.Choices {
		if choice.Delta.ReasoningContent != nil && *choice.Delta.ReasoningContent != "" {
			events = append(events, ResponsesStreamEvent{
				Type:  "response.reasoning_summary_text.delta",
				Delta: *choice.Delta.ReasoningContent,
			})
		}
		if choice.Delta.Content != nil && *choice.Delta.Content != "" {
			events = append(events, ResponsesStreamEvent{
				Type:  "response.output_text.delta",
				Delta: *choice.Delta.Content,
			})
		}
		if choice.FinishReason != nil {
			events = append(events, ResponsesStreamEvent{
				Type: "response.completed",
				Response: &ResponsesResponse{
					ID:     chunk.ID,
					Object: "response",
					Model:  chunk.Model,
					Status: chatFinishReasonToResponsesStatus(*choice.FinishReason),
				},
			})
		}
	}

	return events
}

func firstChatFinishReason(resp *ChatCompletionsResponse) string {
	if resp == nil || len(resp.Choices) == 0 {
		return ""
	}
	return resp.Choices[0].FinishReason
}

func chatFinishReasonToResponsesStatus(finishReason string) string {
	if finishReason == "length" {
		return "incomplete"
	}
	return "completed"
}
