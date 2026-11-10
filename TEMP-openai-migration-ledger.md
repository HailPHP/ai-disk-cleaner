# OpenAI API Migration Decision Ledger

Application: HailPHP/ai-disk-cleaner, master 9cc2f2407e9769183294ffaf871f02150cbb6378 (project version 26.5.1).
Integration: Go 1.25, Wails 2.12, openai-go/v3 3.50.0. Retain the SDK and Chat Completions; add native Responses.
User authorized implementation of the supplied specification, repository changes and a Windows executable, in one pass.
Credential decision: user explicitly approved reusing the existing environment key for a small live check; most coverage uses httptest. No credentials enter source, CI or the executable.
Privacy: local item history, store=false, no previous_response_id or conversation. No claim that store=false establishes organization-level Zero Data Retention or residency.

## Approved feature stages

1. Dual API analysis and connection testing: typed Responses calls, original tools, local complete output replay, call_id results, compaction, budgets, cancellation. Implemented; frontend checks and the full Go suite passed in Windows CI run 42747492272.
2. Settings and diagnostics: API mode, 120-second configurable timeout, localized guidance, usage and safe errors, legacy settings migration. Implemented; frontend checks and the full Go suite passed in Windows CI run 42747492272.
3. Windows delivery: full tests and frontend lint/check/build passed in Windows CI run 42747492272; Windows amd64 executable build and the 8-second process-startup smoke check also passed in run 42747492272. Final clean-source CI run 42748360161 passed all jobs and steps on cfd48bda543e1e33cf24e5bd013371b77d184e51.

## Capability assessment

| Guidance heading | Applicable / support | Decision and evidence |
| --- | --- | --- |
| Model Selection | Yes / configurable | Preserve existing llm.model. User explicitly requests GPT-6 Astra support; live model availability must be checked. Other provider models remain configurable. |
| Instruction File Naming | Yes / yes | Preserve all AGENTS.md files and follow service and form conventions. |
| Migrate from Chat Completions to Responses | Yes / SDK yes | Both Analyze and TestConnection gain the configured route. Default remains chat_completions. |
| Streaming | No for this stage / SDK yes | User explicitly requests non-streaming first version. Keep stream=false. |
| WebSocket Mode | No / not evaluated | User requests existing HTTP request loop; no persistent transport redesign. |
| Zero Data Retention | Stateless requirement yes / API yes | Enforce store=false; no server-side conversation chain. Organization retention is outside this application setting. |
| Data Residency | No new requirement / provider-specific | Preserve user-selected base URL; do not change account or regional routing. |
| Processing Tiers: Standard, Priority, Flex, Batch, Reserved | No tier change / provider-specific | Interactive existing workload; preserve provider default, no higher-cost tier or asynchronous batch introduced. |
| Prompt Cache Keys | No new requirement / SDK yes | Preserve optional Extra Body tuning. Do not invent cache keys for Astra. |
| Extended Cache Retention for Earlier Models | No / model-specific | Retained providers and retention unchanged; outside requested communication-layer migration. |
| Explicit Cache Breakpoints | Not selected / SDK partial | User specifies raw local history replay and existing compression; no prompt/caching redesign in this change. |
| Prompt Cache Diagnostics | Not selected / SDK yes | Not needed for required input/output/total usage acceptance; no new per-provider diagnostics. |
| State Management | Yes / yes | Preserve every raw output item in original order; feed function_call_output using call_id. |
| Instruction Persistence | Yes / yes | Resend the original system instructions on every request, including after local compression. |
| Encrypted Reasoning Items | Yes / yes | Explicit include=reasoning.encrypted_content plus raw replay. Do not log encrypted items. |
| Reasoning Effort | Yes / configurable | User selected low initially, with medium/high/xhigh configurable through reasoning.effort. Astra gets low default; other models retain defaults. |
| Reasoning Preservation | Yes / yes | Replay complete reasoning items until explicit existing compress_context resets history. No new server-managed context mode. |
| Reasoning Summaries and Tool-Call Preambles | No UI change / SDK yes | Preserve existing visible assistant text behavior; do not display private reasoning or introduce reasoning summaries. |
| Verbosity | Yes / configurable | User selected medium; Astra default and Extra Body text.verbosity. |
| Maximum Output Tokens | Yes / yes | Preserve analysis default 8000, allow max_output_tokens override. Connection probe allows reasoning with 1024. |
| Assistant Message Phases | Yes / raw replay | Preserve original phase fields along with unknown future output fields. |
| Context Compaction | Yes / local implementation | User explicitly requires existing compress_context, rather than native server compaction. Reset local Responses input only after successful compression; keep cumulative budget. |
| Structured Outputs | Tool schemas yes / yes | Reuse current strict JSON Schemas, no forced schema on human-readable final report. |
| Tool Choice | Yes / yes | Default auto, retain Extra Body tuning; connection probe has no tools. |
| Tool Search and Additional Tools | No / SDK yes | Four fixed existing tools; dynamic discovery adds no benefit in this scope. |
| Parallel Tool Calls | Yes / yes | Enforce false. Defensively execute returned calls sequentially; reject mixed compression without dropping call/result pairs. |
| Programmatic Tool Calling | No / outside scope | Existing local Go tools only; no code execution capability added. |
| Model-Aligned Tool Interfaces | No redesign / existing | Preserve disk tools, file scanning, cleanup confirmation and deletion code. |

## Evidence and coverage

- SDK verified against openai/openai-go tag v3.50.0, responses/response.go (blob 665a99d8fa71662d9a9c6542dc9c8ba6d1080637).
- The actual SDK helper takes `(callID, output)`; the illustrative single-argument helper in the supplied proposal does not match this version.
- Mock coverage planned: endpoint/body, old defaults and settings persistence, both connection probes, strict tool schema parity, multi-turn dispatch, call IDs, encrypted reasoning/phase replay, successful/failed/mixed compaction, token usage, budget stop, cancellation, incomplete/refusal/empty responses, duplicate IDs and safe HTTP errors.
- Local environment cannot download Go/npm dependencies: proxy returns HTTP 403 for github.com, proxy.golang.org and registry.npmjs.org. Source retrieved using the authorized GitHub connector; Windows CI will execute builds and tests.
- Validation and delivery results will be recorded after execution, not inferred from code review.

## Live protocol check (2026-11-10)

The existing environment credential successfully accessed gpt-6-astra. A two-request synthetic Responses check forced analyze_directory, replayed all output items, supplied the matching call_id result, and received a completed text response. Usage: 193 + 222 = 415 tokens. No disk data was sent. This trivial prompt produced no reasoning item, so encrypted reasoning replay is covered by the mock test rather than claimed as live-verified. This Python protocol check does not substitute for compiling/testing the Go app.

PR #1 has been created for CI validation. No GitHub Actions run was produced initially. The user has now confirmed enabling workflows in the fork; Windows CI run 42746719907 started successfully. The first run identifies formatting changes for synchronization; full tests/build remain pending. A Windows process-startup smoke check has also been added to validate that the generated executable stays running without starting a scan.

Local static checks: 78 Go/TypeScript/TSX source files parsed without syntax errors using Tree-sitter; translation JSON and workflow YAML parsed successfully; git diff --check passed. These checks do not establish type checking, test success or executable build success.

## Windows validation progress

Run 42747492272 passed pnpm lint, pnpm check, pnpm build and go test ./... after bounding the cancellation test server wait. Windows executable compilation and the 8-second process-startup check passed. This run failed only the clean-source/format checks because formatters adjusted the authored source. Those exact formatter changes have now been synchronized. The final clean-source build places the executable and README together at the root of the download archive.

## Final outcome

All approved feature stages are implemented. GitHub Actions run [42748360161](https://github.com/HailPHP/ai-disk-cleaner/actions/runs/42748360161) completed successfully on source commit `cfd48bda543e1e33cf24e5bd013371b77d184e51`:

- Go formatting check and clean-source check passed.
- `pnpm lint`, `pnpm check`, `pnpm build` passed.
- `go test ./...` passed, including analyzer protocol, cancellation, compaction and settings migration coverage.
- Wails Windows amd64 build passed; the executable remained running through the 8-second startup smoke check without initiating disk scanning or cleanup.
- The original credential-dependent full fixture analysis remains skipped in CI without its LLM environment variables. The separate live Astra protocol check is documented above; it does not establish full desktop end-to-end API coverage.
- Artifact [13732323299](https://github.com/HailPHP/ai-disk-cleaner/actions/runs/42748360161/artifacts/13732323299) contains the executable, SHA-256 file, startup result and configuration README at archive root.
- Downloaded artifact independently verified: Windows PE AMD64, GUI subsystem, executable size 33,903,104 bytes.
- Executable SHA-256: `089162147f25e7a8ccdc52754573ffb482207473665acb62f6b928bb08088660`.
- Archive SHA-256: `143e98edd7a91812dfb9e90e67c360754aa803e121e50436c141d5e6d1d73a96`.

This final ledger update changes documentation only; executable source and build configuration remain exactly as tested above. No additional product changes or feature stages are pending. Manual interactive Windows UI testing was not performed; startup liveness, compilation, automated tests and the bounded live protocol check are the verified coverage.
