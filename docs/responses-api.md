# 双 API 模式 / Dual API modes

此版本保留 Chat Completions，并新增原生 Responses API。扫描、候选风险等级、用户确认删除和迁移流程保持原样。

## Windows 使用

下载构建产物中的 `ai-disk-cleaner-windows-amd64.exe`，放入有写权限的文件夹后运行。适用于 64 位 Windows 10/11；Wails 界面需要 Microsoft Edge WebView2 Runtime。软件无需安装 Go、Node.js 或 Python。exe 不包含 API 密钥。

从本仓库 Actions 的 Build 运行下载 `ai-disk-cleaner-windows-amd64` 工件，解压后取得 exe 和 SHA-256 校验文件。构建步骤先运行前端检查和 Go 测试。发布流程仍保留原有 Release 工作流。

## GPT-6 Astra 示例

在设置页面配置并保存：

| 设置 | 值 |
| --- | --- |
| LLM API 模式 | Responses API |
| LLM Secret | 自己的 API 密钥 |
| LLM URL | `https://api.openai.com/v1` |
| LLM Model | `gpt-6-astra` |
| 请求超时 | 默认 120 秒，需要时改为 300 秒 |

URL 仅填写 Base URL，不要附加 `/responses` 或 `/chat/completions`。

Extra Body 示例：

```json
{
  "reasoning": { "effort": "low" },
  "text": { "verbosity": "medium" },
  "tool_choice": "auto",
  "parallel_tool_calls": false
}
```

Astra 默认使用 low / medium，可以按模型支持情况设置 medium、high 或 xhigh。其他 Responses 模型保留服务端默认值，除非在 Extra Body 中指定。Responses 不接受旧的顶层 `reasoning_effort`、`verbosity`、`max_tokens` 或 `max_completion_tokens`；输出上限使用 `max_output_tokens`，分析默认 8000（包括推理 Token）。

切换 API 模式不会自动修改模型或 Extra Body，请同步检查兼容性。旧数据库自动补齐 `llm.api-mode=chat_completions` 和 `llm.request-timeout=120`，已有设置不会被覆盖。

## 协议与上下文

- 两种模式共用现有扫描树和工具实现；不会由模型直接执行文件删除。
- Responses 固定使用 `store:false`、`stream:false`、`parallel_tool_calls:false`。Extra Body 无法替换历史、系统指令、工具集合或开启服务端会话链。
- 本地完整保存每次返回的 Output Items，包括 reasoning 的加密内容与消息 phase；使用 `call_id` 匹配工具结果。
- 成功调用 `compress_context` 后仅保留扫描交接摘要，继续发送原始系统指令，累计 Token 不清零。压缩失败会保留完整调用/结果对。压缩工具必须单独调用。
- 输入、输出、总 Token 分别写入已有任务日志；失败请求或缺失的 usage 不记为已知的零用量。
- API 错误仅显示安全摘要，不将服务端原始错误正文或密钥写入界面和日志。正常工具参数仍按现有行为写入任务日志。
- 连接测试只验证选定接口的文本请求，不代表工具循环已通过；自动测试使用本地模拟服务验证工具循环。

`store:false` 控制 Responses 存储，不等于账户已经启用 Zero Data Retention。

## 开发与验证

```sh
cd frontend
pnpm install --frozen-lockfile
pnpm lint
pnpm check
pnpm build
cd ..
go test ./...
go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0
wails build -platform windows/amd64 -trimpath -o ai-disk-cleaner-windows-amd64.exe
```

Go 版本与依赖使用仓库的 go.mod，前端使用 Node.js 24、pnpm 11。Windows 构建在 Windows runner 上完成。原有需要真实磁盘样例和 API 环境变量的端到端测试在未配置这些变量时会跳过；新增协议测试使用 httptest，不消耗 API 额度。

## English

Select **Responses API** in Settings for native Responses models. Existing installations default to **Chat Completions**. Use a base URL such as `https://api.openai.com/v1`, without an endpoint suffix. Model names remain configurable. Astra defaults to low reasoning effort and medium verbosity; customize `reasoning.effort`, `text.verbosity`, and `max_output_tokens` through Extra Body.

All existing disk tools and cleanup confirmation remain unchanged. Responses uses local raw item replay, encrypted reasoning inclusion, `call_id` tool outputs, no server-side response storage, and sequential tool execution. Successful context compression discards prior items and retains the scan summary and system instructions. The timeout defaults to 120 seconds per attempt and is configurable; SDK retries can increase overall wait time.

The Windows amd64 executable is available in the Build workflow artifact, with a SHA-256 checksum. It requires Windows 10/11 and Microsoft Edge WebView2 Runtime; no development runtime or API credential is bundled.
