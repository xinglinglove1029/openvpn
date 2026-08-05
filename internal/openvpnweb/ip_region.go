package openvpnweb

import (
	"context"
	_ "embed"
	"strings"
	"sync"

	"github.com/lionsoul2014/ip2region/binding/golang/xdb"
)

//go:embed ip2region.xdb
var ip2RegionDB []byte

var (
	ipRegionSearcher *xdb.Searcher
	ipRegionOnce     sync.Once
	ipRegionErr      error
)

// InitIPRegion 初始化 ip2region 解析器
// xdbPath 参数保留用于兼容，实际优先使用内嵌的 xdb 数据
func InitIPRegion(xdbPath string) error {
	ipRegionOnce.Do(func() {
		if len(ip2RegionDB) > 0 {
			searcher, err := xdb.NewWithBuffer(xdb.IPv4, ip2RegionDB)
			if err != nil {
				ipRegionErr = err
				logger.Error(context.Background(), "[InitIPRegion] 从内嵌数据创建搜索器失败: %v", err)
				return
			}
			ipRegionSearcher = searcher
			logger.Info(context.Background(), "[InitIPRegion] ip2region 初始化成功（内嵌模式，%d 字节）", len(ip2RegionDB))
			return
		}

		// 回退：如果嵌入数据为空（理论上不会），尝试从外部文件加载
		if xdbPath == "" {
			logger.Info(context.Background(), "[InitIPRegion] 内嵌数据为空且未指定 xdb 路径，IP 解析功能已禁用")
			return
		}
		searcher, err := xdb.NewWithFileOnly(xdb.IPv4, xdbPath)
		if err != nil {
			ipRegionErr = err
			logger.Error(context.Background(), "[InitIPRegion] 从文件创建搜索器失败: %v", err)
			return
		}
		ipRegionSearcher = searcher
		logger.Info(context.Background(), "[InitIPRegion] ip2region 初始化成功（文件模式: %s）", xdbPath)
	})
	return ipRegionErr
}

// GetIPRegion 解析 IP 地址获取归属地信息
func GetIPRegion(ip string) string {
	if ipRegionSearcher == nil {
		return ""
	}

	ip = strings.TrimSpace(ip)
	if ip == "" {
		return ""
	}

	if !isValidIP(ip) {
		return ""
	}

	result, err := ipRegionSearcher.Search(ip)
	if err != nil {
		return ""
	}

	return result
}

// isValidIP 检查是否为需要解析的有效 IP
func isValidIP(ip string) bool {
	if strings.HasPrefix(ip, "127.") || ip == "::1" || ip == "0.0.0.0" {
		return false
	}

	privateRanges := []string{
		"10.",
		"172.16.", "172.17.", "172.18.", "172.19.",
		"172.20.", "172.21.", "172.22.", "172.23.",
		"172.24.", "172.25.", "172.26.", "172.27.",
		"172.28.", "172.29.", "172.30.", "172.31.",
		"192.168.",
		"169.254.",
		"fe80::",
		"fc", "fd",
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
	if ipRegionSearcher != nil {
		ipRegionSearcher.Close()
		ipRegionSearcher = nil
	}
	logger.Info(context.Background(), "[CloseIPRegion] ip2region 已关闭")
}
