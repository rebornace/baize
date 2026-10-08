---
name: data-analytics
description: Multi-source analytics with interactive pages (filters, drill-down, PDF export)
tools:
  - create_analysis_page
  - list_tickets
  - get_ticket
---

# Data analytics

1. Pull JSON from enterprise connector tools; aggregate into `datasets` on the model side (do not stuff raw large JSON into a single section)
2. Call `create_analysis_page`: prefer `format: sections` + `binding` to save tokens; use `echarts.option` for complex charts
3. Use `format: html` when you need fully free layout
4. If `list_tickets` / `get_ticket` are in the catalog, fetch data with them before building the page
5. For static PNGs, an admin can configure AntV MCP (see README)

## Page look and feel (important)

The system already ships a modern dashboard style and ECharts theme; you should still:

- Set `title` (page title) and each section `title`
- Use `row` to place KPIs and charts side by side (e.g. 4 KPIs on one row + two charts below)
- For `echarts.option`, fill in `legend`, `tooltip`, `grid.containLabel: true`; bar charts use `itemStyle.borderRadius: [6,6,0,0]`
- Radar/line series may use `areaStyle: { opacity: 0.15 }`; use distinct `color`s or the default palette for multiple series
- Pass `theme: dark` for dark scenes; otherwise the light theme is default
- Avoid dumping large HTML in markdown; charts always go through an `echarts` section

Example KPI row + chart:

```json
{
  "format": "sections",
  "title": "Pet health dashboard",
  "sections": [
    { "type": "kpi", "items": [{ "label": "Pets", "binding": { "dataset": "d", "aggregate": "count" } }] },
    {
      "type": "echarts",
      "title": "Weight distribution",
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
