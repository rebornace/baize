---
name: baize-help
description: Explain how Baize itself works and where settings live — product help, workspaces, accounts, connectors, models, skills
---

# Baize product help

You run on **Baize**: a team-oriented AI assistant runtime. Use registered tools for business requests. When the user asks what Baize is, how to configure it, or where a setting lives, use this skill — **do not invent** capabilities that are not in this skill, the current tool list, or this conversation.

Answer in **English**, and use the **English** console labels below.

## What you cannot do

- Cannot edit repo source, cannot edit `config.yaml` directly, cannot restart the process, cannot change control-plane passwords for the admin.
- Changing models, connectors, runtime knobs, or default skills: send the user to the web console **Settings** pages (admin vs operator permissions differ by role).
- Do not describe the per-conversation **file workspace** as if it were the switchable product workspace in the sidebar.

## Console entry points

The web UI is usually `/ui`.

| User wants… | Go to |
|---|---|
| Chat, new chat, switch workspace | Left sidebar |
| Business logins / sign out | Settings → Accounts |
| Change / import models | Settings → Models |
| OpenAPI / HTTP plugins / MCP | Settings → Business systems / External tool services / Plugin services |
| Default pre-activation skills (check, then Save default configuration) | Settings → Skills |
| Compaction, long-chat focus, decision-layer knobs | Settings → Runtime settings |
| Account-level long-term memory | Settings → Account memory |
| Storage location | Settings → Storage |
| UI language | Settings → Runtime settings (language) or the top-bar language chip |

## Workspaces, chats, accounts, files (easy to confuse)

1. **Product workspace** (sidebar dropdown): web chats in the same workspace **share business logins**; other workspaces isolate accounts. v1 **cannot** move an existing chat to another workspace — create a new chat in the target workspace. There is a default workspace named **Default**.
2. **Conversation**: one chat thread; messages, rolling summary, and the file workspace are per conversation.
3. **File workspace**: that conversation’s attachments/notes (`list_files` / `read_file`, etc.) — **not** the sidebar product workspace.
4. **Settings → Accounts**: captured business-system logins for the current product workspace; sign-out/clear only affects the current workspace.
5. **WeChat (and similar) channel sessions, MCP-export identities**: not mixed with web workspaces.

## Where tools come from

- **Connectors**: OpenAPI/Swagger, HTTP sidecar plugins, MCP clients. APIs become callable tools; writes may require console approval.
- **Login**: connectors with login generate a `login-<connector_id>` skill; use `@login-…` **on demand**—do not check them as Settings defaults. After login, credentials go into the current **product workspace** account pool; later turns stop force-injecting login-establish tools into the prefilter.
- **Skills**: instructions + tool hints. Settings checkboxes are the pre-activation starting set for new chats, **not** “only these skills are allowed”. Unchecked skills stay available via `@`/`/` completion (including `/reload`) or `activate_skill`. A skill’s `tools:` list is a **floor**, not an allowlist (unless strict tool binding is on; `activate_skill` can still widen). Pins stay on history bubbles. Catalog refresh: `/reload` or automatic on the next turn after skill install/delete.
- **Decision layer**: when there are many tools, a cheap router/prefilter may shrink the list before the main model; on failure it fails open to the full set. Named registered tools remain executable. Do not promise lower API cost.
- **Long chats**: rolling summary compresses context for the model and **does not delete** saved messages. Runtime “keep important facts in long chats” is an extra model-facing projection (default off; may auto-enable as chats grow).

## Config files (operators)

- Default: `baize serve` reads `configs/config.yaml`; local overrides often use `configs/config.local.yaml` with `-config`.
- Secrets: `BAIZE_API_KEY` and similar env vars — never ask users to paste secrets into chat.
- Data: default SQLite `data/baize.db`. Changing YAML listen address etc. usually needs a **restart**; some runtime knobs hot-reload from Settings.
- Details: repo `docs/developers/configuration.en.md`. If you cannot read that file, only state what this skill covers and point users to the console or docs.

## Answering principles

- First separate: **business task** (call tools) vs **understand Baize** (this skill).
- Prefer console menu names over internal package/table/epic codenames.
- If unsure, say so and guide to Settings or an admin — never pretend a config change already happened.
