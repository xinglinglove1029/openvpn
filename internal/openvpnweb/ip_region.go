package openvpnweb

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/lionsoul2014/ip2region/binding/golang/service"
)

var (
	ipRegionService *service.Ip2Region
	ipRegionOnce    sync.Once
	ipRegionErr     error
)

// InitIPRegion 初始化 ip2region 解析器
// xdbPath 为 xdb 数据文件路径，若为空则尝试从默认路径加载
func InitIPRegion(xdbPath string) error {
	ipRegionOnce.Do(func() {
		if xdbPath == "" {
			// 尝试从可执行文件同目录加载
			execPath, err := os.Executable()
			if err == nil {
				xdbPath = filepath.Join(filepath.Dir(execPath), "ip2region.xdb")
			}
			
			// 如果默认路径不存在，尝试从 internal/openvpnweb 目录加载（开发模式）
			if _, err := os.Stat(xdbPath); os.IsNotExist(err) {
				xdbPath = filepath.Join("internal", "openvpnweb", "ip2region.xdb")
			}
		}

		// 检查文件是否存在
		if _, err := os.Stat(xdbPath); os.IsNotExist(err) {
			logger.Info(context.Background(), "[InitIPRegion] ip2region 数据文件不存在，IP 解析功能已禁用")
			return
		}

		// 使用默认配置（VIndexCache 缓存策略，20 个搜索实例）
		svc, err := service.NewIp2RegionWithPath(xdbPath, "")
		if err != nil {
			ipRegionErr = err
			logger.Error(context.Background(), "[InitIPRegion] 初始化 ip2region 失败: %v", err)
			return
		}
		ipRegionService = svc
		logger.Info(context.Background(), "[InitIPRegion] ip2region 初始化成功，数据文件: %s", xdbPath)
	})
	return ipRegionErr
}

// GetIPRegion 解析 IP 地址获取归属地信息
// 返回格式示例: "中国|0|广东省|深圳市|电信" 或 "未知IP"
func GetIPRegion(ip string) string {
	if ipRegionService == nil {
		return ""
	}
	
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return ""
	}

	// 过滤掉无效的 IP（如 127.0.0.1, 0.0.0.0, IPv6 本地地址等）
	if !isValidIP(ip) {
		return ""
	}

	result, err := ipRegionService.Search(ip)
	if err != nil {
		return ""
	}

	return result
}

// isValidIP 检查是否为需要解析的有效 IP（排除本地、私有、链路本地等）
func isValidIP(ip string) bool {
	// 本地回环地址
	if strings.HasPrefix(ip, "127.") || ip == "::1" || ip == "0.0.0.0" {
		return false
	}
	
	// 私有地址范围
	privateRanges := []string{
		"10.",      // 10.0.0.0/8
		"172.16.",  // 172.16.0.0/12 (部分)
		"172.17.",
		"172.18.",
		"172.19.",
		"172.20.",
		"172.21.",
		"172.22.",
		"172.23.",
		"172.24.",
		"172.25.",
		"172.26.",
		"172.27.",
		"172.28.",
		"172.29.",
		"172.30.",
		"172.31.",
		"192.168.", // 192.168.0.0/16
		"169.254.", // 链路本地
		"fe80::",   // IPv6 链路本地
		"fc",       // IPv6 私有地址
		"fd",       // IPv6 私有地址
	}
	
	lower := strings.ToLower(ip)
	for _, prefix := range privateRanges {
		if strings.HasPrefix(lower, strings.ToLower(prefix)) {
			return false
		}
	}
	
	return true
}

// CloseIPRegion 关闭 ip2region 解析器
func CloseIPRegion() {
	ipRegionService = nil
	logger.Info(context.Background(), "[CloseIPRegion] ip2region 已关闭")
}
