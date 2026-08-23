(function (global) {
  "use strict";

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
      return {
        xAxis: { type: "category", data: [] },
        yAxis: { type: "value" },
        series: [{ type: "bar", data: [] }],
      };
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

    return {
      xAxis: { type: "category", data: categories },
      yAxis: { type: "value" },
      series: [{ type: "bar", data: data }],
    };
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

      var chart = echarts.init(el);
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
        chart.setOption(option);
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
      ".baize-kpi-grid{display:flex;flex-wrap:wrap;gap:1rem}.baize-kpi-item{min-width:8rem;padding:0.75rem 1rem;border:1px solid #e5e5e5;border-radius:6px;background:#fafafa}.baize-kpi-label{font-size:0.85rem;color:#666;margin-bottom:0.25rem}.baize-kpi-value{font-size:1.4rem;font-weight:600}.baize-table{width:100%;border-collapse:collapse;font-size:0.9rem}.baize-table th,.baize-table td{border:1px solid #e5e5e5;padding:0.45rem 0.6rem;text-align:left}.baize-table th{background:#f5f5f5}.baize-chart{width:100%;height:320px}.baize-row{display:flex;flex-wrap:wrap;gap:1rem}.baize-row-item{flex:1 1 280px}.baize-filter{display:flex;flex-direction:column;gap:0.25rem;font-size:0.85rem}.baize-section-title{margin:0 0 0.75rem;font-size:1rem}.baize-detail-overlay{display:none;position:fixed;inset:0;background:rgba(0,0,0,0.35);align-items:center;justify-content:center;z-index:1000}.baize-detail-panel{background:#fff;border-radius:8px;padding:1rem;max-width:90vw;max-height:80vh;overflow:auto}.baize-detail-close{margin-bottom:0.75rem}";
    document.head.appendChild(style);
  }

  function init(page) {
    pageState = page || global.__BAIZE_PAGE__ || {};
    pageState.filterState = initialFilterState(pageState.filters);
    injectStyles();
    renderFilterBar(pageState, pageState.filterState);
    wireExportPDF();
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
