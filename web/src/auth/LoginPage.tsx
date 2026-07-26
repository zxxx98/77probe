import { type FormEvent, useState } from "react";

import { api, apiErrorMessage } from "../api/client";
import { AuthLayout } from "./SetupPage";

interface LoginPageProps {
  onAuthenticated: () => void;
  setupComplete?: boolean;
}

export function LoginPage({ onAuthenticated, setupComplete }: LoginPageProps) {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError("");
    setSubmitting(true);

    try {
      await api.login({ username, password });
      onAuthenticated();
    } catch (submitError) {
      setError(apiErrorMessage(submitError, "暂时无法登录，请稍后重试。"));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <AuthLayout
      heading="欢迎回来"
      description="登录后查看你的服务器状态。"
      notice={setupComplete ? "管理员已创建，现在可以登录了。" : undefined}
    >
      <form className="auth-form" onSubmit={handleSubmit}>
        <div className="field">
          <label htmlFor="login-username">用户名</label>
          <input
            id="login-username"
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
          <label htmlFor="login-password">密码</label>
          <input
            id="login-password"
            name="password"
            type="password"
            autoComplete="current-password"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
            disabled={submitting}
            maxLength={128}
            required
          />
        </div>

        {error ? (
          <p className="form-error" role="alert">
            {error}
          </p>
        ) : null}

        <button className="button button-primary" type="submit" disabled={submitting}>
          {submitting ? "正在登录…" : "登录"}
        </button>
      </form>
    </AuthLayout>
  );
}
