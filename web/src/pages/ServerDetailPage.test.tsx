import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const chart = vi.hoisted(() => ({
  setOption: vi.fn(),
  resize: vi.fn(),
  dispose: vi.fn(),
}));
vi.mock("echarts/charts", () => ({ LineChart: {} }));
vi.mock("echarts/components", () => ({
  GridComponent: {},
  LegendComponent: {},
  TooltipComponent: {},
}));
vi.mock("echarts/core", () => ({ init: vi.fn(() => chart), use: vi.fn() }));
vi.mock("echarts/renderers", () => ({ CanvasRenderer: {} }));

import type { ServerSnapshot } from "../api/types";
import type { HistoryResponse, MinuteRecord } from "../history/types";
import {
  HistoricalChartErrorBoundary,
  ServerDetailPage,
} from "./ServerDetailPage";

const fetchMock = vi.mocked(fetch);

function snapshot(serverId: number, reported = true): ServerSnapshot {
  return {
    serverId,
    serverName: `server-${serverId}`,
    online: reported,
    lastReceivedAt: reported
      ? "2026-07-28T00:12:00Z"
      : "0001-01-01T00:00:00Z",
    sourceIp: reported ? "192.0.2.10" : "",
    report: {
      collectedAtUnix: reported ? 1_753_588_800 : 0,
      agentVersion: reported ? "0.1.0" : "",
      host: {
        hostname: reported ? `server-${serverId}` : "",
        os: reported ? "linux" : "",
        platform: reported ? "ubuntu" : "",
        platformVersion: reported ? "24.04" : "",
        kernelVersion: reported ? "6.8.0" : "",
        architecture: reported ? "amd64" : "",
        cpuModel: reported ? "Tiny CPU" : "",
        cpuCores: reported ? 4 : 0,
        primaryIp: reported ? "192.0.2.10" : "",
        bootTimeUnix: reported ? 1_752_724_800 : 0,
        uptimeSeconds: reported ? 864_000 : 0,
      },
      cpu: { usagePercent: 42, load1: 0.2, load5: 0.3, load15: 0.4 },
      memory: {
        totalBytes: 8_589_934_592,
        usedBytes: 4_294_967_296,
        swapTotalBytes: 0,
        swapUsedBytes: 0,
      },
      disks: reported
        ? [
            {
              mountpoint: "/",
              totalBytes: 107_374_182_400,
              usedBytes: 53_687_091_200,
            },
          ]
        : null,
      diskIo: { readBytesPerSecond: 1_024, writeBytesPerSecond: 2_048 },
      network: {
        interface: reported ? "eth0" : "",
        uploadBytesPerSecond: 4_096,
        downloadBytesPerSecond: 8_192,
        totalUploadBytes: 10_737_418_240,
        totalDownloadBytes: 21_474_836_480,
      },
    },
  };
}

function minute(
  serverId: number,
  minuteUnix: number,
  cpuAverage: number,
  includeDataDisk = false,
): MinuteRecord {
  return {
    serverId,
    minuteUnix,
    payload: {
      cpuUsage: { average: cpuAverage, maximum: cpuAverage + 8 },
      load1: { average: cpuAverage / 100, maximum: cpuAverage / 90 },
      load5: { average: cpuAverage / 120, maximum: cpuAverage / 110 },
      load15: { average: cpuAverage / 140, maximum: cpuAverage / 130 },
      memoryUsage: { average: 50, maximum: 58 },
      swapUsage: { average: 0, maximum: 0 },
      disks: [
        {
          mountpoint: "/",
          usage: { average: 45, maximum: 47 },
          totalBytes: 100_000,
          usedBytes: 45_000,
        },
        ...(includeDataDisk
          ? [
              {
                mountpoint: "/data",
                usage: { average: 65, maximum: 69 },
                totalBytes: 200_000,
                usedBytes: 130_000,
              },
            ]
          : []),
      ],
      diskReadBps: { average: 1_024, maximum: 4_096 },
      diskWriteBps: { average: 2_048, maximum: 8_192 },
      uploadBps: { average: 4_096, maximum: 12_288 },
      downloadBps: { average: 8_192, maximum: 16_384 },
      totalUpload: 10_000 + minuteUnix,
      totalDownload: 20_000 + minuteUnix,
    },
  };
}

function history(serverId: number, cpuAverage = 30): HistoryResponse {
  return {
    fromUnix: 600,
    toUnix: 720,
    points: [
      minute(serverId, 600, cpuAverage - 10, true),
      minute(serverId, 720, cpuAverage),
    ],
  };
}

function deferredResponse() {
  let resolve!: (value: Response) => void;
  const promise = new Promise<Response>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

async function renderDetail(serverId: number, reported = true) {
  fetchMock.mockResolvedValueOnce(Response.json(snapshot(serverId, reported)));
  const view = render(
    <ServerDetailPage serverId={serverId} onNavigate={vi.fn()} />,
  );
  await screen.findByRole("heading", { name: `server-${serverId}` });
  return view;
}

beforeEach(() => {
  chart.setOption.mockClear();
  chart.resize.mockClear();
  chart.dispose.mockClear();
});

describe("ServerDetailPage history", () => {
  it("defaults to the unchanged real-time metrics without requesting history", async () => {
    await renderDetail(407);

    expect(screen.getByRole("button", { name: "实时" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    expect(screen.getByText("0.20 / 0.30 / 0.40")).toBeInTheDocument();
    expect(screen.getByText("未配置 Swap")).toBeInTheDocument();
    expect(
      fetchMock.mock.calls.some(([path]) => String(path).includes("/history?")),
    ).toBe(false);
  });

  it("fetches a selected range and renders all historical metric groups with gaps", async () => {
    await renderDetail(417);
    fetchMock.mockResolvedValueOnce(Response.json(history(417)));

    fireEvent.click(screen.getByRole("button", { name: "1天" }));

    expect(await screen.findByRole("heading", { name: "CPU 使用率" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "系统负载" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "内存使用率" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Swap 使用率" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "/ 使用率" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "/data 使用率" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "磁盘 I/O" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "网络速率" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "累计网络流量" })).toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/servers/417/history?range=1d",
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    );
    expect(screen.getByRole("button", { name: "1天" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    expect(
      chart.setOption.mock.calls.some(([option]) =>
        (option as { series: Array<{ data: Array<[number, number | null]> }> }).series
          .some((series) => series.data.some(([timestamp, value]) =>
            timestamp === 660_000 && value === null,
          )),
      ),
    ).toBe(true);
    const plottedSeries = chart.setOption.mock.calls.flatMap(([option]) =>
      (option as { series: Array<{ name: string }> }).series.map(
        (series) => series.name,
      ),
    );
    expect(plottedSeries).toEqual(
      expect.arrayContaining([
        "Load 1 峰值",
        "Load 5 峰值",
        "Load 15 峰值",
        "读取峰值",
        "写入峰值",
        "上传峰值",
        "下载峰值",
      ]),
    );
  });

  it("shows historical skeletons while a range request is pending", async () => {
    await renderDetail(427);
    const pending = deferredResponse();
    fetchMock.mockImplementationOnce(() => pending.promise);

    fireEvent.click(screen.getByRole("button", { name: "7天" }));

    expect(screen.getByRole("status")).toHaveTextContent("正在读取历史指标");
    expect(document.querySelectorAll(".history-skeleton")).toHaveLength(4);
  });

  it("shows a useful empty state when the selected range has no points", async () => {
    await renderDetail(437);
    fetchMock.mockResolvedValueOnce(
      Response.json({ fromUnix: 600, toUnix: 720, points: [] }),
    );

    fireEvent.click(screen.getByRole("button", { name: "30天" }));

    expect(await screen.findByText("这个时间范围内还没有历史数据")).toBeInTheDocument();
    expect(screen.getByText("探针完成分钟聚合后，趋势会显示在这里。")).toBeInTheDocument();
  });

  it("shows an inline error and retries without reloading the snapshot", async () => {
    await renderDetail(447);
    fetchMock
      .mockResolvedValueOnce(
        Response.json({ error: "历史存储暂时不可用" }, { status: 503 }),
      )
      .mockResolvedValueOnce(Response.json(history(447, 44)));

    fireEvent.click(screen.getByRole("button", { name: "1天" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("历史存储暂时不可用");
    fireEvent.click(screen.getByRole("button", { name: "重试历史数据" }));

    expect(await screen.findByRole("heading", { name: "CPU 使用率" })).toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledTimes(3);
  });

  it("does not let a stale range response replace the current selection", async () => {
    await renderDetail(457);
    const oneDay = deferredResponse();
    fetchMock
      .mockImplementationOnce(() => oneDay.promise)
      .mockResolvedValueOnce(Response.json(history(457, 70)));

    fireEvent.click(screen.getByRole("button", { name: "1天" }));
    fireEvent.click(screen.getByRole("button", { name: "7天" }));

    const summary = await screen.findByRole("group", { name: "CPU 使用率摘要" });
    expect(within(summary).getByText("70.0%")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "7天" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );

    await act(async () => {
      oneDay.resolve(Response.json(history(457, 10)));
    });

    expect(within(summary).getByText("70.0%")).toBeInTheDocument();
    expect(within(summary).queryByText("10.0%")).not.toBeInTheDocument();
  });

  it("keeps history usable when the current snapshot has no report", async () => {
    await renderDetail(467, false);
    fetchMock.mockResolvedValueOnce(Response.json(history(467, 55)));

    fireEvent.click(screen.getByRole("button", { name: "1天" }));

    expect(await screen.findByRole("heading", { name: "CPU 使用率" })).toBeInTheDocument();
    expect(
      within(screen.getByRole("group", { name: "CPU 使用率摘要" })).getByText(
        "55.0%",
      ),
    ).toBeInTheDocument();
  });

  it("contains a historical chunk failure and offers a real page reload recovery", () => {
    const reload = vi.fn();
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => {});
    function RejectingHistory(): never {
      throw new Error("historical chunk rejected");
    }

    render(
      <main>
        <h1>server-477</h1>
        <HistoricalChartErrorBoundary reload={reload}>
          <RejectingHistory />
        </HistoricalChartErrorBoundary>
      </main>,
    );

    expect(screen.getByRole("heading", { name: "server-477" })).toBeInTheDocument();
    expect(screen.getByRole("alert")).toHaveTextContent("历史图表加载失败");
    fireEvent.click(screen.getByRole("button", { name: "重新加载页面" }));
    expect(reload).toHaveBeenCalledTimes(1);
    consoleError.mockRestore();
  });
});
