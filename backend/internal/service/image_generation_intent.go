package service

import (
	"strings"

	"github.com/tidwall/gjson"
)

const (
	openAIResponsesEndpoint          = "/v1/responses"
	openAIResponsesCompactEndpoint   = "/v1/responses/compact"
	responsesLiteHeader              = "X-OpenAI-Internal-Codex-Responses-Lite"
	responsesLiteHeaderKey           = "x-openai-internal-codex-responses-lite"
	responsesLiteWSMetadataKey       = "ws_request_header_x_openai_internal_codex_responses_lite"
	imageGenerationPermissionMessage = "Image generation is not enabled for this group"
)

func isOpenAIResponsesLiteHeader(value string) bool {
	return strings.EqualFold(strings.TrimSpace(value), "true")
}

func isOpenAIResponsesLiteWebSocketPayload(body []byte) bool {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return false
	}
	return isOpenAIResponsesLiteHeader(gjson.GetBytes(body, "client_metadata."+responsesLiteWSMetadataKey).String())
}

// ImageGenerationPermissionMessage returns the stable end-user error text for disabled groups.
func ImageGenerationPermissionMessage() string {
	return imageGenerationPermissionMessage
}

// GroupAllowsImageGeneration preserves ungrouped-key behavior and enforces the flag when a group is present.
func GroupAllowsImageGeneration(group *Group) bool {
	return group == nil || group.AllowImageGeneration
}

// IsImageGenerationIntent classifies requests that can produce generated images.
func IsImageGenerationIntent(endpoint string, requestedModel string, body []byte) bool {
	if IsImageGenerationEndpoint(endpoint) {
		return true
	}
	if isOpenAIResponsesEndpoint(endpoint) {
		return openAIResponsesBodyHasImageGenerationIntent(body)
	}
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return false
	}

	var modelSeen, toolsSeen, inputSeen, toolChoiceSeen bool
	imageIntent := false
	parseRawJSONView(body).ForEach(func(key, value gjson.Result) bool {
		// GetBytes returns the first duplicate key; retain that behavior while walking the root once.
		switch key.Str {
		case "model":
			if !modelSeen {
				modelSeen = true
				imageIntent = isOpenAIImageGenerationModel(strings.TrimSpace(value.String()))
			}
		case "tools":
			if !toolsSeen {
				toolsSeen = true
				imageIntent = openAIJSONToolsContainImageGeneration(value)
			}
		case "input":
			if !inputSeen {
				inputSeen = true
				imageIntent = openAIJSONInputContainsImageGenTool(value)
			}
		case "tool_choice":
			if !toolChoiceSeen {
				toolChoiceSeen = true
				imageIntent = openAIJSONToolChoiceSelectsImageGeneration(value)
			}
		}
		return !imageIntent && (!modelSeen || !toolsSeen || !inputSeen || !toolChoiceSeen)
	})
	return imageIntent
}

// IsExplicitImageGenerationIntent 仅检测原生 image_generation 工具、图片模型和显式 tool_choice，
// 不检测被动的 image_gen namespace 声明。用于 capability 路由决策——被动 namespace 不应
// 强制要求原生 Responses 能力，否则 Chat Completions-only 账号会被误过滤（#4476）。
func IsExplicitImageGenerationIntent(endpoint string, requestedModel string, body []byte) bool {
	if IsImageGenerationEndpoint(endpoint) || isOpenAIImageGenerationModel(requestedModel) {
		return true
	}
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return false
	}
	var modelSeen, toolsSeen, toolChoiceSeen bool
	imageIntent := false
	parseRawJSONView(body).ForEach(func(key, value gjson.Result) bool {
		switch key.Str {
		case "model":
			if !modelSeen {
				modelSeen = true
				imageIntent = isOpenAIImageGenerationModel(strings.TrimSpace(value.String()))
			}
		case "tools":
			if !toolsSeen {
				toolsSeen = true
				imageIntent = openAIJSONToolsContainNativeImageGeneration(value)
			}
		case "tool_choice":
			if !toolChoiceSeen {
				toolChoiceSeen = true
				imageIntent = openAIJSONToolChoiceSelectsExplicitImageGeneration(value)
			}
		}
		return !imageIntent && (!modelSeen || !toolsSeen || !toolChoiceSeen)
	})
	return imageIntent
}

// IsImageGenerationIntentForPlatform applies platform-specific intent rules.
//
// Codex advertises the image_gen namespace on ordinary Responses requests so
// that it is available if the model needs it. Grok strips namespace and
// Responses Lite additional_tools declarations before forwarding, so those
// declarations alone must not turn every Codex request into an image request.
// Native image_generation tools, explicit image selection and image models
// remain image intent. Other platforms retain the original declaration rule.
func IsImageGenerationIntentForPlatform(endpoint string, requestedModel string, body []byte, platform string) bool {
	if !strings.EqualFold(strings.TrimSpace(platform), PlatformGrok) {
		return IsImageGenerationIntent(endpoint, requestedModel, body)
	}
	return isExplicitGrokImageGenerationIntent(endpoint, requestedModel, body)
}

func isExplicitGrokImageGenerationIntent(endpoint string, requestedModel string, body []byte) bool {
	if IsImageGenerationEndpoint(endpoint) || isOpenAIImageGenerationModel(requestedModel) {
		return true
	}
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return false
	}

	var modelSeen, toolsSeen, toolChoiceSeen bool
	imageIntent := false
	parseRawJSONView(body).ForEach(func(key, value gjson.Result) bool {
		switch key.Str {
		case "model":
			if !modelSeen {
				modelSeen = true
				imageIntent = isOpenAIImageGenerationModel(strings.TrimSpace(value.String()))
			}
		case "tools":
			if !toolsSeen {
				toolsSeen = true
				// Grok removes namespace catalogs before forwarding. Native
				// image_generation remains an explicit capability request.
				imageIntent = openAIJSONToolsContainNativeImageGeneration(value)
			}
		case "tool_choice":
			if !toolChoiceSeen {
				toolChoiceSeen = true
				imageIntent = openAIJSONToolChoiceSelectsExplicitImageGeneration(value)
			}
		}
		return !imageIntent && (!modelSeen || !toolsSeen || !toolChoiceSeen)
	})
	return imageIntent
}

// IsImageGenerationIntentMap is the map-backed variant used after service-side request mutation.
func IsImageGenerationIntentMap(endpoint string, requestedModel string, reqBody map[string]any) bool {
	if IsImageGenerationEndpoint(endpoint) {
		return true
	}
	if isOpenAIResponsesEndpoint(endpoint) {
		return openAIResponsesMapHasImageGenerationIntent(reqBody)
	}
	return false
}

// IsImageGenerationEndpoint identifies dedicated generated-image endpoints.
func IsImageGenerationEndpoint(endpoint string) bool {
	switch normalizeImageGenerationEndpoint(endpoint) {
	case "/v1/images/generations", "/v1/images/edits", "/v1/images/variations", "/images/generations", "/images/edits", "/images/variations":
		return true
	default:
		return false
	}
}

func normalizeImageGenerationEndpoint(endpoint string) string {
	endpoint = strings.TrimSpace(strings.ToLower(endpoint))
	if endpoint == "" {
		return ""
	}
	endpoint = strings.TrimPrefix(endpoint, "https://api.openai.com")
	if idx := strings.IndexByte(endpoint, '?'); idx >= 0 {
		endpoint = endpoint[:idx]
	}
	return strings.TrimRight(endpoint, "/")
}

// IsOpenAIResponsesEndpoint identifies Responses endpoints whose body can request generated images.
func IsOpenAIResponsesEndpoint(endpoint string) bool {
	normalized := normalizeImageGenerationEndpoint(endpoint)
	return normalized == openAIResponsesEndpoint || strings.HasPrefix(normalized, openAIResponsesEndpoint+"/")
}

func isOpenAIResponsesEndpoint(endpoint string) bool {
	return IsOpenAIResponsesEndpoint(endpoint)
}

func openAIResponsesBodyHasImageGenerationIntent(body []byte) bool {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return false
	}

	seen := make(map[string]struct{}, 8)
	imageIntent := false
	parseRawJSONView(body).ForEach(func(key, value gjson.Result) bool {
		if _, alreadySeen := seen[key.Str]; alreadySeen {
			return true
		}
		seen[key.Str] = struct{}{}

		switch key.Str {
		case "model":
			imageIntent = isOpenAIImageGenerationModel(strings.TrimSpace(value.String()))
		case "tools":
			imageIntent = openAIJSONToolsContainImageGeneration(value) || openAIJSONValueHasImageGenerationIntent(value)
		case "input":
			imageIntent = openAIJSONInputContainsImageGenTool(value) || openAIJSONValueHasImageGenerationIntent(value)
		case "tool_choice":
			imageIntent = openAIJSONToolChoiceSelectsImageGeneration(value) || openAIJSONValueHasImageGenerationIntent(value)
		case "type", "recipient", "name":
			imageIntent = isOpenAIImageIntentToken(openAIJSONString(value))
		case "modalities", "output_modalities", "response_modalities":
			imageIntent = openAIJSONModalitiesContainImage(value)
		default:
			// Scan unrelated values without revisiting duplicate top-level fields.
			imageIntent = openAIJSONValueHasImageGenerationIntent(value)
		}
		return !imageIntent
	})
	return imageIntent
}

func openAIResponsesMapHasImageGenerationIntent(reqBody map[string]any) bool {
	if reqBody == nil {
		return false
	}
	if isOpenAIImageGenerationModel(firstNonEmptyString(reqBody["model"])) {
		return true
	}
	if hasOpenAIImageGenerationTool(reqBody) {
		return true
	}
	if openAIAnyToolChoiceSelectsImageGeneration(reqBody["tool_choice"]) {
		return true
	}
	return openAIAnyValueHasImageGenerationIntent(reqBody)
}

func openAIJSONValueHasImageGenerationIntent(value gjson.Result) bool {
	switch {
	case value.IsArray():
		found := false
		value.ForEach(func(_, item gjson.Result) bool {
			if openAIJSONValueHasImageGenerationIntent(item) {
				found = true
				return false
			}
			return true
		})
		return found
	case value.IsObject():
		if isOpenAIImageIntentToken(value.Get("type").String()) || isOpenAIImageIntentToken(value.Get("recipient").String()) || isOpenAIImageIntentToken(value.Get("name").String()) {
			return true
		}
		if openAIJSONModalitiesContainImage(value.Get("modalities")) || openAIJSONModalitiesContainImage(value.Get("output_modalities")) || openAIJSONModalitiesContainImage(value.Get("response_modalities")) {
			return true
		}
		found := false
		value.ForEach(func(_, item gjson.Result) bool {
			if openAIJSONValueHasImageGenerationIntent(item) {
				found = true
				return false
			}
			return true
		})
		return found
	default:
		return false
	}
}

func openAIJSONModalitiesContainImage(value gjson.Result) bool {
	if value.Type == gjson.String {
		return isOpenAIImageModality(value.String())
	}
	if !value.IsArray() {
		return false
	}
	found := false
	value.ForEach(func(_, item gjson.Result) bool {
		if item.Type == gjson.String && isOpenAIImageModality(item.String()) {
			found = true
			return false
		}
		return true
	})
	return found
}

func openAIAnyValueHasImageGenerationIntent(value any) bool {
	switch v := value.(type) {
	case map[string]any:
		if isOpenAIImageIntentToken(firstNonEmptyString(v["type"])) || isOpenAIImageIntentToken(firstNonEmptyString(v["recipient"])) || isOpenAIImageIntentToken(firstNonEmptyString(v["name"])) {
			return true
		}
		if openAIAnyModalitiesContainImage(v["modalities"]) || openAIAnyModalitiesContainImage(v["output_modalities"]) || openAIAnyModalitiesContainImage(v["response_modalities"]) {
			return true
		}
		for _, item := range v {
			if openAIAnyValueHasImageGenerationIntent(item) {
				return true
			}
		}
	case []any:
		for _, item := range v {
			if openAIAnyValueHasImageGenerationIntent(item) {
				return true
			}
		}
	}
	return false
}

func openAIAnyModalitiesContainImage(value any) bool {
	switch v := value.(type) {
	case string:
		return isOpenAIImageModality(v)
	case []any:
		for _, item := range v {
			if isOpenAIImageModality(firstNonEmptyString(item)) {
				return true
			}
		}
	case []string:
		for _, item := range v {
			if isOpenAIImageModality(item) {
				return true
			}
		}
	}
	return false
}

func isOpenAIImageModality(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "image", "images", "output_image":
		return true
	default:
		return false
	}
}

func isOpenAIImageIntentToken(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "image_generation", "image_generation_call", "generate_image", "create_image", "output_image", "image_gen", "image-generation", "dalle", "dall-e":
		return true
	default:
		return false
	}
}

func openAIJSONToolsContainImageGeneration(tools gjson.Result) bool {
	if !tools.IsArray() {
		return false
	}
	found := false
	tools.ForEach(func(_, item gjson.Result) bool {
		if isOpenAIImageGenerationType(openAIJSONString(item.Get("type"))) {
			found = true
			return false
		}
		if isImageGenNamespaceTool(item) {
			found = true
			return false
		}
		return true
	})
	return found
}

func openAIJSONToolsContainNativeImageGeneration(tools gjson.Result) bool {
	if !tools.IsArray() {
		return false
	}
	found := false
	tools.ForEach(func(_, item gjson.Result) bool {
		found = isOpenAIImageGenerationType(openAIJSONString(item.Get("type")))
		return !found
	})
	return found
}

func isOpenAIImageGenerationType(value string) bool {
	return strings.TrimSpace(value) == "image_generation"
}

func isOpenAIImageGenNamespaceName(value string) bool {
	return strings.TrimSpace(value) == "image_gen"
}

// isImageGenNamespaceTool detects the namespace advertised by Codex's built-in
// image-generation extension instead of a hosted image_generation tool.
func isImageGenNamespaceTool(tool gjson.Result) bool {
	return openAIJSONString(tool.Get("type")) == "namespace" &&
		isOpenAIImageGenNamespaceName(openAIJSONString(tool.Get("name")))
}

// openAIJSONInputContainsImageGenTool scans Responses input items for
// additional_tools entries that declare the image_gen namespace. This covers
// the "Responses Lite" format where tools are embedded inside input items
// rather than top-level tools.
func openAIJSONInputContainsImageGenTool(input gjson.Result) bool {
	if !input.IsArray() {
		return false
	}
	found := false
	input.ForEach(func(_, item gjson.Result) bool {
		if openAIJSONString(item.Get("type")) != "additional_tools" {
			return true
		}
		found = openAIJSONToolsContainImageGeneration(item.Get("tools"))
		return !found
	})
	return found
}

func openAIRequestBodyHasImageGenerationDeclaration(body []byte) bool {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return false
	}
	return openAIJSONToolsContainImageGeneration(gjson.GetBytes(body, "tools")) ||
		openAIJSONInputContainsImageGenTool(gjson.GetBytes(body, "input")) ||
		openAIJSONToolChoiceSelectsImageGeneration(gjson.GetBytes(body, "tool_choice"))
}

func openAIRequestBodyImageGenerationToolNeedsNormalization(body []byte) bool {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return false
	}
	tools := gjson.GetBytes(body, "tools")
	if !tools.IsArray() {
		return false
	}
	needsNormalization := false
	tools.ForEach(func(_, item gjson.Result) bool {
		if openAIJSONString(item.Get("type")) != "image_generation" {
			return true
		}
		// 只有旧字段或明确的模型不兼容字段需要修正时才进入 map 修改。
		if item.Get("format").Exists() || item.Get("compression").Exists() {
			needsNormalization = true
			return false
		}
		imageModel := strings.ToLower(strings.TrimSpace(item.Get("model").String()))
		if strings.HasPrefix(imageModel, "gpt-image-2") && item.Get("input_fidelity").Exists() {
			needsNormalization = true
			return false
		}
		return true
	})
	return needsNormalization
}

func openAIJSONToolChoiceSelectsImageGeneration(choice gjson.Result) bool {
	if !choice.Exists() {
		return false
	}
	if choice.Type == gjson.String {
		return isOpenAIImageGenerationType(choice.String())
	}
	if !choice.IsObject() {
		return false
	}
	choiceType := openAIJSONString(choice.Get("type"))
	if isOpenAIImageGenerationType(choiceType) {
		return true
	}
	if choiceType == "namespace" &&
		(isOpenAIImageGenNamespaceName(openAIJSONString(choice.Get("name"))) ||
			isOpenAIImageGenNamespaceName(openAIJSONString(choice.Get("namespace")))) {
		return true
	}
	if tool := choice.Get("tool"); tool.IsObject() && openAIJSONToolChoiceSelectsImageGeneration(tool) {
		return true
	}
	if isOpenAIImageGenerationType(openAIJSONString(choice.Get("function.name"))) {
		return true
	}
	return false
}

func openAIJSONToolChoiceSelectsExplicitImageGeneration(choice gjson.Result) bool {
	if openAIJSONToolChoiceSelectsImageGeneration(choice) {
		return true
	}
	if !choice.IsObject() {
		return false
	}
	if tool := choice.Get("tool"); tool.IsObject() && openAIJSONToolChoiceSelectsExplicitImageGeneration(tool) {
		return true
	}
	if isOpenAIImageGenFunctionReference(
		openAIJSONString(choice.Get("namespace")),
		openAIJSONString(choice.Get("name")),
	) {
		return true
	}
	if fn := choice.Get("function"); fn.IsObject() {
		return isOpenAIImageGenFunctionReference(
			openAIJSONString(fn.Get("namespace")),
			openAIJSONString(fn.Get("name")),
		)
	}
	return false
}

func isOpenAIImageGenFunctionReference(namespace string, name string) bool {
	namespace = strings.TrimSpace(namespace)
	name = strings.TrimSpace(name)
	if namespace == "image_gen" && name == "imagegen" {
		return true
	}
	switch name {
	case "image_gen.imagegen", "image_gen__imagegen":
		return true
	default:
		return false
	}
}

func openAIAnyToolChoiceSelectsImageGeneration(choice any) bool {
	switch v := choice.(type) {
	case string:
		return isOpenAIImageGenerationType(v)
	case map[string]any:
		choiceType := strings.TrimSpace(firstNonEmptyString(v["type"]))
		if isOpenAIImageGenerationType(choiceType) {
			return true
		}
		if choiceType == "namespace" &&
			(isOpenAIImageGenNamespaceName(firstNonEmptyString(v["name"])) ||
				isOpenAIImageGenNamespaceName(firstNonEmptyString(v["namespace"]))) {
			return true
		}
		if tool, ok := v["tool"].(map[string]any); ok && openAIAnyToolChoiceSelectsImageGeneration(tool) {
			return true
		}
		if fn, ok := v["function"].(map[string]any); ok && isOpenAIImageGenerationType(firstNonEmptyString(fn["name"])) {
			return true
		}
	}
	return false
}

func getAPIKeyFromContext(c interface{ Get(string) (any, bool) }) *APIKey {
	if c == nil {
		return nil
	}
	v, exists := c.Get("api_key")
	if !exists {
		return nil
	}
	apiKey, _ := v.(*APIKey)
	return apiKey
}

func apiKeyGroup(apiKey *APIKey) *Group {
	if apiKey == nil {
		return nil
	}
	return apiKey.Group
}

type OpenAIResponsesImageBillingConfig struct {
	Model     string
	SizeTier  string
	InputSize string
}

func resolveOpenAIResponsesImageBillingConfigDetailed(reqBody map[string]any, fallbackModel string) (OpenAIResponsesImageBillingConfig, error) {
	imageModel := ""
	imageSize := ""
	hasImageTool := false
	if reqBody != nil {
		rawTools, _ := reqBody["tools"].([]any)
		for _, rawTool := range rawTools {
			toolMap, ok := rawTool.(map[string]any)
			if !ok || strings.TrimSpace(firstNonEmptyString(toolMap["type"])) != "image_generation" {
				continue
			}
			hasImageTool = true
			imageModel = strings.TrimSpace(firstNonEmptyString(toolMap["model"]))
			imageSize = strings.TrimSpace(firstNonEmptyString(toolMap["size"]))
			break
		}
		if imageSize == "" {
			imageSize = strings.TrimSpace(firstNonEmptyString(reqBody["size"]))
		}
	}
	if imageModel == "" && reqBody != nil {
		bodyModel := strings.TrimSpace(firstNonEmptyString(reqBody["model"]))
		if isOpenAIImageBillingModelAlias(bodyModel) || !hasImageTool {
			imageModel = bodyModel
		}
	}
	if imageModel == "" && hasImageTool {
		imageModel = "gpt-image-2"
	}
	if imageModel == "" {
		imageModel = strings.TrimSpace(fallbackModel)
	}
	sizeTier := normalizeOpenAIImageSizeTier(imageSize)
	return OpenAIResponsesImageBillingConfig{
		Model:     imageModel,
		SizeTier:  sizeTier,
		InputSize: imageSize,
	}, nil
}

func resolveOpenAIResponsesImageBillingConfigFromBody(body []byte, fallbackModel string) (string, string, error) {
	cfg, err := resolveOpenAIResponsesImageBillingConfigDetailedFromBody(body, fallbackModel)
	if err != nil {
		return "", "", err
	}
	return cfg.Model, cfg.SizeTier, nil
}

func resolveOpenAIResponsesImageBillingConfigDetailedFromBody(body []byte, fallbackModel string) (OpenAIResponsesImageBillingConfig, error) {
	imageModel := ""
	imageSize := ""
	hasImageTool := false
	if len(body) > 0 && gjson.ValidBytes(body) {
		tools := gjson.GetBytes(body, "tools")
		if tools.IsArray() {
			tools.ForEach(func(_, item gjson.Result) bool {
				if openAIJSONString(item.Get("type")) != "image_generation" {
					return true
				}
				hasImageTool = true
				imageModel = openAIJSONString(item.Get("model"))
				imageSize = openAIJSONString(item.Get("size"))
				return false
			})
		}
		if imageSize == "" {
			imageSize = openAIJSONString(gjson.GetBytes(body, "size"))
		}
		if imageModel == "" {
			bodyModel := openAIJSONString(gjson.GetBytes(body, "model"))
			if isOpenAIImageBillingModelAlias(bodyModel) || !hasImageTool {
				imageModel = bodyModel
			}
		}
	}
	if imageModel == "" && hasImageTool {
		imageModel = "gpt-image-2"
	}
	if imageModel == "" {
		imageModel = strings.TrimSpace(fallbackModel)
	}
	return OpenAIResponsesImageBillingConfig{
		Model:     imageModel,
		SizeTier:  normalizeOpenAIImageSizeTier(imageSize),
		InputSize: imageSize,
	}, nil
}

func isOpenAIImageBillingModelAlias(model string) bool {
	normalized := strings.ToLower(strings.TrimSpace(model))
	if normalized == "" {
		return false
	}
	return isOpenAIImageGenerationModel(normalized) || strings.Contains(normalized, "image")
}

func openAIJSONString(value gjson.Result) string {
	if value.Type != gjson.String {
		return ""
	}
	return strings.TrimSpace(value.String())
}
