import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { RangeTabs } from "./RangeTabs";

describe("RangeTabs", () => {
  it("renders native selectable range buttons with one pressed value", () => {
    const onChange = vi.fn();
    render(<RangeTabs value="7d" onChange={onChange} />);

    expect(screen.getByRole("group", { name: "时间范围" })).toBeInTheDocument();
    expect(
      screen.getAllByRole("button").map((button) => button.textContent),
    ).toEqual(["实时", "1天", "7天", "30天"]);
    expect(screen.getByRole("button", { name: "7天" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    expect(screen.getByRole("button", { name: "1天" })).not.toBeDisabled();

    const realTime = screen.getByRole("button", { name: "实时" });
    realTime.focus();
    expect(realTime).toHaveFocus();
    fireEvent.click(realTime);

    expect(onChange).toHaveBeenCalledWith(null);
  });
});
