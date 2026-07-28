// @ts-expect-error Vitest runs in Node; the browser app intentionally omits Node typings.
import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

const source = readFileSync("src/pages/ServerDetailPage.tsx", "utf8");

describe("ServerDetailPage historical bundle boundary", () => {
  it("lazy loads charts only behind an accessible Suspense fallback", () => {
    expect(source).toMatch(
      /lazy\(\(\)\s*=>\s*import\("\.\.\/history\/HistoricalMetrics"\)/,
    );
    expect(source).toMatch(
      /<Suspense\s+fallback=\{<HistoryChartFallback\s*\/?>\}/,
    );
    expect(source).toMatch(
      /function HistoryChartFallback[\s\S]*?role="status"[\s\S]*?正在准备历史图表/,
    );
  });
});
