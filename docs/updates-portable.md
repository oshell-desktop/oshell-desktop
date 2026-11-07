# 更新与发布

## 程序内更新

在“设置 → 更新”选择 GitHub 仓库并检查更新，默认 `oshell-desktop/oshell-desktop`。每日自动检查只提示，下载和安装仍需确认；安装会关闭连接和传输。

支持 Windows amd64 单文件便携 ZIP 和 Linux amd64 tar.gz 运行包。程序目录必须可写；Linux 包管理器安装或系统目录请使用对应的更新方式。

公开仓库无需令牌。私有仓库先应用仓库地址，再保存具有 Contents 读取权限的令牌。令牌按仓库加密存入本机 `update-auth/`，不回显、不参与配置同步，可替换或清除。

## 配置保留与恢复

更新只替换程序及允许的运行资源，保留配置、凭据、密钥、主机指纹和同步状态。下载、校验及备份使用程序目录下的 `.oshell-update-*` 临时目录。

新版启动并完成窗口、配置、密码库初始化且版本匹配后，才确认升级。启动失败、提前退出或 90 秒内未确认，会恢复旧版并重新启动；Windows 更新过程不弹 cmd 窗口。

若权限、杀毒软件或文件占用阻止恢复，保留备份和事务日志，并提示所在目录。解决原因后再次启动以重试；不要提前删除备份。结果可在“设置 → 更新”查看。

## 发布步骤

1. 更新 `mygo.json` 的三段正式版本号，例如 `0.3.2`。
2. 按用户要求提交代码。创建对应的附注标签（例如 `v0.3.2`），在标签说明中简要列出用户可见的变更，再推送提交和标签。发布工作流使用标签说明作为 Release 正文。
3. 用户明确要求后，在该标签上手动运行 **Build and test**。标签推送本身不会触发。
4. Windows / Linux 的构建、测试及分片审计全部通过后，工作流创建 Release；版本不匹配或 Release 已存在会拒绝发布。

| 附件 | 内容 |
| --- | --- |
| `OShell-Desktop-windows-amd64-single-file.zip` | 仅 `windows-amd64-single-file/OShell Desktop.exe` |
| `OShell-Desktop-linux-amd64.tar.gz` | `oshell-desktop`、`libghostty-vt.so` 及构建生成的资源 |
| `SHA256SUMS.txt` | 两个发布包的 SHA-256 |

更新器从该仓库的 GitHub latest Release 选择更高的正式三段版本，拒绝草稿、预发布、降级及跨仓库附件。附件须有 GitHub `sha256:` digest；缺失时应重新上传。下载校验长度、摘要及归属，解压拒绝越界路径、符号链接、重复路径和配置文件。

信任依据为 GitHub HTTPS API、仓库发布权限和附件摘要，不是独立发布者数字签名。API 缓存五分钟，失败后退避一分钟。

## 验证

```sh
go test ./internal/core -run 'Test(Release|Portable)'
```

覆盖版本选择、来源和摘要校验、取消、危险压缩包、配置保留与回滚。`TestPortableHelperLifecycle` 用真实子进程验证重启、崩溃和中断恢复。

Windows 实包验证：

```powershell
./scripts/test-update-package.ps1 -Executable <EXE> -FixtureExecutable <预编译的update-fixture.exe>
```

使用合成配置验证 GUI 启动、升级、配置哈希及密码解密，不使用真实用户数据。Linux 原生桌面验证由 Ubuntu CI 执行，本机 Windows 测试不能替代。
