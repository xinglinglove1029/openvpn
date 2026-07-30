package openvpnweb

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	neturl "net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"
)

const (
	pkiDirName        = "pki"
	pkiPrivateDirName = "private"
	pkiIssuedDirName  = "issued"
	caCertFile        = "ca.crt"
	caKeyFile         = "ca.key"
	tcKeyFile         = "tc.key"
	serverCertFile    = "server.crt"
	serverKeyFile     = "server.key"
	crlFile           = "crl.pem"
)

func pkiDir() string {
	return filepath.Join(ovData, pkiDirName)
}

func pkiPrivateDir() string {
	return filepath.Join(pkiDir(), pkiPrivateDirName)
}

func pkiIssuedDir() string {
	return filepath.Join(pkiDir(), pkiIssuedDirName)
}

func caCertPath() string {
	return filepath.Join(pkiDir(), caCertFile)
}

func caKeyPath() string {
	return filepath.Join(pkiPrivateDir(), caKeyFile)
}

func tcKeyPath() string {
	return filepath.Join(pkiDir(), tcKeyFile)
}

func serverCertPath() string {
	return filepath.Join(pkiIssuedDir(), serverCertFile)
}

func serverKeyPath() string {
	return filepath.Join(pkiPrivateDir(), serverKeyFile)
}

func clientCertPath(name string) string {
	return filepath.Join(pkiIssuedDir(), name+".crt")
}

func clientKeyPath(name string) string {
	return filepath.Join(pkiPrivateDir(), name+".key")
}

func crlPath() string {
	return filepath.Join(pkiDir(), crlFile)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func ensurePKIDirs() error {
	dirs := []string{pkiDir(), pkiPrivateDir(), pkiIssuedDir()}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0700); err != nil {
			return err
		}
	}
	return nil
}

func generateECKey() (*ecdsa.PrivateKey, error) {
	return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
}

func encodePrivateKeyToPEM(key *ecdsa.PrivateKey) ([]byte, error) {
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}
	block := &pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: der,
	}
	return pem.EncodeToMemory(block), nil
}

func encodeCertToPEM(cert []byte) []byte {
	block := &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert,
	}
	return pem.EncodeToMemory(block)
}

func randomSerial() (*big.Int, error) {
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, err
	}
	return serialNumber, nil
}

func initPKI() error {
	if err := ensurePKIDirs(); err != nil {
		return err
	}

	serverName := viperGetString("system.base.server_name", "server")
	serverCN := viperGetString("system.base.server_cn", "OpenVPN CA")

	if !fileExists(caCertPath()) || !fileExists(caKeyPath()) {
		caKey, err := generateECKey()
		if err != nil {
			return fmt.Errorf("生成 CA 私钥失败: %w", err)
		}

		serialNumber, err := randomSerial()
		if err != nil {
			return err
		}

		caTemplate := &x509.Certificate{
			SerialNumber: serialNumber,
			Subject: pkix.Name{
				CommonName: serverCN,
			},
			NotBefore:             time.Now(),
			NotAfter:              time.Now().AddDate(1, 0, 0),
			KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
			BasicConstraintsValid: true,
			IsCA:                  true,
			MaxPathLenZero:        true,
		}

		caCertDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
		if err != nil {
			return fmt.Errorf("生成 CA 证书失败: %w", err)
		}

		caKeyPEM, err := encodePrivateKeyToPEM(caKey)
		if err != nil {
			return err
		}
		if err := os.WriteFile(caKeyPath(), caKeyPEM, 0600); err != nil {
			return err
		}

		if err := os.WriteFile(caCertPath(), encodeCertToPEM(caCertDER), 0644); err != nil {
			return err
		}
	}

	if !fileExists(serverCertPath()) || !fileExists(serverKeyPath()) {
		caCertPEM, err := os.ReadFile(caCertPath())
		if err != nil {
			return err
		}
		caKeyPEM, err := os.ReadFile(caKeyPath())
		if err != nil {
			return err
		}

		caCert, caKey, err := parseCertAndKey(caCertPEM, caKeyPEM)
		if err != nil {
			return fmt.Errorf("解析 CA 证书失败: %w", err)
		}

		serverKey, err := generateECKey()
		if err != nil {
			return fmt.Errorf("生成服务器私钥失败: %w", err)
		}

		serialNumber, err := randomSerial()
		if err != nil {
			return err
		}

		serverTemplate := &x509.Certificate{
			SerialNumber: serialNumber,
			Subject: pkix.Name{
				CommonName: serverName,
			},
			NotBefore: time.Now(),
			NotAfter:  time.Now().AddDate(1, 0, 0),
			KeyUsage:  x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
			ExtKeyUsage: []x509.ExtKeyUsage{
				x509.ExtKeyUsageServerAuth,
			},
			BasicConstraintsValid: true,
			IsCA:                  false,
		}

		serverCertDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caCert, &serverKey.PublicKey, caKey)
		if err != nil {
			return fmt.Errorf("生成服务器证书失败: %w", err)
		}

		serverKeyPEM, err := encodePrivateKeyToPEM(serverKey)
		if err != nil {
			return err
		}
		if err := os.WriteFile(serverKeyPath(), serverKeyPEM, 0600); err != nil {
			return err
		}

		if err := os.WriteFile(serverCertPath(), encodeCertToPEM(serverCertDER), 0644); err != nil {
			return err
		}
	}

	if !fileExists(tcKeyPath()) {
		tcKey := make([]byte, 256)
		if _, err := rand.Read(tcKey); err != nil {
			return fmt.Errorf("生成 TLS-Crypt 密钥失败: %w", err)
		}

		tcContent := fmt.Sprintf("#\n# 256 bit OpenVPN static key\n#\n-----BEGIN OpenVPN Static key V1-----\n%x\n-----END OpenVPN Static key V1-----\n", tcKey)
		if err := os.WriteFile(tcKeyPath(), []byte(tcContent), 0600); err != nil {
			return err
		}
	}

	if !fileExists(crlPath()) {
		if err := generateCRL(); err != nil {
			return err
		}
	}

	return nil
}

func generateCRL() error {
	caCertPEM, err := os.ReadFile(caCertPath())
	if err != nil {
		return err
	}
	caKeyPEM, err := os.ReadFile(caKeyPath())
	if err != nil {
		return err
	}

	caCert, caKey, err := parseCertAndKey(caCertPEM, caKeyPEM)
	if err != nil {
		return err
	}

	crlTemplate := &x509.RevocationList{
		Number:     big.NewInt(1),
		ThisUpdate: time.Now(),
		NextUpdate: time.Now().AddDate(1, 0, 0),
	}

	crlDER, err := x509.CreateRevocationList(rand.Reader, crlTemplate, caCert, caKey)
	if err != nil {
		return fmt.Errorf("生成 CRL 失败: %w", err)
	}

	block := &pem.Block{
		Type:  "X509 CRL",
		Bytes: crlDER,
	}
	return os.WriteFile(crlPath(), pem.EncodeToMemory(block), 0644)
}

func parseCertAndKey(certPEM, keyPEM []byte) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil {
		return nil, nil, fmt.Errorf("无法解析证书 PEM")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, nil, err
	}

	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, nil, fmt.Errorf("无法解析私钥 PEM")
	}

	var key *ecdsa.PrivateKey
	if parsedKey, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes); err == nil {
		if ecKey, ok := parsedKey.(*ecdsa.PrivateKey); ok {
			key = ecKey
		} else {
			return nil, nil, fmt.Errorf("PKCS#8 私钥不是 ECDSA 类型")
		}
	} else if ecKey, err2 := x509.ParseECPrivateKey(keyBlock.Bytes); err2 == nil {
		key = ecKey
	} else {
		return nil, nil, fmt.Errorf("解析私钥失败: PKCS#8: %v, EC: %v", err, err2)
	}

	return cert, key, nil
}

func generateClientCert(name string) error {
	if fileExists(clientCertPath(name)) && fileExists(clientKeyPath(name)) {
		return nil
	}

	if err := ensurePKIDirs(); err != nil {
		return err
	}

	if !fileExists(caCertPath()) || !fileExists(caKeyPath()) {
		if err := initPKI(); err != nil {
			return err
		}
	}

	caCertPEM, err := os.ReadFile(caCertPath())
	if err != nil {
		return err
	}
	caKeyPEM, err := os.ReadFile(caKeyPath())
	if err != nil {
		return err
	}

	caCert, caKey, err := parseCertAndKey(caCertPEM, caKeyPEM)
	if err != nil {
		return fmt.Errorf("解析 CA 证书失败: %w", err)
	}

	clientKey, err := generateECKey()
	if err != nil {
		return fmt.Errorf("生成客户端私钥失败: %w", err)
	}

	serialNumber, err := randomSerial()
	if err != nil {
		return err
	}

	clientTemplate := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: name,
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().AddDate(1, 0, 0),
		KeyUsage:  x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageClientAuth,
		},
		BasicConstraintsValid: true,
		IsCA:                  false,
	}

	clientCertDER, err := x509.CreateCertificate(rand.Reader, clientTemplate, caCert, &clientKey.PublicKey, caKey)
	if err != nil {
		return fmt.Errorf("生成客户端证书失败: %w", err)
	}

	clientKeyPEM, err := encodePrivateKeyToPEM(clientKey)
	if err != nil {
		return err
	}
	if err := os.WriteFile(clientKeyPath(name), clientKeyPEM, 0600); err != nil {
		return err
	}

	if err := os.WriteFile(clientCertPath(name), encodeCertToPEM(clientCertDER), 0644); err != nil {
		return err
	}

	return nil
}

func readFileContent(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func generateOvpnFile(name, serverAddr, port, proto string, ipv6 bool, customConfig string, mfa bool) error {
	caCert, err := readFileContent(caCertPath())
	if err != nil {
		return err
	}
	clientCert, err := readFileContent(clientCertPath(name))
	if err != nil {
		return err
	}
	clientKey, err := readFileContent(clientKeyPath(name))
	if err != nil {
		return err
	}
	tcKey, err := readFileContent(tcKeyPath())
	if err != nil {
		return err
	}

	serverName := viperGetString("system.base.server_name", "server")
	protoLine := proto
	if ipv6 && !strings.Contains(proto, "6") {
		protoLine = proto + "6"
	}

	authUserPass := "#auth-user-pass"
	serverConfPath := filepath.Join(ovData, "server.conf")
	if fileExists(serverConfPath) {
		confData, err := os.ReadFile(serverConfPath)
		if err == nil && strings.Contains(string(confData), "auth-user-pass-verify") {
			authUserPass = "auth-user-pass"
		}
	}

	mfaLine := ""
	if mfa {
		mfaLine = "static-challenge \"Enter MFA code\" 1"
	}

	ipv6Lines := ""
	if ipv6 {
		ipv6Lines = "tun-mtu 1400\nmssfix 1360"
	}

	explicitExitNotify := ""
	if strings.Contains(proto, "udp") {
		explicitExitNotify = "explicit-exit-notify"
	}

	ovpnContent := fmt.Sprintf(`client
proto %s
remote %s %s
dev tun
resolv-retry infinite
nobind
persist-key
persist-tun
remote-cert-tls server
verify-x509-name %s name
auth SHA256
%s
cipher AES-128-GCM
tls-client
tls-version-min 1.2
tls-cipher TLS-ECDHE-ECDSA-WITH-AES-128-GCM-SHA256
verb 3
%s
%s
%s

## Custom configuration ##
%s
## end ##

<ca>
%s</ca>
<cert>
%s</cert>
<key>
%s</key>
<tls-crypt>
%s</tls-crypt>
`, protoLine, serverAddr, port, serverName, authUserPass, mfaLine, ipv6Lines, explicitExitNotify,
		customConfig, caCert, clientCert, clientKey, tcKey)

	clientsDir := filepath.Join(ovData, "clients")
	if err := os.MkdirAll(clientsDir, 0755); err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(clientsDir, name+".ovpn"), []byte(ovpnContent), 0644)
}

func viperGetString(key, defaultValue string) string {
	val := viper.GetString(key)
	if val == "" {
		return defaultValue
	}
	return val
}

func generateClientConfigGo(name, serverAddr, port, proto string, ipv6 bool, customConfig string, mfa bool) error {
	if name == "" {
		return fmt.Errorf("客户端名称不能为空")
	}

	clientsDir := filepath.Join(ovData, "clients")
	ovpnFile := filepath.Join(clientsDir, name+".ovpn")
	if fileExists(ovpnFile) {
		return nil
	}

	if err := generateClientCert(name); err != nil {
		return err
	}

	if err := generateOvpnFile(name, serverAddr, port, proto, ipv6, customConfig, mfa); err != nil {
		return err
	}

	return nil
}

func parseIPv6(s string) bool {
	ip := net.ParseIP(s)
	return ip != nil && ip.To4() == nil
}

// generateClientConfig 自动生成客户端配置（仅传客户端名称）
// 这是 generateClientConfigGo 的包装函数：从 viper 配置中
// 读取服务器地址、端口、协议、IPv6 等参数，再调用底层生成逻辑。
// 用于"添加用户时自动创建客户端"场景。
// mfaEnabled: 是否在客户端配置中包含 static-challenge（MFA 验证）
func generateClientConfig(name string, mfaEnabled bool) error {
	if name == "" {
		return fmt.Errorf("客户端名称不能为空")
	}

	// 1) 服务器地址：优先用 system.base.server_addr；否则从 site_url 解析 host；
	//    都没有则回退到 server.conf 中的 local 指令；最后兜底 127.0.0.1
	serverAddr := strings.TrimSpace(viper.GetString("system.base.server_addr"))
	if serverAddr == "" {
		siteURL := viper.GetString("system.base.site_url")
		if siteURL != "" {
			if u, err := neturl.Parse(siteURL); err == nil && u.Hostname() != "" {
				serverAddr = u.Hostname()
			}
		}
	}
	if serverAddr == "" {
		if v, err := readServerConfKey("local"); err == nil && strings.TrimSpace(v) != "" {
			serverAddr = strings.TrimSpace(v)
		}
	}
	if serverAddr == "" {
		serverAddr = "127.0.0.1"
	}

	// 2) 端口/协议/IPv6：直接从 viper 读取（与 upadteOvpnConfig 等逻辑保持一致）
	port := strings.TrimSpace(viper.GetString("openvpn.ovpn_port"))
	if port == "" {
		port = "1194"
	}
	proto := strings.TrimSpace(viper.GetString("openvpn.ovpn_proto"))
	if proto == "" {
		proto = "udp"
	}
	ipv6 := viper.GetBool("openvpn.ovpn_ipv6")

	// 3) 自定义配置：暂留空（如需透传额外 client 配置可扩展）
	customConfig := ""

	return generateClientConfigGo(name, serverAddr, port, proto, ipv6, customConfig, mfaEnabled)
}

// readServerConfKey 从 server.conf 中读取指定指令的第一个参数
// 找不到返回空字符串和 nil 错误；文件不存在则返回对应错误。
func readServerConfKey(key string) (string, error) {
	serverConfPath := filepath.Join(ovData, "server.conf")
	data, err := os.ReadFile(serverConfPath)
	if err != nil {
		return "", err
	}
	keyPrefix := key + " "
	for _, line := range strings.Split(string(data), "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "#") || strings.HasPrefix(trim, ";") {
			continue
		}
		if strings.HasPrefix(trim, keyPrefix) {
			return strings.TrimSpace(trim[len(keyPrefix):]), nil
		}
	}
	return "", nil
}

func SetMFAInClientConfig(name string, mfaEnabled bool) error {
	if name == "" {
		return fmt.Errorf("客户端名称不能为空")
	}

	clientFilePath := filepath.Join(ovData, "clients", name+".ovpn")
	data, err := os.ReadFile(clientFilePath)
	if err != nil {
		return fmt.Errorf("读取客户端配置失败: %w", err)
	}

	content := string(data)
	mfaLine := "static-challenge \"Enter MFA code\" 1"

	lines := strings.Split(content, "\n")
	var result []string
	found := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "static-challenge") {
			found = true
			if mfaEnabled {
				result = append(result, mfaLine)
			}
			continue
		}
		result = append(result, line)
	}

	if mfaEnabled && !found {
		var newLines []string
		inserted := false
		for _, line := range result {
			newLines = append(newLines, line)
			trimmed := strings.TrimSpace(line)
			if trimmed == "auth SHA256" && !inserted {
				newLines = append(newLines, mfaLine)
				inserted = true
			}
		}
		result = newLines
	}

	newContent := strings.Join(result, "\n")
	if err := os.WriteFile(clientFilePath, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("写入客户端配置失败: %w", err)
	}

	return nil
}

func RegenerateUserClientConfigs(username string, mfaEnabled bool) ([]string, error) {
	if username == "" {
		return nil, fmt.Errorf("用户名不能为空")
	}

	var user User
	db.Where("username = ?", username).First(&user)

	var updated []string

	if user.OvpnConfig != "" {
		configName := strings.TrimSuffix(user.OvpnConfig, ".ovpn")
		if err := SetMFAInClientConfig(configName, mfaEnabled); err != nil {
			return updated, fmt.Errorf("更新配置 %s 失败: %w", configName, err)
		}
		updated = append(updated, configName)
	}

	clientsDir := filepath.Join(ovData, "clients")
	files, err := os.ReadDir(clientsDir)
	if err != nil {
		return updated, nil
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}
		name := strings.TrimSuffix(file.Name(), filepath.Ext(file.Name()))
		if name == username {
			alreadyUpdated := false
			for _, u := range updated {
				if u == name {
					alreadyUpdated = true
					break
				}
			}
			if !alreadyUpdated {
				if err := SetMFAInClientConfig(name, mfaEnabled); err != nil {
					return updated, fmt.Errorf("更新配置 %s 失败: %w", name, err)
				}
				updated = append(updated, name)
			}
		}
	}

	return updated, nil
}
