# Quality Guidelines

> Code quality standards for backend development.

---

## Overview

<!--
Document your project's quality standards here.

Questions to answer:
- What patterns are forbidden?
- What linting rules do you enforce?
- What are your testing requirements?
- What code review standards apply?
-->

(To be filled by the team)

---

## Forbidden Patterns

<!-- Patterns that should never be used and why -->

(To be filled by the team)

---

## Required Patterns

### Prompt Audit & Gate Conventions

1. **Deterministic Protocol Field Dispatch**:
   - Never recursively scan arbitrary JSON maps.
   - Dispatch explicitly by concrete relay DTOs (`*dto.GeneralOpenAIRequest`, `*dto.ClaudeRequest`, `*dto.GeminiChatRequest`, etc.).
   - Explicitly extract all text-bearing fields: system, developer, user, assistant, tool, reasoning/thinking, FIM prefix/suffix, edits instructions, and code execution outputs.

2. **Media vs Text URL Discrimination**:
   - **Do NOT** use naive prefix checks like `strings.HasPrefix(s, "http://")` to skip media. Prompts such as `https://example.com summarize this` or Midjourney's `https://s.mj.run/abc a futuristic city` MUST be scanned.
   - Only standalone, whitespace-free URLs (`!strings.ContainsAny(s, " \t\r\n")`), data URLs, and base64 strings should be excluded.

3. **Multi-stage & Handshake Protocol Bypass**:
   - Protocols that use a separate frame-level gate (e.g. OpenAI Realtime WebSocket) enter HTTP controllers with placeholder requests (`*dto.BaseRequest`).
   - The unified HTTP relay gate must explicitly bypass these protocols, allowing subsequent WebSocket frame gates to perform per-event auditing, rather than failing closed at handshake with `unsupported_protocol`.

4. **Fail-Closed & Zero Upstream Guarantee**:
   - Any Block or infrastructure error (`prompt_guard_unavailable`, `prompt_guard_invalid_response`, `prompt_audit_config_degraded`, `prompt_audit_record_failed`, `prompt_audit_unsupported_protocol`) must set `types.ErrOptionWithSkipRetry()`.
   - Ensure upstream call count is 0 and quota pre-consumption count is 0 before returning.

5. **Request Body Reusability & Zero-Value Preservation**:
   - Any controller or gate inspecting the request body must use `common.UnmarshalBodyReusable`.
   - Downstream request processing must preserve explicit zero values (e.g. `temperature: 0.0`, `stream: false`).

---

## Testing Requirements

- **Assert Invariants**: Assert upstream call count = 0 and pre-consume quota count = 0 on every blocked or failed-closed request. Do not just assert HTTP status code.
- **Zero Leakage**: Assert no FullPrompt, ScanText, Guard response, or Token appears in logs or error bodies.
- **RelayKit Build Boundary**: Verify `cd relaykit && GOWORK=off go build ./...` passes whenever relay-related packages are touched.

