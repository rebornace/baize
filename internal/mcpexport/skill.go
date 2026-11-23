package mcpexport

// ExportServerInstructions is bilingual guidance in MCP InitializeResult.instructions.
func ExportServerInstructions() string {
	return `白泽 MCP 导出服务 / Baize MCP export server（中英双语）

【中文】鉴权 / 托管登录：
- 工具失败且 structured code 为 hosted_login_required，或服务端返回 JSON-RPC -32042（需要 URL elicitation）时：必须把工具结果里的完整登录 URL（或 hosted_login_url）交给用户在浏览器打开；不要只说「去白泽设置」。
- 切勿要求用户在对话中粘贴密码、令牌或 MFA 验证码。
- 用户确认浏览器登录完成前，不要重试白泽导出工具。
- 完整规则见 prompt：baize_export_workflow。

【中文】审批：部分工具需要客户端 Allow/Deny；若用户拒绝，勿重试或改用其它工具绕过。

[English] Auth / hosted login:
- If a tool fails with structured code hosted_login_required, or the server returns JSON-RPC -32042 (URL elicitation required), you MUST give the user the full http(s) URL from the tool text or hosted_login_url to open in a browser; do not only say "update Baize settings".
- Never ask the user to paste passwords, tokens, or MFA codes into chat.
- Do not retry Baize export tools until the user confirms browser sign-in completed.
- Prefer the baize_export_workflow prompt for the full rules.

[English] Approval: Some tools require in-client Allow/Deny elicitation. If the user declines, do not retry or workaround with other tools.`
}

// ExportWorkflowSkillMarkdown returns bilingual guidance for agents using the Baize MCP export server.
func ExportWorkflowSkillMarkdown() string {
	return `# 白泽 MCP 导出工作流 / Baize MCP export workflow

你已连接到白泽 MCP 导出服务。请同时遵守下列中英文规则（国际客户端与中文用户均适用）。

You are connected to a Baize MCP export server. Follow the Chinese and English rules below for every session.

## 工具审批 / Tool approval (client elicitation)

部分白泽工具在运行前需要客户端审批。出现审批提示时，等待用户在 MCP 客户端选择允许或拒绝。

Some Baize tools require in-client approval before they run. When a tool returns an approval prompt, wait for the user to Allow or Deny in the MCP client.

- 若用户**拒绝** / If the user **declines**：不要重试同一工具，也不要用其它写入类工具绕过 / do not retry the same tool or switch to other write tools as a workaround.
- 不要用另一工具达到相同效果来绕过审批 / Do not bypass approval by calling a different tool that achieves the same outcome.

## 鉴权过期 / 托管登录 · Auth expired / hosted login

以下情况视为硬停止（不是谜题）/ Treat these as hard stops (not puzzles):

1. **MCP 错误 -32042**（需要 URL elicitation）：在客户端完成 URL elicitation（打开浏览器链接）。不要编造凭证。 / complete the URL elicitation in the client (open the browser URL). Do not invent credentials.
2. **工具结果**含 structured 字段 ` + "`code: hosted_login_required`" + ` 与 ` + "`hosted_login_url`" + `：请用户在浏览器打开该 URL 登录。 / ask the user to open that URL in a browser and sign in there.
3. 文案提到 ` + "`/v0/auth/hosted/`" + ` 或导出身份会话已过期：同样只走浏览器登录。 / same rule — browser sign-in only.

**切勿**要求用户在对话中粘贴密码、令牌或 MFA。用户确认登录完成前**不要重试**白泽工具。

**Never** ask users to paste passwords, tokens, or MFA codes into chat. **Do not retry** Baize tools until they confirm sign-in finished.

若没有托管 URL：请用户在白泽设置中更新导出身份（或打开随后提供的托管链接），确认后再继续。

If there is no hosted URL, ask them to update the export identity in Baize settings (or open the hosted login link when provided), then continue only after they confirm.

## 错误不是谜题 / Errors are not puzzles

将鉴权失败与审批拒绝视为白泽给出的最终说明，不要当作可用其它工具、请求头或客户端绕过的谜题，除非白泽明确另有指示。

Treat auth failures and approval denials as final instructional messages from Baize. Do not treat them as puzzles to workaround with alternate tools, headers, or clients unless Baize explicitly instructs otherwise.
`
}
