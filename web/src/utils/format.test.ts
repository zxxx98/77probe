import { describe, expect, it } from "vitest";

import type { ServerSnapshot } from "../api/types";
import { hasReport, highestDiskUsage } from "./format";

const placeholder: ServerSnapshot = {
  serverId: 9,
  serverName: "never-reported",
  online: false,
  lastReceivedAt: "0001-01-01T00:00:00Z",
  sourceIp: "",
  report: {
    collectedAtUnix: 0,
    agentVersion: "",
    host: {
      hostname: "",
      os: "",
      platform: "",
      platformVersion: "",
      kernelVersion: "",
      architecture: "",
      cpuModel: "",
      cpuCores: 0,
      primaryIp: "",
      bootTimeUnix: 0,
      uptimeSeconds: 0,
    },
    cpu: { usagePercent: 0, load1: 0, load5: 0, load15: 0 },
    memory: {
      totalBytes: 0,
      usedBytes: 0,
      swapTotalBytes: 0,
      swapUsedBytes: 0,
    },
    disks: null,
    diskIo: { readBytesPerSecond: 0, writeBytesPerSecond: 0 },
    network: {
      interface: "",
      uploadBytesPerSecond: 0,
      downloadBytesPerSecond: 0,
      totalUploadBytes: 0,
      totalDownloadBytes: 0,
    },
  },
};

describe("placeholder report formatting", () => {
  it("accepts the Go null disks payload without treating it as a report", () => {
    expect(hasReport(placeholder)).toBe(false);
    expect(highestDiskUsage(placeholder.report.disks)).toBeNull();
  });
});
