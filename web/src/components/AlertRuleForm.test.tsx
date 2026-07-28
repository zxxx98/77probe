import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { AlertRuleForm } from "./AlertRuleForm";

describe("AlertRuleForm", () => {
  it("forces offline rules to zero duration", () => {
    render(<AlertRuleForm servers={[{ id: 1, name: "home-lab", enabled: true, agentVersion: "", createdAt: "", updatedAt: "" }]} saving={false} onSave={vi.fn().mockResolvedValue(undefined)} />);
    fireEvent.change(screen.getByLabelText("指标"), { target: { value: "offline" } });
    expect(screen.getByLabelText("持续时间（秒）")).toHaveValue(0);
    expect(screen.getByLabelText("持续时间（秒）")).toBeDisabled();
  });
});
