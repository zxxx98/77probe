import { useEffect, useState } from "react";

import { AuthGate } from "../auth/AuthGate";
import { AppNav } from "../components/AppNav";
import { OverviewPage } from "../pages/OverviewPage";
import { ServerDetailPage } from "../pages/ServerDetailPage";

const detailPath = /^\/servers\/(\d+)\/?$/;

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
  return (
    <div className="app-shell">
      <a className="skip-link" href="#main-content">
        跳到主要内容
      </a>
      <AppNav pathname={detailMatch ? pathname : "/"} onNavigate={navigate} />
      {detailMatch ? (
        <ServerDetailPage serverId={Number(detailMatch[1])} onNavigate={navigate} />
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
