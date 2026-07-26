import { act, fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { AuthGate } from "./AuthGate";

const fetchMock = vi.mocked(fetch);

function renderGate() {
  render(
    <AuthGate>
      <div>private app</div>
    </AuthGate>,
  );
}

function jsonError(status: number, message: string) {
  return Response.json({ error: message }, { status });
}

function deferredResponse() {
  let resolve!: (response: Response) => void;
  const promise = new Promise<Response>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

function fillCredentials(username: string, password: string) {
  fireEvent.change(screen.getByLabelText("用户名"), {
    target: { value: username },
  });
  fireEvent.change(screen.getByLabelText("密码"), {
    target: { value: password },
  });
}

describe("AuthGate", () => {
  it("shows first-run setup when no administrator exists", async () => {
    fetchMock.mockResolvedValueOnce(Response.json({ setupRequired: true }));

    renderGate();

    expect(
      await screen.findByRole("heading", { name: "创建管理员" }),
    ).toBeInTheDocument();
    expect(screen.queryByText("private app")).not.toBeInTheDocument();
  });

  it("checks setup before me and renders authenticated children", async () => {
    fetchMock
      .mockResolvedValueOnce(Response.json({ setupRequired: false }))
      .mockResolvedValueOnce(Response.json({ id: 1, username: "xiaodi" }));

    renderGate();

    expect(await screen.findByText("private app")).toBeInTheDocument();
    expect(fetchMock).toHaveBeenNthCalledWith(1, "/api/setup/status", {
      headers: { "Content-Type": "application/json" },
      credentials: "same-origin",
    });
    expect(fetchMock).toHaveBeenNthCalledWith(2, "/api/me", {
      headers: { "Content-Type": "application/json" },
      credentials: "same-origin",
    });
  });

  it("renders login when me returns 401", async () => {
    fetchMock
      .mockResolvedValueOnce(Response.json({ setupRequired: false }))
      .mockResolvedValueOnce(jsonError(401, "unauthenticated"));

    renderGate();

    expect(
      await screen.findByRole("heading", { name: "欢迎回来" }),
    ).toBeInTheDocument();
    expect(screen.queryByText("private app")).not.toBeInTheDocument();
  });

  it("shows a non-401 load error and retries setup before me", async () => {
    fetchMock
      .mockResolvedValueOnce(Response.json({ setupRequired: false }))
      .mockResolvedValueOnce(jsonError(503, "状态服务暂时不可用"));

    renderGate();

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "状态服务暂时不可用",
    );

    fetchMock
      .mockResolvedValueOnce(Response.json({ setupRequired: false }))
      .mockResolvedValueOnce(Response.json({ id: 1, username: "xiaodi" }));
    fireEvent.click(screen.getByRole("button", { name: "重试" }));

    expect(await screen.findByText("private app")).toBeInTheDocument();
    expect(fetchMock.mock.calls.map(([path]) => path)).toEqual([
      "/api/setup/status",
      "/api/me",
      "/api/setup/status",
      "/api/me",
    ]);
  });

  it("submits setup JSON with same-origin credentials and disables the form", async () => {
    const setupResponse = deferredResponse();
    fetchMock
      .mockResolvedValueOnce(Response.json({ setupRequired: true }))
      .mockImplementationOnce(() => setupResponse.promise);

    renderGate();
    await screen.findByRole("heading", { name: "创建管理员" });
    fillCredentials("xiaodi", "correct horse battery staple");
    fireEvent.click(screen.getByRole("button", { name: "创建管理员" }));

    expect(screen.getByRole("button", { name: "正在创建…" })).toBeDisabled();
    expect(screen.getByLabelText("用户名")).toBeDisabled();
    expect(screen.getByLabelText("密码")).toBeDisabled();
    expect(fetchMock).toHaveBeenNthCalledWith(2, "/api/setup", {
      method: "POST",
      body: JSON.stringify({
        username: "xiaodi",
        password: "correct horse battery staple",
      }),
      headers: { "Content-Type": "application/json" },
      credentials: "same-origin",
    });

    await act(async () => {
      setupResponse.resolve(Response.json({ id: 1, username: "xiaodi" }));
    });

    expect(
      await screen.findByRole("heading", { name: "欢迎回来" }),
    ).toBeInTheDocument();
    expect(screen.getByText("管理员已创建，现在可以登录了。")).toBeInTheDocument();
  });

  it("shows a setup server error and re-enables submission", async () => {
    fetchMock
      .mockResolvedValueOnce(Response.json({ setupRequired: true }))
      .mockResolvedValueOnce(jsonError(409, "管理员已经存在"));

    renderGate();
    await screen.findByRole("heading", { name: "创建管理员" });
    fillCredentials("xiaodi", "correct horse battery staple");
    fireEvent.click(screen.getByRole("button", { name: "创建管理员" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("管理员已经存在");
    expect(screen.getByRole("button", { name: "创建管理员" })).toBeEnabled();
    expect(
      screen.queryByRole("heading", { name: "欢迎回来" }),
    ).not.toBeInTheDocument();
  });

  it("submits login JSON with same-origin credentials and disables the form", async () => {
    const loginResponse = deferredResponse();
    fetchMock
      .mockResolvedValueOnce(Response.json({ setupRequired: false }))
      .mockResolvedValueOnce(jsonError(401, "unauthenticated"))
      .mockImplementationOnce(() => loginResponse.promise);

    renderGate();
    await screen.findByRole("heading", { name: "欢迎回来" });
    fillCredentials("xiaodi", "correct horse battery staple");
    fireEvent.click(screen.getByRole("button", { name: "登录" }));

    expect(screen.getByRole("button", { name: "正在登录…" })).toBeDisabled();
    expect(screen.getByLabelText("用户名")).toBeDisabled();
    expect(screen.getByLabelText("密码")).toBeDisabled();
    expect(fetchMock).toHaveBeenNthCalledWith(3, "/api/login", {
      method: "POST",
      body: JSON.stringify({
        username: "xiaodi",
        password: "correct horse battery staple",
      }),
      headers: { "Content-Type": "application/json" },
      credentials: "same-origin",
    });

    await act(async () => {
      loginResponse.resolve(Response.json({ ok: true }));
    });

    expect(await screen.findByText("private app")).toBeInTheDocument();
  });

  it("shows a login server error and re-enables submission", async () => {
    fetchMock
      .mockResolvedValueOnce(Response.json({ setupRequired: false }))
      .mockResolvedValueOnce(jsonError(401, "unauthenticated"))
      .mockResolvedValueOnce(jsonError(401, "用户名或密码不正确"));

    renderGate();
    await screen.findByRole("heading", { name: "欢迎回来" });
    fillCredentials("xiaodi", "wrong password");
    fireEvent.click(screen.getByRole("button", { name: "登录" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "用户名或密码不正确",
    );
    expect(screen.getByRole("button", { name: "登录" })).toBeEnabled();
    expect(screen.queryByText("private app")).not.toBeInTheDocument();
  });
});
