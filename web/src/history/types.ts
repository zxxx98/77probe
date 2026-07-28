export type Range = "1d" | "7d" | "30d";

export interface Pair {
  average: number;
  maximum: number;
}

export interface DiskMinute {
  mountpoint: string;
  usage: Pair;
  totalBytes: number;
  usedBytes: number;
}

export interface MinutePayload {
  cpuUsage: Pair;
  load1: Pair;
  load5: Pair;
  load15: Pair;
  memoryUsage: Pair;
  swapUsage: Pair;
  disks: DiskMinute[];
  diskReadBps: Pair;
  diskWriteBps: Pair;
  uploadBps: Pair;
  downloadBps: Pair;
  totalUpload: number;
  totalDownload: number;
}

export interface MinuteRecord {
  serverId: number;
  minuteUnix: number;
  payload: MinutePayload;
}

export interface HistoryResponse {
  fromUnix: number;
  toUnix: number;
  points: MinuteRecord[];
}
