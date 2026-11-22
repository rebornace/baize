---
name: project-create
description: 显式项目创建与归档示例（mock-project）；需要调用方提供 project_title
domain: project
tools:
  - list_projects
  - create_project
  - update_project_status
---

# 项目创建 / 归档示例

- 只在调用方明确要新建项目时激活；属于示例技能，不在默认 agent skills。
- 必需 `input_args.project_title`；可选 `input_args.target_status`（`active` / `archived`）。
- workflow 先 `list_projects`，再 `create_project`；仅当 `target_status=archived` 时执行 `update_project_status`。
- 创建后需要改状态的通用链路请用独立技能或直接调用工具，不要把本示例当业务默认。
