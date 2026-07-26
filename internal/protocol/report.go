package protocol

type HostInfo struct {
	Hostname        string `json:"hostname"`
	OS              string `json:"os"`
	Platform        string `json:"platform"`
	PlatformVersion string `json:"platformVersion"`
	KernelVersion   string `json:"kernelVersion"`
	Architecture    string `json:"architecture"`
	CPUModel        string `json:"cpuModel"`
	CPUCores        int    `json:"cpuCores"`
	PrimaryIP       string `json:"primaryIp"`
	BootTimeUnix    int64  `json:"bootTimeUnix"`
	UptimeSeconds   uint64 `json:"uptimeSeconds"`
}

type CPUStats struct {
	UsagePercent float64 `json:"usagePercent"`
	Load1        float64 `json:"load1"`
	Load5        float64 `json:"load5"`
	Load15       float64 `json:"load15"`
}

type MemoryStats struct {
	TotalBytes     uint64 `json:"totalBytes"`
	UsedBytes      uint64 `json:"usedBytes"`
	SwapTotalBytes uint64 `json:"swapTotalBytes"`
	SwapUsedBytes  uint64 `json:"swapUsedBytes"`
}

type DiskStats struct {
	Mountpoint string `json:"mountpoint"`
	TotalBytes uint64 `json:"totalBytes"`
	UsedBytes  uint64 `json:"usedBytes"`
}

type DiskIOStats struct {
	ReadBytesPerSecond  uint64 `json:"readBytesPerSecond"`
	WriteBytesPerSecond uint64 `json:"writeBytesPerSecond"`
}

type NetworkStats struct {
	Interface              string `json:"interface"`
	UploadBytesPerSecond   uint64 `json:"uploadBytesPerSecond"`
	DownloadBytesPerSecond uint64 `json:"downloadBytesPerSecond"`
	TotalUploadBytes       uint64 `json:"totalUploadBytes"`
	TotalDownloadBytes     uint64 `json:"totalDownloadBytes"`
}

type AgentReport struct {
	CollectedAtUnix int64        `json:"collectedAtUnix"`
	AgentVersion    string       `json:"agentVersion"`
	Host            HostInfo     `json:"host"`
	CPU             CPUStats     `json:"cpu"`
	Memory          MemoryStats  `json:"memory"`
	Disks           []DiskStats  `json:"disks"`
	DiskIO          DiskIOStats  `json:"diskIo"`
	Network         NetworkStats `json:"network"`
}
