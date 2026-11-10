# OpenAI API Migration Decision Ledger

Application: HailPHP/ai-disk-cleaner, master 9cc2f2407e9769183294ffaf871f02150cbb6378 (project version 26.5.1).
Integration: Go 1.25, Wails 2.12, openai-go/v3 3.50.0. Retain the SDK and Chat Completions; add native Responses.
User authorized implementation of the supplied specification, repository changes and a Windows executable, in one pass.
Credential decision: user explicitly approved reusing the existing environment key for a small live check; most coverage uses httptest. No credentials enter source, CI or the executable.
Privacy: local item history, store=false, no previous_response_id or conversation. No claim that store=false establishes organization-level Zero Data Retention or residency.

## Approved feature stages

1. Dual API analysis and connection testing: typed Responses calls, original tools, local complete output replay, call_id results, compaction, budgets, cancellation. Implementation underway; validation pending.
2. Settings and diagnostics: API mode, 120-second configurable timeout, localized guidance, usage and safe errors, legacy settings migration. Implementation underway; validation pending.
3. Windows delivery: repository CI, full tests, frontend lint/check/build, Windows amd64 executable and SHA-256. Pending.

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
