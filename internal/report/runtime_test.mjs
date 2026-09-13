import assert from "node:assert/strict";
import { describe, it } from "node:test";

import {
  aggregateCount,
  applyDrilldownSetFilter,
  applyFilters,
  exportPDF,
  groupByBarOption,
  mapDrilldownValue,
} from "./runtime-core.mjs";

const columns = ["id", "status", "priority", "created_at"];
const rows = [
  ["1", "open", "high", "2026-08-01"],
  ["2", "closed", "low", "2026-08-10"],
  ["3", "open", "medium", "2026-08-15"],
  ["4", "closed", "high", "2026-08-20"],
];

const filters = [
  {
    id: "status",
    type: "select",
    label: "状态",
    dataset: "tickets",
    field: "status",
    options: ["open", "closed"],
    default: "",
  },
  {
    id: "date_range",
    type: "date_range",
    label: "创建时间",
    dataset: "tickets",
    field: "created_at",
  },
];

describe("applyFilters", () => {
  it("filters rows by select control", () => {
    const filtered = applyFilters(rows, { status: "open" }, filters, columns);
    assert.deepEqual(filtered, [
      ["1", "open", "high", "2026-08-01"],
      ["3", "open", "medium", "2026-08-15"],
    ]);
  });

  it("ignores empty select value (all)", () => {
    const filtered = applyFilters(rows, { status: "" }, filters, columns);
    assert.equal(filtered.length, rows.length);
  });

  it("filters rows by date_range", () => {
    const filtered = applyFilters(
      rows,
      { date_range: { from: "2026-08-10", to: "2026-08-15" } },
      filters,
      columns
    );
    assert.deepEqual(filtered, [
      ["2", "closed", "low", "2026-08-10"],
      ["3", "open", "medium", "2026-08-15"],
    ]);
  });
});

describe("binding helpers", () => {
  it("aggregateCount counts all rows", () => {
    assert.equal(aggregateCount(rows, { aggregate: "count" }, columns), 4);
  });

  it("aggregateCount counts rows matching where", () => {
    assert.equal(
      aggregateCount(rows, { aggregate: "count", where: { status: "open" } }, columns),
      2
    );
  });

  it("groupByBarOption builds bar chart option", () => {
    const option = groupByBarOption(
      rows,
      {
        category: "status",
        value: { aggregate: "count" },
      },
      columns
    );
    assert.deepEqual(option.xAxis.data, ["open", "closed"]);
    assert.deepEqual(option.series[0].data, [2, 2]);
    assert.equal(option.series[0].type, "bar");
  });
});

describe("drilldown set_filter", () => {
  it("maps click params to filter value", () => {
    assert.equal(
      mapDrilldownValue({ name: "open", seriesName: "Count" }, { value_from: "category" }),
      "open"
    );
    assert.equal(
      mapDrilldownValue({ name: "open", seriesName: "Count" }, { value_from: "seriesName" }),
      "Count"
    );
  });

  it("updates filter state from chart click", () => {
    const next = applyDrilldownSetFilter(
      { name: "closed" },
      { action: "set_filter", filter_id: "status", value_from: "category" },
      { status: "" }
    );
    assert.deepEqual(next, { status: "closed" });
  });
});

describe("exportPDF", () => {
  it("calls window.print when invoked", () => {
    let called = false;
    exportPDF(() => {
      called = true;
    });
    assert.equal(called, true);
  });
});
