import { describe, expect, it } from "vitest";

const nodeProcess = (
  globalThis as unknown as {
    process: {
      cwd(): string;
      getBuiltinModule(name: "fs"): {
        readFileSync(path: string, encoding: "utf8"): string;
      };
    };
  }
).process;
const { readFileSync } = nodeProcess.getBuiltinModule("fs");
const baseCss = readFileSync(`${nodeProcess.cwd()}/src/styles/base.css`, "utf8");
const tokensCss = readFileSync(
  `${nodeProcess.cwd()}/src/styles/tokens.css`,
  "utf8",
);

type Oklch = [lightness: number, chroma: number, hue: number];

function token(name: string): Oklch {
  const declaration = tokensCss
    .split(";")
    .find((candidate) => candidate.includes(`${name}:`));
  const value = declaration?.match(/oklch\(([^)]+)\)/)?.[1];
  if (!value) {
    throw new Error(`missing ${name}`);
  }
  const channels = value.trim().split(/\s+/).map(Number);
  return [channels[0], channels[1], channels[2]];
}

function relativeLuminance([lightness, chroma, hue]: Oklch) {
  const radians = (hue * Math.PI) / 180;
  const a = chroma * Math.cos(radians);
  const b = chroma * Math.sin(radians);
  const lRoot = lightness + 0.3963377774 * a + 0.2158037573 * b;
  const mRoot = lightness - 0.1055613458 * a - 0.0638541728 * b;
  const sRoot = lightness - 0.0894841775 * a - 1.291485548 * b;
  const l = lRoot ** 3;
  const m = mRoot ** 3;
  const s = sRoot ** 3;
  const channels = [
    4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s,
    -1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s,
    -0.0041960863 * l - 0.7034186147 * m + 1.707614701 * s,
  ].map((channel) => Math.min(1, Math.max(0, channel)));
  return 0.2126 * channels[0] + 0.7152 * channels[1] + 0.0722 * channels[2];
}

function contrastRatio(foreground: Oklch, background: Oklch) {
  const lighter = Math.max(
    relativeLuminance(foreground),
    relativeLuminance(background),
  );
  const darker = Math.min(
    relativeLuminance(foreground),
    relativeLuminance(background),
  );
  return (lighter + 0.05) / (darker + 0.05);
}

describe("authentication accessibility colors", () => {
  it("uses a focus color with at least 3:1 contrast on page and panel surfaces", () => {
    const focus = token("--color-primary");

    expect(contrastRatio(focus, token("--color-bg"))).toBeGreaterThanOrEqual(3);
    expect(
      contrastRatio(focus, token("--color-surface-strong")),
    ).toBeGreaterThanOrEqual(3);
    expect(baseCss).toMatch(
      /\.skip-link:focus-visible[\s\S]*?outline: 3px solid var\(--color-primary\)/,
    );
    expect(baseCss).toMatch(
      /\.button:focus-visible,[\s\S]*?outline: 3px solid var\(--color-primary\)/,
    );
    expect(baseCss).toMatch(
      /\.field input:focus-visible[\s\S]*?border-color: var\(--color-primary\)/,
    );
  });

  it("uses danger text with at least 4.5:1 contrast on its error surface", () => {
    const dangerText = token("--color-danger-text");

    expect(
      contrastRatio(dangerText, token("--color-surface-strong")),
    ).toBeGreaterThanOrEqual(4.5);
    expect(baseCss).toMatch(
      /\.form-error[\s\S]*?color: var\(--color-danger-text\)/,
    );
  });
});
