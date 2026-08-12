import assert from "node:assert/strict";
import test from "node:test";
import { buildBudgetSummary } from "../podcasts-resource-audit.mjs";

test("browser resource budgets use P95 transfer bytes for cold and warm runs", () => {
  const runs = [
    {
      cold: { transferBytes: 500_000, failedCount: 0, fontUrls: [] },
      warm: { transferBytes: 20_000, failedCount: 0, fontUrls: [] },
    },
    {
      cold: { transferBytes: 700_000, failedCount: 0, fontUrls: [] },
      warm: { transferBytes: 30_000, failedCount: 0, fontUrls: [] },
    },
    {
      cold: { transferBytes: 950_000, failedCount: 0, fontUrls: [] },
      warm: { transferBytes: 40_000, failedCount: 0, fontUrls: [] },
    },
  ];

  assert.deepEqual(buildBudgetSummary(runs, 900_000, 50_000), {
    coldP95Bytes: 950_000,
    warmP95Bytes: 40_000,
    coldBudgetBytes: 900_000,
    warmBudgetBytes: 50_000,
    coldFailedCount: 0,
    warmFailedCount: 0,
    unexpectedFontUrls: [],
    coldWithinBudget: false,
    warmWithinBudget: true,
    requestsSucceeded: true,
    fontPolicyPassed: true,
  });
});

test("browser resource budget fails when a request returns an error", () => {
  const runs = [
    {
      cold: { transferBytes: 500_000, failedCount: 1, fontUrls: [] },
      warm: { transferBytes: 20_000, failedCount: 0, fontUrls: [] },
    },
  ];

  assert.deepEqual(buildBudgetSummary(runs, 900_000, 50_000), {
    coldP95Bytes: 500_000,
    warmP95Bytes: 20_000,
    coldBudgetBytes: 900_000,
    warmBudgetBytes: 50_000,
    coldFailedCount: 1,
    warmFailedCount: 0,
    unexpectedFontUrls: [],
    coldWithinBudget: true,
    warmWithinBudget: true,
    requestsSucceeded: false,
    fontPolicyPassed: true,
  });
});

test("browser resource budget rejects CJK display-font shards on podcasts", () => {
  const fontUrl =
    "http://localhost/_next/static/media/lxgwwenkaigbscreen-subset-1.woff2";
  const runs = [
    {
      cold: {
        transferBytes: 500_000,
        failedCount: 0,
        fontUrls: [fontUrl],
      },
      warm: { transferBytes: 20_000, failedCount: 0, fontUrls: [] },
    },
  ];

  assert.deepEqual(buildBudgetSummary(runs, 900_000, 50_000), {
    coldP95Bytes: 500_000,
    warmP95Bytes: 20_000,
    coldBudgetBytes: 900_000,
    warmBudgetBytes: 50_000,
    coldFailedCount: 0,
    warmFailedCount: 0,
    unexpectedFontUrls: [fontUrl],
    coldWithinBudget: true,
    warmWithinBudget: true,
    requestsSucceeded: true,
    fontPolicyPassed: false,
  });
});
