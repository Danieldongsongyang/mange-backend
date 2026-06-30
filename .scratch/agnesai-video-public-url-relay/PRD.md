Status: ready-for-agent

# PRD: AgnesAI Video Relay Uses JSON and Public Image URLs

## Problem Statement

Canvas users cannot reliably create AgnesAI video tasks through the backend gateway when the request includes reference images.

The current Canvas video flow sends `multipart/form-data` to the backend, with reference images attached as `input_reference[]` files. The backend routes AgnesAI video tasks through the same video task adapter used for Sora/OpenAI-compatible video requests. That adapter rebuilds and forwards multipart file uploads to the upstream provider.

AgnesAI Video V2.0 does not accept this request shape for video creation. Its official video API expects `POST /v1/videos` with `Content-Type: application/json`. For image-to-video, multi-image video generation, and keyframe animation, AgnesAI requires publicly accessible image URLs. Single-image input is represented as a top-level `image` value, and multi-image/keyframe input is represented as `extra_body.image`.

As a result, AgnesAI/LiteLLM receives an uploaded file object instead of JSON containing public image URLs, then fails while trying to serialize that file object to JSON. Users see video task creation fail with an upstream 500 error. The backend also lacks explicit pricing configuration for `agnes-video-v2.0`, causing an unexpectedly high pre-consume amount before the failed task is refunded.

From the user's perspective, selecting a reference image in Canvas and asking for AgnesAI video generation should create a video task successfully, charge predictably, and later return playable video content. Today it fails before the task is created.

## Solution

Add provider-specific AgnesAI video relay behavior behind the existing asynchronous task adapter interface.

AgnesAI video requests should no longer be treated as Sora/OpenAI-compatible multipart video requests. Instead, AgnesAI video task creation should be converted into the request contract documented by AgnesAI:

- Submit video task creation as JSON to the AgnesAI video endpoint.
- Send `Content-Type: application/json` to AgnesAI.
- Use top-level `image` for a single public reference image URL.
- Use `extra_body.image` for multiple public reference image URLs or keyframe inputs.
- Preserve text-to-video behavior when no reference image is supplied.
- Prefer AgnesAI's `video_id`-based result retrieval while preserving compatibility with task-id retrieval where needed.
- Extract the final playable video URL from AgnesAI's completed task response and expose it through the existing video content flow.
- Add billing configuration for `agnes-video-v2.0` so task pre-consume is intentional rather than falling through to a misleading default.

If Canvas sends reference image files, the backend must not forward those files to AgnesAI as multipart uploads. The implementation should either publish those files to publicly accessible URLs before submitting the AgnesAI task, or return a clear client-facing validation error if no public image publishing capability is available in this deployment. Silent multipart passthrough to AgnesAI is not acceptable.

## User Stories

1. As a Canvas user, I want to create an AgnesAI text-to-video task without reference images, so that I can generate video from a prompt.
2. As a Canvas user, I want to create an AgnesAI image-to-video task from one reference image, so that I can animate an uploaded image.
3. As a Canvas user, I want to create an AgnesAI multi-image video task from multiple reference images, so that I can guide video generation with several visual references.
4. As a Canvas user, I want reference images to be sent to AgnesAI in a supported format, so that video creation does not fail with an internal serialization error.
5. As a Canvas user, I want video task creation failures to return a clear message, so that I understand whether the problem is unsupported upload handling, invalid input, or upstream failure.
6. As a Canvas user, I want the backend to avoid charging an unexpectedly high pre-consume amount for AgnesAI video, so that my wallet display feels predictable.
7. As a Canvas user, I want failed task creation to continue refunding any pre-consumed amount, so that transient upstream errors do not cost balance.
8. As a Canvas user, I want a created AgnesAI video task to return the backend's public task ID, so that Canvas can poll the same task consistently.
9. As a Canvas user, I want Canvas polling to reflect AgnesAI task states, so that I can see whether the task is queued, processing, completed, or failed.
10. As a Canvas user, I want a completed AgnesAI video task to expose a playable video URL, so that I can preview or download the generated video.
11. As a Canvas user, I want the existing video content endpoint to work for AgnesAI completed tasks, so that Canvas does not need provider-specific content fetching logic.
12. As a Canvas user, I want AgnesAI video generation to work with the existing `/api/canvas/relay/videos` route, so that the frontend flow does not need a separate AgnesAI-only endpoint.
13. As a Canvas user, I want invalid AgnesAI reference image uploads to fail before calling upstream, so that I get faster feedback.
14. As a Canvas user, I want unsupported multipart file inputs to fail with a clear validation error if public URL publishing is unavailable, so that the issue is actionable.
15. As an API gateway operator, I want AgnesAI video relay behavior isolated from Sora relay behavior, so that provider-specific protocol differences do not break other providers.
16. As an API gateway operator, I want AgnesAI video requests to always send a JSON content type when the body is JSON, so that upstream sees a consistent request.
17. As an API gateway operator, I want upstream `video_id` values preserved for result retrieval, so that polling can use AgnesAI's recommended retrieval method.
18. As an API gateway operator, I want upstream `task_id` compatibility retained where needed, so that existing task storage and legacy retrieval remain robust.
19. As an API gateway operator, I want AgnesAI completed responses parsed for the documented final video URL field, so that successful tasks do not appear complete without usable media.
20. As an API gateway operator, I want the AgnesAI video model price configured explicitly, so that billing behavior is auditable and not dependent on fallback logic.
21. As an API gateway operator, I want the solution to preserve existing Sora/OpenAI video behavior, so that fixing AgnesAI does not regress other video providers.
22. As an API gateway operator, I want task billing ratios such as duration and resolution to remain consistent with the existing asynchronous video billing model, so that settlement remains coherent.
23. As an API gateway operator, I want provider-specific errors to be wrapped consistently with existing task errors, so that logs and client responses remain understandable.
24. As a backend developer, I want the AgnesAI video adapter to hide AgnesAI-specific JSON mapping, result retrieval, and URL extraction behind the existing task adapter interface, so that callers do not learn provider-specific details.
25. As a backend developer, I want tests to verify observable behavior through the task relay interface, so that future refactors do not break tests unnecessarily.
26. As a backend developer, I want multipart file handling to be tested at the provider boundary, so that a future change cannot accidentally reintroduce raw UploadFile passthrough.
27. As a backend developer, I want public image URL publishing to be represented as an external-system seam, so that tests can substitute a fake publisher without mocking internal adapter behavior.
28. As a backend developer, I want JSON field mapping for single-image and multi-image requests covered by tests, so that AgnesAI receives the documented request contract.
29. As a backend developer, I want result parsing covered by tests, so that AgnesAI's unusual final video URL field is not lost.
30. As a backend developer, I want pricing configuration covered by a focused test or existing pricing verification path, so that `agnes-video-v2.0` does not disappear from billing configuration unnoticed.

## Implementation Decisions

- AgnesAI video tasks should use a provider-specific task adapter rather than sharing the Sora/OpenAI video task adapter.
- The existing asynchronous task adapter interface is the primary seam for this feature. The caller should continue to submit and fetch tasks through the existing task relay flow.
- The routing from channel type to task adapter should map AgnesAI to the AgnesAI video adapter, while Sora and OpenAI continue using their current video adapter.
- AgnesAI video task creation should target the AgnesAI video creation endpoint with a JSON body.
- The outbound request header for AgnesAI video creation must set `Content-Type: application/json` whenever the adapter produces a JSON body.
- Text-to-video requests without reference images should remain supported and should not require image URL publishing.
- Single-reference image input should be represented as top-level `image` when one public image URL is available.
- Multi-reference image input should be represented as `extra_body.image` when multiple public image URLs are available.
- Keyframe-style multi-image input should be represented with `extra_body.image`, and mode-related fields should be preserved where compatible with AgnesAI's documented API.
- The implementation should treat AgnesAI video reference images as public URL inputs, not Data URI inputs. AgnesAI's image API supports Data URI Base64, but that capability must not be assumed for AgnesAI video.
- If Canvas sends multipart reference image files, the adapter must not forward file parts to AgnesAI.
- If the codebase already has a public image publishing capability, the adapter should use it to publish uploaded reference images before creating the AgnesAI video task.
- If the codebase does not have a public image publishing capability, multipart file image-to-video should fail with a clear validation error explaining that AgnesAI video requires publicly accessible image URLs.
- Existing requests that already provide public image URLs should be supported without requiring a file upload.
- AgnesAI task creation responses include both task identifiers and a `video_id`; the implementation should preserve enough upstream identity to support subsequent polling.
- Result retrieval should prefer AgnesAI's recommended `video_id` retrieval method when a video id is available.
- Legacy task-id retrieval should remain available as a fallback where needed for compatibility.
- Completed AgnesAI task responses should be parsed for the documented final video URL field and any compatible URL aliases observed in practice.
- The final video URL should be stored in the task's private result data so existing video content retrieval can serve the generated media.
- The OpenAI-compatible video response returned to Canvas should continue using the backend's public task ID rather than leaking the upstream task ID as the primary client identifier.
- AgnesAI video status mapping should translate upstream queued, in-progress, completed, and failed states into the existing task status model.
- `agnes-video-v2.0` should be added to the model pricing configuration so pre-consume is intentional and no longer falls through to an unexpected high amount.
- The pricing decision should reflect current product policy. AgnesAI's public documentation lists a standard per-second price and a current zero price; the implementation should choose the configured gateway policy explicitly rather than relying on fallback behavior.
- Existing provider behavior for Sora, OpenAI-compatible video, Gemini/Veo, and other task adapters should remain unchanged.
- The implementation must continue using the project's JSON wrapper functions for marshal and unmarshal operations.
- The implementation must preserve database compatibility across SQLite, MySQL, and PostgreSQL if any task storage changes are needed.
- No frontend-specific AgnesAI endpoint is required; the existing Canvas video relay route should continue to be the frontend entrypoint.

## Testing Decisions

- Tests should verify observable behavior through public or high-level interfaces, not private helper functions.
- The highest preferred seam is the existing asynchronous task adapter interface and task relay behavior.
- A new public image publishing seam may be introduced only if multipart file uploads must be converted to public URLs. This seam represents an external system boundary and may be faked in tests.
- Internal adapter collaborators should not be mocked. Tests should exercise real request validation, request body construction, response parsing, and status mapping where practical.
- The tracer-bullet test should prove that an AgnesAI video request with image references does not produce an outbound multipart request to AgnesAI and instead produces JSON with public image URLs.
- A test should verify that an AgnesAI video JSON request sets `Content-Type: application/json` on the outbound request.
- A test should verify single public image URL mapping to top-level `image`.
- A test should verify multiple public image URL mapping to `extra_body.image`.
- A test should verify multipart file reference behavior. If public URL publishing exists, the test should prove that uploaded files are published and the resulting URLs are sent in JSON. If publishing does not exist, the test should prove the request fails with a clear validation error before calling upstream.
- A test should verify AgnesAI task creation response handling, including backend public task ID exposure and preservation of upstream identifiers required for polling.
- A test should verify result parsing for completed AgnesAI video tasks, including extraction of the final playable video URL from the documented AgnesAI result field.
- A test should verify status mapping for queued, in-progress, completed, and failed AgnesAI task results.
- A focused pricing test or configuration assertion should verify that `agnes-video-v2.0` has explicit pricing configuration.
- Existing AgnesAI image tests are useful prior art for provider-specific JSON conversion, but video tests must not copy the Data URI assumption from image-to-image behavior.
- Existing Sora task adapter tests and task relay tests are useful prior art for asynchronous video task creation and OpenAI-compatible video responses.
- Tests should be added incrementally with TDD: one failing behavior test, minimal implementation to pass, then the next behavior test.
- Refactoring should only happen after the current test cycle is green.

## Out of Scope

- Rewriting the Canvas video UI is out of scope.
- Creating a new Canvas-only AgnesAI video endpoint is out of scope.
- Changing Sora/OpenAI video multipart behavior is out of scope except for separating AgnesAI from that adapter.
- Assuming Data URI Base64 support for AgnesAI video is out of scope unless AgnesAI later documents or verifies that support.
- Building a full media asset management system is out of scope. If public URL publishing is missing, this PRD only requires a clear validation failure or the smallest integration with an existing publishing capability.
- Changing user authentication, channel selection, or general task distribution behavior is out of scope.
- Changing database engines or database compatibility policy is out of scope.
- Reworking the entire billing system is out of scope; only explicit AgnesAI video model pricing and normal task billing integration are included.
- Removing or renaming protected project or organization identity is out of scope and prohibited by project policy.

## Further Notes

- The original failure signature includes an upstream error stating that an uploaded file object is not JSON serializable. The fix should make that class of error impossible for AgnesAI video by preventing raw multipart file passthrough.
- AgnesAI Video V2.0's official documentation should be treated as the source of truth for request shape: JSON request body, public image URLs, top-level `image`, and `extra_body.image`.
- AgnesAI Image 2.1 Flash documentation explicitly supports Data URI Base64 input for image workflows, but that is not sufficient evidence for video workflows.
- AgnesAI's video result documentation identifies `video_id` as the recommended retrieval identifier and shows the final playable video URL in a field whose name is not a generic `url`. The implementation should be defensive enough to preserve the documented behavior and practical compatible aliases.
- The task relay should continue returning stable public task IDs to Canvas while keeping upstream identifiers internal.
- Any implementation agent should start with the TDD tracer bullet and avoid writing all tests before implementation.
