import { useEffect, useRef, useState } from "react";

import { AuthGate } from "../auth/AuthGate";
import { AppNav } from "../components/AppNav";
import { OverviewPage } from "../pages/OverviewPage";
import { ServerDetailPage } from "../pages/ServerDetailPage";
import {
  ServersPage,
  type OneTimeToken,
} from "../pages/ServersPage";

const detailPath = /^\/servers\/([^/]+)\/?$/;

interface InvalidServerPageProps {
  onNavigate: (path: string) => void;
}

function InvalidServerPage({ onNavigate }: InvalidServerPageProps) {
  return (
    <main className="dashboard-content" id="main-content">
      <a
        className="back-link"
        href="/"
        onClick={(event) => {
          event.preventDefault();
          onNavigate("/");
        }}
      >
        <span aria-hidden="true">←</span> 返回概览
      </a>
      <section className="dashboard-state" role="alert">
        <div>
          <h1>服务器地址无效</h1>
          <p>请从概览中选择一台服务器。</p>
        </div>
      </section>
    </main>
  );
}

export function DashboardRouter() {
  const [pathname, setPathname] = useState(window.location.pathname);
  const [oneTimeToken, setOneTimeToken] = useState<OneTimeToken | null>(null);
  const [tokenRequestPending, setTokenRequestPending] = useState(false);
  const [tokenRequestServerId, setTokenRequestServerId] = useState<number | null>(null);
  const [managementGeneration, setManagementGeneration] = useState(0);
  const tokenRef = useRef<OneTimeToken | null>(null);
  const tokenLockRef = useRef(false);

  useEffect(() => {
    const handlePopState = () => setPathname(window.location.pathname);
    window.addEventListener("popstate", handlePopState);
    return () => window.removeEventListener("popstate", handlePopState);
  }, []);

  const navigate = (path: string) => {
    if (path !== window.location.pathname) {
      window.history.pushState(null, "", path);
    }
    setPathname(path);
  };

  useEffect(() => {
    if (!tokenRequestPending && oneTimeToken === null) {
      return;
    }
    const protectToken = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", protectToken);
    return () => window.removeEventListener("beforeunload", protectToken);
  }, [oneTimeToken, tokenRequestPending]);

  const startTokenRequest = (serverId: number | null) => {
    if (tokenLockRef.current) {
      return false;
    }
    tokenLockRef.current = true;
    setTokenRequestPending(true);
    setTokenRequestServerId(serverId);
    return true;
  };

  const failTokenRequest = () => {
    setTokenRequestPending(false);
    setTokenRequestServerId(null);
    tokenLockRef.current = tokenRef.current !== null;
  };

  const publishToken = (token: OneTimeToken) => {
    tokenRef.current = token;
    tokenLockRef.current = true;
    setOneTimeToken(token);
    setTokenRequestPending(false);
    setTokenRequestServerId(null);
    setManagementGeneration((current) => current + 1);
    navigate("/servers");
  };

  const clearToken = () => {
    tokenRef.current = null;
    tokenLockRef.current = false;
    setOneTimeToken(null);
  };

  const clearTokenForDeletedServer = (serverId: number) => {
    setOneTimeToken((current) => {
      if (current?.serverId !== serverId) {
        return current;
      }
      tokenRef.current = null;
      tokenLockRef.current = false;
      return null;
    });
  };

  const renameTokenServer = (serverId: number, serverName: string) => {
    setOneTimeToken((current) => {
      if (current?.serverId !== serverId) {
        return current;
      }
      const next = { ...current, serverName };
      tokenRef.current = next;
      return next;
    });
  };

  const detailMatch = pathname.match(detailPath);
  const managementPath = pathname === "/servers" || pathname === "/servers/";
  const serverId = detailMatch ? Number(detailMatch[1]) : null;
  const validServerId =
    serverId !== null && Number.isSafeInteger(serverId) && serverId > 0;
  return (
    <div className="app-shell">
      <a className="skip-link" href="#main-content">
        跳到主要内容
      </a>
      <AppNav pathname={pathname} onNavigate={navigate} />
      {!managementPath && tokenRequestPending ? (
        <aside className="token-reminder" aria-live="polite">
          正在生成一次性 Agent Token；完成后会自动返回安装面板。
        </aside>
      ) : null}
      {!managementPath && oneTimeToken ? (
        <aside className="token-reminder" aria-live="polite">
          <span>一次性 Agent Token 尚未保存。</span>
          <button
            className="button button-primary"
            type="button"
            onClick={() => navigate("/servers")}
          >
            返回保存 Token
          </button>
        </aside>
      ) : null}
      {detailMatch ? (
        validServerId ? (
          <ServerDetailPage
            key={serverId}
            serverId={serverId}
            onNavigate={navigate}
          />
        ) : (
          <InvalidServerPage onNavigate={navigate} />
        )
      ) : managementPath ? (
        <ServersPage
          key={managementGeneration}
          oneTimeToken={oneTimeToken}
          tokenRequestPending={tokenRequestPending}
          tokenRequestServerId={tokenRequestServerId}
          onTokenRequestStarted={startTokenRequest}
          onTokenRequestFailed={failTokenRequest}
          onTokenPublished={publishToken}
          onTokenCleared={clearToken}
          onTokenServerDeleted={clearTokenForDeletedServer}
          onTokenServerRenamed={renameTokenServer}
        />
      ) : (
        <OverviewPage onNavigate={navigate} />
      )}
    </div>
  );
}

export function App() {
  return (
    <AuthGate>
      <DashboardRouter />
    </AuthGate>
  );
}
