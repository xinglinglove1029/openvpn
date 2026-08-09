package openvpnweb

import (
	"context"
	"fmt"
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

// AIConfig AI 助手配置
type AIConfig struct {
	Enabled      bool    `json:"enabled" mapstructure:"enabled"`
	Provider     string  `json:"provider" mapstructure:"provider"`         // ollama | openai | deepseek | customize
	BaseURL      string  `json:"base_url" mapstructure:"base_url"`
	APIKey       string  `json:"api_key" mapstructure:"api_key"`          // API 密钥，GET 时返回脱敏值
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
	Openvpn OvpnConfig `json:"openvpn" mapstructure:"openvpn"`
	AI      AIConfig  `json:"ai" mapstructure:"ai"`
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

func initConfig() {
	sk := genRandomString(50)
	passwd, _ := bcrypt.GenerateFromPassword([]byte("admin"), 12)

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
	viper.SetDefault("openvpn.ovpn_gateway", false)
	viper.SetDefault("openvpn.ovpn_management", "127.0.0.1:7505")
	viper.SetDefault("openvpn.ovpn_ipv6", false)
	viper.SetDefault("openvpn.ovpn_subnet6", "fdaf:f178:e916:6dd0::/64")
	viper.SetDefault("openvpn.ovpn_push_dns1", "8.8.8.8")
	viper.SetDefault("openvpn.ovpn_push_dns2", "2001:4860:4860::8888")

	// AI 助手配置
	viper.SetDefault("ai.enabled", false)
	viper.SetDefault("ai.provider", "ollama")
	viper.SetDefault("ai.base_url", "http://127.0.0.1:11434")
	viper.SetDefault("ai.api_key", "")
	viper.SetDefault("ai.model", "qwen2.5:7b")
	viper.SetDefault("ai.system_prompt", "你是 OpenVPN 运维控制台的智能助手，具备全面的运维管理能力。\n\n你可以调用工具直接执行以下操作：\n\n## 用户管理\n- 创建用户（自动生成 .ovpn 客户端配置 + 发送开通邮件，与页面流程完全一致）\n- 列出用户、更新用户（启用/禁用、设有效期、固定IP）、删除用户\n- 重置密码、重置 MFA、绑定角色\n\n## VPN 客户端管理\n- 列出所有客户端配置、删除客户端（吊销证书）\n- 更新 CCD 配置（设置固定IP、推送路由）\n- 重新生成客户端配置、生成新客户端\n- 查看在线客户端、断开连接\n\n## 防火墙管理\n- 列出防火墙规则、拉黑/解黑 IP、设置/移除限速\n\n## 证书管理\n- 查看 CA 证书、CRL 吊销列表、已签发客户端证书的状态和有效期\n\n## 通知渠道管理\n- 列出/创建/更新/删除通知渠道（邮件、Webhook 等）\n\n## 审计与监控\n- 查询操作审计日志（按模块、操作类型筛选）\n- 获取系统仪表盘摘要（服务器状态、用户统计、在线数、风险项）\n\n## 绝对重要的使用原则（违反会导致误报）\n1. **禁止幻觉**：不要在工具未实际调用、或工具调用失败时告诉用户\"已执行完成\"。如果不确定工具是否真的执行了，必须先调用工具确认。\n2. **等待工具返回**：每次需要执行操作时，必须真正发出 function_call 并等待工具返回结果，再基于工具的实际返回内容（success 字段、message 字段）回答用户。\n3. **失败必须明示**：工具返回 success=false 或返回 error 时，必须明确告知用户失败原因，不得掩饰为\"已成功\"。\n4. **复合任务必须分别调用**：当用户要求\"先删除再创建\"或\"同时做多件事\"时，对每个动作都要单独调用对应工具；不要因为意图里同时含多个动作就只调用一部分。\n5. **查询用工具**：用户问\"系统有多少用户\"\"当前谁在线\"等问题，必须先调用工具获取实时数据，不要凭印象回答。\n6. **执行敏感操作前简要说明**：删除用户、断开连接等动作前先一句话告知用户。\n7. **权限不足直接告知**：工具返回权限不足时，直接告诉用户需要相应权限，不要反复重试。\n8. **用简洁专业的中文回答**")
	viper.SetDefault("ai.max_tokens", 4096)
	viper.SetDefault("ai.temperature", 0.7)

	viper.SetConfigName("config")
	viper.SetConfigType("json")
	viper.SetConfigPermissions(0600)
	viper.AddConfigPath(ovData)

	viper.SafeWriteConfig()

	err := viper.ReadInConfig()
	if err != nil {
		panic(err)
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

		loadConfig()
		upadteOvpnConfig()
	})

	viper.WatchConfig()
}

func upadteOvpnConfig() {
	if viper.GetBool("system.base.auto_update_ovpn_config") {
		cfg, err := initOvpnConfig()
		if err != nil {
			logger.Error(context.Background(), err.Error())
			return
		}

		for k, v := range viper.GetStringMap("openvpn") {
			if k == "ovpn_push_dns1" || k == "ovpn_push_dns2" {
				continue
			}

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
