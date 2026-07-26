import { request } from "../api/client";

export interface ServerRecord {
  id: number;
  name: string;
  enabled: boolean;
  agentVersion: string;
  createdAt: string;
  updatedAt: string;
}

export interface ServerTokenResult {
  server: ServerRecord;
  token: string;
}

export interface ServerUpdateInput {
  name?: string;
  enabled?: boolean;
}

export const serverApi = {
  list: () => request<ServerRecord[]>("/api/servers"),
  create: (name: string) =>
    request<ServerTokenResult>("/api/servers", {
      method: "POST",
      body: JSON.stringify({ name }),
    }),
  update: (id: number, input: ServerUpdateInput) =>
    request<ServerRecord>(`/api/servers/${id}`, {
      method: "PATCH",
      body: JSON.stringify(input),
    }),
  remove: (id: number) =>
    request<void>(`/api/servers/${id}`, { method: "DELETE" }),
  rotateToken: (id: number) =>
    request<ServerTokenResult>(`/api/servers/${id}/token`, {
      method: "POST",
    }),
};
