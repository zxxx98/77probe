// @ts-expect-error Vitest runs in Node; the browser app intentionally omits Node typings.
import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

const dashboardCss = readFileSync("src/styles/dashboard.css", "utf8");

describe("server management responsive contract", () => {
  it("keeps management controls touch-sized and keyboard-visible", () => {
    expect(dashboardCss).toMatch(
      /\.architecture-tabs button\s*\{[^}]*min-height:\s*44px/s,
    );
    expect(dashboardCss).toMatch(
      /\.rename-form input\s*\{[^}]*min-height:\s*44px/s,
    );
    expect(dashboardCss).toMatch(
      /\.architecture-tabs button:focus-visible,[\s\S]*outline:\s*3px solid var\(--color-primary\)/,
    );
  });

  it("contains long commands without page-level horizontal overflow", () => {
    expect(dashboardCss).toMatch(
      /\.install-copy-block\s*\{[^}]*min-width:\s*0/s,
    );
    expect(dashboardCss).toMatch(
      /\.install-copy-block pre\s*\{[^}]*max-width:\s*100%[^}]*overflow-x:\s*auto/s,
    );
    expect(dashboardCss).toMatch(
      /\.managed-server-row\s*\{[^}]*min-width:\s*0/s,
    );
    expect(dashboardCss).toMatch(
      /\.managed-server-actions\s*\{[^}]*flex-wrap:\s*wrap/s,
    );
  });

  it("stacks management headers, rows, confirmations, and copy headings on mobile", () => {
    const mobile = dashboardCss.slice(
      dashboardCss.indexOf("@media (max-width: 46rem)"),
    );

    for (const selector of [
      ".management-heading",
      ".managed-server-row",
      ".inline-confirmation",
      ".install-panel-heading",
      ".install-copy-heading",
    ]) {
      expect(mobile).toMatch(
        new RegExp(
          `${selector.replace(".", "\\.")}[\\s\\S]*?flex-direction:\\s*column`,
        ),
      );
    }
  });
});
