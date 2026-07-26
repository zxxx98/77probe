import { useEffect, useState, type FormEvent } from "react";

import { apiErrorMessage } from "../api/client";
import { ServerForm } from "../components/ServerForm";
import { ServerInstallPanel } from "../components/ServerInstallPanel";
import { serverApi, type ServerRecord } from "../servers/api";

type Confirmation = "delete" | "rotate";
type BusyAction = "rename" | "toggle" | Confirmation;

interface BusyState {
  serverId: number;
  action: BusyAction;
}

interface TokenState {
  serverId: number;
  serverName: string;
  token: string;
}

function replaceServer(servers: ServerRecord[], next: ServerRecord) {
  return servers.map((server) => (server.id === next.id ? next : server));
}

export function ServersPage() {
  const [servers, setServers] = useState<ServerRecord[]>([]);
  const [loadState, setLoadState] = useState<"loading" | "success" | "error">(
    "loading",
  );
  const [loadError, setLoadError] = useState("");
  const [showCreate, setShowCreate] = useState(false);
  const [creating, setCreating] = useState(false);
  const [editingId, setEditingId] = useState<number | null>(null);
  const [editName, setEditName] = useState("");
  const [confirmation, setConfirmation] = useState<{
    serverId: number;
    action: Confirmation;
  } | null>(null);
  const [busy, setBusy] = useState<BusyState | null>(null);
  const [operationError, setOperationError] = useState("");
  const [tokenState, setTokenState] = useState<TokenState | null>(null);

  const loadServers = async () => {
    setLoadState("loading");
    setLoadError("");
    try {
      setServers(await serverApi.list());
      setLoadState("success");
    } catch (error) {
      setLoadError(
        apiErrorMessage(error, "暂时无法读取服务器列表，请稍后重试。"),
      );
      setLoadState("error");
    }
  };

  useEffect(() => {
    let active = true;
    const load = async () => {
      try {
        const result = await serverApi.list();
        if (active) {
          setServers(result);
          setLoadState("success");
        }
      } catch (error) {
        if (active) {
          setLoadError(
            apiErrorMessage(error, "暂时无法读取服务器列表，请稍后重试。"),
          );
          setLoadState("error");
        }
      }
    };
    void load();
    return () => {
      active = false;
    };
  }, []);

  const create = async (name: string) => {
    setCreating(true);
    setOperationError("");
    try {
      const result = await serverApi.create(name);
      setServers((current) => [...current, result.server]);
      setTokenState({
        serverId: result.server.id,
        serverName: result.server.name,
        token: result.token,
      });
      setShowCreate(false);
    } catch (error) {
      setOperationError(
        apiErrorMessage(error, "暂时无法创建服务器，请稍后重试。"),
      );
    } finally {
      setCreating(false);
    }
  };

  const beginRename = (server: ServerRecord) => {
    setEditingId(server.id);
    setEditName(server.name);
    setConfirmation(null);
    setOperationError("");
  };

  const rename = async (event: FormEvent<HTMLFormElement>, server: ServerRecord) => {
    event.preventDefault();
    const name = editName.trim();
    if (name === "" || name === server.name) {
      return;
    }
    setBusy({ serverId: server.id, action: "rename" });
    setOperationError("");
    try {
      const updated = await serverApi.update(server.id, { name });
      setServers((current) => replaceServer(current, updated));
      setTokenState((current) =>
        current?.serverId === updated.id
          ? { ...current, serverName: updated.name }
          : current,
      );
      setEditingId(null);
    } catch (error) {
      setOperationError(
        apiErrorMessage(error, "暂时无法重命名服务器，请稍后重试。"),
      );
    } finally {
      setBusy(null);
    }
  };

  const toggle = async (server: ServerRecord) => {
    const enabled = !server.enabled;
    setBusy({ serverId: server.id, action: "toggle" });
    setOperationError("");
    try {
      const updated = await serverApi.update(server.id, { enabled });
      setServers((current) => replaceServer(current, updated));
    } catch (error) {
      setOperationError(
        apiErrorMessage(
          error,
          `暂时无法${enabled ? "启用" : "停用"}服务器，请稍后重试。`,
        ),
      );
    } finally {
      setBusy(null);
    }
  };

  const remove = async (server: ServerRecord) => {
    setBusy({ serverId: server.id, action: "delete" });
    setOperationError("");
    try {
      await serverApi.remove(server.id);
      setServers((current) => current.filter((item) => item.id !== server.id));
      setConfirmation(null);
      if (tokenState?.serverId === server.id) {
        setTokenState(null);
      }
    } catch (error) {
      setOperationError(
        apiErrorMessage(error, "暂时无法删除服务器，请稍后重试。"),
      );
    } finally {
      setBusy(null);
    }
  };

  const rotate = async (server: ServerRecord) => {
    setBusy({ serverId: server.id, action: "rotate" });
    setOperationError("");
    try {
      const result = await serverApi.rotateToken(server.id);
      setServers((current) => replaceServer(current, result.server));
      setTokenState({
        serverId: result.server.id,
        serverName: result.server.name,
        token: result.token,
      });
      setConfirmation(null);
    } catch (error) {
      setOperationError(
        apiErrorMessage(error, "暂时无法重新生成 Token，请稍后重试。"),
      );
    } finally {
      setBusy(null);
    }
  };

  return (
    <main className="dashboard-content management-content" id="main-content">
      <section className="management-heading" aria-labelledby="servers-title">
        <div>
          <p className="calm-status">
            <span className="status-dot" aria-hidden="true" />
            每台服务器使用独立的 Agent Token
          </p>
          <h1 id="servers-title">服务器管理</h1>
          <p>
            添加需要照看的 Linux 服务器，并在这里管理名称、启停状态和安装 Token。
          </p>
        </div>
        <button
          className="button button-primary management-add-button"
          type="button"
          aria-expanded={showCreate}
          disabled={creating || loadState === "loading"}
          onClick={() => {
            setShowCreate((current) => !current);
            setOperationError("");
          }}
        >
          {showCreate ? "收起添加" : "添加服务器"}
        </button>
      </section>

      {showCreate ? (
        <section className="server-create-panel" aria-label="添加服务器">
          <ServerForm
            disabled={creating || loadState === "loading"}
            submitting={creating}
            onSubmit={create}
            onCancel={() => setShowCreate(false)}
          />
        </section>
      ) : null}

      {operationError ? (
        <p className="management-error" role="alert">
          {operationError}
        </p>
      ) : null}

      {tokenState ? (
        <ServerInstallPanel
          serverName={tokenState.serverName}
          token={tokenState.token}
          onTokenCleared={() => setTokenState(null)}
        />
      ) : null}

      {loadState === "loading" ? (
        <section className="management-loading" role="status">
          <span className="sr-only">正在读取服务器列表…</span>
          <span aria-hidden="true" />
          <span aria-hidden="true" />
        </section>
      ) : null}

      {loadState === "error" ? (
        <section className="dashboard-state" role="alert">
          <div>
            <h2>服务器列表没有加载成功</h2>
            <p>{loadError}</p>
          </div>
          <button
            className="button button-secondary"
            type="button"
            onClick={() => void loadServers()}
          >
            重新加载
          </button>
        </section>
      ) : null}

      {loadState === "success" && servers.length === 0 ? (
        <section className="dashboard-state dashboard-empty">
          <div>
            <h2>还没有添加服务器</h2>
            <p>从“添加服务器”开始，创建后会立即拿到一次性安装 Token。</p>
          </div>
        </section>
      ) : null}

      {loadState === "success" && servers.length > 0 ? (
        <section className="managed-server-section" aria-labelledby="managed-list-title">
          <div className="server-section-heading">
            <div>
              <h2 id="managed-list-title">已经添加的服务器</h2>
              <p>{servers.length} 台服务器；停用后 Agent 将不能继续上报。</p>
            </div>
          </div>
          <div className="managed-server-list">
            {servers.map((server) => {
              const rowBusy = busy?.serverId === server.id;
              const renaming = editingId === server.id;
              const confirming = confirmation?.serverId === server.id;
              return (
                <article
                  className="managed-server-row"
                  data-testid={`managed-server-${server.id}`}
                  key={server.id}
                >
                  <div className="managed-server-main">
                    <span
                      className={`server-status-dot server-status-dot--${server.enabled ? "online" : "offline"}`}
                      aria-hidden="true"
                    />
                    <div>
                      {renaming ? (
                        <form
                          className="rename-form"
                          onSubmit={(event) => void rename(event, server)}
                        >
                          <label className="sr-only" htmlFor={`rename-${server.id}`}>
                            重命名 {server.name}
                          </label>
                          <input
                            id={`rename-${server.id}`}
                            autoFocus
                            disabled={rowBusy}
                            maxLength={120}
                            value={editName}
                            onChange={(event) => setEditName(event.target.value)}
                          />
                          <button
                            className="button button-quiet"
                            type="submit"
                            disabled={
                              rowBusy ||
                              editName.trim() === "" ||
                              editName.trim() === server.name
                            }
                            aria-label={`保存 ${server.name} 的新名称`}
                          >
                            保存
                          </button>
                          <button
                            className="button button-quiet"
                            type="button"
                            disabled={rowBusy}
                            onClick={() => setEditingId(null)}
                          >
                            取消
                          </button>
                        </form>
                      ) : (
                        <>
                          <h3>{server.name}</h3>
                          <p>
                            <span>{server.enabled ? "已启用" : "已停用"}</span>
                            <span aria-hidden="true"> · </span>
                            <span>
                              {server.agentVersion
                                ? `Agent ${server.agentVersion}`
                                : "Agent 尚未上报"}
                            </span>
                          </p>
                        </>
                      )}
                    </div>
                  </div>

                  {!renaming ? (
                    <div className="managed-server-actions">
                      <button
                        className="button button-quiet"
                        type="button"
                        disabled={rowBusy}
                        aria-label={`重命名 ${server.name}`}
                        onClick={() => beginRename(server)}
                      >
                        重命名
                      </button>
                      <button
                        className="button button-quiet"
                        type="button"
                        disabled={rowBusy}
                        aria-label={`${server.enabled ? "停用" : "启用"} ${server.name}`}
                        onClick={() => void toggle(server)}
                      >
                        {rowBusy && busy?.action === "toggle"
                          ? "处理中…"
                          : server.enabled
                            ? "停用"
                            : "启用"}
                      </button>
                      <button
                        className="button button-quiet"
                        type="button"
                        disabled={rowBusy}
                        aria-label={`重新生成 ${server.name} 的 Token`}
                        onClick={() => {
                          setConfirmation({ serverId: server.id, action: "rotate" });
                          setOperationError("");
                        }}
                      >
                        重新生成 Token
                      </button>
                      <button
                        className="button button-danger-quiet"
                        type="button"
                        disabled={rowBusy}
                        aria-label={`删除 ${server.name}`}
                        onClick={() => {
                          setConfirmation({ serverId: server.id, action: "delete" });
                          setOperationError("");
                        }}
                      >
                        删除
                      </button>
                    </div>
                  ) : null}

                  {confirming ? (
                    <div
                      className={`inline-confirmation inline-confirmation--${confirmation.action}`}
                      role="group"
                      aria-label={`${confirmation.action === "delete" ? "删除" : "重新生成 Token"}确认`}
                    >
                      <div>
                        <strong>
                          {confirmation.action === "delete"
                            ? "删除后该 Agent Token 将立即失效。"
                            : "重新生成后，当前 Agent Token 将立即失效。"}
                        </strong>
                        <p>
                          {confirmation.action === "delete"
                            ? "该服务器的实时数据和历史数据会由数据库级联一并删除，无法恢复。"
                            : "请在新 Token 显示后立即保存，并更新这台服务器的 Agent 配置。"}
                        </p>
                      </div>
                      <div className="management-actions">
                        <button
                          className={`button ${confirmation.action === "delete" ? "button-danger" : "button-primary"}`}
                          type="button"
                          disabled={rowBusy}
                          aria-label={`确认${confirmation.action === "delete" ? "删除" : "重新生成"} ${server.name}${confirmation.action === "rotate" ? " 的 Token" : ""}`}
                          onClick={() =>
                            void (confirmation.action === "delete"
                              ? remove(server)
                              : rotate(server))
                          }
                        >
                          {rowBusy ? "处理中…" : "确认"}
                        </button>
                        <button
                          className="button button-quiet"
                          type="button"
                          disabled={rowBusy}
                          onClick={() => setConfirmation(null)}
                        >
                          取消
                        </button>
                      </div>
                    </div>
                  ) : null}
                </article>
              );
            })}
          </div>
        </section>
      ) : null}
    </main>
  );
}
