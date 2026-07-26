import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ApiError } from "../api/client";
import { serverApi, type ServerRecord } from "../servers/api";
import { ServersPage } from "./ServersPage";

vi.mock("../servers/api", () => ({
  serverApi: {
    list: vi.fn(),
    create: vi.fn(),
    update: vi.fn(),
    remove: vi.fn(),
    rotateToken: vi.fn(),
  },
}));

const listServers = vi.mocked(serverApi.list);
const createServer = vi.mocked(serverApi.create);
const updateServer = vi.mocked(serverApi.update);
const deleteServer = vi.mocked(serverApi.remove);
const rotateToken = vi.mocked(serverApi.rotateToken);

const serverFixture: ServerRecord = {
  id: 7,
  name: "home-lab",
  enabled: true,
  agentVersion: "0.1.0",
  createdAt: "2026-07-26T04:00:00Z",
  updatedAt: "2026-07-26T04:00:00Z",
};

function renderPage() {
  return render(<ServersPage />);
}

function deferredServers() {
  let resolve!: (servers: ServerRecord[]) => void;
  const promise = new Promise<ServerRecord[]>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

async function loadRow() {
  renderPage();
  return screen.findByTestId("managed-server-7");
}

describe("ServersPage", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    listServers.mockReset();
    createServer.mockReset();
    updateServer.mockReset();
    deleteServer.mockReset();
    rotateToken.mockReset();
    listServers.mockResolvedValue([serverFixture]);
  });

  it("lists servers and reveals server creation inline", async () => {
    await loadRow();

    expect(screen.getByText("home-lab")).toBeInTheDocument();
    expect(screen.getByText("Agent 0.1.0")).toBeInTheDocument();
    expect(screen.queryByLabelText("服务器名称")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "添加服务器" }));

    expect(screen.getByLabelText("服务器名称")).toHaveFocus();
    expect(screen.getByRole("button", { name: "创建" })).toBeDisabled();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("disables creation while the server list is pending", async () => {
    const pending = deferredServers();
    listServers.mockReturnValueOnce(pending.promise);
    renderPage();

    expect(screen.getByRole("button", { name: "添加服务器" })).toBeDisabled();

    pending.resolve([]);

    await waitFor(() =>
      expect(screen.getByRole("button", { name: "添加服务器" })).toBeEnabled(),
    );
  });

  it("shows a created token once and clears it from the DOM without persistence", async () => {
    listServers.mockResolvedValueOnce([]);
    createServer.mockResolvedValueOnce({
      server: serverFixture,
      token: "tp_secret",
    });
    const storageWrite = vi.spyOn(Storage.prototype, "setItem");
    renderPage();
    await screen.findByText("还没有添加服务器");

    fireEvent.click(screen.getByRole("button", { name: "添加服务器" }));
    fireEvent.change(screen.getByLabelText("服务器名称"), {
      target: { value: "home-lab" },
    });
    fireEvent.click(screen.getByRole("button", { name: "创建" }));

    expect(createServer).toHaveBeenCalledWith("home-lab");
    expect(await screen.findByText("tp_secret")).toBeInTheDocument();
    expect(screen.getByText("home-lab")).toBeInTheDocument();
    expect(storageWrite).not.toHaveBeenCalled();
    expect(window.location.href).not.toContain("tp_secret");

    fireEvent.click(screen.getByRole("button", { name: "我已保存 Token" }));

    expect(screen.queryByText("tp_secret")).not.toBeInTheDocument();
  });

  it("renames a server inline", async () => {
    updateServer.mockResolvedValueOnce({ ...serverFixture, name: "office-lab" });
    await loadRow();

    fireEvent.click(screen.getByRole("button", { name: "重命名 home-lab" }));
    const input = screen.getByRole("textbox", { name: "重命名 home-lab" });
    expect(input).toHaveFocus();
    fireEvent.change(input, { target: { value: "office-lab" } });
    fireEvent.click(
      screen.getByRole("button", { name: "保存 home-lab 的新名称" }),
    );

    expect(updateServer).toHaveBeenCalledWith(7, { name: "office-lab" });
    expect(await screen.findByText("office-lab")).toBeInTheDocument();
    expect(screen.queryByText("home-lab")).not.toBeInTheDocument();
  });

  it("updates enable and disable state in place after each successful request", async () => {
    updateServer
      .mockResolvedValueOnce({ ...serverFixture, enabled: false })
      .mockResolvedValueOnce({ ...serverFixture, enabled: true });
    await loadRow();

    fireEvent.click(screen.getByRole("button", { name: "停用 home-lab" }));

    expect(await screen.findByText("已停用")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "启用 home-lab" })).toBeEnabled();
    fireEvent.click(screen.getByRole("button", { name: "启用 home-lab" }));

    expect(await screen.findByText("已启用")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "停用 home-lab" })).toBeEnabled();
    expect(updateServer).toHaveBeenNthCalledWith(1, 7, { enabled: false });
    expect(updateServer).toHaveBeenNthCalledWith(2, 7, { enabled: true });
    expect(listServers).toHaveBeenCalledTimes(1);
  });

  it("requires an inline confirmation before deleting and explains the cascade", async () => {
    deleteServer.mockResolvedValueOnce();
    await loadRow();

    fireEvent.click(screen.getByRole("button", { name: "删除 home-lab" }));

    expect(
      screen.getByText("删除后该 Agent Token 将立即失效。"),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        "该服务器的实时数据和历史数据会由数据库级联一并删除，无法恢复。",
      ),
    ).toBeInTheDocument();
    expect(deleteServer).not.toHaveBeenCalled();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();

    fireEvent.click(
      screen.getByRole("button", { name: "确认删除 home-lab" }),
    );

    await waitFor(() => expect(deleteServer).toHaveBeenCalledWith(7));
    expect(screen.queryByTestId("managed-server-7")).not.toBeInTheDocument();
  });

  it("does not clear another server's one-time token when duplicate names exist", async () => {
    const duplicate = { ...serverFixture, id: 8 };
    createServer.mockResolvedValueOnce({ server: duplicate, token: "tp_duplicate" });
    deleteServer.mockResolvedValueOnce();
    await loadRow();
    fireEvent.click(screen.getByRole("button", { name: "添加服务器" }));
    fireEvent.change(screen.getByLabelText("服务器名称"), {
      target: { value: "home-lab" },
    });
    fireEvent.click(screen.getByRole("button", { name: "创建" }));
    expect(await screen.findByText("tp_duplicate")).toBeInTheDocument();

    const originalRow = screen.getByTestId("managed-server-7");
    fireEvent.click(
      within(originalRow).getByRole("button", { name: "删除 home-lab" }),
    );
    fireEvent.click(
      within(originalRow).getByRole("button", { name: "确认删除 home-lab" }),
    );

    await waitFor(() => expect(deleteServer).toHaveBeenCalledWith(7));
    expect(screen.getByText("tp_duplicate")).toBeInTheDocument();
  });

  it("requires inline confirmation before rotation and shows the replacement token", async () => {
    rotateToken.mockResolvedValueOnce({
      server: { ...serverFixture, updatedAt: "2026-07-26T05:00:00Z" },
      token: "tp_rotated",
    });
    await loadRow();

    fireEvent.click(
      screen.getByRole("button", { name: "重新生成 home-lab 的 Token" }),
    );

    expect(
      screen.getByText("重新生成后，当前 Agent Token 将立即失效。"),
    ).toBeInTheDocument();
    expect(rotateToken).not.toHaveBeenCalled();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();

    fireEvent.click(
      screen.getByRole("button", { name: "确认重新生成 home-lab 的 Token" }),
    );

    await waitFor(() => expect(rotateToken).toHaveBeenCalledWith(7));
    expect(await screen.findByText("tp_rotated")).toBeInTheDocument();
  });

  it("shows the server message when the initial list request fails", async () => {
    listServers.mockRejectedValueOnce(new ApiError(503, "服务器列表暂时不可用"));
    renderPage();

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "服务器列表暂时不可用",
    );
    expect(screen.getByRole("button", { name: "重新加载" })).toBeEnabled();
  });

  it("keeps creation input available and shows a friendly create failure", async () => {
    listServers.mockResolvedValueOnce([]);
    createServer.mockRejectedValueOnce(new Error("network details"));
    renderPage();
    await screen.findByText("还没有添加服务器");
    fireEvent.click(screen.getByRole("button", { name: "添加服务器" }));
    fireEvent.change(screen.getByLabelText("服务器名称"), {
      target: { value: "home-lab" },
    });

    fireEvent.click(screen.getByRole("button", { name: "创建" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "暂时无法创建服务器，请稍后重试。",
    );
    expect(screen.getByLabelText("服务器名称")).toHaveValue("home-lab");
    expect(screen.getByRole("button", { name: "创建" })).toBeEnabled();
  });

  it.each([
    {
      label: "rename",
      start: () => {
        updateServer.mockRejectedValueOnce(new ApiError(400, "名称不能使用"));
        fireEvent.click(screen.getByRole("button", { name: "重命名 home-lab" }));
        fireEvent.change(screen.getByRole("textbox", { name: "重命名 home-lab" }), {
          target: { value: "bad-name" },
        });
        fireEvent.click(
          screen.getByRole("button", { name: "保存 home-lab 的新名称" }),
        );
      },
      message: "名称不能使用",
    },
    {
      label: "toggle",
      start: () => {
        updateServer.mockRejectedValueOnce(new Error("network details"));
        fireEvent.click(screen.getByRole("button", { name: "停用 home-lab" }));
      },
      message: "暂时无法停用服务器，请稍后重试。",
    },
    {
      label: "delete",
      start: () => {
        deleteServer.mockRejectedValueOnce(new ApiError(500, "删除没有成功"));
        fireEvent.click(screen.getByRole("button", { name: "删除 home-lab" }));
        fireEvent.click(
          screen.getByRole("button", { name: "确认删除 home-lab" }),
        );
      },
      message: "删除没有成功",
    },
    {
      label: "rotate",
      start: () => {
        rotateToken.mockRejectedValueOnce(new Error("network details"));
        fireEvent.click(
          screen.getByRole("button", { name: "重新生成 home-lab 的 Token" }),
        );
        fireEvent.click(
          screen.getByRole("button", {
            name: "确认重新生成 home-lab 的 Token",
          }),
        );
      },
      message: "暂时无法重新生成 Token，请稍后重试。",
    },
  ])("shows an actionable $label failure and keeps the row", async ({ start, message }) => {
    await loadRow();

    start();

    expect(await screen.findByRole("alert")).toHaveTextContent(message);
    expect(screen.getByTestId("managed-server-7")).toBeInTheDocument();
  });
});
