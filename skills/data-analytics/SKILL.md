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

## 页面观感（重要）

系统已内置现代仪表盘样式与 ECharts 主题；你仍应：

- 设置 `title`（页面标题）与各 section 的 `title`
- 用 `row` 把 KPI 与图表并排（如 4 个 KPI 一行 + 下方双列图表）
- `echarts.option` 时补全 `legend`、`tooltip`、`grid.containLabel: true`；柱状图用 `itemStyle.borderRadius: [6,6,0,0]`
- 雷达/折线可加 `areaStyle: { opacity: 0.15 }`；多系列用不同 `color` 或交给默认色板
- 暗色场景传 `theme: dark`；否则默认浅色主题
- 避免在 markdown 里堆大段 HTML；图表一律走 `echarts` section

示例 KPI 行 + 图表：

```json
{
  "format": "sections",
  "title": "宠物健康看板",
  "sections": [
    { "type": "kpi", "items": [{ "label": "宠物数", "binding": { "dataset": "d", "aggregate": "count" } }] },
    {
      "type": "echarts",
      "title": "体重分布",
      "option": {
        "tooltip": { "trigger": "axis" },
        "xAxis": { "type": "category", "data": ["A", "B"] },
        "yAxis": { "type": "value" },
        "series": [{ "type": "bar", "data": [6.5, 3.1], "itemStyle": { "borderRadius": [6, 6, 0, 0] } }]
      }
    }
  ]
}
```
