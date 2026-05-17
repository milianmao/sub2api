import { describe, expect, it } from "vitest";

import {
  buildOpenAICompatPayload,
  createDefaultMessagesDispatchFormState,
  isOpenAICompatPlatform,
  messagesDispatchConfigToFormState,
  messagesDispatchFormStateToConfig,
  resetOpenAICompatFormState,
  resetMessagesDispatchFormState,
} from "../groupsMessagesDispatch";

describe("groupsMessagesDispatch", () => {
  it("returns the expected default form state", () => {
    expect(createDefaultMessagesDispatchFormState()).toEqual({
      allow_messages_dispatch: false,
      opus_mapped_model: "gpt-5.4",
      sonnet_mapped_model: "gpt-5.3-codex",
      haiku_mapped_model: "gpt-5.4-mini",
      exact_model_mappings: [],
    });
  });

  it("sanitizes exact model mapping rows when converting to config", () => {
    const config = messagesDispatchFormStateToConfig({
      allow_messages_dispatch: true,
      opus_mapped_model: " gpt-5.4 ",
      sonnet_mapped_model: "gpt-5.3-codex",
      haiku_mapped_model: " gpt-5.4-mini ",
      exact_model_mappings: [
        {
          claude_model: " claude-sonnet-4-5-20250929 ",
          target_model: " gpt-5.2 ",
        },
        { claude_model: "", target_model: "gpt-5.4" },
        { claude_model: "claude-opus-4-6", target_model: " " },
      ],
    });

    expect(config).toEqual({
      opus_mapped_model: "gpt-5.4",
      sonnet_mapped_model: "gpt-5.3-codex",
      haiku_mapped_model: "gpt-5.4-mini",
      exact_model_mappings: {
        "claude-sonnet-4-5-20250929": "gpt-5.2",
      },
    });
  });

  it("hydrates form state from api config", () => {
    expect(
      messagesDispatchConfigToFormState({
        opus_mapped_model: "gpt-5.4",
        sonnet_mapped_model: "gpt-5.2",
        haiku_mapped_model: "gpt-5.4-mini",
        exact_model_mappings: {
          "claude-opus-4-6": "gpt-5.4",
          "claude-haiku-4-5-20251001": "gpt-5.4-mini",
        },
      }),
    ).toEqual({
      allow_messages_dispatch: false,
      opus_mapped_model: "gpt-5.4",
      sonnet_mapped_model: "gpt-5.2",
      haiku_mapped_model: "gpt-5.4-mini",
      exact_model_mappings: [
        {
          claude_model: "claude-haiku-4-5-20251001",
          target_model: "gpt-5.4-mini",
        },
        { claude_model: "claude-opus-4-6", target_model: "gpt-5.4" },
      ],
    });
  });

  it("resets mutable form state when platform switches away from openai", () => {
    const state = {
      allow_messages_dispatch: true,
      opus_mapped_model: "gpt-5.2",
      sonnet_mapped_model: "gpt-5.4",
      haiku_mapped_model: "gpt-5.1",
      exact_model_mappings: [
        { claude_model: "claude-opus-4-6", target_model: "gpt-5.4" },
      ],
    };

    resetMessagesDispatchFormState(state);

    expect(state).toEqual({
      allow_messages_dispatch: false,
      opus_mapped_model: "gpt-5.4",
      sonnet_mapped_model: "gpt-5.3-codex",
      haiku_mapped_model: "gpt-5.4-mini",
      exact_model_mappings: [],
    });
  });

  it("only treats anthropic and antigravity as OpenAI-compatible platforms", () => {
    expect(isOpenAICompatPlatform("anthropic")).toBe(true);
    expect(isOpenAICompatPlatform("antigravity")).toBe(true);
    expect(isOpenAICompatPlatform("openai")).toBe(false);
    expect(isOpenAICompatPlatform("gemini")).toBe(false);
    expect(isOpenAICompatPlatform("deepseek" as any)).toBe(false);
  });

  it("normalizes the OpenAI compatibility payload by platform", () => {
    expect(
      buildOpenAICompatPayload({
        platform: "anthropic",
        allow_openai_compat: true,
      }),
    ).toEqual({ allow_openai_compat: true });

    expect(
      buildOpenAICompatPayload({
        platform: "openai",
        allow_openai_compat: true,
      }),
    ).toEqual({ allow_openai_compat: false });
  });

  it("resets OpenAI compatibility state when platform switches away", () => {
    const state = {
      platform: "gemini" as const,
      allow_openai_compat: true,
    };

    resetOpenAICompatFormState(state);

    expect(state.allow_openai_compat).toBe(false);
  });
});
