package openvpnweb

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path"
	"slices"
	"strings"
	"sync"

	"github.com/spf13/viper"
)

type VPNConfig struct {
	ConfigPath string
	Lines      []string
	mu         sync.Mutex
}

func initOvpnConfig() (*VPNConfig, error) {
	configPath := path.Join(ovData, "server.conf")

	f, err := os.Open(configPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return &VPNConfig{ConfigPath: configPath, Lines: lines}, scanner.Err()
}

func (cfg *VPNConfig) Get(key string) (val string) {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()

	keyPrefix := key + " "
	for _, line := range cfg.Lines {
		if strings.HasPrefix(line, keyPrefix) {
			return strings.TrimSpace(line[len(keyPrefix):])
		}
	}
	return ""
}

func (cfg *VPNConfig) Set(key, value string) {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()

	found := false
	keyPrefix := key + " "
	newLine := fmt.Sprintf("%s %s", key, value)

	for i, line := range cfg.Lines {
		trim := strings.TrimSpace(line)

		isComment := false
		if strings.HasPrefix(trim, "#") {
			isComment = true
			trim = strings.TrimSpace(trim[1:])
		}

		if key == "push" {
			if keyPrefix+value == trim {
				if isComment {
					cfg.Lines[i] = newLine
				}

				found = true
				break
			}
		} else {
			if strings.HasPrefix(trim, keyPrefix) {
				cfg.Lines[i] = newLine
				found = true
				break
			}
		}

	}

	if !found {
		cfg.Lines = append(cfg.Lines, fmt.Sprintf("%s %s", key, value))
	}
}

func (cfg *VPNConfig) SetLine(index int, content string) {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()

	if index >= 0 && index < len(cfg.Lines) {
		cfg.Lines[index] = content
	} else {
		cfg.Lines = append(cfg.Lines, content)
	}
}

// SetDNSPushResolvers replaces every existing pushed DNS resolver with the
// supplied resolver list. Keeping this operation atomic avoids stale third (or
// older) DNS lines when settings are saved repeatedly or upgraded from older
// server.conf layouts.
func (cfg *VPNConfig) SetDNSPushResolvers(resolvers ...string) {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()

	newLines := make([]string, 0, len(cfg.Lines)+len(resolvers))
	insertAt := -1
	for _, line := range cfg.Lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "#") {
			trim = strings.TrimSpace(strings.TrimPrefix(trim, "#"))
		}
		if strings.HasPrefix(trim, `push "dhcp-option DNS `) {
			if insertAt < 0 {
				insertAt = len(newLines)
			}
			continue
		}
		newLines = append(newLines, line)
	}
	if insertAt < 0 {
		insertAt = len(newLines)
	}

	pushLines := make([]string, 0, len(resolvers))
	seen := make(map[string]struct{}, len(resolvers))
	for _, raw := range resolvers {
		resolver := strings.TrimSpace(raw)
		if net.ParseIP(resolver) == nil {
			continue
		}
		if _, exists := seen[resolver]; exists {
			continue
		}
		seen[resolver] = struct{}{}
		pushLines = append(pushLines, fmt.Sprintf(`push "dhcp-option DNS %s"`, resolver))
	}

	cfg.Lines = append(newLines[:insertAt], append(pushLines, newLines[insertAt:]...)...)
}

func (cfg *VPNConfig) Delete(key string) {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()

	keyPrefix := key + " "

	var newLines []string
	for _, line := range cfg.Lines {
		trim := strings.TrimSpace(line)

		if strings.HasPrefix(trim, "push") {
			if key == trim {
				continue
			}
		} else {
			if strings.HasPrefix(trim, keyPrefix) {
				continue
			}
		}

		newLines = append(newLines, line)
	}

	cfg.Lines = newLines
}

func (cfg *VPNConfig) DeleteLines(indexes []int) {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()

	var newLines []string
	for i, line := range cfg.Lines {
		if !slices.Contains(indexes, i) {
			newLines = append(newLines, line)
		}
	}

	cfg.Lines = newLines
}

func (cfg *VPNConfig) Save() {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()
	os.WriteFile(cfg.ConfigPath, []byte(strings.Join(cfg.Lines, "\n")+"\n"), 0644)
}

// normalizeServerTopology removes directives that can reintroduce the legacy
// net30 layout after the startup migration has converted server.conf to an
// explicit subnet pool. The config watcher calls Update for every OpenVPN
// setting; calling cfg.Set("server", ...) there used to append a second
// server directive and made the effective topology depend on directive order.
func (cfg *VPNConfig) normalizeServerTopology() {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()

	hasServer := false
	hasModeServer := false
	hasIfconfigPool := false
	hasPushedTopology := false
	for _, line := range cfg.Lines {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") || strings.HasPrefix(trim, ";") {
			continue
		}
		fields := strings.Fields(trim)
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "server":
			hasServer = true
		case "push":
			if strings.HasPrefix(trim, `push "topology`) {
				hasPushedTopology = true
			}
		case "mode":
			hasModeServer = hasModeServer || (len(fields) > 1 && fields[1] == "server")
		case "ifconfig-pool":
			hasIfconfigPool = true
		}
	}

	// Only normalize configurations that are actually using an IPv4 server
	// pool. server-ipv6 is deliberately not considered here.
	if !hasServer && !(hasModeServer && hasIfconfigPool) {
		return
	}
	hasExplicitPool := hasModeServer && hasIfconfigPool

	newLines := make([]string, 0, len(cfg.Lines)+2)
	topologyWritten := false
	pushedTopologyWritten := false
	for _, line := range cfg.Lines {
		trim := strings.TrimSpace(line)
		if trim != "" && !strings.HasPrefix(trim, "#") && !strings.HasPrefix(trim, ";") {
			fields := strings.Fields(trim)
			if len(fields) > 0 {
				if fields[0] == "topology" {
					if !topologyWritten {
						newLines = append(newLines, "topology subnet")
						topologyWritten = true
					}
					continue
				}
				if fields[0] == "push" && strings.HasPrefix(trim, `push "topology`) {
					if !pushedTopologyWritten {
						newLines = append(newLines, `push "topology subnet"`)
						pushedTopologyWritten = true
					}
					continue
				}
				// A legacy server helper must not coexist with the explicit
				// mode/ifconfig-pool layout. Keep server-ipv6 untouched because
				// it has a different directive name.
				if hasExplicitPool && fields[0] == "server" {
					continue
				}
			}
		}
		newLines = append(newLines, line)
	}
	if !topologyWritten {
		newLines = append([]string{"topology subnet"}, newLines...)
	}
	if hasExplicitPool && !hasPushedTopology && !pushedTopologyWritten {
		newLines = append(newLines, `push "topology subnet"`)
	}
	cfg.Lines = newLines
}

func (cfg *VPNConfig) Update(key string, val string) {
	switch key {
	case "openvpn.ovpn_port":
		cfg.Set("port", val)
	case "openvpn.ovpn_proto":
		cfg.Set("proto", val)
	case "openvpn.ovpn_max_clients":
		cfg.Set("max-clients", val)
	case "openvpn.ovpn_subnet":
		oldSubnet := cfg.Get("server")
		ip, ipnet, err := net.ParseCIDR(val)
		if err != nil {
			logger.Error(context.Background(), err.Error())
			return
		}
		val = fmt.Sprintf("%s %s", ip.String(), net.IP(ipnet.Mask).String())
		cfg.Set("server", val)

		ipt := "iptables-nft"
		checkCmd := exec.Command("iptables-legacy", "-L", "-n", "-t", "nat")
		if err := checkCmd.Run(); err == nil {
			ipt = "iptables-legacy"
		}

		if oldSubnet != "" && oldSubnet != val {
			getOldCmd := exec.Command(ipt, "-t", "nat", "-C", "POSTROUTING", "-s", strings.ReplaceAll(oldSubnet, " ", "/"), "-j", "MASQUERADE")
			if err := getOldCmd.Run(); err == nil {
				delOldCmd := exec.Command(ipt, "-t", "nat", "-D", "POSTROUTING", "-s", strings.ReplaceAll(oldSubnet, " ", "/"), "-j", "MASQUERADE")
				if out, err := delOldCmd.CombinedOutput(); err != nil {
					if len(out) == 0 {
						out = []byte(err.Error())
					}
					logger.Error(context.Background(), string(out))
				}
			}
		}

		getCmd := exec.Command(ipt, "-t", "nat", "-C", "POSTROUTING", "-s", strings.ReplaceAll(val, " ", "/"), "-j", "MASQUERADE")
		if err := getCmd.Run(); err != nil {
			addCmd := exec.Command(ipt, "-t", "nat", "-A", "POSTROUTING", "-s", strings.ReplaceAll(val, " ", "/"), "-j", "MASQUERADE")
			if out, err := addCmd.CombinedOutput(); err != nil {
				if len(out) == 0 {
					out = []byte(err.Error())
				}
				logger.Error(context.Background(), string(out))
			}
		}
	case "openvpn.ovpn_gateway":
		if val == "true" {
			cfg.SetDNSPushResolvers(viper.GetString("openvpn.ovpn_push_dns1"), viper.GetString("openvpn.ovpn_push_dns2"))
			cfg.Set("push", `"redirect-gateway def1 ipv6 bypass-dhcp"`)
		} else {
			cfg.SetDNSPushResolvers()
			cfg.Delete(`push "redirect-gateway def1 ipv6 bypass-dhcp"`)
		}
	case "openvpn.ovpn_management":
		cfg.Set("management", strings.ReplaceAll(val, ":", " "))
	case "openvpn.ovpn_ipv6":
		ipt := "ip6tables-nft"
		checkCmd := exec.Command("ip6tables-legacy", "-L", "-n", "-t", "nat")
		if err := checkCmd.Run(); err == nil {
			ipt = "ip6tables-legacy"
		}

		if val == "true" {
			proto := conf.Openvpn.OvpnProto
			if !strings.HasSuffix(proto, "6") {
				proto = fmt.Sprintf("%s6", proto)
			}
			cfg.Set("proto", proto)
			cfg.Set("server-ipv6", conf.Openvpn.OvpnSubnet6)

			getCmd := exec.Command(ipt, "-t", "nat", "-C", "POSTROUTING", "-s", conf.Openvpn.OvpnSubnet6, "-j", "MASQUERADE")
			if err := getCmd.Run(); err != nil {
				addCmd := exec.Command(ipt, "-t", "nat", "-A", "POSTROUTING", "-s", conf.Openvpn.OvpnSubnet6, "-j", "MASQUERADE")
				if out, err := addCmd.CombinedOutput(); err != nil {
					if len(out) == 0 {
						out = []byte(err.Error())
					}
					logger.Error(context.Background(), string(out))
				}
			}
		} else {
			cfg.Set("proto", conf.Openvpn.OvpnProto)
			cfg.Delete("server-ipv6")

			getCmd := exec.Command(ipt, "-t", "nat", "-C", "POSTROUTING", "-s", conf.Openvpn.OvpnSubnet6, "-j", "MASQUERADE")
			if err := getCmd.Run(); err == nil {
				delCmd := exec.Command(ipt, "-t", "nat", "-D", "POSTROUTING", "-s", conf.Openvpn.OvpnSubnet6, "-j", "MASQUERADE")
				if out, err := delCmd.CombinedOutput(); err != nil {
					if len(out) == 0 {
						out = []byte(err.Error())
					}
					logger.Error(context.Background(), string(out))
				}
			}
		}
	case "openvpn.ovpn_subnet6":
		if viper.GetBool("openvpn.ovpn_ipv6") {
			oldSubnet6 := cfg.Get("server-ipv6")

			cfg.Set("server-ipv6", val)

			ipt := "ip6tables-nft"
			checkCmd := exec.Command("ip6tables-legacy", "-L", "-n", "-t", "nat")
			if err := checkCmd.Run(); err == nil {
				ipt = "ip6tables-legacy"
			}

			if oldSubnet6 != "" && oldSubnet6 != val {
				getOldCmd := exec.Command(ipt, "-t", "nat", "-C", "POSTROUTING", "-s", oldSubnet6, "-j", "MASQUERADE")
				if err := getOldCmd.Run(); err == nil {
					delOldCmd := exec.Command(ipt, "-t", "nat", "-D", "POSTROUTING", "-s", oldSubnet6, "-j", "MASQUERADE")
					if out, err := delOldCmd.CombinedOutput(); err != nil {
						if len(out) == 0 {
							out = []byte(err.Error())
						}
						logger.Error(context.Background(), string(out))
					}
				}
			}

			getCmd := exec.Command(ipt, "-t", "nat", "-C", "POSTROUTING", "-s", val, "-j", "MASQUERADE")
			if err := getCmd.Run(); err != nil {
				addCmd := exec.Command(ipt, "-t", "nat", "-A", "POSTROUTING", "-s", val, "-j", "MASQUERADE")
				if out, err := addCmd.CombinedOutput(); err != nil {
					if len(out) == 0 {
						out = []byte(err.Error())
					}
					logger.Error(context.Background(), string(out))
				}
			}
		}
	case "openvpn.ovpn_push_dns1", "openvpn.ovpn_push_dns2":
		if viper.GetBool("openvpn.ovpn_gateway") {
			cfg.SetDNSPushResolvers(viper.GetString("openvpn.ovpn_push_dns1"), viper.GetString("openvpn.ovpn_push_dns2"))
		}
	}

	cfg.normalizeServerTopology()
	cfg.Save()
}
