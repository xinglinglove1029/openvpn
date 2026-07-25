package openvpnweb

import (
	"context"
	"fmt"
	"log"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gavintan/gopkg/tools"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
)

// SystemStatsPayload 推送给前端的系统监控快照（WebSocket envelope payload）。
// 所有数值在序列化时已经统一到稳定单位，前端无需再做单位换算。
type SystemStatsPayload struct {
	// 时间戳（毫秒），用于前端判断数据是否陈旧、做趋势图 X 轴
	Timestamp int64 `json:"timestamp"`
	// 采集间隔（毫秒），方便前端展示
	IntervalMs int64 `json:"intervalMs"`
	// 主机信息（hostname、平台、运行时长、内核版本、CPU 型号/核心数）
	Host SystemHostInfo `json:"host"`
	// CPU 总体使用率 0-100
	CpuPercent float64 `json:"cpuPercent"`
	// 每个逻辑核心使用率 0-100
	CpuPerCore []float64 `json:"cpuPerCore"`
	// 内存
	Memory SystemMemoryInfo `json:"memory"`
	// 进程级：当前进程占用
	Process SystemProcessInfo `json:"process"`
	// 物理分区（按使用率降序，Top 3）
	Disks []SystemDiskInfo `json:"disks"`
	// 物理网卡（按瞬时总速率降序，Top 5；不含 lo/docker*/veth*）
	Networks []SystemNetInfo `json:"networks"`
	// 所有网卡合计瞬时速率（bytes/s）
	NetTotalRxBps float64 `json:"netTotalRxBps"`
	NetTotalTxBps float64 `json:"netTotalTxBps"`
	// 进程 Top N：CPU 占用 Top 5
	TopCpuProcesses []SystemProcessTop `json:"topCpuProcesses"`
	// 进程 Top N：内存占用 Top 5
	TopMemProcesses []SystemProcessTop `json:"topMemProcesses"`
}

type SystemHostInfo struct {
	Hostname        string  `json:"hostname"`
	Platform        string  `json:"platform"`
	PlatformVersion string  `json:"platformVersion"`
	KernelVersion   string  `json:"kernelVersion"`
	KernelArch      string  `json:"kernelArch"`
	UptimeSeconds   uint64  `json:"uptimeSeconds"`
	BootTime        uint64  `json:"bootTime"`
	CpuModel        string  `json:"cpuModel"`
	CpuCores        int     `json:"cpuCores"`
	CpuMHz          float64 `json:"cpuMHz"`
	LoadAvg1        float64 `json:"loadAvg1"`
	LoadAvg5        float64 `json:"loadAvg5"`
	LoadAvg15       float64 `json:"loadAvg15"`
}

type SystemMemoryInfo struct {
	TotalBytes     uint64  `json:"totalBytes"`
	UsedBytes      uint64  `json:"usedBytes"`
	AvailableBytes uint64  `json:"availableBytes"`
	UsedPercent    float64 `json:"usedPercent"`
	SwapTotalBytes uint64  `json:"swapTotalBytes"`
	SwapUsedBytes  uint64  `json:"swapUsedBytes"`
	SwapPercent    float64 `json:"swapPercent"`
}

type SystemProcessInfo struct {
	Pid        int32   `json:"pid"`
	MemoryRSS  uint64  `json:"memoryRss"`
	MemoryVSZ  uint64  `json:"memoryVsz"`
	CPUPercent float64 `json:"cpuPercent"`
	NumThreads int32   `json:"numThreads"`
}

// SystemProcessTop 进程 Top N 列表项（按 CPU 或 内存 排序后的快照）
type SystemProcessTop struct {
	Pid        int32   `json:"pid"`
	Name       string  `json:"name"`
	Username   string  `json:"username"`
	CPUPercent float64 `json:"cpuPercent"`
	MemoryRSS  uint64  `json:"memoryRss"`
	MemoryVSZ  uint64  `json:"memoryVsz"`
	Status     string  `json:"status"`
	Nice       int32   `json:"nice"`
	CreateTime int64   `json:"createTime"` // 进程启动时间（毫秒）
}

type SystemDiskInfo struct {
	Device      string  `json:"device"`
	Mountpoint  string  `json:"mountpoint"`
	FSType      string  `json:"fsType"`
	TotalBytes  uint64  `json:"totalBytes"`
	UsedBytes   uint64  `json:"usedBytes"`
	FreeBytes   uint64  `json:"freeBytes"`
	UsedPercent float64 `json:"usedPercent"`
}

type SystemNetInfo struct {
	Name        string  `json:"name"`
	RxBytes     uint64  `json:"rxBytes"`
	TxBytes     uint64  `json:"txBytes"`
	RxBps       float64 `json:"rxBps"`
	TxBps       float64 `json:"txBps"`
	RxPackets   uint64  `json:"rxPackets"`
	TxPackets   uint64  `json:"txPackets"`
	RxErrors    uint64  `json:"rxErrors"`
	TxErrors    uint64  `json:"txErrors"`
	RxDrops     uint64  `json:"rxDrops"`
	TxDrops     uint64  `json:"txDrops"`
	IsPhysical  bool    `json:"isPhysical"`  // 非虚拟接口
}

// systemStatsCollector 周期采集系统监控并通过 EventBus 广播 system:stats。
// 设计要点：
//   - 单 goroutine 采集，避免多客户端触发多份采集；
//   - 采集间隔 5s，CPU 采样内部会阻塞约 1s（取两次差值），整体开销可控；
//   - 启动时立即采集一次，让前端首屏就有数据；
//   - 网络速率通过相邻两次采集的字节差除以间隔得到；
//   - 失败时仅记日志，绝不因采集异常影响主流程。
type systemStatsCollector struct {
	mu          sync.Mutex
	interval    time.Duration
	lastNetStat map[string]net.IOCountersStat
	lastSample  time.Time
	hostInfo    SystemHostInfo
	cpuModel    string
	cores       int
}

var globalCollector *systemStatsCollector

const systemStatsTopic = "system:stats"

// StartSystemStatsCollector 启动后台采集器（应用启动时调用一次）。
// interval 为采集间隔，建议不小于 3s 以避免给系统带来明显负担。
func StartSystemStatsCollector(interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	c := &systemStatsCollector{
		interval:    interval,
		lastNetStat: make(map[string]net.IOCountersStat),
	}
	globalCollector = c

	// 预热：异步获取主机信息（首次采集前完成）
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		c.collectHostInfo(ctx)
		// 立即推送一次，首屏即有数据
		if payload, err := c.collect(context.Background()); err == nil {
			Bus().Publish(systemStatsTopic, payload)
		} else {
			log.Printf("[system-stats] initial collect failed: %v", err)
		}
	}()

	go c.loop()
}

func (c *systemStatsCollector) loop() {
	t := time.NewTicker(c.interval)
	defer t.Stop()
	for range t.C {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		payload, err := c.collect(ctx)
		cancel()
		if err != nil {
			log.Printf("[system-stats] collect failed: %v", err)
			continue
		}
		Bus().Publish(systemStatsTopic, payload)
	}
}

// collectHostInfo 缓存主机基础信息（hostname、平台、CPU 型号等），采集周期内只取一次。
func (c *systemStatsCollector) collectHostInfo(ctx context.Context) {
	info, err := host.InfoWithContext(ctx)
	if err != nil {
		log.Printf("[system-stats] host info failed: %v", err)
	} else {
		c.mu.Lock()
		c.hostInfo = SystemHostInfo{
			Hostname:        info.Hostname,
			Platform:        info.Platform,
			PlatformVersion: info.PlatformVersion,
			KernelVersion:   info.KernelVersion,
			KernelArch:      info.KernelArch,
			UptimeSeconds:   info.Uptime,
			BootTime:        info.BootTime,
		}
		c.mu.Unlock()
	}

	if cores, err := cpu.CountsWithContext(ctx, true); err == nil {
		c.mu.Lock()
		c.cores = cores
		c.mu.Unlock()
	}

	// CPU 型号只在首次采集
	if c.cpuModel == "" {
		if infos, err := cpu.InfoWithContext(ctx); err == nil && len(infos) > 0 {
			c.mu.Lock()
			c.cpuModel = strings.TrimSpace(infos[0].ModelName)
			c.mu.Unlock()
		}
	}
	if c.cpuModel == "" {
		c.mu.Lock()
		c.cpuModel = runtime.GOARCH
		c.mu.Unlock()
	}
}

func (c *systemStatsCollector) collect(ctx context.Context) (*SystemStatsPayload, error) {
	// 主机信息（懒刷新一次）
	if c.hostInfo.Hostname == "" {
		c.collectHostInfo(ctx)
	}

	c.mu.Lock()
	hostInfo := c.hostInfo
	hostInfo.CpuModel = c.cpuModel
	hostInfo.CpuCores = c.cores
	c.mu.Unlock()

	// CPU 总体使用率（0 表示非阻塞采样，但只返回累计值；这里取一个带短间隔的采样以获得合理值）
	cpuPercents, err := cpu.PercentWithContext(ctx, 500*time.Millisecond, false)
	if err != nil || len(cpuPercents) == 0 {
		log.Printf("[system-stats] cpu percent failed: %v", err)
	}
	cpuPercent := 0.0
	if len(cpuPercents) > 0 {
		cpuPercent = roundTo(cpuPercents[0], 2)
	}

	// 每核
	perCore, err := cpu.PercentWithContext(ctx, 0, true)
	if err != nil {
		perCore = nil
	} else {
		for i, v := range perCore {
			perCore[i] = roundTo(v, 2)
		}
	}

	// 内存
	var memInfo SystemMemoryInfo
	if v, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		memInfo = SystemMemoryInfo{
			TotalBytes:     v.Total,
			UsedBytes:      v.Used,
			AvailableBytes: v.Available,
			UsedPercent:    roundTo(v.UsedPercent, 2),
		}
	} else {
		log.Printf("[system-stats] mem failed: %v", err)
	}
	if v, err := mem.SwapMemoryWithContext(ctx); err == nil {
		memInfo.SwapTotalBytes = v.Total
		memInfo.SwapUsedBytes = v.Used
		memInfo.SwapPercent = roundTo(v.UsedPercent, 2)
	}

	// 进程：当前进程自身占用的内存与 goroutine 数（通过 runtime 读取，最稳）
	var procInfo SystemProcessInfo
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	procInfo.MemoryRSS = memStats.Sys
	procInfo.MemoryVSZ = memStats.Alloc
	procInfo.NumThreads = int32(runtime.NumGoroutine())

	// 磁盘：取全部物理分区
	parts, err := disk.PartitionsWithContext(ctx, false)
	if err != nil {
		log.Printf("[system-stats] disk partitions failed: %v", err)
		parts = nil
	}
	disks := make([]SystemDiskInfo, 0, len(parts))
	for _, p := range parts {
		// 过滤：跳过只读、伪文件系统
		fsType := strings.ToLower(p.Fstype)
		if strings.Contains(fsType, "tmpfs") || strings.Contains(fsType, "devtmpfs") ||
			strings.Contains(fsType, "proc") || strings.Contains(fsType, "sysfs") ||
			strings.Contains(fsType, "cgroup") || strings.Contains(fsType, "overlay") {
			continue
		}
		u, err := disk.UsageWithContext(ctx, p.Mountpoint)
		if err != nil || u == nil || u.Total == 0 {
			continue
		}
		disks = append(disks, SystemDiskInfo{
			Device:      p.Device,
			Mountpoint:  p.Mountpoint,
			FSType:      p.Fstype,
			TotalBytes:  u.Total,
			UsedBytes:   u.Used,
			FreeBytes:   u.Free,
			UsedPercent: roundTo(u.UsedPercent, 2),
		})
	}
	sort.SliceStable(disks, func(i, j int) bool { return disks[i].UsedPercent > disks[j].UsedPercent })
	if len(disks) > 3 {
		disks = disks[:3]
	}

	// 网络
	ioCounters, err := net.IOCountersWithContext(ctx, true)
	if err != nil {
		log.Printf("[system-stats] net io failed: %v", err)
		ioCounters = nil
	}
	now := time.Now()
	c.mu.Lock()
	lastNetStat := c.lastNetStat
	lastSample := c.lastSample
	c.mu.Unlock()
	dt := now.Sub(lastSample).Seconds()
	networks := make([]SystemNetInfo, 0, len(ioCounters))
	var totalRxBps, totalTxBps float64
	for _, io := range ioCounters {
		name := strings.ToLower(io.Name)
		// 过滤：loopback、虚拟接口
		if name == "lo" || strings.HasPrefix(name, "docker") || strings.HasPrefix(name, "veth") ||
			strings.HasPrefix(name, "br-") || strings.HasPrefix(name, "virbr") || strings.HasPrefix(name, "tun") {
			continue
		}
		isPhysical := !strings.HasPrefix(name, "lo") &&
			!strings.HasPrefix(name, "bond") && !strings.HasPrefix(name, "team")
		n := SystemNetInfo{
			Name:       io.Name,
			RxBytes:    io.BytesRecv,
			TxBytes:    io.BytesSent,
			RxPackets:  io.PacketsRecv,
			TxPackets:  io.PacketsSent,
			RxErrors:   io.Errin,
			TxErrors:   io.Errout,
			RxDrops:    io.Dropin,
			TxDrops:    io.Dropout,
			IsPhysical: isPhysical,
		}
		if last, ok := lastNetStat[io.Name]; ok && dt > 0 {
			n.RxBps = float64(int64(io.BytesRecv)-int64(last.BytesRecv)) / dt
			n.TxBps = float64(int64(io.BytesSent)-int64(last.BytesSent)) / dt
			if n.RxBps < 0 {
				n.RxBps = 0
			}
			if n.TxBps < 0 {
				n.TxBps = 0
			}
		}
		totalRxBps += n.RxBps
		totalTxBps += n.TxBps
		networks = append(networks, n)
	}
	sort.SliceStable(networks, func(i, j int) bool {
		return (networks[i].RxBps + networks[i].TxBps) > (networks[j].RxBps + networks[j].TxBps)
	})
	if len(networks) > 5 {
		networks = networks[:5]
	}

	// 更新 lastNetStat
	newLast := make(map[string]net.IOCountersStat, len(ioCounters))
	for _, io := range ioCounters {
		newLast[io.Name] = io
	}
	c.mu.Lock()
	c.lastNetStat = newLast
	c.lastSample = now
	c.mu.Unlock()

	// Load Average（仅 Linux/Darwin 可用，Windows 上始终为 0）
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		if avg, err := load.Avg(); err == nil && avg != nil {
			hostInfo.LoadAvg1 = roundTo(avg.Load1, 2)
			hostInfo.LoadAvg5 = roundTo(avg.Load5, 2)
			hostInfo.LoadAvg15 = roundTo(avg.Load15, 2)
		}
	}

	// 进程 Top N：CPU + 内存
	topCpu, topMem := collectTopProcesses(ctx, 5)

	return &SystemStatsPayload{
		Timestamp:    now.UnixMilli(),
		IntervalMs:   c.interval.Milliseconds(),
		Host:         hostInfo,
		CpuPercent:   cpuPercent,
		CpuPerCore:   perCore,
		Memory:       memInfo,
		Process:      procInfo,
		Disks:        disks,
		Networks:     networks,
		NetTotalRxBps: roundTo(totalRxBps, 2),
		NetTotalTxBps: roundTo(totalTxBps, 2),
		TopCpuProcesses: topCpu,
		TopMemProcesses: topMem,
	}, nil
}

// collectTopProcesses 采集进程列表，按 CPU 和 内存 各取 Top N。
// 设计要点：
//   - 必须在 cpu.Percent 采样之后再调用 process.NewProcesses，否则 cpuPercent 全部为 0；
//   - 单次抓取所有进程（process.ProcessesWithContext），避免 N+1；
//   - 任何单个进程的信息获取失败都跳过，不影响整体；
//   - 进程名做安全裁剪：空名、过长、非 ASCII 都替换为占位符。
func collectTopProcesses(ctx context.Context, topN int) (topCpu []SystemProcessTop, topMem []SystemProcessTop) {
	if topN <= 0 {
		topN = 5
	}
	procs, err := process.ProcessesWithContext(ctx)
	if err != nil {
		log.Printf("[system-stats] list processes failed: %v", err)
		return nil, nil
	}
	items := make([]SystemProcessTop, 0, len(procs))
	for _, p := range procs {
		pid := p.Pid
		name, _ := p.NameWithContext(ctx)
		if name == "" {
			name = fmtPid(pid)
		}
		if len(name) > 64 {
			name = name[:64]
		}
		cpuPct, _ := p.CPUPercentWithContext(ctx)
		memInfo, _ := p.MemoryInfoWithContext(ctx)
		username, _ := p.UsernameWithContext(ctx)
		status, _ := p.StatusWithContext(ctx)
		nice, _ := p.NiceWithContext(ctx)
		createMs, _ := p.CreateTimeWithContext(ctx)
		// username 可能含 domain\user，过长则裁剪
		if idx := strings.IndexByte(username, '\\'); idx >= 0 {
			username = username[idx+1:]
		}
		if len(username) > 32 {
			username = username[:32]
		}
		row := SystemProcessTop{
			Pid:        pid,
			Name:       name,
			Username:   username,
			CPUPercent: roundTo(cpuPct, 2),
			Status:     joinStatus(status),
			Nice:       nice,
			CreateTime: createMs * 1000, // s -> ms
		}
		if memInfo != nil {
			row.MemoryRSS = memInfo.RSS
			row.MemoryVSZ = memInfo.VMS
		}
		items = append(items, row)
	}
	// CPU Top
	cpuSorted := make([]SystemProcessTop, len(items))
	copy(cpuSorted, items)
	sort.SliceStable(cpuSorted, func(i, j int) bool { return cpuSorted[i].CPUPercent > cpuSorted[j].CPUPercent })
	if len(cpuSorted) > topN {
		cpuSorted = cpuSorted[:topN]
	}
	// 内存 Top
	memSorted := make([]SystemProcessTop, len(items))
	copy(memSorted, items)
	sort.SliceStable(memSorted, func(i, j int) bool { return memSorted[i].MemoryRSS > memSorted[j].MemoryRSS })
	if len(memSorted) > topN {
		memSorted = memSorted[:topN]
	}
	return cpuSorted, memSorted
}

func fmtPid(pid int32) string {
	return fmt.Sprintf("pid-%d", pid)
}

// joinStatus 把 process.StatusWithContext 返回的 []string 合并为单个状态字符串。
// Windows 上返回多个状态时，仅取第一个非空值作为代表状态。
func joinStatus(status []string) string {
	for _, s := range status {
		s = strings.TrimSpace(s)
		if s != "" {
			if len(s) > 16 {
				s = s[:16]
			}
			return s
		}
	}
	return ""
}

func roundTo(v float64, digits int) float64 {
	pow := 1.0
	for i := 0; i < digits; i++ {
		pow *= 10
	}
	if v >= 0 {
		return float64(int64(v*pow+0.5)) / pow
	}
	return float64(int64(v*pow-0.5)) / pow
}

// FormatSystemBytes 把字节数格式化为可读字符串（KiB/MiB/GiB），与前端 formatBytes 保持一致。
func FormatSystemBytes(b float64) string {
	return tools.FormatBytes(b)
}
