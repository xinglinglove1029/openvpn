// 临时工具：重置 admin 密码并打印新哈希
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	dataDir := os.Getenv("OVPN_DATA")
	if dataDir == "" {
		dataDir = "."
	}

	newPass := "Admin@2026VPN"
	if len(os.Args) > 1 {
		newPass = os.Args[1]
	}

	// 生成 bcrypt 哈希（cost=12，与 config.go 保持一致）
	hash, err := bcrypt.GenerateFromPassword([]byte(newPass), 12)
	if err != nil {
		fmt.Println("生成哈希失败:", err)
		os.Exit(1)
	}

	// 读取并更新 config.json
	viper.SetConfigName("config")
	viper.SetConfigType("json")
	viper.AddConfigPath(dataDir)

	if err := viper.ReadInConfig(); err != nil {
		fmt.Println("读取配置失败:", err)
		os.Exit(1)
	}

	viper.Set("system.base.admin_password", string(hash))
	if err := viper.WriteConfig(); err != nil {
		fmt.Println("写入配置失败:", err)
		os.Exit(1)
	}

	// 验证：读回来再比对一次
	viper.ReadInConfig()
	stored := viper.GetString("system.base.admin_password")
	if bcrypt.CompareHashAndPassword([]byte(stored), []byte(newPass)) == nil {
		fmt.Println("✅ 密码重置成功并验证通过")
	} else {
		fmt.Println("❌ 验证失败")
		os.Exit(1)
	}

	fmt.Println("新哈希:", string(hash))
	fmt.Println("配置文件:", filepath.Join(dataDir, "config.json"))
	fmt.Println("登录密码:", newPass)
}
