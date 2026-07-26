import { describe, expect, it, vi } from "vitest";

import { api, ApiError, apiErrorMessage } from "./client";

const fetchMock = vi.mocked(fetch);

async function rejectedApiError(response: Response) {
  fetchMock.mockResolvedValueOnce(response);
  return api.getMe().catch((error: unknown) => error);
}

describe("API client responses", () => {
  it("uses a validated non-empty JSON error message", async () => {
    const error = await rejectedApiError(
      Response.json({ error: "服务器暂时不可用" }, { status: 503 }),
    );

    expect(error).toBeInstanceOf(ApiError);
    expect(apiErrorMessage(error, "友好提示")).toBe("服务器暂时不可用");
  });

  it.each([
    ["plain text", "upstream exploded"],
    ["HTML", "<html><body>proxy error</body></html>"],
    ["malformed JSON", '{"error":'],
    ["empty body", ""],
    ["empty JSON error", '{"error":"   "}'],
    ["non-string JSON error", '{"error":42}'],
  ])("uses the fixed friendly fallback for %s errors", async (_label, body) => {
    const error = await rejectedApiError(
      new Response(body, { status: 502, headers: { "Content-Type": "text/plain" } }),
    );

    expect(error).toBeInstanceOf(ApiError);
    expect(apiErrorMessage(error, "暂时无法完成请求，请稍后重试。")).toBe(
      "暂时无法完成请求，请稍后重试。",
    );
    expect((error as Error).message).toBe("请求失败，请稍后重试。");
  });

  it("returns normal JSON responses", async () => {
    fetchMock.mockResolvedValueOnce(
      Response.json({ id: 1, username: "xiaodi" }),
    );

    await expect(api.getMe()).resolves.toEqual({ id: 1, username: "xiaodi" });
  });

  it("returns undefined for 204 responses without parsing JSON", async () => {
    fetchMock.mockResolvedValueOnce(new Response(null, { status: 204 }));

    await expect(api.logout()).resolves.toBeUndefined();
  });
});
