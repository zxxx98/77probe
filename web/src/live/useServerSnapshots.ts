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
  const fetchedIds = new Set(fetched.map((snapshot) => snapshot.serverId));
  const reconciled = fetched.map((incoming) => {
    const existing = currentById.get(incoming.serverId);
    if (!existing) {
      return incoming;
    }
    return (eventRevisions.get(incoming.serverId) ?? 0) > requestRevision
      ? existing
      : incoming;
  });
  for (const existing of current) {
    if (
      !fetchedIds.has(existing.serverId) &&
      (eventRevisions.get(existing.serverId) ?? 0) > requestRevision
    ) {
      reconciled.push(existing);
    }
  }
  return sortSnapshots(reconciled);
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
  error: string | null;
  refresh: () => Promise<void>;
}

type InitialLoadStatus = "pending" | "success" | "error";

interface OverviewServerSnapshotsState extends ServerSnapshotsState {
  initialLoadStatus: InitialLoadStatus;
  connectionFailed: boolean;
  requestError: string | null;
}

function useServerSnapshotsState(): OverviewServerSnapshotsState {
  const [snapshots, setSnapshots] = useState<ServerSnapshot[]>([]);
  const [liveConnected, setLiveConnected] = useState(false);
  const [connectionFailed, setConnectionFailed] = useState(false);
  const [initialLoadStatus, setInitialLoadStatus] =
    useState<InitialLoadStatus>("pending");
  const [error, setError] = useState<string | null>(null);
  const [requestError, setRequestError] = useState<string | null>(null);
  const mountedRef = useRef(true);
  const openedRef = useRef(false);
  const connectionFailedRef = useRef(false);
  const controllersRef = useRef(new Set<AbortController>());
  const eventRevisionRef = useRef(0);
  const serverEventRevisionsRef = useRef(new Map<number, number>());
  const requestGenerationRef = useRef(0);

  async function refresh() {
    const controller = new AbortController();
    const requestRevision = eventRevisionRef.current;
    const requestGeneration = requestGenerationRef.current + 1;
    requestGenerationRef.current = requestGeneration;
    controllersRef.current.add(controller);
    try {
      const next = await api.getServerStatuses(controller.signal);
      if (
        !mountedRef.current ||
        requestGeneration !== requestGenerationRef.current
      ) {
        return;
      }
      setInitialLoadStatus("success");
      setSnapshots((current) =>
        reconcileFetch(
          current,
          next,
          serverEventRevisionsRef.current,
          requestRevision,
        ),
      );
      setRequestError(null);
      setError((current) => (current === CONNECTION_ERROR ? current : null));
    } catch (loadError) {
      if (
        !mountedRef.current ||
        controller.signal.aborted ||
        requestGeneration !== requestGenerationRef.current
      ) {
        return;
      }
      setInitialLoadStatus((current) =>
        current === "success" ? current : "error",
      );
      const message = apiErrorMessage(loadError, LOAD_ERROR);
      setRequestError(message);
      setError(message);
    } finally {
      controllersRef.current.delete(controller);
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
      setLiveConnected(true);
      setConnectionFailed(false);
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
      setLiveConnected(false);
      setConnectionFailed(true);
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

  return {
    snapshots,
    connected: liveConnected,
    error,
    refresh,
    initialLoadStatus,
    connectionFailed,
    requestError,
  };
}

export function useServerSnapshots(): ServerSnapshotsState {
  const state = useServerSnapshotsState();
  return {
    snapshots: state.snapshots,
    connected: state.connected,
    error: state.error,
    refresh: state.refresh,
  };
}

export function useOverviewServerSnapshots(): OverviewServerSnapshotsState {
  return useServerSnapshotsState();
}
