// @ts-expect-error Vitest runs in Node; the browser app intentionally omits Node typings.
import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

const dashboardCss = readFileSync("src/styles/dashboard.css", "utf8");

const REM_PIXELS = 16;

function blockFor(source: string, marker: string): string {
  const markerIndex = source.indexOf(marker);
  if (markerIndex === -1) {
    return "";
  }
  const openIndex = source.indexOf("{", markerIndex);
  let depth = 0;
  for (let index = openIndex; index < source.length; index += 1) {
    if (source[index] === "{") {
      depth += 1;
    } else if (source[index] === "}") {
      depth -= 1;
      if (depth === 0) {
        return source.slice(openIndex + 1, index);
      }
    }
  }
  return "";
}

function declaration(block: string, property: string): string {
  return block.match(new RegExp(`${property}:\\s*([^;]+);`))?.[1].trim() ?? "";
}

function horizontalPaddingRem(value: string): number {
  const values = [...value.matchAll(/([\d.]+)rem/g)].map((match) => Number(match[1]));
  return values.length === 1 ? values[0] : values[1] ?? 0;
}

function gridMinimum(grid: string): { columns: number; rem: number } {
  let columns = 0;
  let rem = 0;
  const withoutRepeats = grid.replace(
    /repeat\((\d+),\s*minmax\(([\d.]+)rem,[^)]+\)\)/g,
    (_match, countValue: string, minimumValue: string) => {
      const count = Number(countValue);
      columns += count;
      rem += count * Number(minimumValue);
      return "";
    },
  );
  for (const match of withoutRepeats.matchAll(/minmax\(([\d.]+)rem,[^)]+\)/g)) {
    columns += 1;
    rem += Number(match[1]);
  }
  return { columns, rem };
}

describe("dashboard responsive grid contract", () => {
  it("switches layouts no later than the full row minimum viewport width", () => {
    const desktopCss = dashboardCss.slice(0, dashboardCss.indexOf("@media"));
    const row = blockFor(desktopCss, ".server-row {");
    const content = blockFor(desktopCss, ".dashboard-content {");
    const grid = gridMinimum(declaration(row, "grid-template-columns"));
    const gapRem = Number.parseFloat(declaration(row, "gap"));
    const rowPaddingRem = horizontalPaddingRem(declaration(row, "padding"));
    const contentPaddingRem = horizontalPaddingRem(declaration(content, "padding"));
    const requiredViewportRem =
      grid.rem +
      (grid.columns - 1) * gapRem +
      rowPaddingRem * 2 +
      contentPaddingRem * 2;
    const firstBreakpoint = Number(
      dashboardCss.match(/@media \(max-width: ([\d.]+)rem\)/)?.[1],
    );

    expect(firstBreakpoint * REM_PIXELS).toBeGreaterThanOrEqual(
      requiredViewportRem * REM_PIXELS,
    );
  });

  it("hides only optional fields at the tablet and mobile breakpoints", () => {
    const cumulativeBreakpoint = blockFor(
      dashboardCss,
      "@media (max-width: 76.5rem)",
    );
    const compactBreakpoint = blockFor(
      dashboardCss,
      "@media (max-width: 61.25rem)",
    );
    const mobileBreakpoint = blockFor(
      dashboardCss,
      "@media (max-width: 46rem)",
    );

    expect(blockFor(cumulativeBreakpoint, ".server-row-field--cumulative"))
      .toMatch(/display:\s*none/);
    expect(blockFor(compactBreakpoint, ".server-row-field--secondary"))
      .toMatch(/display:\s*none/);
    expect(blockFor(mobileBreakpoint, ".server-row"))
      .toMatch(/grid-template-columns:\s*repeat\(2,/);
    expect(blockFor(mobileBreakpoint, ".server-row-identity"))
      .toMatch(/grid-column:\s*1 \/ -1/);
    expect(dashboardCss).not.toMatch(
      /\.server-row-field--mobile-core\s*\{[^}]*display:\s*none/s,
    );
  });
});
