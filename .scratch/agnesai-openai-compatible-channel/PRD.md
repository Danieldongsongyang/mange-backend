# PRD: AgnesAI OpenAI-Compatible Channel

Status: ready-for-agent
Labels: ready-for-agent

## Problem Statement

用户想在当前 AI API 网关中新增一个 AgnesAI 渠道。AgnesAI 与 OpenAI API 高度兼容，但不是完全兼容。用户当前的直觉是复制一份 OpenAI 渠道实现，然后改成 AgnesAI；但现有 OpenAI adaptor 是全局共享基座，包含 Azure、OpenRouter、Xinference、Realtime、Audio、Images、Responses、Responses Compact、Rerank 等大量历史兼容逻辑，直接复制会带来维护成本和未来分叉风险。

用户需要一个清晰、可继续讨论和实现的产品需求文档，在新窗口中无需重新探索全局项目结构，就能继续围绕 AgnesAI 的接入方式做决策和实现。

## Solution

新增 AgnesAI 作为独立的 channel type 和 API type，但实现上采用“OpenAI-compatible thin adaptor”策略：不复制完整 OpenAI 渠道包，而是新增一个 AgnesAI provider adaptor，在大多数兼容行为上复用 OpenAI 基座能力，只在 AgnesAI 与 OpenAI 不一致的地方覆盖 URL、鉴权 header、请求字段转换、响应处理和模型列表。

从用户视角看，AgnesAI 应作为后台可配置的独立渠道出现，有自己的渠道名称、默认 base URL、模型列表、key 提示和测试入口。对 API 调用方来说，AgnesAI 应尽可能通过现有 OpenAI-compatible endpoint 工作，例如 chat completions、embeddings、responses 或其他 AgnesAI 实际支持的能力。对维护者来说，AgnesAI 的实现应足够薄，后续 OpenAI 基座修复和通用能力增强可以自然复用，避免维护一个复制出来的大型分叉。

## User Stories

1. As an 网关管理员, I want AgnesAI to appear as a distinct channel type, so that I can configure AgnesAI credentials separately from generic OpenAI channels.
2. As an 网关管理员, I want AgnesAI to have its own display name and default base URL, so that channel creation is straightforward and less error-prone.
3. As an 网关管理员, I want AgnesAI to support a configured API key, so that requests can authenticate to the AgnesAI upstream.
4. As an 网关管理员, I want AgnesAI channel tests to use the same gateway test flow as other channels, so that I can verify connectivity before enabling the channel.
5. As an 网关管理员, I want AgnesAI to expose its supported model list in the dashboard, so that I can select and enable models without guessing model IDs.
6. As an 网关管理员, I want AgnesAI to work with model mapping, so that user-facing model names can map to upstream AgnesAI model names.
7. As an 网关管理员, I want AgnesAI to work with channel priority and weight, so that it participates in load distribution like other providers.
8. As an 网关管理员, I want AgnesAI to work with channel auto-disable behavior, so that repeated upstream failures can protect users from broken routing.
9. As an API caller, I want OpenAI-compatible chat completions requests to route to AgnesAI, so that existing clients need minimal code changes.
10. As an API caller, I want streaming chat completions to work when AgnesAI supports streaming, so that interactive clients keep their current UX.
11. As an API caller, I want explicit zero values in request parameters to be preserved, so that values like `temperature: 0` or `stream: false` behave intentionally.
12. As an API caller, I want unsupported OpenAI request fields to be safely omitted or adapted, so that AgnesAI does not reject otherwise usable requests.
13. As an API caller, I want AgnesAI errors to be returned through the gateway's normal error path, so that client behavior stays consistent with other channels.
14. As an API caller, I want usage accounting to be correct for non-stream responses, so that billing and logs reflect actual upstream usage.
15. As an API caller, I want usage accounting to be correct for stream responses, so that streamed completions do not undercount or overcount quota.
16. As a 网关维护者, I want AgnesAI implemented as a thin adaptor over OpenAI-compatible behavior, so that the implementation remains small and understandable.
17. As a 网关维护者, I want AgnesAI to override only provider-specific differences, so that common OpenAI-compatible fixes continue to benefit AgnesAI.
18. As a 网关维护者, I want AgnesAI to avoid copying the full OpenAI adaptor, so that future divergence and duplicated bug fixes are minimized.
19. As a 网关维护者, I want AgnesAI to have an independent API type, so that future provider-specific logic can be added without changing generic OpenAI behavior.
20. As a 网关维护者, I want AgnesAI to have an independent channel type, so that database records and frontend configuration remain explicit.
21. As a 网关维护者, I want the adaptor to use the existing common request-sending path, so that header overrides, timeout behavior, proxy behavior, and request logging stay consistent.
22. As a 网关维护者, I want AgnesAI's request conversion behavior to be tested through public relay behavior where possible, so that tests protect real user behavior rather than implementation details.
23. As a 网关维护者, I want provider-specific request conversion to be covered by focused tests when needed, so that AgnesAI differences are documented and stable.
24. As a 网关维护者, I want any JSON serialization logic to use the project JSON wrapper, so that the implementation follows project conventions.
25. As a 网关维护者, I want database-facing changes to avoid schema churn, so that adding AgnesAI does not introduce cross-database migration risk.
26. As a frontend admin user, I want AgnesAI to appear in the default frontend channel selector, so that I can configure it without manual database edits.
27. As a frontend admin user, I want AgnesAI to have a sensible icon and key prompt, so that the channel form is understandable.
28. As a classic frontend user, I want AgnesAI to appear in the classic frontend if that theme remains supported, so that both themes stay functionally aligned.
29. As a support/debugging user, I want AgnesAI logs to show the correct channel owner/name, so that request diagnosis is not confused with generic OpenAI.
30. As a future contributor, I want the AgnesAI implementation documented as OpenAI-compatible but not a full OpenAI copy, so that future edits preserve the intended architecture.

## Implementation Decisions

- AgnesAI will be modeled as a first-class channel type rather than using the generic OpenAI channel type.
- AgnesAI will receive a distinct API type so the relay adaptor registry can select AgnesAI-specific behavior.
- AgnesAI will use a new provider adaptor that composes or delegates to the OpenAI-compatible adaptor rather than copying the entire OpenAI implementation.
- The AgnesAI adaptor will call the shared request-sending path so that existing header override, timeout, proxy, logging, request body length, and transport behavior are reused.
- AgnesAI will override request URL construction only if its upstream endpoint paths differ from standard OpenAI-compatible paths.
- AgnesAI will override request headers only where its authentication or provider-specific headers differ from OpenAI's default Bearer-token behavior.
- AgnesAI will implement OpenAI-compatible request conversion as the central provider-specific seam.
- AgnesAI will not inherit OpenAI behavior through implicit method promotion where that could bypass AgnesAI overrides. Explicit delegation is preferred so the code path is clear.
- AgnesAI will reuse existing OpenAI response handlers when response bodies and usage structures are compatible.
- AgnesAI will introduce custom response handling only for modes where AgnesAI's response or streaming usage format differs from OpenAI.
- AgnesAI will only advertise supported capabilities. Unsupported relay modes should fail clearly rather than pretending compatibility.
- AgnesAI stream support will be decided after confirming whether AgnesAI supports streaming and whether it supports `stream_options.include_usage`.
- AgnesAI should not be added to the stream-options support map until that upstream behavior is confirmed.
- AgnesAI model list should be provider-specific, even if some model names overlap with OpenAI-compatible conventions.
- AgnesAI should participate in existing model mapping, group enablement, channel distribution, and logging without special database schema changes.
- AgnesAI should appear in the default frontend channel configuration if the feature is intended to be usable from the admin UI.
- AgnesAI should also be mirrored in the classic frontend channel options if classic remains a supported theme for this installation.
- AgnesAI should not modify protected project identity, metadata, branding, package names, or attribution.
- No database schema changes are expected for the first implementation. The existing channel model should be sufficient.
- Any provider-specific DTOs should preserve explicit zero values for optional scalar request fields by using pointer semantics.
- Any new JSON marshal/unmarshal behavior should use the project's common JSON wrapper functions.
- Existing OpenAI-compatible channel behavior must not regress as part of AgnesAI implementation.

## Testing Decisions

- The highest-value test seam is the relay behavior for an AgnesAI-configured channel against a controlled upstream response, because that verifies routing, adaptor selection, request construction, response handling, and usage behavior through the user-visible API surface.
- A focused adaptor-level seam is acceptable for AgnesAI-only request conversion rules, especially when the provider requires deleting, renaming, or adapting specific OpenAI fields.
- Header construction should be tested through the request-sending seam when possible, because header override precedence is a shared behavior that should remain intact.
- Stream behavior should be tested only after the expected AgnesAI streaming contract is known.
- If AgnesAI reuses OpenAI response handlers, tests should verify external response and usage behavior rather than duplicating OpenAI handler internals.
- If AgnesAI introduces custom streaming response parsing, tests should cover streamed chunks, final usage, malformed chunks, and missing usage fallback behavior.
- Model list behavior should be tested through the existing model listing/dashboard expectations if static model exposure is part of the feature.
- Channel test behavior should be tested if AgnesAI has a non-standard model-fetching or health-check endpoint.
- Good tests should assert externally observable behavior: selected adaptor behavior, upstream request shape, response body, usage, and error handling.
- Tests should avoid asserting private helper call order, internal struct mutation details, or implementation-specific delegation choices unless those are the only stable seam for a provider-specific conversion.
- Prior art exists in existing provider adaptor tests and response handler tests for OpenAI-compatible providers, Claude conversion, Gemini conversion, usage patching, image streaming, and channel request header behavior.
- Regression tests should include at least one case proving existing OpenAI channels still keep their expected behavior when AgnesAI is added.

## Out of Scope

- Building a full fork of the OpenAI channel implementation.
- Rewriting the OpenAI adaptor into a new abstraction as part of AgnesAI.
- Changing the global relay architecture.
- Changing database schema unless later AgnesAI-specific credential or settings requirements force it.
- Adding billing expression changes unless AgnesAI introduces model-specific pricing requirements in a later task.
- Implementing unsupported AgnesAI capabilities just because the OpenAI adaptor has handlers for them.
- Guaranteeing support for Responses, Images, Audio, Realtime, Rerank, or Claude/Gemini-format inputs before AgnesAI's actual upstream contract is confirmed.
- Renaming or removing protected project branding, metadata, package names, or attribution.

## Further Notes

The current architectural recommendation is: do not copy the full OpenAI channel. Add AgnesAI as an independent provider adaptor that delegates to OpenAI-compatible behavior and overrides only the differences.

The next implementation window should begin by reading the global project map and this PRD, then confirming AgnesAI's exact upstream API contract before writing code. The most important unknowns are endpoint paths, authentication headers, supported request fields, supported relay modes, streaming usage contract, model-list endpoint, and error/usage response compatibility.

The PRD assumes AgnesAI is highly OpenAI-compatible but not guaranteed to be fully compatible. Where the provider contract is unknown, implementation should choose conservative behavior: support the confirmed subset, fail clearly for unsupported modes, and avoid adding optimistic compatibility that could create hidden billing or response bugs.

## Confirmed AgnesAI Contract

Confirmed on 2026-06-29 from user-provided docs and live checks:

- Default backend base URL: `https://apihub.agnes-ai.com`.
- Official SDK-style base URL may be `https://apihub.agnes-ai.com/v1`; backend URL construction must avoid duplicate `/v1`.
- Chat Completions endpoint is standard: `{base}/v1/chat/completions`.
- Authentication is standard Bearer token: `Authorization: Bearer <key>`.
- `/v1/models` is supported and returns OpenAI-like `data[].id` plus an extra top-level `success: true`.
- Known models: `agnes-1.5-flash`, `agnes-2.0-flash`, `agnes-image-2.0-flash`, `agnes-image-2.1-flash`, `agnes-video-v2.0`.
- Non-stream Chat Completions are supported.
- Streaming Chat Completions are supported.
- `stream_options.include_usage` is supported and should be preserved.
- Responses API `/v1/responses` is supported, but should be tested because AgnesAI docs mainly promote Chat Completions.
- Embeddings are not supported; `/v1/embeddings` returned 404 in testing.
- Audio is not confirmed and should not be exposed in the first implementation.
- Rerank is not supported.
- Realtime is not supported in the first implementation.
- Image generation is supported via `/v1/images/generations`.
- OpenAI multipart image edits should not be exposed as supported. Agnes image edit/image-to-image behavior uses JSON image fields or `extra_body.image` on `/v1/images/generations`.
- Video is supported through `/v1/videos`; polling can use old-style `/v1/videos/{task_id}` compatibility, while official guidance recommends `GET https://apihub.agnes-ai.com/agnesapi?video_id=...`.

Field compatibility:

- `tools` and `tool_choice` are documented as supported.
- `parallel_tool_calls` is not confirmed; it may be tolerated but should not be treated as a guaranteed capability.
- `store` and `metadata` are not confirmed for Chat Completions semantics, although they may be tolerated and Responses bodies include similar fields.
- `reasoning_effort` should not be treated as an AgnesAI thinking control. AgnesAI thinking is controlled through `chat_template_kwargs.enable_thinking`.
- `response_format` for Chat appears tolerated for JSON-style output, but exact OpenAI parity is not guaranteed.
- For Images, top-level `response_format` should not be sent directly; map it to `extra_body.response_format` or equivalent provider-specific fields.
- `modalities` and audio-related fields should not be exposed in the first implementation.

Usage and error compatibility:

- Chat Completions usage is core-compatible with OpenAI: `prompt_tokens`, `completion_tokens`, `total_tokens`.
- Streaming usage with `include_usage` is supported, but final chunk shape is not guaranteed to be byte-for-byte OpenAI style. Tests should cover this.
- Responses usage follows Responses-style `input_tokens`, `output_tokens`, `total_tokens` and details fields.
- Error responses are OpenAI-like under `error.message`, `error.type`, `error.code`, but not fully equivalent. `type`, `param`, `code`, and HTTP status should not be assumed to match OpenAI exactly.

First implementation scope:

- Support Chat Completions non-stream and stream.
- Preserve `stream_options.include_usage`.
- Support `/v1/models`.
- Support `/v1/responses`.
- Support `/v1/images/generations`.
- Route `agnes-video-v2.0` through the OpenAI/Sora video task path if old `/v1/videos/{task_id}` compatibility is sufficient.
- Do not expose Embeddings, Audio, Rerank, Realtime, or OpenAI multipart image edits as supported AgnesAI capabilities.

## Appendix: 最重要判断点

在继续实现 AgnesAI 之前，必须先回答这些问题。这些问题决定 AgnesAI 是一个非常薄的 OpenAI-compatible adaptor，还是需要更多自定义逻辑。

1. AgnesAI 的 base URL 是否能直接拼接标准 OpenAI-compatible 路径，例如 chat completions、embeddings、responses？
2. AgnesAI 的鉴权是否是标准 `Authorization: Bearer <key>`？
3. AgnesAI 是否支持 `/v1/models` 或等价的模型列表接口？
4. AgnesAI 是否支持非流式 chat completions？
5. AgnesAI 是否支持流式 chat completions？
6. AgnesAI 是否支持 `stream_options.include_usage`？
7. AgnesAI 是否支持 OpenAI Responses API？
8. AgnesAI 是否支持 embeddings？
9. AgnesAI 是否支持 images、audio、realtime、rerank 等 OpenAI adaptor 已经覆盖但 AgnesAI 未必支持的模式？
10. AgnesAI 是否支持 Claude-format 或 Gemini-format 客户端请求经网关转换后转发？
11. AgnesAI 不支持哪些常见 OpenAI 请求字段，例如 store、metadata、tools、tool_choice、parallel_tool_calls、reasoning_effort、response_format、stream_options？
12. AgnesAI 是否要求新增 provider-specific 请求字段或 header？
13. AgnesAI 的非流式 usage 结构是否与 OpenAI 完全一致？
14. AgnesAI 的流式 usage 结构是否与 OpenAI 完全一致？
15. AgnesAI 的错误响应结构是否与 OpenAI 兼容，还是需要转换成网关已有错误格式？
16. AgnesAI 的模型名是否需要映射、后缀适配、或特殊 reasoning/search 参数转换？
17. AgnesAI 的默认模型列表和默认 base URL 是什么？
18. AgnesAI 是否需要在后台提供特殊 key 格式提示？
19. AgnesAI 是否需要特殊渠道设置，还是复用现有 channel setting / other setting 即可？
20. AgnesAI 的首版目标是支持最小 chat completions，还是同时支持 embeddings、responses、stream、model fetch 等完整体验？
