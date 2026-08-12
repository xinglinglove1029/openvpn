package openvpnweb

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
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
	revokedListFile   = "revoked.json"
)

type revokedEntry struct {
	SerialNumber string    `json:"serial"`
	Subject      string    `json:"subject"`
	RevokedAt    time.Time `json:"revoked_at"`
}

func revokedListPath() string {
	return filepath.Join(ovData, pkiDirName, revokedListFile)
}

// crlReloadPendingPath records that revoked.json has changed and the running
// OpenVPN process still needs a freshly generated CRL loaded. It is durable so
// a failed management reload can be retried after a request or process restart.
func crlReloadPendingPath() string {
	return filepath.Join(pkiDir(), "crl.reload-pending")
}

func markCRLReloadPending() error {
	return os.WriteFile(crlReloadPendingPath(), []byte(time.Now().UTC().Format(time.RFC3339Nano)), 0600)
}

func hasCRLReloadPending() bool {
	info, err := os.Lstat(crlReloadPendingPath())
	return err == nil && !info.IsDir()
}

func clearCRLReloadPending() error {
	err := os.Remove(crlReloadPendingPath())
	if err == nil || os.IsNotExist(err) {
		return nil
	}
	return err
}

func loadRevokedList() ([]revokedEntry, error) {
	var list []revokedEntry
	data, err := os.ReadFile(revokedListPath())
	if err != nil {
		if os.IsNotExist(err) {
			return list, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return list, nil
	}
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, err
	}
	return list, nil
}

func saveRevokedList(list []revokedEntry) error {
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(revokedListPath(), data, 0600)
}

// appendRevoked 将指定证书追加到吊销列表；若已存在相同序列号则不重复。
func appendRevoked(cert *x509.Certificate) error {
	list, err := loadRevokedList()
	if err != nil {
		return err
	}
	serial := cert.SerialNumber.String()
	for _, e := range list {
		if e.SerialNumber == serial {
			return nil
		}
	}
	list = append(list, revokedEntry{
		SerialNumber: serial,
		Subject:      cert.Subject.String(),
		RevokedAt:    time.Now(),
	})
	return saveRevokedList(list)
}

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

	revokedList, err := loadRevokedList()
	if err != nil {
		return fmt.Errorf("读取吊销列表失败: %w", err)
	}

	revokedCerts := make([]pkix.RevokedCertificate, 0, len(revokedList))
	for _, e := range revokedList {
		serial, ok := new(big.Int).SetString(e.SerialNumber, 10)
		if !ok {
			continue
		}
		revokedCerts = append(revokedCerts, pkix.RevokedCertificate{
			SerialNumber:   serial,
			RevocationTime: e.RevokedAt,
		})
	}

	nextCrlNumber := big.NewInt(1)
	if fileExists(crlPath()) {
		old, err := os.ReadFile(crlPath())
		if err == nil {
			block, _ := pem.Decode(old)
			if block != nil {
				if list, lerr := x509.ParseRevocationList(block.Bytes); lerr == nil && list.Number != nil {
					nextCrlNumber = new(big.Int).Add(list.Number, big.NewInt(1))
				}
			}
		}
	}

	crlTemplate := &x509.RevocationList{
		Number:              nextCrlNumber,
		ThisUpdate:          time.Now(),
		NextUpdate:          time.Now().AddDate(1, 0, 0),
		RevokedCertificates: revokedCerts,
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

// RevokeByName 按名称（CN）吊销证书：找到客户端或服务器证书，写入吊销列表并刷新 CRL。
// 名称匹配：优先级 clientCertPath(name) → serverCertFile（serverName 匹配）。
// CA 证书不允许被此函数吊销。
func RevokeByName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("证书名称不能为空")
	}

	caCertPEM, err := os.ReadFile(caCertPath())
	if err != nil {
		return err
	}
	caCert, _, err := parseCertAndKey(caCertPEM, []byte{})
	if err != nil {
		// 不强制要求加载 CA 私钥
		caBlock, _ := pem.Decode(caCertPEM)
		if caBlock == nil {
			return fmt.Errorf("解析 CA 证书失败")
		}
		caCert, err = x509.ParseCertificate(caBlock.Bytes)
		if err != nil {
			return err
		}
	}

	// 1) 客户端证书
	certPath := ""
	if fileExists(clientCertPath(name)) {
		certPath = clientCertPath(name)
	} else if name == viperGetString("system.base.server_name", "server") && fileExists(serverCertPath()) {
		certPath = serverCertPath()
	}

	if certPath == "" {
		// 找不到证书文件时，尝试遍历 issued 目录按 Subject CN 匹配
		entries, derr := os.ReadDir(pkiIssuedDir())
		if derr == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				fp := filepath.Join(pkiIssuedDir(), entry.Name())
				data, rerr := os.ReadFile(fp)
				if rerr != nil {
					continue
				}
				block, _ := pem.Decode(data)
				if block == nil {
					continue
				}
				c, perr := x509.ParseCertificate(block.Bytes)
				if perr != nil {
					continue
				}
				if c.Subject.CommonName == name && c.SerialNumber.Cmp(caCert.SerialNumber) != 0 {
					certPath = fp
					break
				}
			}
		}
	}
	if certPath == "" {
		return fmt.Errorf("未找到名称为 %q 的证书文件", name)
	}

	data, err := os.ReadFile(certPath)
	if err != nil {
		return fmt.Errorf("读取证书文件失败: %w", err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return fmt.Errorf("无法解析证书 PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("解析证书失败: %w", err)
	}
	if cert.SerialNumber.Cmp(caCert.SerialNumber) == 0 {
		return fmt.Errorf("不能吊销 CA 证书")
	}

	if err := appendRevoked(cert); err != nil {
		return fmt.Errorf("写入吊销记录失败: %w", err)
	}
	// Keep a durable marker until the running daemon accepts the refreshed CRL.
	// If generation or SIGHUP fails, later cleanup calls can retry safely.
	if err := markCRLReloadPending(); err != nil {
		return fmt.Errorf("mark CRL pending reload: %w", err)
	}
	if err := generateCRL(); err != nil {
		return fmt.Errorf("刷新 CRL 失败: %w", err)
	}
	return nil
}

// renewX509Cert 续签（重新签发）一个证书：
// - oldCert：原证书（决定 Subject、SAN、KeyUsage 等字段）
// - oldKey 可以为 nil；为 nil 时重新生成 P-256 密钥对
// - issuerCert / issuerKey：签发者（自签名 CA 则 issuerCert = 生成模板）
// - days：新有效期天数
// 返回新的 PEM 证书字节、私钥 PEM 字节、错误。
func renewX509Cert(oldCert *x509.Certificate, oldKey *ecdsa.PrivateKey, issuerCert *x509.Certificate, issuerKey *ecdsa.PrivateKey, days int) (certPEM []byte, keyPEM []byte, err error) {
	if days <= 0 {
		return nil, nil, fmt.Errorf("续签天数必须大于 0")
	}

	key := oldKey
	if key == nil {
		k, e := generateECKey()
		if e != nil {
			return nil, nil, fmt.Errorf("生成私钥失败: %w", e)
		}
		key = k
	}

	serial, err := randomSerial()
	if err != nil {
		return nil, nil, err
	}

	isSelfSignedCA := oldCert.IsCA && issuerCert != nil && oldCert.SerialNumber.Cmp(issuerCert.SerialNumber) == 0

	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               oldCert.Subject,
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(0, 0, days),
		KeyUsage:              oldCert.KeyUsage,
		BasicConstraintsValid: true,
		IsCA:                  oldCert.IsCA,
		MaxPathLen:            oldCert.MaxPathLen,
		MaxPathLenZero:        oldCert.MaxPathLenZero,
		ExtKeyUsage:           oldCert.ExtKeyUsage,
		UnknownExtKeyUsage:    oldCert.UnknownExtKeyUsage,
		DNSNames:              append([]string(nil), oldCert.DNSNames...),
		IPAddresses:           append([]net.IP(nil), oldCert.IPAddresses...),
		URIs:                  append([]*neturl.URL(nil), oldCert.URIs...),
		EmailAddresses:        append([]string(nil), oldCert.EmailAddresses...),
		PermittedDNSDomains:   append([]string(nil), oldCert.PermittedDNSDomains...),
		ExcludedDNSDomains:    append([]string(nil), oldCert.ExcludedDNSDomains...),
		PermittedIPRanges:     append([]*net.IPNet(nil), oldCert.PermittedIPRanges...),
		ExcludedIPRanges:      append([]*net.IPNet(nil), oldCert.ExcludedIPRanges...),
	}

	signingCert := issuerCert
	signingKey := issuerKey
	if isSelfSignedCA {
		// CA 自签名续签：签发者就是新证书本身（template），但签名私钥必须是 CA 的私钥（此处由调用方从 caKeyPath 读出后传入，作为 issuerKey）
		signingCert = template
		signingKey = issuerKey
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, signingCert, &key.PublicKey, signingKey)
	if err != nil {
		return nil, nil, fmt.Errorf("签发证书失败: %w", err)
	}

	keyBytes, err := encodePrivateKeyToPEM(key)
	if err != nil {
		return nil, nil, err
	}

	return encodeCertToPEM(certDER), keyBytes, nil
}

// writeCertAndKey 将新证书与私钥写入原位置；如果原证书存在则先写入吊销（CA 除外）。
func writeCertAndKey(certPath, keyPath string, certPEM, keyPEM []byte, oldCert *x509.Certificate) error {
	if oldCert != nil && !oldCert.IsCA {
		// revoke-renewed 语义：旧证书列入吊销
		if err := appendRevoked(oldCert); err != nil {
			return fmt.Errorf("写入旧证书吊销记录失败: %w", err)
		}
	}
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		return fmt.Errorf("写入私钥失败: %w", err)
	}
	if err := os.WriteFile(certPath, certPEM, 0644); err != nil {
		return fmt.Errorf("写入证书失败: %w", err)
	}
	return nil
}

// RenewCA 续签 CA 证书：保持私钥不变，生成新 CA 证书（自签名）。
// 如存在 server 证书，会同步续签服务器证书（吊销旧服务器证书）并刷新 CRL。
func RenewCA(days int) (messages []string, err error) {
	messages = make([]string, 0, 4)
	if days <= 0 {
		return messages, fmt.Errorf("续签天数必须大于 0")
	}

	caCertPEM, err := os.ReadFile(caCertPath())
	if err != nil {
		return messages, fmt.Errorf("读取 CA 证书失败: %w", err)
	}
	caKeyPEM, err := os.ReadFile(caKeyPath())
	if err != nil {
		return messages, fmt.Errorf("读取 CA 私钥失败: %w", err)
	}
	caCert, caKey, err := parseCertAndKey(caCertPEM, caKeyPEM)
	if err != nil {
		return messages, fmt.Errorf("解析 CA 证书/私钥失败: %w", err)
	}

	newCaPEM, _, err := renewX509Cert(caCert, caKey, caCert, caKey, days)
	if err != nil {
		return messages, fmt.Errorf("续签 CA 失败: %w", err)
	}
	// CA 私钥保持不变，私钥文件不重写
	if err := os.WriteFile(caCertPath(), newCaPEM, 0644); err != nil {
		return messages, fmt.Errorf("写入 CA 证书失败: %w", err)
	}
	messages = append(messages, fmt.Sprintf("CA 证书续签成功（%d 天）", days))

	// 重新加载新 CA 证书作为后续签发者
	newCaBlock, _ := pem.Decode(newCaPEM)
	if newCaBlock == nil {
		return messages, fmt.Errorf("无法解析新 CA PEM")
	}
	newCaCert, err := x509.ParseCertificate(newCaBlock.Bytes)
	if err != nil {
		return messages, fmt.Errorf("解析新 CA 证书失败: %w", err)
	}

	// 同步续签服务器证书
	serverName := viperGetString("system.base.server_name", "server")
	if serverName != "" && fileExists(serverCertPath()) && fileExists(serverKeyPath()) {
		oldServPEM, err := os.ReadFile(serverCertPath())
		if err != nil {
			return messages, fmt.Errorf("读取服务器证书失败: %w", err)
		}
		oldServKeyPEM, err := os.ReadFile(serverKeyPath())
		if err != nil {
			return messages, fmt.Errorf("读取服务器私钥失败: %w", err)
		}
		oldServCert, oldServKey, err := parseCertAndKey(oldServPEM, oldServKeyPEM)
		if err != nil {
			return messages, fmt.Errorf("解析服务器证书失败: %w", err)
		}
		servCertPEM, servKeyPEM, err := renewX509Cert(oldServCert, oldServKey, newCaCert, caKey, days)
		if err != nil {
			return messages, fmt.Errorf("续签服务器证书失败: %w", err)
		}
		if err := writeCertAndKey(serverCertPath(), serverKeyPath(), servCertPEM, servKeyPEM, oldServCert); err != nil {
			return messages, fmt.Errorf("写入服务器证书失败: %w", err)
		}
		messages = append(messages, fmt.Sprintf("服务器证书 %s 续签成功（%d 天）", serverName, days))
	}

	if err := generateCRL(); err != nil {
		return messages, fmt.Errorf("刷新 CRL 失败: %w", err)
	}
	messages = append(messages, "CRL 刷新成功")
	return messages, nil
}

// RenewServer 续签服务器证书（单独调用路径）。
func RenewServer(days int) (string, error) {
	if days <= 0 {
		return "", fmt.Errorf("续签天数必须大于 0")
	}
	serverName := viperGetString("system.base.server_name", "server")
	if serverName == "" || !fileExists(serverCertPath()) || !fileExists(serverKeyPath()) {
		return "", fmt.Errorf("服务器证书文件不存在")
	}
	caCertPEM, err := os.ReadFile(caCertPath())
	if err != nil {
		return "", err
	}
	caKeyPEM, err := os.ReadFile(caKeyPath())
	if err != nil {
		return "", err
	}
	caCert, caKey, err := parseCertAndKey(caCertPEM, caKeyPEM)
	if err != nil {
		return "", err
	}
	oldServPEM, err := os.ReadFile(serverCertPath())
	if err != nil {
		return "", err
	}
	oldServKeyPEM, err := os.ReadFile(serverKeyPath())
	if err != nil {
		return "", err
	}
	oldServCert, oldServKey, err := parseCertAndKey(oldServPEM, oldServKeyPEM)
	if err != nil {
		return "", err
	}
	servCertPEM, servKeyPEM, err := renewX509Cert(oldServCert, oldServKey, caCert, caKey, days)
	if err != nil {
		return "", fmt.Errorf("续签服务器证书失败: %w", err)
	}
	if err := writeCertAndKey(serverCertPath(), serverKeyPath(), servCertPEM, servKeyPEM, oldServCert); err != nil {
		return "", fmt.Errorf("写入服务器证书失败: %w", err)
	}
	if err := generateCRL(); err != nil {
		return "", fmt.Errorf("刷新 CRL 失败: %w", err)
	}
	return fmt.Sprintf("服务器证书 %s 续签成功（%d 天），CRL 已刷新", serverName, days), nil
}

// RenewByName 按名称续签证书：支持 CA、服务器证书、客户端证书。
// 返回操作摘要字符串与错误。
func RenewByName(name string, days int) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("证书名称不能为空")
	}
	if days <= 0 {
		return "", fmt.Errorf("续签天数必须大于 0")
	}

	// 加载 CA（签发证书需要）
	caCertPEM, err := os.ReadFile(caCertPath())
	if err != nil {
		return "", err
	}
	caKeyPEM, err := os.ReadFile(caKeyPath())
	if err != nil {
		return "", err
	}
	caCert, caKey, err := parseCertAndKey(caCertPEM, caKeyPEM)
	if err != nil {
		return "", err
	}

	// 匹配目标证书：按传入 CN 与 CA Subject 匹配 → CA；否则按 serverName；否则客户端文件
	serverName := viperGetString("system.base.server_name", "server")
	caSubject := caCert.Subject.CommonName

	switch {
	case name == caSubject:
		// CA：调用完整续签链路（含服务器联动 + CRL）
		msgs, err := RenewCA(days)
		if err != nil {
			return "", err
		}
		return strings.Join(msgs, "；"), nil

	case name == serverName:
		return RenewServer(days)

	default:
		// 客户端证书
		if !fileExists(clientCertPath(name)) {
			return "", fmt.Errorf("未找到客户端证书: %s", name)
		}
		if !fileExists(clientKeyPath(name)) {
			return "", fmt.Errorf("未找到客户端私钥: %s", name)
		}
		oldClientPEM, err := os.ReadFile(clientCertPath(name))
		if err != nil {
			return "", err
		}
		oldClientKeyPEM, err := os.ReadFile(clientKeyPath(name))
		if err != nil {
			return "", err
		}
		oldClientCert, oldClientKey, err := parseCertAndKey(oldClientPEM, oldClientKeyPEM)
		if err != nil {
			return "", fmt.Errorf("解析客户端证书失败: %w", err)
		}
		newCertPEM, newKeyPEM, err := renewX509Cert(oldClientCert, oldClientKey, caCert, caKey, days)
		if err != nil {
			return "", fmt.Errorf("续签客户端证书失败: %w", err)
		}
		if err := writeCertAndKey(clientCertPath(name), clientKeyPath(name), newCertPEM, newKeyPEM, oldClientCert); err != nil {
			return "", fmt.Errorf("写入客户端证书失败: %w", err)
		}
		if err := generateCRL(); err != nil {
			return "", fmt.Errorf("刷新 CRL 失败: %w", err)
		}
		return fmt.Sprintf("客户端证书 %s 续签成功（%d 天），旧证书已吊销，CRL 已刷新", name, days), nil
	}
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

	if len(keyPEM) == 0 {
		return cert, nil, nil
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

func isCertificateRevoked(cert *x509.Certificate) (bool, error) {
	revokedList, err := loadRevokedList()
	if err != nil {
		return false, err
	}

	serial := cert.SerialNumber.String()
	for _, entry := range revokedList {
		if entry.SerialNumber == serial {
			return true, nil
		}
	}
	return false, nil
}

// removeClientCredentials removes the revoked client's locally stored certificate and key.
// The revocation remains in revoked.json and the CRL, so removing these artifacts does not restore access.
func removeClientCredentials(name string) error {
	for _, path := range []string{clientCertPath(name), clientKeyPath(name)} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove client credential %q: %w", path, err)
		}
	}
	return nil
}

func generateClientCert(name string) error {
	if err := ensurePKIDirs(); err != nil {
		return err
	}

	if !fileExists(caCertPath()) || !fileExists(caKeyPath()) {
		if err := initPKI(); err != nil {
			return err
		}
	}

	// A deleted client is added to the CRL. Reusing its name must issue a new certificate;
	// otherwise the newly downloaded .ovpn embeds a certificate OpenVPN will reject.
	if fileExists(clientCertPath(name)) && fileExists(clientKeyPath(name)) {
		certPEM, err := os.ReadFile(clientCertPath(name))
		if err != nil {
			return fmt.Errorf("read existing client certificate: %w", err)
		}
		block, _ := pem.Decode(certPEM)
		if block == nil {
			return fmt.Errorf("parse existing client certificate PEM")
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return fmt.Errorf("parse existing client certificate: %w", err)
		}
		revoked, err := isCertificateRevoked(cert)
		if err != nil {
			return fmt.Errorf("check existing client certificate revocation status: %w", err)
		}
		if !revoked {
			return nil
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
