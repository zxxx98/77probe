import { useEffect, useState } from "react";

import { AuthGate } from "../auth/AuthGate";
import { AppNav } from "../components/AppNav";
import { OverviewPage } from "../pages/OverviewPage";
import { ServerDetailPage } from "../pages/ServerDetailPage";
import { ServersPage } from "../pages/ServersPage";

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
        <ServersPage />
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
