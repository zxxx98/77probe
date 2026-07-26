export interface Admin {
  id: number;
  username: string;
}

export interface Credentials {
  username: string;
  password: string;
}

interface SetupStatus {
  setupRequired: boolean;
}

interface LoginResult {
  ok: boolean;
}

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      ...(init?.headers ?? {}),
    },
    credentials: "same-origin",
  });

  if (!response.ok) {
    throw new ApiError(response.status, await response.text());
  }
  if (response.status === 204) {
    return undefined as T;
  }
  return response.json() as Promise<T>;
}

function postJSON<T>(path: string, credentials: Credentials): Promise<T> {
  return request<T>(path, {
    method: "POST",
    body: JSON.stringify(credentials),
  });
}

export const api = {
  getSetupStatus: () => request<SetupStatus>("/api/setup/status"),
  setupAdmin: (credentials: Credentials) =>
    postJSON<Admin>("/api/setup", credentials),
  login: (credentials: Credentials) =>
    postJSON<LoginResult>("/api/login", credentials),
  logout: () => request<void>("/api/logout", { method: "POST" }),
  getMe: () => request<Admin>("/api/me"),
};

export function apiErrorMessage(error: unknown, fallback: string): string {
  if (!(error instanceof ApiError)) {
    return fallback;
  }

  try {
    const value = JSON.parse(error.message) as { error?: unknown };
    return typeof value.error === "string" ? value.error : fallback;
  } catch {
    return error.message || fallback;
  }
}
