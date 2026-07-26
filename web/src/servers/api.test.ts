import { beforeEach, describe, expect, it, vi } from "vitest";

import { serverApi, type ServerRecord } from "./api";

const fetchMock = vi.mocked(fetch);

const server: ServerRecord = {
  id: 7,
  name: "home-lab",
  enabled: true,
  agentVersion: "0.1.0",
  createdAt: "2026-07-26T04:00:00Z",
  updatedAt: "2026-07-26T04:00:00Z",
};

describe("serverApi", () => {
  beforeEach(() => {
    fetchMock.mockReset();
  });

  it("uses the server-management endpoints and JSON payloads", async () => {
    fetchMock
      .mockResolvedValueOnce(Response.json([server]))
      .mockResolvedValueOnce(Response.json({ server, token: "tp_created" }, { status: 201 }))
      .mockResolvedValueOnce(Response.json({ ...server, name: "office-lab" }))
      .mockResolvedValueOnce(new Response(null, { status: 204 }));

    await expect(serverApi.list()).resolves.toEqual([server]);
    await expect(serverApi.create("home-lab")).resolves.toEqual({
      server,
      token: "tp_created",
    });
    await expect(
      serverApi.update(7, { name: "office-lab", enabled: false }),
    ).resolves.toMatchObject({ name: "office-lab" });
    await expect(serverApi.remove(7)).resolves.toBeUndefined();

    expect(fetchMock).toHaveBeenNthCalledWith(
      1,
      "/api/servers",
      expect.objectContaining({ credentials: "same-origin" }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      "/api/servers",
      expect.objectContaining({ method: "POST", body: '{"name":"home-lab"}' }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      3,
      "/api/servers/7",
      expect.objectContaining({
        method: "PATCH",
        body: '{"name":"office-lab","enabled":false}',
      }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      4,
      "/api/servers/7",
      expect.objectContaining({ method: "DELETE" }),
    );
  });

  it("preserves both server and token from token rotation", async () => {
    const rotatedServer = {
      ...server,
      updatedAt: "2026-07-26T05:00:00Z",
    };
    fetchMock.mockResolvedValueOnce(
      Response.json({ server: rotatedServer, token: "tp_rotated" }, { status: 201 }),
    );

    await expect(serverApi.rotateToken(7)).resolves.toEqual({
      server: rotatedServer,
      token: "tp_rotated",
    });
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/servers/7/token",
      expect.objectContaining({ method: "POST" }),
    );
  });
});
