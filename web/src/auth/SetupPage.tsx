import { type FormEvent, type ReactNode, useState } from "react";

import { api, apiErrorMessage } from "../api/client";

interface SetupPageProps {
  onComplete: () => void;
}

export function SetupPage({ onComplete }: SetupPageProps) {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError("");
    setSubmitting(true);

    try {
      await api.setupAdmin({ username, password });
      onComplete();
    } catch (submitError) {
      setError(apiErrorMessage(submitError, "暂时无法创建管理员，请稍后重试。"));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <AuthLayout
      heading="创建管理员"
      description="先设置唯一的管理员账户，以后就用它进入你的监控站。"
    >
      <form className="auth-form" onSubmit={handleSubmit}>
        <div className="field">
          <label htmlFor="setup-username">用户名</label>
          <input
            id="setup-username"
            name="username"
            type="text"
            autoComplete="username"
            spellCheck={false}
            value={username}
            onChange={(event) => setUsername(event.target.value)}
            disabled={submitting}
            maxLength={64}
            required
          />
        </div>

        <div className="field">
          <label htmlFor="setup-password">密码</label>
          <input
            id="setup-password"
            name="password"
            type="password"
            autoComplete="new-password"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
            disabled={submitting}
            minLength={12}
            maxLength={128}
            aria-describedby="setup-password-hint"
            required
          />
          <p className="field-hint" id="setup-password-hint">
            至少 12 个字符，建议使用容易记住的长密码。
          </p>
        </div>

        {error ? (
          <p className="form-error" role="alert">
            {error}
          </p>
        ) : null}

        <button className="button button-primary" type="submit" disabled={submitting}>
          {submitting ? "正在创建…" : "创建管理员"}
        </button>
      </form>
    </AuthLayout>
  );
}

interface AuthLayoutProps {
  heading: string;
  description: string;
  notice?: string;
  children: ReactNode;
}

export function AuthLayout({
  heading,
  description,
  notice,
  children,
}: AuthLayoutProps) {
  return (
    <>
      <a className="skip-link" href="#main-content">
        跳到主要内容
      </a>
      <main className="auth-page" id="main-content">
        <section className="auth-intro" aria-labelledby="auth-product-name">
          <a
            className="brand"
            href="/"
            id="auth-product-name"
            aria-label="TinyProbe 首页"
          >
            <span className="brand-mark" aria-hidden="true">
              T
            </span>
            <span translate="no">TinyProbe</span>
          </a>
          <div className="auth-intro-copy">
            <p className="calm-status">
              <span className="status-dot" aria-hidden="true" />
              只属于你的小型监控站
            </p>
            <h1>看看服务器，心里就有底。</h1>
            <p>
              在明亮、安静的日常里，随手确认 1–10 台 Linux 服务器是否一切安稳。
            </p>
          </div>
        </section>

        <section className="auth-panel" aria-labelledby="auth-heading">
          <div className="auth-panel-heading">
            <h2 id="auth-heading">{heading}</h2>
            <p>{description}</p>
          </div>
          {notice ? <p className="form-notice">{notice}</p> : null}
          {children}
        </section>
      </main>
    </>
  );
}
