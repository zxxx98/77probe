package history

type Pair struct {
	Average float64 `json:"average"`
	Maximum float64 `json:"maximum"`
}

type DiskMinute struct {
	Mountpoint string `json:"mountpoint"`
	Usage      Pair   `json:"usage"`
	TotalBytes uint64 `json:"totalBytes"`
	UsedBytes  uint64 `json:"usedBytes"`
}

type MinutePayload struct {
	CPUUsage      Pair         `json:"cpuUsage"`
	Load1         Pair         `json:"load1"`
	Load5         Pair         `json:"load5"`
	Load15        Pair         `json:"load15"`
	MemoryUsage   Pair         `json:"memoryUsage"`
	SwapUsage     Pair         `json:"swapUsage"`
	Disks         []DiskMinute `json:"disks"`
	DiskReadBPS   Pair         `json:"diskReadBps"`
	DiskWriteBPS  Pair         `json:"diskWriteBps"`
	UploadBPS     Pair         `json:"uploadBps"`
	DownloadBPS   Pair         `json:"downloadBps"`
	TotalUpload   uint64       `json:"totalUpload"`
	TotalDownload uint64       `json:"totalDownload"`
}

type MinuteRecord struct {
	ServerID   int64         `json:"serverId"`
	MinuteUnix int64         `json:"minuteUnix"`
	Payload    MinutePayload `json:"payload"`
}
