---
name: data-analytics
description: 多源数据统计与完整交互分析页（筛选、下钻、导出 PDF）
tools:
  - create_analysis_page
  - list_tickets
  - get_ticket
---

# 数据分析

1. 从企业 Connector 工具拉 JSON，在模型侧重聚合为 `datasets`（勿把原始大 JSON 塞进单个 section）
2. 调用 `create_analysis_page`：优先 `format: sections` + `binding` 省 token；复杂图用 `echarts.option`
3. 需要版式完全自由时用 `format: html`
4. 若目录中已注册 `list_tickets` / `get_ticket`，先用其取数再出页
5. 需要静态 PNG 时由管理员配置 AntV MCP（见 README）
