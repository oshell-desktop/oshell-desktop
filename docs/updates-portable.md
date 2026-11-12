# 更新与发布

## 程序内更新

点击工具栏“更新”，客户端会查询所选 GitHub 仓库的最新正式 Release。默认仓库为 `oshell-desktop/oshell-desktop`。发现新版后，选择“下载并安装”，客户端会校验安装包、替换程序并重新启动。有活动连接时会先询问是否关闭会话和传输。当前版本等于或高于线上版本时，更新页会显示检查结果。

在“设置 → 更新”可以更换仓库、启用每日检查或选择“仅下载更新”。每日检查只提示。选择仅下载后，可稍后点击“安装并重新启动”。

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
4. 发布前，仓库管理员需在 Actions Secrets 中配置 `OSHELL_UPDATE_SIGNING_KEY`，内容为与 `internal/core/update_public_key.txt` 匹配的 Ed25519 PKCS8 PEM 私钥。签名步骤从标准输入读取密钥。构建、测试和审计通过后，发布作业为两个最终附件签名，用客户端的验证逻辑回读清单，再创建 Release。缺少密钥或公私钥不匹配时，发布作业会在创建 Release 前停止。

| 附件 | 内容 |
| --- | --- |
| `OShell-Desktop-windows-amd64-single-file.zip` | 仅 `windows-amd64-single-file/OShell Desktop.exe` |
| `OShell-Desktop-linux-amd64.tar.gz` | `oshell-desktop`、`libghostty-vt.so` 及构建生成的资源 |
| `SHA256SUMS.txt` | 两个发布包的 SHA-256 |
| `latest.json` | 用内置公钥验证的版本、发布说明、平台、附件地址、大小、摘要及签名 |

更新器从该仓库的 GitHub latest Release 选择更高的正式三段版本，拒绝草稿、预发布、降级及跨仓库附件。附件须有 GitHub `sha256:` digest；缺失时应重新上传。下载校验长度、摘要及归属，解压拒绝越界路径、符号链接、重复路径和配置文件。

公开和私有仓库都从 `/repos/{owner}/{repo}/releases/latest` 查询。下载新版前要校验 Release 中的 Ed25519 签名清单和附件信息；收到安装包后再次校验长度、SHA-256 和签名，再解包。检查结果缓存五分钟，失败后至少退避一分钟。早期缺少 `latest.json` 的 Release 可参与版本比较，无法作为安装来源。仍使用旧 Pages 更新入口的客户端需手动换用支持 Release 更新的便携版一次。

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
