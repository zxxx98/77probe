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
    public serverMessage?: string,
  ) {
    super(serverMessage ?? "请求失败，请稍后重试。");
    this.name = "ApiError";
  }
}

async function validatedErrorMessage(
  response: Response,
): Promise<string | undefined> {
  let value: unknown;
  try {
    value = JSON.parse(await response.text());
  } catch {
    return undefined;
  }

  if (value === null || Array.isArray(value) || typeof value !== "object") {
    return undefined;
  }
  const message = (value as { error?: unknown }).error;
  if (typeof message !== "string" || message.trim() === "") {
    return undefined;
  }
  return message.trim();
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
    throw new ApiError(response.status, await validatedErrorMessage(response));
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
  return error instanceof ApiError ? (error.serverMessage ?? fallback) : fallback;
}
