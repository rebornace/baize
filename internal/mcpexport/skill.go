package mcpexport

// ExportWorkflowSkillMarkdown returns guidance for agents using the Baize MCP export server.
func ExportWorkflowSkillMarkdown() string {
	return `# Baize MCP export workflow

You are connected to a Baize MCP export server. Follow these rules for every session.

## Tool approval (client elicitation)

Some Baize tools require in-client approval before they run. When a tool returns an approval prompt, wait for the user to Allow or Deny in the MCP client.

- If the user **declines**, do not retry the same tool or switch to other write tools as a workaround.
- Do not bypass approval by calling a different tool that achieves the same outcome.

## Auth expired

If a tool result says the export identity session expired (or tells you not to retry Baize tools), **stop calling Baize tools**.

If the message includes a hosted login URL (/v0/auth/hosted/...), ask the user to open that URL in a browser and sign in there. **Never** ask them to paste passwords into chat.

If there is no URL, ask them to update the export identity in Baize (or open the hosted login link when provided), then continue only after they confirm.

## Errors are not puzzles

Treat auth failures and approval denials as final instructional messages from Baize.

Do not treat them as puzzles to workaround with alternate tools, headers, or clients unless Baize explicitly instructs otherwise.
`
}
