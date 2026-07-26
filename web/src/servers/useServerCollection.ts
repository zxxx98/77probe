import { useCallback, useEffect, useRef, useState } from "react";

import type { ServerRecord } from "./api";

export type ServerPendingAction = "rename" | "toggle" | "delete" | "rotate";

interface PendingServerRequest {
  action: ServerPendingAction;
  generation: number;
}

interface MutationResult {
  applied: boolean;
}

function replaceServer(servers: ServerRecord[], next: ServerRecord) {
  return servers.map((server) => (server.id === next.id ? next : server));
}

export function useServerCollection(
  load: () => Promise<ServerRecord[]>,
) {
  const [servers, setServers] = useState<ServerRecord[]>([]);
  const [loadState, setLoadState] = useState<
    "loading" | "success" | "error"
  >("loading");
  const [loadError, setLoadError] = useState<unknown>(null);
  const [pendingByServer, setPendingByServer] = useState<
    Record<number, PendingServerRequest | undefined>
  >({});
  const nextGeneration = useRef(0);
  const latestListGeneration = useRef(0);
  const latestServerGeneration = useRef(new Map<number, number>());
  const mutationRevision = useRef(0);

  const loadServers = useCallback(async () => {
    const generation = ++nextGeneration.current;
    const revisionAtStart = mutationRevision.current;
    latestListGeneration.current = generation;
    setLoadState("loading");
    setLoadError(null);
    try {
      const result = await load();
      if (latestListGeneration.current !== generation) {
        return;
      }
      if (mutationRevision.current === revisionAtStart) {
        setServers(result);
      }
      setLoadState("success");
    } catch (error) {
      if (latestListGeneration.current !== generation) {
        return;
      }
      setLoadError(error);
      setLoadState("error");
    }
  }, [load]);

  useEffect(() => {
    void loadServers();
  }, [loadServers]);

  const beginServerRequest = (
    serverId: number,
    action: ServerPendingAction,
  ) => {
    const generation = ++nextGeneration.current;
    mutationRevision.current++;
    latestServerGeneration.current.set(serverId, generation);
    setPendingByServer((current) => ({
      ...current,
      [serverId]: { action, generation },
    }));
    return generation;
  };

  const isLatestServerRequest = (serverId: number, generation: number) =>
    latestServerGeneration.current.get(serverId) === generation;

  const finishServerRequest = (serverId: number, generation: number) => {
    setPendingByServer((current) => {
      if (current[serverId]?.generation !== generation) {
        return current;
      }
      const next = { ...current };
      delete next[serverId];
      return next;
    });
  };

  const updateServer = async (
    serverId: number,
    action: Exclude<ServerPendingAction, "delete">,
    request: () => Promise<ServerRecord>,
  ): Promise<MutationResult> => {
    const generation = beginServerRequest(serverId, action);
    try {
      const updated = await request();
      if (!isLatestServerRequest(serverId, generation)) {
        return { applied: false };
      }
      mutationRevision.current++;
      setServers((current) => replaceServer(current, updated));
      return { applied: true };
    } catch (error) {
      if (isLatestServerRequest(serverId, generation)) {
        throw error;
      }
      return { applied: false };
    } finally {
      finishServerRequest(serverId, generation);
    }
  };

  const removeServer = async (
    serverId: number,
    request: () => Promise<void>,
  ): Promise<MutationResult> => {
    const generation = beginServerRequest(serverId, "delete");
    try {
      await request();
      if (!isLatestServerRequest(serverId, generation)) {
        return { applied: false };
      }
      mutationRevision.current++;
      setServers((current) =>
        current.filter((server) => server.id !== serverId),
      );
      return { applied: true };
    } catch (error) {
      if (isLatestServerRequest(serverId, generation)) {
        throw error;
      }
      return { applied: false };
    } finally {
      finishServerRequest(serverId, generation);
    }
  };

  const addServer = (server: ServerRecord) => {
    mutationRevision.current++;
    setServers((current) => [
      ...current.filter((item) => item.id !== server.id),
      server,
    ]);
  };

  return {
    servers,
    loadState,
    loadError,
    pendingByServer,
    loadServers,
    addServer,
    updateServer,
    removeServer,
  };
}
