import { AuthGate } from "../auth/AuthGate";

function ApplicationShell() {
  return (
    <div className="app-shell">
      <a className="skip-link" href="#main-content">
        跳到主要内容
      </a>
      <header className="app-header">
        <a className="brand" href="/" aria-label="TinyProbe 首页">
          <span className="brand-mark" aria-hidden="true">
            T
          </span>
          <span translate="no">TinyProbe</span>
        </a>
        <nav aria-label="主导航">
          <a className="nav-link nav-link-current" href="/">
            概览
          </a>
        </nav>
      </header>
      <main className="app-content" id="main-content">
        <p className="calm-status">
          <span className="status-dot" aria-hidden="true" />
          已安全登录
        </p>
        <h1>服务器们都很安稳</h1>
        <p>监控概览会在下一阶段出现在这里。</p>
      </main>
    </div>
  );
}

export function App() {
  return (
    <AuthGate>
      <ApplicationShell />
    </AuthGate>
  );
}
