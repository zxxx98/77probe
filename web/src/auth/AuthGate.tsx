import { type ReactNode, useEffect, useState } from "react";

import { api, ApiError, apiErrorMessage } from "../api/client";
import { LoginPage } from "./LoginPage";
import { AuthLayout, SetupPage } from "./SetupPage";

type AuthView =
  | { kind: "loading" }
  | { kind: "setup" }
  | { kind: "login"; setupComplete?: boolean }
  | { kind: "authenticated" }
  | { kind: "error"; message: string };

interface AuthGateProps {
  children: ReactNode;
}

async function resolveAuthView(): Promise<AuthView> {
  const status = await api.getSetupStatus();
  if (status.setupRequired) {
    return { kind: "setup" };
  }

  try {
    await api.getMe();
    return { kind: "authenticated" };
  } catch (error) {
    if (error instanceof ApiError && error.status === 401) {
      return { kind: "login" };
    }
    throw error;
  }
}

export function AuthGate({ children }: AuthGateProps) {
  const [view, setView] = useState<AuthView>({ kind: "loading" });
  const [requestVersion, setRequestVersion] = useState(0);

  useEffect(() => {
    let active = true;

    resolveAuthView()
      .then((nextView) => {
        if (active) {
          setView(nextView);
        }
      })
      .catch((error: unknown) => {
        if (active) {
          setView({
            kind: "error",
            message: apiErrorMessage(error, "暂时无法确认登录状态，请稍后重试。"),
          });
        }
      });

    return () => {
      active = false;
    };
  }, [requestVersion]);

  if (view.kind === "setup") {
    return <SetupPage onComplete={() => setView({ kind: "login", setupComplete: true })} />;
  }
  if (view.kind === "login") {
    return (
      <LoginPage
        onAuthenticated={() => setView({ kind: "authenticated" })}
        setupComplete={view.setupComplete}
      />
    );
  }
  if (view.kind === "authenticated") {
    return children;
  }
  if (view.kind === "error") {
    return (
      <AuthLayout heading="暂时连接不上" description="TinyProbe 没能确认当前登录状态。">
        <p className="form-error" role="alert">
          {view.message}
        </p>
        <button
          className="button button-secondary"
          type="button"
          onClick={() => {
            setView({ kind: "loading" });
            setRequestVersion((version) => version + 1);
          }}
        >
          重试
        </button>
      </AuthLayout>
    );
  }

  return (
    <main className="auth-loading" aria-live="polite">
      <span className="loading-indicator" aria-hidden="true" />
      <p>正在确认安全状态…</p>
    </main>
  );
}
