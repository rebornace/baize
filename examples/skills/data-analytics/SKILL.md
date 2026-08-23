---
name: data-analytics
description: 多源数据统计与完整交互分析页（筛选、下钻、导出 PDF）
tools:
  - list_tickets
  - get_ticket
  - create_analysis_page
---

# 数据分析

1. 从企业工具拉 JSON，在模型侧重聚合为 `datasets`（勿把原始大 JSON 塞进单个 section）
2. 优先 `format: sections` + `binding` 省 token；复杂图用 `echarts.option`
3. 需要版式完全自由时用 `format: html`
4. 需要静态 PNG 时由管理员配置 AntV MCP（见 README）
