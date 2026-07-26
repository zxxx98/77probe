import type { DiskStats, ServerSnapshot } from "../api/types";

export function hasReport(snapshot: ServerSnapshot): boolean {
  return snapshot.report.collectedAtUnix > 0 && !snapshot.lastReceivedAt.startsWith("0001-");
}

export function percentage(used: number, total: number): number | null {
  if (total <= 0 || used < 0) {
    return null;
  }
  return (used / total) * 100;
}

export function highestDiskUsage(disks: DiskStats[]): number | null {
  let highest: number | null = null;
  for (const disk of disks) {
    const usage = percentage(disk.usedBytes, disk.totalBytes);
    if (usage !== null && (highest === null || usage > highest)) {
      highest = usage;
    }
  }
  return highest;
}

export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes < 0) {
    return "—";
  }
  const units = ["B", "KiB", "MiB", "GiB", "TiB", "PiB"];
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit += 1;
  }
  const digits = unit === 0 || value >= 100 ? 0 : value >= 10 ? 1 : 2;
  return `${value.toFixed(digits)} ${units[unit]}`;
}

export function formatRate(bytesPerSecond: number): string {
  const formatted = formatBytes(bytesPerSecond);
  return formatted === "—" ? formatted : `${formatted}/s`;
}

export function formatDuration(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) {
    return "—";
  }
  const days = Math.floor(seconds / 86_400);
  const hours = Math.floor((seconds % 86_400) / 3_600);
  const minutes = Math.floor((seconds % 3_600) / 60);
  if (days > 0) {
    return `${days}天 ${hours}小时`;
  }
  if (hours > 0) {
    return `${hours}小时 ${minutes}分钟`;
  }
  return `${Math.max(1, minutes)}分钟`;
}

function formatDate(date: Date): string {
  if (Number.isNaN(date.getTime())) {
    return "时间未知";
  }
  return new Intl.DateTimeFormat("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
  }).format(date);
}

export function formatTimestamp(value: string): string {
  if (!value || value.startsWith("0001-")) {
    return "尚未上报";
  }
  return formatDate(new Date(value));
}

export function formatUnixTimestamp(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) {
    return "尚无数据";
  }
  return formatDate(new Date(seconds * 1000));
}
