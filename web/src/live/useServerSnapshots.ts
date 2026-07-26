import { useEffect, useRef, useState } from "react";

import { api, apiErrorMessage } from "../api/client";
import type { LiveSnapshotEvent, ServerSnapshot } from "../api/types";

const LOAD_ERROR = "暂时无法获取服务器状态，请稍后重试。";
const CONNECTION_ERROR = "实时连接已断开，正在自动重连；当前数据可能不是最新。";

function sortSnapshots(snapshots: ServerSnapshot[]): ServerSnapshot[] {
  return [...snapshots].sort((left, right) => {
    if (left.online !== right.online) {
      return left.online ? 1 : -1;
    }
    return left.serverName.localeCompare(right.serverName, "zh-CN");
  });
}

function mergeSnapshot(
  snapshots: ServerSnapshot[],
  incoming: ServerSnapshot,
): ServerSnapshot[] {
  const index = snapshots.findIndex(
    (snapshot) => snapshot.serverId === incoming.serverId,
  );
  if (index === -1) {
    return sortSnapshots([...snapshots, incoming]);
  }
  const next = [...snapshots];
  next[index] = incoming;
  return sortSnapshots(next);
}

function reconcileFetch(
  current: ServerSnapshot[],
  fetched: ServerSnapshot[],
  eventRevisions: Map<number, number>,
  requestRevision: number,
): ServerSnapshot[] {
  const currentById = new Map(
    current.map((snapshot) => [snapshot.serverId, snapshot]),
  );
  return sortSnapshots(
    fetched.map((incoming) => {
      const existing = currentById.get(incoming.serverId);
      if (!existing) {
        return incoming;
      }
      return (eventRevisions.get(incoming.serverId) ?? 0) > requestRevision
        ? existing
        : incoming;
    }),
  );
}

function parseSnapshotEvent(event: MessageEvent<string>): ServerSnapshot | null {
  try {
    const value = JSON.parse(event.data) as Partial<LiveSnapshotEvent>;
    if (
      (value.type !== "snapshot.updated" && value.type !== "snapshot.offline") ||
      !value.snapshot ||
      typeof value.snapshot.serverId !== "number"
    ) {
      return null;
    }
    return value.snapshot;
  } catch {
    return null;
  }
}

export interface ServerSnapshotsState {
  snapshots: ServerSnapshot[];
  connected: boolean;
  loading: boolean;
  error: string | null;
  refresh: () => Promise<void>;
}

export function useServerSnapshots(): ServerSnapshotsState {
  const [snapshots, setSnapshots] = useState<ServerSnapshot[]>([]);
  const [connected, setConnected] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const mountedRef = useRef(true);
  const openedRef = useRef(false);
  const connectionFailedRef = useRef(false);
  const controllersRef = useRef(new Set<AbortController>());
  const eventRevisionRef = useRef(0);
  const serverEventRevisionsRef = useRef(new Map<number, number>());

  async function refresh() {
    const controller = new AbortController();
    const requestRevision = eventRevisionRef.current;
    controllersRef.current.add(controller);
    try {
      const next = await api.getServerStatuses(controller.signal);
      if (!mountedRef.current) {
        return;
      }
      setSnapshots((current) =>
        reconcileFetch(
          current,
          next,
          serverEventRevisionsRef.current,
          requestRevision,
        ),
      );
      setError((current) => (current === CONNECTION_ERROR ? current : null));
    } catch (loadError) {
      if (!mountedRef.current || controller.signal.aborted) {
        return;
      }
      setError(apiErrorMessage(loadError, LOAD_ERROR));
    } finally {
      controllersRef.current.delete(controller);
      if (mountedRef.current) {
        setLoading(false);
      }
    }
  }

  useEffect(() => {
    mountedRef.current = true;
    void refresh();

    const source = new EventSource("/api/live");
    const handleSnapshot = (event: Event) => {
      const snapshot = parseSnapshotEvent(event as MessageEvent<string>);
      if (snapshot && mountedRef.current) {
        eventRevisionRef.current += 1;
        serverEventRevisionsRef.current.set(
          snapshot.serverId,
          eventRevisionRef.current,
        );
        setSnapshots((current) => mergeSnapshot(current, snapshot));
      }
    };

    source.addEventListener("snapshot.updated", handleSnapshot);
    source.addEventListener("snapshot.offline", handleSnapshot);
    source.onopen = () => {
      if (!mountedRef.current) {
        return;
      }
      setConnected(true);
      setError((current) => (current === CONNECTION_ERROR ? null : current));
      if (openedRef.current || connectionFailedRef.current) {
        void refresh();
      }
      openedRef.current = true;
      connectionFailedRef.current = false;
    };
    source.onerror = () => {
      if (!mountedRef.current) {
        return;
      }
      setConnected(false);
      connectionFailedRef.current = true;
      setError(CONNECTION_ERROR);
    };

    return () => {
      mountedRef.current = false;
      source.close();
      controllersRef.current.forEach((controller) => controller.abort());
      controllersRef.current.clear();
    };
  }, []);

  return { snapshots, connected, loading, error, refresh };
}
