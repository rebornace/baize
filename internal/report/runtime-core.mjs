/**
 * Pure filter/binding/drilldown helpers for Baize analysis pages.
 * Imported by runtime_test.mjs; mirrored in runtime.js for the browser bundle.
 */

export function columnIndex(columns, field) {
  if (!columns || field == null || field === "") return -1;
  return columns.indexOf(field);
}

export function filterRowsByWhere(rows, where, columns) {
  if (!where || Object.keys(where).length === 0) return rows;
  return rows.filter((row) =>
    Object.entries(where).every(([field, expected]) => {
      const idx = columnIndex(columns, field);
      if (idx < 0) return false;
      return String(row[idx]) === String(expected);
    })
  );
}

/**
 * Apply page-level filter controls to dataset rows.
 * @param {Array<Array<any>>} rows
 * @param {Record<string, any>} filterState
 * @param {Array<{id:string,type:string,field:string,dataset?:string}>} filters
 * @param {string[]} columns
 */
export function applyFilters(rows, filterState, filters, columns) {
  if (!rows || rows.length === 0) return [];
  let result = rows.slice();
  for (const filter of filters || []) {
    const state = filterState?.[filter.id];
    if (state == null || state === "") continue;

    const idx = columnIndex(columns, filter.field);
    if (idx < 0) continue;

    if (filter.type === "select") {
      result = result.filter((row) => String(row[idx]) === String(state));
      continue;
    }

    if (filter.type === "date_range") {
      const from = state.from ?? "";
      const to = state.to ?? "";
      if (!from && !to) continue;
      result = result.filter((row) => {
        const value = row[idx];
        if (value == null || value === "") return false;
        const text = String(value);
        if (from && text < from) return false;
        if (to && text > to) return false;
        return true;
      });
    }
  }
  return result;
}

/**
 * Aggregate rows according to a binding shorthand.
 */
export function aggregateCount(rows, binding, columns) {
  const filtered = filterRowsByWhere(rows, binding?.where, columns);
  const aggregate = binding?.aggregate || binding?.value?.aggregate || "count";
  const field = binding?.field || binding?.value?.field;

  if (aggregate === "count") {
    return filtered.length;
  }

  const idx = columnIndex(columns, field);
  if (idx < 0) return 0;

  const numbers = filtered
    .map((row) => Number(row[idx]))
    .filter((n) => !Number.isNaN(n));

  if (aggregate === "sum") {
    return numbers.reduce((sum, n) => sum + n, 0);
  }

  if (aggregate === "avg") {
    if (numbers.length === 0) return 0;
    return numbers.reduce((sum, n) => sum + n, 0) / numbers.length;
  }

  return filtered.length;
}

/**
 * Build a bar chart ECharts option from grouped rows.
 */
export function groupByBarOption(rows, binding, columns) {
  const categoryField = binding?.category;
  const catIdx = columnIndex(columns, categoryField);
  if (catIdx < 0) {
    return {
      xAxis: { type: "category", data: [] },
      yAxis: { type: "value" },
      series: [{ type: "bar", data: [] }],
    };
  }

  const filtered = filterRowsByWhere(rows, binding?.where, columns);
  const groups = new Map();

  for (const row of filtered) {
    const key = String(row[catIdx]);
    if (!groups.has(key)) groups.set(key, []);
    groups.get(key).push(row);
  }

  const categories = [];
  const data = [];
  const valueBinding = binding?.value || {};
  const aggregate = valueBinding.aggregate || binding?.aggregate || "count";
  const valueField = valueBinding.field || binding?.field;

  for (const [category, groupRows] of groups) {
    categories.push(category);
    if (aggregate === "count") {
      data.push(groupRows.length);
    } else {
      data.push(aggregateCount(groupRows, { aggregate, field: valueField }, columns));
    }
  }

  return {
    xAxis: { type: "category", data: categories },
    yAxis: { type: "value" },
    series: [{ type: "bar", data }],
  };
}

export function compileBinding(binding, datasets, filterState, filters) {
  if (!binding?.dataset) return null;
  const dataset = datasets?.[binding.dataset];
  if (!dataset) return null;

  const columns = dataset.columns || [];
  let rows = dataset.rows || [];
  rows = applyFilters(rows, filterState, filters, columns);

  if (binding.chart === "bar") {
    return { kind: "echarts", option: groupByBarOption(rows, binding, columns) };
  }

  if (binding.columns && binding.columns.length > 0) {
    const indices = binding.columns.map((col) => columnIndex(columns, col));
    const tableRows = rows.map((row) =>
      indices.map((idx) => (idx >= 0 ? row[idx] : null))
    );
    return { kind: "table", columns: binding.columns.slice(), rows: tableRows };
  }

  if (binding.aggregate || binding.field || binding.value) {
    return {
      kind: "kpi",
      value: aggregateCount(rows, binding, columns),
    };
  }

  return { kind: "rows", rows, columns };
}

export function mapDrilldownValue(params, drilldown) {
  const source = drilldown?.value_from || "category";
  if (source === "seriesName") return params?.seriesName ?? "";
  if (source === "name") return params?.name ?? "";
  return params?.name ?? "";
}

export function applyDrilldownSetFilter(params, drilldown, filterState) {
  if (!drilldown?.filter_id) return filterState;
  const next = { ...(filterState || {}) };
  next[drilldown.filter_id] = mapDrilldownValue(params, drilldown);
  return next;
}

export function exportPDF(printFn) {
  const fn = printFn || (typeof window !== "undefined" ? window.print : null);
  if (typeof fn === "function") {
    fn();
  }
}

export function initialFilterState(filters) {
  const state = {};
  for (const filter of filters || []) {
    if (filter.type === "date_range") {
      state[filter.id] = { from: "", to: "" };
    } else {
      state[filter.id] = filter.default ?? "";
    }
  }
  return state;
}
