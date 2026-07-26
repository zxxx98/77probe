import { render, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";

import { AuthGate } from "./AuthGate";

it("shows first-run setup when no administrator exists", async () => {
  vi.mocked(fetch).mockResolvedValueOnce(
    Response.json({ setupRequired: true }),
  );

  render(
    <AuthGate>
      <div>private app</div>
    </AuthGate>,
  );

  expect(
    await screen.findByRole("heading", { name: "创建管理员" }),
  ).toBeInTheDocument();
  expect(screen.queryByText("private app")).not.toBeInTheDocument();
});
