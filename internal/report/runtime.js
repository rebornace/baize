(function (global) {
  "use strict";

  var CHART_COLORS = [
    "#3B66F5",
    "#22C55E",
    "#F59E0B",
    "#EF4444",
    "#8B5CF6",
    "#06B6D4",
    "#EC4899",
    "#64748B",
  ];
  var themesRegistered = false;

  function isDarkTheme() {
    return document.body && document.body.getAttribute("data-theme") === "dark";
  }

  function registerChartThemes() {
    if (themesRegistered || typeof echarts === "undefined") return;
    themesRegistered = true;
    var font =
      "system-ui, -apple-system, 'PingFang SC', 'Microsoft YaHei', sans-serif";
    echarts.registerTheme("baize-light", {
      color: CHART_COLORS,
      backgroundColor: "transparent",
      textStyle: { fontFamily: font, color: "#64748b" },
      title: { textStyle: { color: "#0f172a", fontWeight: 600 } },
      legend: { textStyle: { color: "#64748b" } },
      tooltip: {
        backgroundColor: "rgba(255,255,255,0.98)",
        borderColor: "#e2e8f0",
        borderWidth: 1,
        textStyle: { color: "#0f172a" },
        extraCssText:
          "box-shadow: 0 8px 24px rgba(15,23,42,0.12); border-radius: 8px;",
      },
      categoryAxis: {
        axisLine: { lineStyle: { color: "#e2e8f0" } },
        axisLabel: { color: "#64748b" },
        axisTick: { show: false },
      },
      valueAxis: {
        axisLine: { show: false },
        axisLabel: { color: "#64748b" },
        splitLine: { lineStyle: { color: "#f1f5f9", type: "dashed" } },
      },
      bar: {
        itemStyle: { borderRadius: [6, 6, 0, 0] },
        barMaxWidth: 48,
      },
      line: { smooth: true, symbolSize: 7, lineStyle: { width: 3 } },
      radar: {
        axisName: { color: "#64748b" },
        splitLine: { lineStyle: { color: "#e2e8f0" } },
        splitArea: {
          areaStyle: {
            color: ["rgba(59,102,245,0.02)", "rgba(59,102,245,0.06)"],
          },
        },
      },
    });
    echarts.registerTheme("baize-dark", {
      color: CHART_COLORS,
      backgroundColor: "transparent",
      textStyle: { fontFamily: font, color: "#94a3b8" },
      title: { textStyle: { color: "#f1f5f9", fontWeight: 600 } },
      legend: { textStyle: { color: "#94a3b8" } },
      tooltip: {
        backgroundColor: "rgba(15,23,42,0.95)",
        borderColor: "#334155",
        borderWidth: 1,
        textStyle: { color: "#f1f5f9" },
        extraCssText:
          "box-shadow: 0 12px 32px rgba(0,0,0,0.35); border-radius: 8px;",
      },
      categoryAxis: {
        axisLine: { lineStyle: { color: "#334155" } },
        axisLabel: { color: "#94a3b8" },
        axisTick: { show: false },
      },
      valueAxis: {
        axisLine: { show: false },
        axisLabel: { color: "#94a3b8" },
        splitLine: { lineStyle: { color: "#1e293b", type: "dashed" } },
      },
      bar: {
        itemStyle: { borderRadius: [6, 6, 0, 0] },
        barMaxWidth: 48,
      },
      line: { smooth: true, symbolSize: 7, lineStyle: { width: 3 } },
      radar: {
        axisName: { color: "#94a3b8" },
        splitLine: { lineStyle: { color: "#334155" } },
        splitArea: {
          areaStyle: {
            color: ["rgba(96,165,250,0.04)", "rgba(96,165,250,0.1)"],
          },
        },
      },
    });
  }

  function polishChartOption(option) {
    if (!option || typeof option !== "object") return option;
    if (!option.color) option.color = CHART_COLORS;
    if (!option.grid && (option.xAxis || option.yAxis || option.radar)) {
      option.grid = {
        left: 12,
        right: 12,
        top: option.legend || option.title ? 56 : 32,
        bottom: 12,
        containLabel: true,
      };
    }
    if (!option.tooltip) {
      option.tooltip = option.radar ? { trigger: "item" } : { trigger: "axis" };
    }
    if (option.series && Array.isArray(option.series)) {
      for (var si = 0; si < option.series.length; si++) {
        var s = option.series[si];
        if (s.type === "bar") {
          if (!s.barMaxWidth) s.barMaxWidth = 48;
          s.itemStyle = s.itemStyle || {};
          if (!s.itemStyle.borderRadius) s.itemStyle.borderRadius = [6, 6, 0, 0];
        }
        if (s.type === "line" && s.smooth == null) s.smooth = true;
        if (s.type === "radar" && !s.areaStyle) {
          s.areaStyle = { opacity: 0.12 };
        }
      }
    }
    return option;
  }

  function columnIndex(columns, field) {
    if (!columns || field == null || field === "") return -1;
    return columns.indexOf(field);
  }

  function filterRowsByWhere(rows, where, columns) {
    if (!where || Object.keys(where).length === 0) return rows;
    return rows.filter(function (row) {
      return Object.keys(where).every(function (field) {
        var idx = columnIndex(columns, field);
        if (idx < 0) return false;
        return String(row[idx]) === String(where[field]);
      });
    });
  }

  function applyFilters(rows, filterState, filters, columns) {
    if (!rows || rows.length === 0) return [];
    var result = rows.slice();
    for (var i = 0; i < (filters || []).length; i++) {
      var filter = filters[i];
      var state = filterState ? filterState[filter.id] : undefined;
      if (state == null || state === "") continue;

      var idx = columnIndex(columns, filter.field);
      if (idx < 0) continue;

      if (filter.type === "select") {
        result = result.filter(function (row) {
          return String(row[idx]) === String(state);
        });
        continue;
      }

      if (filter.type === "date_range") {
        var from = state.from != null ? state.from : "";
        var to = state.to != null ? state.to : "";
        if (!from && !to) continue;
        result = result.filter(function (row) {
          var value = row[idx];
          if (value == null || value === "") return false;
          var text = String(value);
          if (from && text < from) return false;
          if (to && text > to) return false;
          return true;
        });
      }
    }
    return result;
  }

  function aggregateCount(rows, binding, columns) {
    var filtered = filterRowsByWhere(rows, binding && binding.where, columns);
    var aggregate =
      (binding && binding.aggregate) ||
      (binding && binding.value && binding.value.aggregate) ||
      "count";
    var field = (binding && binding.field) || (binding && binding.value && binding.value.field);

    if (aggregate === "count") {
      return filtered.length;
    }

    var colIdx = columnIndex(columns, field);
    if (colIdx < 0) return 0;

    var numbers = filtered
      .map(function (row) {
        return Number(row[colIdx]);
      })
      .filter(function (n) {
        return !Number.isNaN(n);
      });

    if (aggregate === "sum") {
      return numbers.reduce(function (sum, n) {
        return sum + n;
      }, 0);
    }

    if (aggregate === "avg") {
      if (numbers.length === 0) return 0;
      return (
        numbers.reduce(function (sum, n) {
          return sum + n;
        }, 0) / numbers.length
      );
    }

    return filtered.length;
  }

  function groupByBarOption(rows, binding, columns) {
    var categoryField = binding && binding.category;
    var catIdx = columnIndex(columns, categoryField);
    if (catIdx < 0) {
      return polishChartOption({
        xAxis: { type: "category", data: [] },
        yAxis: { type: "value" },
        series: [{ type: "bar", data: [], type: "bar" }],
      });
    }

    var filtered = filterRowsByWhere(rows, binding && binding.where, columns);
    var groups = new Map();

    for (var i = 0; i < filtered.length; i++) {
      var row = filtered[i];
      var key = String(row[catIdx]);
      if (!groups.has(key)) groups.set(key, []);
      groups.get(key).push(row);
    }

    var categories = [];
    var data = [];
    var valueBinding = (binding && binding.value) || {};
    var aggregate = valueBinding.aggregate || (binding && binding.aggregate) || "count";
    var valueField = valueBinding.field || (binding && binding.field);

    groups.forEach(function (groupRows, category) {
      categories.push(category);
      if (aggregate === "count") {
        data.push(groupRows.length);
      } else {
        data.push(
          aggregateCount(groupRows, { aggregate: aggregate, field: valueField }, columns)
        );
      }
    });

    return polishChartOption({
      tooltip: { trigger: "axis" },
      xAxis: {
        type: "category",
        data: categories,
        axisLabel: { rotate: categories.length > 6 ? 28 : 0, interval: 0 },
      },
      yAxis: { type: "value", splitLine: { show: true } },
      series: [
        {
          type: "bar",
          data: data,
          barMaxWidth: 48,
          itemStyle: { borderRadius: [6, 6, 0, 0] },
        },
      ],
    });
  }

  function compileBinding(binding, datasets, filterState, filters) {
    if (!binding || !binding.dataset) return null;
    var dataset = datasets && datasets[binding.dataset];
    if (!dataset) return null;

    var columns = dataset.columns || [];
    var rows = dataset.rows || [];
    rows = applyFilters(rows, filterState, filters, columns);

    if (binding.chart === "bar") {
      return { kind: "echarts", option: groupByBarOption(rows, binding, columns) };
    }

    if (binding.columns && binding.columns.length > 0) {
      var indices = binding.columns.map(function (col) {
        return columnIndex(columns, col);
      });
      var tableRows = rows.map(function (row) {
        return indices.map(function (idx) {
          return idx >= 0 ? row[idx] : null;
        });
      });
      return { kind: "table", columns: binding.columns.slice(), rows: tableRows };
    }

    if (binding.aggregate || binding.field || binding.value) {
      return {
        kind: "kpi",
        value: aggregateCount(rows, binding, columns),
      };
    }

    return { kind: "rows", rows: rows, columns: columns };
  }

  function mapDrilldownValue(params, drilldown) {
    var source = (drilldown && drilldown.value_from) || "category";
    if (source === "seriesName") return params && params.seriesName != null ? params.seriesName : "";
    if (source === "name") return params && params.name != null ? params.name : "";
    return params && params.name != null ? params.name : "";
  }

  function applyDrilldownSetFilter(params, drilldown, filterState) {
    if (!drilldown || !drilldown.filter_id) return filterState;
    var next = Object.assign({}, filterState || {});
    next[drilldown.filter_id] = mapDrilldownValue(params, drilldown);
    return next;
  }

  function exportPDF(printFn) {
    var fn = printFn || (typeof global.print === "function" ? global.print : null);
    if (typeof fn === "function") {
      fn();
    }
  }

  function initialFilterState(filters) {
    var state = {};
    for (var i = 0; i < (filters || []).length; i++) {
      var filter = filters[i];
      if (filter.type === "date_range") {
        state[filter.id] = { from: "", to: "" };
      } else {
        state[filter.id] = filter.default != null ? filter.default : "";
      }
    }
    return state;
  }

  function escapeHTML(text) {
    return String(text)
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;");
  }

  function formatCell(value) {
    if (value == null) return "";
    if (typeof value === "number" && !Number.isInteger(value)) {
      return value.toFixed(2);
    }
    return String(value);
  }

  var chartInstances = {};
  var pageState = null;

  function findSection(page, index) {
    var sections = page.sections || [];
    if (index < 0 || index >= sections.length) return null;
    return sections[index];
  }

  function sectionElement(index) {
    return document.querySelector('[data-section-index="' + index + '"]');
  }

  function renderKPI(section, filterState) {
    var items = section.items || [];
    var html = ['<div class="baize-kpi-grid">'];
    for (var i = 0; i < items.length; i++) {
      var item = items[i];
      var value = item.value;
      if (item.binding && pageState) {
        var compiled = compileBinding(
          item.binding,
          pageState.datasets,
          filterState,
          pageState.filters
        );
        if (compiled && compiled.kind === "kpi") {
          value = compiled.value;
        }
      }
      html.push(
        '<div class="baize-kpi-item"><div class="baize-kpi-label">' +
          escapeHTML(item.label || "") +
          '</div><div class="baize-kpi-value">' +
          escapeHTML(formatCell(value)) +
          "</div></div>"
      );
    }
    html.push("</div>");
    return html.join("");
  }

  function renderTable(section, filterState) {
    var columns = section.columns || [];
    var rows = section.rows || [];

    if (section.binding && pageState) {
      var compiled = compileBinding(
        section.binding,
        pageState.datasets,
        filterState,
        pageState.filters
      );
      if (compiled && compiled.kind === "table") {
        columns = compiled.columns;
        rows = compiled.rows;
      }
    }

    var html = ['<table class="baize-table"><thead><tr>'];
    for (var c = 0; c < columns.length; c++) {
      html.push("<th>" + escapeHTML(columns[c]) + "</th>");
    }
    html.push("</tr></thead><tbody>");
    for (var r = 0; r < rows.length; r++) {
      html.push("<tr>");
      for (var j = 0; j < rows[r].length; j++) {
        html.push("<td>" + escapeHTML(formatCell(rows[r][j])) + "</td>");
      }
      html.push("</tr>");
    }
    html.push("</tbody></table>");
    return html.join("");
  }

  function renderECharts(section, index, filterState) {
    var containerId = "baize-chart-" + index;
    var html =
      (section.title
        ? '<h3 class="baize-section-title">' + escapeHTML(section.title) + "</h3>"
        : "") + '<div class="baize-chart" id="' + containerId + '"></div>';

    requestAnimationFrame(function () {
      var el = document.getElementById(containerId);
      if (!el || typeof echarts === "undefined") return;

      if (chartInstances[index]) {
        chartInstances[index].dispose();
      }

      registerChartThemes();
      var themeName = isDarkTheme() ? "baize-dark" : "baize-light";
      var chart = echarts.init(el, themeName);
      chartInstances[index] = chart;

      var option = section.option;
      if (section.binding && pageState) {
        var compiled = compileBinding(
          section.binding,
          pageState.datasets,
          filterState,
          pageState.filters
        );
        if (compiled && compiled.kind === "echarts") {
          option = compiled.option;
        }
      }

      if (option) {
        chart.setOption(polishChartOption(option));
      }

      if (section.drilldown && section.drilldown.on === "click") {
        chart.off("click");
        chart.on("click", function (params) {
          handleDrilldown(section, params, filterState);
        });
      }
    });

    return html;
  }

  function renderRow(section, filterState, baseIndex) {
    var children = section.sections || [];
    var html = ['<div class="baize-row">'];
    for (var i = 0; i < children.length; i++) {
      html.push(
        '<div class="baize-row-item">' +
          renderSectionContent(children[i], baseIndex + "-" + i, filterState) +
          "</div>"
      );
    }
    html.push("</div>");
    return html.join("");
  }

  function renderSectionContent(section, key, filterState) {
    switch (section.type) {
      case "kpi":
        return renderKPI(section, filterState);
      case "table":
        return (
          (section.title
            ? '<h3 class="baize-section-title">' + escapeHTML(section.title) + "</h3>"
            : "") + renderTable(section, filterState)
        );
      case "echarts":
        return renderECharts(section, key, filterState);
      case "row":
        return renderRow(section, filterState, key);
      default:
        return "";
    }
  }

  function renderSection(index, filterState) {
    if (!pageState) return;
    var section = findSection(pageState, index);
    var el = sectionElement(index);
    if (!section || !el) return;

    if (section.type === "markdown") return;

    var title = section.title
      ? '<h3 class="baize-section-title">' + escapeHTML(section.title) + "</h3>"
      : "";

    switch (section.type) {
      case "kpi":
        el.innerHTML = title + renderKPI(section, filterState);
        break;
      case "table":
        el.innerHTML = title + renderTable(section, filterState);
        break;
      case "echarts":
        el.innerHTML = renderECharts(section, index, filterState);
        break;
      case "row":
        el.innerHTML = title + renderRow(section, filterState, String(index));
        break;
      default:
        break;
    }
  }

  function renderAllSections(filterState) {
    if (!pageState) return;
    var sections = pageState.sections || [];
    for (var i = 0; i < sections.length; i++) {
      renderSection(i, filterState);
    }
  }

  function syncFilterControls(filterState) {
    if (!pageState) return;
    var filters = pageState.filters || [];
    for (var i = 0; i < filters.length; i++) {
      var filter = filters[i];
      if (filter.type === "select") {
        var select = document.querySelector('[data-filter-id="' + filter.id + '"]');
        if (select) select.value = filterState[filter.id] != null ? filterState[filter.id] : "";
      } else if (filter.type === "date_range") {
        var fromInput = document.querySelector('[data-filter-id="' + filter.id + '"][data-range="from"]');
        var toInput = document.querySelector('[data-filter-id="' + filter.id + '"][data-range="to"]');
        var range = filterState[filter.id] || { from: "", to: "" };
        if (fromInput) fromInput.value = range.from || "";
        if (toInput) toInput.value = range.to || "";
      }
    }
  }

  function handleDrilldown(section, params, filterState) {
    var drilldown = section.drilldown;
    if (!drilldown) return;

    if (drilldown.action === "set_filter") {
      var next = applyDrilldownSetFilter(params, drilldown, filterState);
      onFilterChange(next);
      return;
    }

    if (drilldown.action === "goto_section" && drilldown.target_section_id) {
      if (drilldown.filter_id) {
        var updated = applyDrilldownSetFilter(params, drilldown, filterState);
        onFilterChange(updated);
      }
      var target = document.getElementById(drilldown.target_section_id);
      if (!target) {
        target = document.querySelector('[data-section-id="' + drilldown.target_section_id + '"]');
      }
      if (target) target.scrollIntoView({ behavior: "smooth", block: "start" });
      return;
    }

    if (drilldown.action === "detail") {
      showDetailModal(section, params, drilldown, filterState);
    }
  }

  function showDetailModal(section, params, drilldown, filterState) {
    if (!pageState || !section.binding) return;
    var dataset = pageState.datasets[section.binding.dataset];
    if (!dataset) return;

    var columns = dataset.columns || [];
    var rows = applyFilters(dataset.rows || [], filterState, pageState.filters, columns);
    var match = drilldown.match || {};

    rows = rows.filter(function (row) {
      return Object.keys(match).every(function (field) {
        var fromKey = match[field];
        var expected =
          fromKey === "category" || fromKey === "name"
            ? params.name
            : fromKey === "seriesName"
              ? params.seriesName
              : params[fromKey];
        var idx = columnIndex(columns, field);
        if (idx < 0) return false;
        return String(row[idx]) === String(expected);
      });
    });

    var overlay = document.getElementById("baize-detail-overlay");
    if (!overlay) {
      overlay = document.createElement("div");
      overlay.id = "baize-detail-overlay";
      overlay.className = "baize-detail-overlay";
      overlay.innerHTML =
        '<div class="baize-detail-panel"><button type="button" class="baize-detail-close">关闭</button><div class="baize-detail-body"></div></div>';
      document.body.appendChild(overlay);
      overlay.querySelector(".baize-detail-close").addEventListener("click", function () {
        overlay.style.display = "none";
      });
      overlay.addEventListener("click", function (event) {
        if (event.target === overlay) overlay.style.display = "none";
      });
    }

    var body = overlay.querySelector(".baize-detail-body");
    body.innerHTML = renderTable({ columns: columns, rows: rows }, filterState);
    overlay.style.display = "flex";
  }

  function onFilterChange(nextState) {
    if (!pageState) return;
    pageState.filterState = nextState;
    syncFilterControls(nextState);
    renderAllSections(nextState);
  }

  function renderFilterBar(page, filterState) {
    var bar = document.getElementById("filter-bar");
    if (!bar) return;

    var filters = page.filters || [];
    if (filters.length === 0) {
      bar.style.display = "none";
      return;
    }

    bar.innerHTML = "";
    bar.style.display = "";

    for (var i = 0; i < filters.length; i++) {
      (function (filter) {
        var wrapper = document.createElement("label");
        wrapper.className = "baize-filter";
        wrapper.textContent = filter.label || filter.id;
        wrapper.insertAdjacentHTML(
          "afterbegin",
          '<span class="baize-filter-label">' + escapeHTML(filter.label || filter.id) + "</span>"
        );
        wrapper.textContent = "";

        if (filter.type === "select") {
          var select = document.createElement("select");
          select.setAttribute("data-filter-id", filter.id);
          var allOption = document.createElement("option");
          allOption.value = "";
          allOption.textContent = "全部";
          select.appendChild(allOption);
          var options = filter.options || [];
          for (var j = 0; j < options.length; j++) {
            var opt = document.createElement("option");
            opt.value = options[j];
            opt.textContent = options[j];
            select.appendChild(opt);
          }
          select.value = filterState[filter.id] != null ? filterState[filter.id] : "";
          select.addEventListener("change", function () {
            var next = Object.assign({}, pageState.filterState);
            next[filter.id] = select.value;
            onFilterChange(next);
          });
          wrapper.appendChild(select);
        } else if (filter.type === "date_range") {
          var fromInput = document.createElement("input");
          fromInput.type = "date";
          fromInput.setAttribute("data-filter-id", filter.id);
          fromInput.setAttribute("data-range", "from");
          var toInput = document.createElement("input");
          toInput.type = "date";
          toInput.setAttribute("data-filter-id", filter.id);
          toInput.setAttribute("data-range", "to");
          var range = filterState[filter.id] || { from: "", to: "" };
          fromInput.value = range.from || "";
          toInput.value = range.to || "";
          var onDateChange = function () {
            var next = Object.assign({}, pageState.filterState);
            next[filter.id] = { from: fromInput.value, to: toInput.value };
            onFilterChange(next);
          };
          fromInput.addEventListener("change", onDateChange);
          toInput.addEventListener("change", onDateChange);
          wrapper.appendChild(fromInput);
          wrapper.appendChild(document.createTextNode(" – "));
          wrapper.appendChild(toInput);
        }

        bar.appendChild(wrapper);
      })(filters[i]);
    }
  }

  function wireExportPDF() {
    var button = document.getElementById("export-pdf");
    if (!button) return;
    button.addEventListener("click", function () {
      exportPDF();
    });
  }

  function injectStyles() {
    if (document.getElementById("baize-runtime-styles")) return;
    var style = document.createElement("style");
    style.id = "baize-runtime-styles";
    style.textContent =
      ".baize-kpi-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(10rem,1fr));gap:0.75rem}.baize-kpi-item{padding:0.9rem 1rem;border:1px solid var(--border,#e2e8f0);border-radius:12px;background:linear-gradient(145deg,var(--surface-2,#f8fafc),var(--surface,#fff))}.baize-kpi-label{font-size:0.78rem;color:var(--muted,#64748b);margin-bottom:0.35rem;font-weight:500}.baize-kpi-value{font-size:1.55rem;font-weight:700;letter-spacing:-0.02em;color:var(--text,#0f172a);font-variant-numeric:tabular-nums}.baize-table{width:100%;border-collapse:separate;border-spacing:0;font-size:0.88rem}.baize-table th,.baize-table td{padding:0.55rem 0.75rem;text-align:left;border-bottom:1px solid var(--border,#e2e8f0)}.baize-table th{font-size:0.75rem;font-weight:600;color:var(--muted,#64748b);text-transform:uppercase;letter-spacing:0.04em;background:var(--surface-2,#f8fafc)}.baize-table tbody tr:hover td{background:var(--accent-soft,rgba(59,102,245,0.06))}.baize-table tbody tr:last-child td{border-bottom:none}.baize-chart{width:100%;height:360px;min-height:280px}.baize-row{display:flex;flex-wrap:wrap;gap:1rem}.baize-row-item{flex:1 1 300px}.baize-filter{display:flex;flex-direction:column;gap:0.3rem;font-size:0.82rem;padding:0.5rem 0.75rem;background:var(--surface,#fff);border:1px solid var(--border,#e2e8f0);border-radius:10px}.baize-filter-label{font-weight:500;color:var(--muted,#64748b)}.baize-filter select,.baize-filter input{padding:0.35rem 0.5rem;border:1px solid var(--border,#e2e8f0);border-radius:8px;background:var(--surface,#fff);color:var(--text,#0f172a);font-size:0.85rem}.baize-section-title{margin:0 0 0.85rem;font-size:1.02rem;font-weight:600;color:var(--text,#0f172a)}.baize-detail-overlay{display:none;position:fixed;inset:0;background:rgba(15,23,42,0.45);align-items:center;justify-content:center;z-index:1000;backdrop-filter:blur(4px)}.baize-detail-panel{background:var(--surface,#fff);border-radius:14px;padding:1rem 1.25rem;max-width:90vw;max-height:80vh;overflow:auto;box-shadow:0 20px 48px rgba(15,23,42,0.2)}.baize-detail-close{margin-bottom:0.75rem;padding:0.35rem 0.75rem;border:1px solid var(--border,#e2e8f0);border-radius:8px;background:var(--surface-2,#f8fafc);cursor:pointer}";
    document.head.appendChild(style);
  }

  function applyPageTheme(page) {
    var theme = page && String(page.theme).toLowerCase() === "dark" ? "dark" : "light";
    document.body.setAttribute("data-theme", theme);
    registerChartThemes();
  }

  function wireChartResize() {
    if (global.__baizeChartResize) return;
    global.__baizeChartResize = true;
    window.addEventListener("resize", function () {
      var keys = Object.keys(chartInstances);
      for (var i = 0; i < keys.length; i++) {
        var c = chartInstances[keys[i]];
        if (c && c.resize) c.resize();
      }
    });
  }

  function init(page) {
    pageState = page || global.__BAIZE_PAGE__ || {};
    pageState.filterState = initialFilterState(pageState.filters);
    applyPageTheme(pageState);
    injectStyles();
    renderFilterBar(pageState, pageState.filterState);
    wireExportPDF();
    wireChartResize();
    renderAllSections(pageState.filterState);
  }

  global.BaizeReport = {
    init: init,
    renderSection: renderSection,
    applyFilters: applyFilters,
    aggregateCount: aggregateCount,
    groupByBarOption: groupByBarOption,
    compileBinding: compileBinding,
    mapDrilldownValue: mapDrilldownValue,
    applyDrilldownSetFilter: applyDrilldownSetFilter,
    exportPDF: exportPDF,
    initialFilterState: initialFilterState,
  };
})(typeof window !== "undefined" ? window : globalThis);
