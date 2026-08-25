package openvpnweb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
	"golang.org/x/crypto/bcrypt"
)

type SysBeseConfig struct {
	SiteUrl              string `json:"site_url" mapstructure:"site_url"`
	ServerAddr           string `json:"server_addr" mapstructure:"server_addr"`
	WebPort              string `json:"web_port" mapstructure:"web_port"`
	AdminUsername        string `json:"admin_username" mapstructure:"admin_username"`
	AdminPassword        string `json:"admin_password" mapstructure:"admin_password"`
	AutoUpdateOvpnConfig bool   `json:"auto_update_ovpn_config" mapstructure:"auto_update_ovpn_config"`
	MaxDuplicateLogin    int    `json:"max_duplicate_login" mapstructure:"max_duplicate_login"`
	HistoryMaxDays       int    `json:"history_max_days" mapstructure:"history_max_days"`
	RenewDays            int    `json:"renew_days" mapstructure:"renew_days"`
	ValidateClientConfig bool   `json:"validate_client_config" mapstructure:"validate_client_config"`
	WebAuditEnabled      bool   `json:"web_audit_enabled" mapstructure:"web_audit_enabled"`
	// WebAuditStrictDNS captures all ordinary DNS requests from tun0 rather than
	// only the configured upstream resolvers. It remains opt-in because it can
	// override a client's hard-coded DNS configuration.
	WebAuditStrictDNS bool `json:"web_audit_strict_dns" mapstructure:"web_audit_strict_dns"`
	// WebAuditBlockDoT blocks only tun0 TCP/853 while domain auditing is enabled.
	WebAuditBlockDoT bool `json:"web_audit_block_dot" mapstructure:"web_audit_block_dot"`
	// Suricata EVE import is intentionally disabled by default. The container
	// control process creates this persistent JSONL file only for local capture.
	SuricataEVEEnabled     bool   `json:"suricata_eve_enabled" mapstructure:"suricata_eve_enabled"`
	SuricataEVEPath        string `json:"suricata_eve_path" mapstructure:"suricata_eve_path"`
	SuricataEVEPollSeconds int    `json:"suricata_eve_poll_seconds" mapstructure:"suricata_eve_poll_seconds"`
	SuricataEVEMaxDays     int    `json:"suricata_eve_max_days" mapstructure:"suricata_eve_max_days"`
}

type SysLdapConfig struct {
	LdapAuth               bool   `json:"ldap_auth" mapstructure:"ldap_auth"`
	LdapUrl                string `json:"ldap_url" mapstructure:"ldap_url"`
	LdapBaseDn             string `json:"ldap_base_dn" mapstructure:"ldap_base_dn"`
	LdapUserAttribute      string `json:"ldap_user_attribute" mapstructure:"ldap_user_attribute"`
	LdapUserGroupFilter    bool   `json:"ldap_user_group_filter" mapstructure:"ldap_user_group_filter"`
	LdapUserGroupDn        string `json:"ldap_user_group_dn" mapstructure:"ldap_user_group_dn"`
	LdapUserAttrIpaddrName string `json:"ldap_user_attr_ipaddr_name" mapstructure:"ldap_user_attr_ipaddr_name"`
	LdapUserAttrConfigName string `json:"ldap_user_attr_config_name" mapstructure:"ldap_user_attr_config_name"`
	LdapBindUserDn         string `json:"ldap_bind_user_dn" mapstructure:"ldap_bind_user_dn"`
	LdapBindPassword       string `json:"ldap_bind_password" mapstructure:"ldap_bind_password"`
}

type SysEmailConfig struct {
	SendSubjectPrefix string  `json:"send_subject_prefix" mapstructure:"send_subject_prefix"`
	SendFrom          string  `json:"send_from" mapstructure:"send_from"`
	Host              string  `json:"host" mapstructure:"host"`
	Port              int     `json:"port" mapstructure:"port"`
	Username          string  `json:"username" mapstructure:"username"`
	Password          string  `json:"password" mapstructure:"password"`
	Security          *string `json:"security" mapstructure:"security"`
}

type SysNotifyConfig struct {
	Enabled    bool   `json:"enabled" mapstructure:"enabled"`
	Provider   string `json:"provider" mapstructure:"provider"`
	Webhook    string `json:"webhook" mapstructure:"webhook"`
	Secret     string `json:"secret" mapstructure:"secret"`
	MentionAll bool   `json:"mention_all" mapstructure:"mention_all"`
}

type ClientUrlConfig struct {
	Windows string `json:"windows" mapstructure:"windows"`
	Linux   string `json:"linux" mapstructure:"linux"`
	Macos   string `json:"macos" mapstructure:"macos"`
	Ios     string `json:"ios" mapstructure:"ios"`
	Android string `json:"android" mapstructure:"android"`
}

type OvpnConfig struct {
	OvpnPort       int    `json:"ovpn_port" mapstructure:"ovpn_port"`
	OvpnProto      string `json:"ovpn_proto" mapstructure:"ovpn_proto"`
	OvpnSubnet     string `json:"ovpn_subnet" mapstructure:"ovpn_subnet"`
	OvpnMaxClients int    `json:"ovpn_max_clients" mapstructure:"ovpn_max_clients"`
	OvpnGateway    bool   `json:"ovpn_gateway" mapstructure:"ovpn_gateway"`
	OvpnManagement string `json:"ovpn_management" mapstructure:"ovpn_management"`
	OvpnIpv6       bool   `json:"ovpn_ipv6" mapstructure:"ovpn_ipv6"`
	OvpnSubnet6    string `json:"ovpn_subnet6" mapstructure:"ovpn_subnet6"`
	OvpnPushDns1   string `json:"ovpn_push_dns1" mapstructure:"ovpn_push_dns1"`
	OvpnPushDns2   string `json:"ovpn_push_dns2" mapstructure:"ovpn_push_dns2"`
}

// DatabaseConfig 数据库配置
type DatabaseConfig struct {
	Type            string `json:"type" mapstructure:"type"` // sqlite | mysql | postgres
	Host            string `json:"host" mapstructure:"host"` // mysql/postgres 主机
	Port            int    `json:"port" mapstructure:"port"` // mysql/postgres 端口，0 表示按类型取默认（3306/5432）
	User            string `json:"user" mapstructure:"user"` // mysql/postgres 用户名
	Password        string `json:"password" mapstructure:"password"`
	Name            string `json:"name" mapstructure:"name"`         // mysql/postgres 数据库名
	Path            string `json:"path" mapstructure:"path"`         // sqlite 文件路径（相对 OVPN_DATA 或绝对路径）
	Charset         string `json:"charset" mapstructure:"charset"`   // mysql 字符集
	SSLMode         string `json:"ssl_mode" mapstructure:"ssl_mode"` // postgres sslmode
	MaxOpenConns    int    `json:"max_open_conns" mapstructure:"max_open_conns"`
	MaxIdleConns    int    `json:"max_idle_conns" mapstructure:"max_idle_conns"`
	ConnMaxLifetime int    `json:"conn_max_lifetime_seconds" mapstructure:"conn_max_lifetime_seconds"` // 秒
}

// AIConfig AI 助手配置
type AIConfig struct {
	Enabled      bool    `json:"enabled" mapstructure:"enabled"`
	Provider     string  `json:"provider" mapstructure:"provider"` // ollama | openai | deepseek | customize
	BaseURL      string  `json:"base_url" mapstructure:"base_url"`
	APIKey       string  `json:"api_key" mapstructure:"api_key"` // API 密钥，GET 时返回脱敏值
	Model        string  `json:"model" mapstructure:"model"`
	SystemPrompt string  `json:"system_prompt" mapstructure:"system_prompt"`
	MaxTokens    int     `json:"max_tokens" mapstructure:"max_tokens"`
	Temperature  float64 `json:"temperature" mapstructure:"temperature"`
}

type config struct {
	System struct {
		Base   SysBeseConfig   `json:"base" mapstructure:"base"`
		Ldap   SysLdapConfig   `json:"ldap" mapstructure:"ldap"`
		Email  SysEmailConfig  `json:"email" mapstructure:"email"`
		Notify SysNotifyConfig `json:"notify" mapstructure:"notify"`
	} `json:"system" mapstructure:"system"`
	Client struct {
		ClientUrl ClientUrlConfig `json:"client_url" mapstructure:"client_url"`
	} `json:"client" mapstructure:"client"`
	Openvpn  OvpnConfig     `json:"openvpn" mapstructure:"openvpn"`
	AI       AIConfig       `json:"ai" mapstructure:"ai"`
	Database DatabaseConfig `json:"database" mapstructure:"database"`
}

var (
	webPort                string
	secretKey              string
	adminUsername          string
	adminPassword          string
	ldapAuth               bool
	ldapURL                string
	ldapBaseDn             string
	ldapUserAttribute      string
	ldapUserGroupFilter    bool
	ldapUserGroupDn        string
	ldapUserAttrIpaddrName string
	ldapUserAttrConfigName string
	ldapBindUserDn         string
	ldapBindPassword       string
	nftTableName           string

	ovManage string
)

// initialAdminPassword reads a native-install bootstrap password from a root-only
// file. Docker keeps the historical admin/admin first-start behavior because it
// does not set OPENVPN_WEB_INITIAL_ADMIN_PASSWORD_FILE.
func initialAdminPassword() string {
	passwordFile := strings.TrimSpace(os.Getenv("OPENVPN_WEB_INITIAL_ADMIN_PASSWORD_FILE"))
	if passwordFile == "" {
		return "admin"
	}

	info, err := os.Stat(passwordFile)
	if err != nil {
		if os.IsNotExist(err) {
			panic(fmt.Sprintf("initial admin password file %s is required while config.json has no administrator password", passwordFile))
		}
		panic(fmt.Sprintf("read initial admin password file: %v", err))
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		panic(fmt.Sprintf("initial admin password file %s must be a root-only regular file", passwordFile))
	}

	password, err := os.ReadFile(passwordFile)
	if err != nil {
		panic(fmt.Sprintf("read initial admin password file: %v", err))
	}
	value := strings.TrimSpace(string(password))
	if len(value) < 16 {
		panic("initial admin password must contain at least 16 characters")
	}
	return value
}

// configHasAdminPassword reports whether an existing config already owns the
// admin credential. It lets native operators remove the bootstrap password
// file after the first successful initialization without weakening a future
// first-start after accidental data loss.
func configHasAdminPassword(configFile string) bool {
	content, err := os.ReadFile(configFile)
	if err != nil {
		if os.IsNotExist(err) {
			return false
		}
		panic(fmt.Sprintf("read config file %s: %v", configFile, err))
	}

	var raw struct {
		System struct {
			Base struct {
				AdminPassword string `json:"admin_password"`
			} `json:"base"`
		} `json:"system"`
	}
	if err := json.Unmarshal(content, &raw); err != nil {
		// ReadInConfig below remains responsible for reporting malformed JSON.
		// Treat it as incomplete here so native first-start never silently falls
		// back to a weak password while an explicit bootstrap file is configured.
		return false
	}
	return strings.TrimSpace(raw.System.Base.AdminPassword) != ""
}

func initConfig() {
	sk := genRandomString(50)
	initialPassword := "admin"
	if !configHasAdminPassword(filepath.Join(ovData, "config.json")) {
		initialPassword = initialAdminPassword()
	}
	passwd, err := bcrypt.GenerateFromPassword([]byte(initialPassword), 12)
	if err != nil {
		panic(fmt.Sprintf("hash initial admin password: %v", err))
	}

	viper.SetDefault("system.base.site_url", "http://127.0.0.1:8888")
	viper.SetDefault("system.base.web_port", "8888")
	viper.SetDefault("system.base.secret_key", sk)
	viper.SetDefault("system.base.server_cn", "ovpn_"+genRandomString(16))
	viper.SetDefault("system.base.server_name", "server_"+genRandomString(16))
	viper.SetDefault("system.base.admin_username", "admin")
	viper.SetDefault("system.base.admin_password", string(passwd))
	viper.SetDefault("system.base.auto_update_ovpn_config", false)
	viper.SetDefault("system.base.max_duplicate_login", 0)
	viper.SetDefault("system.base.validate_client_config", false)
	viper.SetDefault("system.base.history_max_days", 90)
	// DNS domain auditing is privacy-sensitive. Existing and fresh deployments
	// stay opt-in until an administrator explicitly enables it in Settings.
	viper.SetDefault("system.base.web_audit_enabled", false)
	// High-coverage domain-audit policies are explicit opt-ins. Upgrades retain
	// the legacy, resolver-scoped DNS audit behavior until an administrator opts in.
	viper.SetDefault("system.base.web_audit_strict_dns", false)
	viper.SetDefault("system.base.web_audit_block_dot", false)
	viper.SetDefault("system.base.suricata_eve_enabled", false)
	viper.SetDefault("system.base.suricata_eve_path", suricataBuiltInEVEPath)
	viper.SetDefault("system.base.suricata_eve_poll_seconds", 5)
	viper.SetDefault("system.base.suricata_eve_max_days", 0)
	viper.SetDefault("system.base.renew_days", 365)
	viper.SetDefault("system.base.nft_table_name", "openvpn-nft")
	viper.SetDefault("system.ldap.ldap_auth", false)
	viper.SetDefault("system.ldap.ldap_url", "ldap://example.org:389")
	viper.SetDefault("system.ldap.ldap_base_dn", "dc=example,dc=org")
	viper.SetDefault("system.ldap.ldap_user_attribute", "uid")
	viper.SetDefault("system.ldap.ldap_user_group_filter", false)
	viper.SetDefault("system.ldap.ldap_user_group_dn", "cn=vpn,ou=groups,dc=example,dc=org")
	viper.SetDefault("system.ldap.ldap_user_attr_ipaddr_name", "ipaddr")
	viper.SetDefault("system.ldap.ldap_user_attr_config_name", "config")
	viper.SetDefault("system.ldap.ldap_bind_user_dn", "cn=admin,dc=example,dc=org")
	viper.SetDefault("system.ldap.ldap_bind_password", "adminpassword")
	viper.SetDefault("system.email.send_subject_prefix", "【openvpn-web】")
	viper.SetDefault("system.email.send_from", "")
	viper.SetDefault("system.email.host", "")
	viper.SetDefault("system.email.port", 25)
	viper.SetDefault("system.email.username", "")
	viper.SetDefault("system.email.password", "")
	viper.SetDefault("system.email.security", nil)
	viper.SetDefault("system.notify.enabled", false)
	viper.SetDefault("system.notify.provider", "dingtalk")
	viper.SetDefault("system.notify.webhook", "")
	viper.SetDefault("system.notify.secret", "")
	viper.SetDefault("system.notify.mention_all", false)

	viper.SetDefault("client.client_url.windows", "https://openvpn.net/downloads/openvpn-connect-v3-windows.msi")
	viper.SetDefault("client.client_url.macos", "https://openvpn.net/downloads/openvpn-connect-v3-macos.dmg")
	viper.SetDefault("client.client_url.linux", "https://openvpn.net/openvpn-client-for-linux/")
	viper.SetDefault("client.client_url.android", "https://play.google.com/store/apps/details?id=net.openvpn.openvpn")
	viper.SetDefault("client.client_url.ios", "https://itunes.apple.com/us/app/openvpn-connect/id590379981?mt=8")

	viper.SetDefault("openvpn.ovpn_port", 1194)
	viper.SetDefault("openvpn.ovpn_proto", "udp")
	viper.SetDefault("openvpn.ovpn_subnet", "10.8.0.0/24")
	viper.SetDefault("openvpn.ovpn_max_clients", 200)
	viper.SetDefault("openvpn.ovpn_gateway", true)
	viper.SetDefault("openvpn.ovpn_management", "127.0.0.1:7505")
	viper.SetDefault("openvpn.ovpn_ipv6", false)
	viper.SetDefault("openvpn.ovpn_subnet6", "fdaf:f178:e916:6dd0::/64")
	viper.SetDefault("openvpn.ovpn_push_dns1", "8.8.8.8")
	viper.SetDefault("openvpn.ovpn_push_dns2", "2001:4860:4860::8888")

	// AI 助手配置
	viper.SetDefault("database.type", "sqlite")
	viper.SetDefault("database.path", "ovpn.db")
	viper.SetDefault("database.host", "127.0.0.1")
	viper.SetDefault("database.port", 0)
	viper.SetDefault("database.user", "root")
	viper.SetDefault("database.password", "")
	viper.SetDefault("database.name", "openvpn-web")
	viper.SetDefault("database.charset", "utf8mb4")
	viper.SetDefault("database.ssl_mode", "disable")
	viper.SetDefault("database.max_open_conns", 0)
	viper.SetDefault("database.max_idle_conns", 0)
	viper.SetDefault("database.conn_max_lifetime_seconds", 0)

	// A clean source checkout (including GitHub Actions) does not include the
	// runtime data directory because it contains credentials and PKI material.
	// Create it before asking Viper to write the first config file; otherwise
	// SafeWriteConfig fails silently and ReadInConfig panics with "Config File
	// config Not Found" during package initialization.
	if err := os.MkdirAll(ovData, 0750); err != nil {
		panic(fmt.Sprintf("create data directory %s: %v", ovData, err))
	}

	viper.SetConfigName("config")
	viper.SetConfigType("json")
	viper.SetConfigPermissions(0600)
	viper.AddConfigPath(ovData)

	if err := viper.SafeWriteConfig(); err != nil {
		var alreadyExists viper.ConfigFileAlreadyExistsError
		if !errors.As(err, &alreadyExists) {
			panic(fmt.Sprintf("write initial config in %s: %v", ovData, err))
		}
	}

	err = viper.ReadInConfig()
	if err != nil {
		panic(fmt.Sprintf("read config from %s: %v", ovData, err))
	}

	// Earlier releases could install a tun0 UDP/443 REJECT rule to force
	// HTTP/3 clients back to TCP for DNS-domain auditing. In production this
	// breaks or severely degrades Google, YouTube and other QUIC-first sites.
	// Retire the switch during upgrade so an existing config cannot preserve the
	// unsafe policy; the audit reconciler removes the old commented rules.
	if retireWebAuditQUICBlock() {
		if err := viper.WriteConfig(); err != nil {
			panic(fmt.Sprintf("disable retired web audit UDP/443 block: %v", err))
		}
	}

	if !viper.IsSet("system.base.token") {
		viper.Set("system.base.token", "ovpntoken"+genRandomString(16))
		viper.WriteConfig()
	}

	var lastEventTime time.Time
	viper.OnConfigChange(func(e fsnotify.Event) {
		now := time.Now()
		if now.Sub(lastEventTime) < 500*time.Millisecond {
			return
		}
		lastEventTime = now

		// External edits or an older cached UI must not re-enable the retired
		// QUIC block. Keep the persisted config converged as well.
		if retireWebAuditQUICBlock() {
			if err := viper.WriteConfig(); err != nil {
				fmt.Printf("disable retired web audit UDP/443 block: %v\n", err)
			}
		}

		loadConfig()
		upadteOvpnConfig()
		// Apply external config-file edits too. The reconciler is a no-op when
		// the DNS audit switch and upstream DNS values are unchanged.
		reconcileWebAuditDNSConfig()
		suricataEVE.reconcile()
	})

	viper.WatchConfig()
}

// retireWebAuditQUICBlock disables the legacy setting in memory. It returns
// true when callers should persist the migration. The web-audit runtime also
// hard-disables this policy, so a failed write never reintroduces blocking.
func retireWebAuditQUICBlock() bool {
	const key = "system.base.web_audit_block_udp_443"
	if !viper.GetBool(key) {
		return false
	}
	viper.Set(key, false)
	return true
}

func upadteOvpnConfig() {
	if viper.GetBool("system.base.auto_update_ovpn_config") {
		cfg, err := initOvpnConfig()
		if err != nil {
			logger.Error(context.Background(), err.Error())
			return
		}

		for k, v := range viper.GetStringMap("openvpn") {
			// Keep OpenVPN's pushed resolver addresses and the DNS audit redirect
			// targets in lockstep. VPNConfig.Update replaces the existing push lines.
			cfg.Update("openvpn."+k, fmt.Sprintf("%v", v))
		}

		cfg.Set("setenv auth_api", fmt.Sprintf("http://127.0.0.1:%s/login", webPort))
		cfg.Set("setenv ovpn_auth_api", fmt.Sprintf("http://127.0.0.1:%s/ovpn/login", webPort))
		cfg.Set("setenv ovpn_history_api", fmt.Sprintf("http://127.0.0.1:%s/ovpn/history", webPort))
		cfg.Save()
	}
}

func loadConfig() {
	secretKey = viper.GetString("system.base.secret_key")
	nftTableName = viper.GetString("system.base.nft_table_name")

	viper.Unmarshal(&conf)

	webPort = conf.System.Base.WebPort
	adminUsername = conf.System.Base.AdminUsername
	adminPassword = conf.System.Base.AdminPassword
	ldapAuth = conf.System.Ldap.LdapAuth
	ldapURL = conf.System.Ldap.LdapUrl
	ldapBaseDn = conf.System.Ldap.LdapBaseDn
	ldapUserAttribute = conf.System.Ldap.LdapUserAttribute
	ldapUserGroupFilter = conf.System.Ldap.LdapUserGroupFilter
	ldapUserGroupDn = conf.System.Ldap.LdapUserGroupDn
	ldapUserAttrIpaddrName = conf.System.Ldap.LdapUserAttrIpaddrName
	ldapUserAttrConfigName = conf.System.Ldap.LdapUserAttrConfigName
	ldapBindUserDn = conf.System.Ldap.LdapBindUserDn
	ldapBindPassword = conf.System.Ldap.LdapBindPassword

	ovManage = conf.Openvpn.OvpnManagement
}
