# OpenVPN Connect 新建账号仍出现不支持选项

## 原因

v0.1.99 仅修复了两个 shell genclient 模板。页面新增账号、AI 创建客户端实际还会调用 Go generateClientConfigGo / generateOvpnFile；此模板仍写入 topology、resolv-retry、persist-key、persist-tun、explicit-exit-notify。配置预览接口的 normalizeClientTopology 还会给旧文件补写 topology subnet。

Windows OpenVPN Connect 日志明确报 UNUSED_OPTIONS_ERROR：topology 只允许由服务端推送，其余四项被标记为 ignored。删除重建账号仍使用同一段 Go 代码，因而不能消除问题。

## 修复

- Go 生成模板删除五类选项；统一 normalizeClientProfile 也清理自定义配置和历史文件中的同类指令。
- JSON 配置预览、用户自助下载、附件下载、手工配置保存和已有配置复用均进行兼容处理，下载不再补回 topology。
- 服务端保留 push "topology subnet"；客户端证书、私钥、tls-crypt、认证、MFA、自定义路由不变。
- 附件下载禁用缓存；已导入 OpenVPN Connect 的配置是独立副本，必须重新导入修复后的文件。

## 验证

测试真实 Go 客户端生成、UDP/TCP、带旧指令的自定义配置、MFA、证书保留、已有文件修复，以及附件下载修复并持久化旧文件。归一化测试验证 CRLF、注释、内联密钥和幂等性。旧实现的测试会复现 topology 被重新加入和 Go 生成仍包含不支持选项。
