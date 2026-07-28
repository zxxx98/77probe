import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { WebhookForm } from "./WebhookForm";

describe("WebhookForm", () => {
  it("preserves a masked authorization header when saving", () => {
    const save = vi.fn().mockResolvedValue(undefined);
    render(<WebhookForm initial={{ url: "https://example.test/hook", enabled: true, headers: { Authorization: "••••••" }, bodyTemplate: `{"status":"{{.Status}}"}` }} saving={false} onSave={save} onTest={vi.fn().mockResolvedValue({ success: true, attempts: [] })} />);
    expect(screen.getByLabelText("请求头值 Authorization")).toHaveValue("••••••");
    fireEvent.click(screen.getByRole("button", { name: "保存 Webhook" }));
    expect(save).toHaveBeenCalledWith(expect.objectContaining({ headers: { Authorization: "••••••" } }));
  });
});
