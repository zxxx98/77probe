export interface HostInfo {
  hostname: string;
  os: string;
  platform: string;
  platformVersion: string;
  kernelVersion: string;
  architecture: string;
  cpuModel: string;
  cpuCores: number;
  primaryIp: string;
  bootTimeUnix: number;
  uptimeSeconds: number;
}

export interface CPUStats {
  usagePercent: number;
  load1: number;
  load5: number;
  load15: number;
}

export interface MemoryStats {
  totalBytes: number;
  usedBytes: number;
  swapTotalBytes: number;
  swapUsedBytes: number;
}

export interface DiskStats {
  mountpoint: string;
  totalBytes: number;
  usedBytes: number;
}

export interface DiskIOStats {
  readBytesPerSecond: number;
  writeBytesPerSecond: number;
}

export interface NetworkStats {
  interface: string;
  uploadBytesPerSecond: number;
  downloadBytesPerSecond: number;
  totalUploadBytes: number;
  totalDownloadBytes: number;
}

export interface AgentReport {
  collectedAtUnix: number;
  agentVersion: string;
  host: HostInfo;
  cpu: CPUStats;
  memory: MemoryStats;
  disks: DiskStats[] | null;
  diskIo: DiskIOStats;
  network: NetworkStats;
}

export interface ServerSnapshot {
  serverId: number;
  serverName: string;
  online: boolean;
  lastReceivedAt: string;
  sourceIp: string;
  report: AgentReport;
}

export interface LiveSnapshotEvent {
  type: "snapshot.updated" | "snapshot.offline";
  snapshot: ServerSnapshot;
}
