import { useEffect, useRef, useState, type FormEvent } from "react";

import { apiErrorMessage } from "../api/client";
import { ServerForm } from "../components/ServerForm";
import { ServerInstallPanel } from "../components/ServerInstallPanel";
import { serverApi, type ServerRecord } from "../servers/api";
import { useServerCollection } from "../servers/useServerCollection";

type Confirmation = "delete" | "rotate";
export interface OneTimeToken {
  serverId: number;
  serverName: string;
  token: string;
  returnFocus:
    | { kind: "create" }
    | { kind: "rotate"; serverId: number };
}

interface ServersPageProps {
  oneTimeToken: OneTimeToken | null;
  tokenRequestPending: boolean;
  tokenRequestServerId: number | null;
  onTokenRequestStarted: (serverId: number | null) => boolean;
  onTokenRequestFailed: (message: string) => void;
  onTokenPublished: (token: OneTimeToken) => void;
  onTokenCleared: () => void;
  onTokenServerDeleted: (serverId: number) => void;
  onTokenServerRenamed: (serverId: number, serverName: string) => void;
}

type FocusTarget =
  | { kind: "add" }
  | { kind: "rotate"; serverId: number }
  | { kind: "server"; serverId: number };

export function ServersPage({
  oneTimeToken,
  tokenRequestPending,
  tokenRequestServerId,
  onTokenRequestStarted,
  onTokenRequestFailed,
  onTokenPublished,
  onTokenCleared,
  onTokenServerDeleted,
  onTokenServerRenamed,
}: ServersPageProps) {
  const {
    servers,
    loadState,
    loadError,
    pendingByServer,
    loadServers,
    addServer,
    updateServer,
    removeServer,
  } = useServerCollection(serverApi.list);
  const [showCreate, setShowCreate] = useState(false);
  const [creating, setCreating] = useState(false);
  const [editingId, setEditingId] = useState<number | null>(null);
  const [editName, setEditName] = useState("");
  const [confirmation, setConfirmation] = useState<{
    serverId: number;
    action: Confirmation;
  } | null>(null);
  const [operationError, setOperationError] = useState("");
  const addButtonRef = useRef<HTMLButtonElement>(null);
  const focusAfterUpdate = useRef<FocusTarget | null>(null);
  const tokenLocked = tokenRequestPending || oneTimeToken !== null;

  useEffect(() => {
    const requested = focusAfterUpdate.current;
    if (!requested) {
      return;
    }
    let target: HTMLElement | null = null;
    if (requested.kind === "rotate") {
      target = document.getElementById(`rotate-token-${requested.serverId}`);
    } else if (requested.kind === "server") {
      target = document.getElementById(`server-primary-action-${requested.serverId}`);
    }
    if (!target || (target instanceof HTMLButtonElement && target.disabled)) {
      target = addButtonRef.current;
    }
    if (target && !(target instanceof HTMLButtonElement && target.disabled)) {
      target.focus();
      focusAfterUpdate.current = null;
    }
  }, [oneTimeToken, servers, tokenRequestPending]);

  const create = async (name: string) => {
    if (!onTokenRequestStarted(null)) {
      return;
    }
    setCreating(true);
    setOperationError("");
    try {
      const result = await serverApi.create(name);
      addServer(result.server);
      onTokenPublished({
        serverId: result.server.id,
        serverName: result.server.name,
        token: result.token,
        returnFocus: { kind: "create" },
      });
      setShowCreate(false);
    } catch (error) {
      const message = apiErrorMessage(
        error,
        "暂时无法创建服务器，请稍后重试。",
      );
      onTokenRequestFailed(message);
      setOperationError(message);
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
    setOperationError("");
    try {
      const outcome = await updateServer(
        server.id,
        "rename",
        () => serverApi.update(server.id, { name }),
      );
      if (outcome.applied) {
        onTokenServerRenamed(server.id, name);
        setEditingId(null);
      }
    } catch (error) {
      setOperationError(
        apiErrorMessage(error, "暂时无法重命名服务器，请稍后重试。"),
      );
    }
  };

  const toggle = async (server: ServerRecord) => {
    const enabled = !server.enabled;
    setOperationError("");
    try {
      await updateServer(
        server.id,
        "toggle",
        () => serverApi.update(server.id, { enabled }),
      );
    } catch (error) {
      setOperationError(
        apiErrorMessage(
          error,
          `暂时无法${enabled ? "启用" : "停用"}服务器，请稍后重试。`,
        ),
      );
    }
  };

  const remove = async (server: ServerRecord) => {
    const index = servers.findIndex((item) => item.id === server.id);
    const survivor = servers[index + 1] ?? servers[index - 1];
    focusAfterUpdate.current = survivor
      ? { kind: "server", serverId: survivor.id }
      : { kind: "add" };
    setOperationError("");
    try {
      const outcome = await removeServer(
        server.id,
        () => serverApi.remove(server.id),
      );
      if (outcome.applied) {
        setConfirmation(null);
        onTokenServerDeleted(server.id);
      }
    } catch (error) {
      focusAfterUpdate.current = null;
      setOperationError(
        apiErrorMessage(error, "暂时无法删除服务器，请稍后重试。"),
      );
    }
  };

  const rotate = async (server: ServerRecord) => {
    if (!onTokenRequestStarted(server.id)) {
      return;
    }
    setOperationError("");
    try {
      let token = "";
      const outcome = await updateServer(server.id, "rotate", async () => {
        const result = await serverApi.rotateToken(server.id);
        token = result.token;
        return result.server;
      });
      if (outcome.applied) {
        onTokenPublished({
          serverId: server.id,
          serverName: server.name,
          token,
          returnFocus: { kind: "rotate", serverId: server.id },
        });
        setConfirmation(null);
      } else {
        onTokenRequestFailed("Token 请求已被更新的操作替代，请重试。");
      }
    } catch (error) {
      const message = apiErrorMessage(
        error,
        "暂时无法重新生成 Token，请稍后重试。",
      );
      onTokenRequestFailed(message);
      setOperationError(message);
    }
  };

  const clearToken = () => {
    if (oneTimeToken?.returnFocus.kind === "rotate") {
      focusAfterUpdate.current = oneTimeToken.returnFocus;
    } else {
      focusAfterUpdate.current = { kind: "add" };
    }
    onTokenCleared();
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
          ref={addButtonRef}
          className="button button-primary management-add-button"
          type="button"
          aria-expanded={showCreate}
          disabled={creating || loadState === "loading" || tokenLocked}
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
            disabled={creating || loadState === "loading" || tokenLocked}
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

      {oneTimeToken ? (
        <ServerInstallPanel
          serverName={oneTimeToken.serverName}
          token={oneTimeToken.token}
          onTokenCleared={clearToken}
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
            <p>
              {apiErrorMessage(
                loadError,
                "暂时无法读取服务器列表，请稍后重试。",
              )}
            </p>
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
              const rowPending = pendingByServer[server.id];
              const rowBusy =
                rowPending !== undefined ||
                (tokenRequestPending && tokenRequestServerId === server.id);
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
                        id={`server-primary-action-${server.id}`}
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
                        {rowPending?.action === "toggle"
                          ? "处理中…"
                          : server.enabled
                            ? "停用"
                            : "启用"}
                      </button>
                      <button
                        id={`rotate-token-${server.id}`}
                        className="button button-quiet"
                        type="button"
                        disabled={rowBusy || tokenLocked}
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
                          disabled={
                            rowBusy ||
                            (confirmation.action === "rotate" && tokenLocked)
                          }
                          aria-label={`确认${confirmation.action === "delete" ? "删除" : "重新生成"} ${server.name}${confirmation.action === "rotate" ? " 的 Token" : ""}`}
                          onClick={() =>
                            void (confirmation.action === "delete"
                              ? remove(server)
                              : rotate(server))
                          }
                        >
                          {rowPending?.action === confirmation.action
                            ? "处理中…"
                            : "确认"}
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
