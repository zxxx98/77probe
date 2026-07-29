import { useRef, useState } from "react";
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
import { ServersPage, type OneTimeToken } from "./ServersPage";

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
const writeText = vi.fn<(value: string) => Promise<void>>();

const uninstallCommand = `sudo systemctl disable --now tinyprobe-agent.service 2>/dev/null || true
sudo rm -f /etc/systemd/system/tinyprobe-agent.service
sudo rm -f /etc/tinyprobe-agent.env
sudo rm -f /usr/local/bin/tinyprobe-agent
sudo systemctl daemon-reload
sudo systemctl reset-failed tinyprobe-agent.service 2>/dev/null || true`;

const serverFixture: ServerRecord = {
  id: 7,
  name: "home-lab",
  enabled: true,
  agentVersion: "0.1.0",
  createdAt: "2026-07-26T04:00:00Z",
  updatedAt: "2026-07-26T04:00:00Z",
};

function renderPage() {
  function TestServersPage() {
    const [oneTimeToken, setOneTimeToken] = useState<OneTimeToken | null>(null);
    const [tokenRequestPending, setTokenRequestPending] = useState(false);
    const [tokenRequestServerId, setTokenRequestServerId] = useState<number | null>(null);
    const tokenRef = useRef<OneTimeToken | null>(null);
    const tokenLock = useRef(false);

    return (
      <ServersPage
        oneTimeToken={oneTimeToken}
        tokenRequestPending={tokenRequestPending}
        tokenRequestServerId={tokenRequestServerId}
        onTokenRequestStarted={(serverId) => {
          if (tokenLock.current) {
            return false;
          }
          tokenLock.current = true;
          setTokenRequestPending(true);
          setTokenRequestServerId(serverId);
          return true;
        }}
        onTokenRequestFailed={() => {
          setTokenRequestPending(false);
          setTokenRequestServerId(null);
          tokenLock.current = tokenRef.current !== null;
        }}
        onTokenPublished={(token) => {
          tokenRef.current = token;
          tokenLock.current = true;
          setOneTimeToken(token);
          setTokenRequestPending(false);
          setTokenRequestServerId(null);
        }}
        onTokenCleared={() => {
          tokenRef.current = null;
          tokenLock.current = false;
          setOneTimeToken(null);
        }}
        onTokenServerDeleted={(serverId) => {
          setOneTimeToken((current) => {
            if (current?.serverId !== serverId) {
              return current;
            }
            tokenRef.current = null;
            tokenLock.current = false;
            return null;
          });
        }}
        onTokenServerRenamed={(serverId, serverName) => {
          setOneTimeToken((current) => {
            if (current?.serverId !== serverId) {
              return current;
            }
            const next = { ...current, serverName };
            tokenRef.current = next;
            return next;
          });
        }}
      />
    );
  }

  return render(<TestServersPage />);
}

function deferredServers() {
  let resolve!: (servers: ServerRecord[]) => void;
  const promise = new Promise<ServerRecord[]>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((resolvePromise) => {
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
    writeText.mockReset();
    writeText.mockResolvedValue();
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText },
    });
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
    expect(
      screen.getByRole("heading", { name: "安装 home-lab 的 Agent" }),
    ).toHaveFocus();
    expect(screen.getByText("home-lab")).toBeInTheDocument();
    expect(storageWrite).not.toHaveBeenCalled();
    expect(window.location.href).not.toContain("tp_secret");

    fireEvent.click(screen.getByRole("button", { name: "我已保存 Token" }));

    expect(screen.queryByText("tp_secret")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "添加服务器" })).toHaveFocus();
  });

  it("blocks create and rotation while a token request or unsaved token exists", async () => {
    const secondServer = { ...serverFixture, id: 8, name: "office-lab" };
    const pendingCreate = deferred<{
      server: ServerRecord;
      token: string;
    }>();
    listServers.mockResolvedValueOnce([serverFixture, secondServer]);
    createServer.mockReturnValueOnce(pendingCreate.promise);
    await loadRow();

    fireEvent.click(screen.getByRole("button", { name: "添加服务器" }));
    fireEvent.change(screen.getByLabelText("服务器名称"), {
      target: { value: "new-lab" },
    });
    fireEvent.click(screen.getByRole("button", { name: "创建" }));

    expect(
      screen.getByRole("button", { name: "重新生成 home-lab 的 Token" }),
    ).toBeDisabled();
    expect(
      screen.getByRole("button", { name: "重新生成 office-lab 的 Token" }),
    ).toBeDisabled();

    pendingCreate.resolve({
      server: { ...serverFixture, id: 9, name: "new-lab" },
      token: "tp_pending",
    });
    expect(await screen.findByText("tp_pending")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "添加服务器" })).toBeDisabled();
    expect(
      screen.getByRole("button", { name: "重新生成 home-lab 的 Token" }),
    ).toBeDisabled();

    fireEvent.click(screen.getByRole("button", { name: "我已保存 Token" }));
    expect(screen.getByRole("button", { name: "添加服务器" })).toBeEnabled();
    expect(
      screen.getByRole("button", { name: "重新生成 home-lab 的 Token" }),
    ).toBeEnabled();
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

  it("reveals and copies the idempotent Agent uninstall command", async () => {
    await loadRow();

    const action = screen.getByRole("button", {
      name: "卸载 home-lab 的 Agent",
    });
    expect(action).toHaveAttribute("aria-expanded", "false");

    fireEvent.click(action);

    expect(action).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByTestId("uninstall-command-7").textContent).toBe(
      uninstallCommand,
    );
    fireEvent.click(screen.getByRole("button", { name: "复制卸载命令" }));
    expect(writeText).toHaveBeenCalledWith(uninstallCommand);
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
    expect(
      screen.getByRole("heading", { name: "安装 home-lab 的 Agent" }),
    ).toHaveFocus();

    fireEvent.click(screen.getByRole("button", { name: "我已保存 Token" }));
    expect(
      screen.getByRole("button", { name: "重新生成 home-lab 的 Token" }),
    ).toHaveFocus();
  });

  it("does not let a delayed deletion clear a newer token from another server", async () => {
    const secondServer = { ...serverFixture, id: 8, name: "office-lab" };
    const pendingDelete = deferred<void>();
    listServers.mockResolvedValueOnce([serverFixture, secondServer]);
    rotateToken
      .mockResolvedValueOnce({ server: serverFixture, token: "tp_first" })
      .mockResolvedValueOnce({ server: secondServer, token: "tp_second" });
    deleteServer.mockReturnValueOnce(pendingDelete.promise);
    await loadRow();

    fireEvent.click(
      screen.getByRole("button", { name: "重新生成 home-lab 的 Token" }),
    );
    fireEvent.click(
      screen.getByRole("button", { name: "确认重新生成 home-lab 的 Token" }),
    );
    expect(await screen.findByText("tp_first")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "删除 home-lab" }));
    fireEvent.click(screen.getByRole("button", { name: "确认删除 home-lab" }));
    fireEvent.click(screen.getByRole("button", { name: "我已保存 Token" }));

    fireEvent.click(
      screen.getByRole("button", { name: "重新生成 office-lab 的 Token" }),
    );
    fireEvent.click(
      screen.getByRole("button", { name: "确认重新生成 office-lab 的 Token" }),
    );
    expect(await screen.findByText("tp_second")).toBeInTheDocument();

    pendingDelete.resolve();
    await waitFor(() =>
      expect(screen.queryByTestId("managed-server-7")).not.toBeInTheDocument(),
    );
    expect(screen.getByText("tp_second")).toBeInTheDocument();
  });

  it("moves focus to a surviving row after deletion", async () => {
    const secondServer = { ...serverFixture, id: 8, name: "office-lab" };
    listServers.mockResolvedValueOnce([serverFixture, secondServer]);
    deleteServer.mockResolvedValueOnce();
    await loadRow();

    fireEvent.click(screen.getByRole("button", { name: "删除 home-lab" }));
    fireEvent.click(screen.getByRole("button", { name: "确认删除 home-lab" }));

    await waitFor(() =>
      expect(screen.queryByTestId("managed-server-7")).not.toBeInTheDocument(),
    );
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "重命名 office-lab" }),
      ).toHaveFocus(),
    );
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
