package history

import (
	"reflect"
	"testing"

	"probe.local/monitor/internal/protocol"
)

func TestNumericPair(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		var value numeric
		if got := value.pair(); got != (Pair{}) {
			t.Fatalf("pair() = %+v, want zero pair", got)
		}
	})

	t.Run("average and maximum", func(t *testing.T) {
		var value numeric
		value.add(10)
		value.add(30)

		if value.Count != 2 || value.Sum != 40 || value.Max != 30 {
			t.Fatalf("numeric = %+v, want count 2, sum 40, max 30", value)
		}
		if got, want := value.pair(), (Pair{Average: 20, Maximum: 30}); got != want {
			t.Fatalf("pair() = %+v, want %+v", got, want)
		}
	})

	t.Run("negative first sample initializes maximum", func(t *testing.T) {
		var value numeric
		value.add(-10)
		value.add(-30)

		if got, want := value.pair(), (Pair{Average: -20, Maximum: -10}); got != want {
			t.Fatalf("pair() = %+v, want %+v", got, want)
		}
	})
}

func TestAccumulatorAggregatesMinutePayload(t *testing.T) {
	var accumulator Accumulator
	accumulator.Add(protocol.AgentReport{
		CPU: protocol.CPUStats{
			UsagePercent: 10,
			Load1:        1,
			Load5:        5,
			Load15:       15,
		},
		Memory: protocol.MemoryStats{
			TotalBytes:     1000,
			UsedBytes:      250,
			SwapTotalBytes: 0,
			SwapUsedBytes:  50,
		},
		Disks: []protocol.DiskStats{
			{Mountpoint: "/var", TotalBytes: 1000, UsedBytes: 100},
			{Mountpoint: "/", TotalBytes: 2000, UsedBytes: 500},
		},
		DiskIO: protocol.DiskIOStats{
			ReadBytesPerSecond:  100,
			WriteBytesPerSecond: 500,
		},
		Network: protocol.NetworkStats{
			UploadBytesPerSecond:   100,
			DownloadBytesPerSecond: 200,
			TotalUploadBytes:       1000,
			TotalDownloadBytes:     2000,
		},
	})
	accumulator.Add(protocol.AgentReport{
		CPU: protocol.CPUStats{
			UsagePercent: 30,
			Load1:        3,
			Load5:        7,
			Load15:       17,
		},
		Memory: protocol.MemoryStats{
			TotalBytes:     1000,
			UsedBytes:      750,
			SwapTotalBytes: 200,
			SwapUsedBytes:  50,
		},
		Disks: []protocol.DiskStats{
			{Mountpoint: "/", TotalBytes: 2200, UsedBytes: 1100},
			{Mountpoint: "/var", TotalBytes: 1200, UsedBytes: 360},
		},
		DiskIO: protocol.DiskIOStats{
			ReadBytesPerSecond:  300,
			WriteBytesPerSecond: 900,
		},
		Network: protocol.NetworkStats{
			UploadBytesPerSecond:   300,
			DownloadBytesPerSecond: 600,
			TotalUploadBytes:       1500,
			TotalDownloadBytes:     2600,
		},
	})

	got := accumulator.Finish(7, 1_722_121_200)
	want := MinuteRecord{
		ServerID:   7,
		MinuteUnix: 1_722_121_200,
		Payload: MinutePayload{
			CPUUsage:    Pair{Average: 20, Maximum: 30},
			Load1:       Pair{Average: 2, Maximum: 3},
			Load5:       Pair{Average: 6, Maximum: 7},
			Load15:      Pair{Average: 16, Maximum: 17},
			MemoryUsage: Pair{Average: 50, Maximum: 75},
			SwapUsage:   Pair{Average: 12.5, Maximum: 25},
			Disks: []DiskMinute{
				{Mountpoint: "/", Usage: Pair{Average: 37.5, Maximum: 50}, TotalBytes: 2200, UsedBytes: 1100},
				{Mountpoint: "/var", Usage: Pair{Average: 20, Maximum: 30}, TotalBytes: 1200, UsedBytes: 360},
			},
			DiskReadBPS:   Pair{Average: 200, Maximum: 300},
			DiskWriteBPS:  Pair{Average: 700, Maximum: 900},
			UploadBPS:     Pair{Average: 200, Maximum: 300},
			DownloadBPS:   Pair{Average: 400, Maximum: 600},
			TotalUpload:   1500,
			TotalDownload: 2600,
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Finish() =\n%+v\nwant:\n%+v", got, want)
	}
}
