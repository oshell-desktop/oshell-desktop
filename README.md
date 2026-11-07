# OShell Desktop

面向 Windows 和 Linux 的 SSH 终端与文件管理工具，使用 Go、mygo 原生 UI 和 Ghostty VT。支持与 macOS OShell 同步配置，本项目不构建 macOS 版本。

## 下载与运行

从 [GitHub Releases](https://github.com/oshell-desktop/oshell-desktop/releases/latest) 下载：

| 平台 | 安装包 | 运行方式 |
| --- | --- | --- |
| Windows amd64 | `OShell-Desktop-windows-amd64-single-file.zip` | 解压后运行 `OShell Desktop.exe` |
| Linux amd64 | `OShell-Desktop-linux-amd64.tar.gz` | 完整解压后运行 `oshell-desktop` |

Windows 单文件版会自动释放内嵌运行库，无需另放 DLL。配置保存在 EXE 同级目录；迁移时复制整个目录，手动升级时退出程序并只替换 EXE。

在 Windows 上使用安恒运维审计系统 USM 9.1.2 时，进入 **运维配置 → SSH / TELNET / Rlogin**，将本地客户端设为 **PuTTY**，编码选择 **UTF-8**。在 USM 选择本地 PuTTY 程序时，指定平时使用的 `OShell Desktop.exe`。从 USM 点击 SSH 资产即可打开临时标签页，无需另建 OShell 会话。首次连接仍需核对主机指纹。

该入口也识别 USM 的 Xshell SSH URL、SecureCRT 显式 SSH 参数和 MobaXterm 的 `sshpass` SSH 参数模板。SecureCRT 的 `/SCRIPT` 和依赖原客户端界面自动化的调用方式请改用 PuTTY 模式。

使用 Windows 单点登录控件 SsoTool 5.3.1.3 时，打开随控件安装的 **SsoDBSettings**，在 **Xshell** 一栏选择 `OShell Desktop.exe` 并保存。网页运维时仍选择 Xshell。OShell 会在控件删除临时会话文件前读取 SSH 地址和票据，并在现有窗口打开标签。Windows EXE 的文件主版本设为 6，供控件选择 Xshell 6 会话格式；产品版本仍显示 OShell 的实际版本。

Linux 需要图形桌面、GTK 3 和 WebKitGTK。Debian / Ubuntu 可安装 `libwebkit2gtk-4.1-0`；请保留包内运行库和资源文件。

## 主要功能

- SSH、本地终端、分屏、标签组、多窗口和快速命令。
- 会话目录树、搜索、复制粘贴、导入导出、代理与跳板机。
- SFTP / FTP / SCP / ZMODEM 文件传输、独立限速和传输记录。
- SFTP 文件和目录断点续传，支持重启后恢复、旧片段校验和同名文件另存。
- 主密码保护、共享目录 / WebDAV 同步、GitHub Release 自更新。
- 深浅主题、自定义光标配色、自定义快捷键、选词方式和双宽字符。
- 图片粘贴路径、会话置顶、本机标题设置、全心专注及独立同步进度窗口。

头部栏提供会话管理、文件管理、侧栏显隐、设置和更新按钮。常用操作和功能限制见 [使用指南](docs/usage.md)。

## 本地构建

需要 Go 1.27.1+、Python 3 和 Git；首次构建下载已校验的 Zig 工具链及依赖，后续复用缓存。

```powershell
# Windows 单文件便携包
python scripts/build.py --target windows/amd64 --single-file --archive
```

```sh
# Linux 运行包
python3 scripts/build.py --target linux/amd64 --archive
```

产物统一位于 `dist/`。输出目录已有用户配置时，脚本会拒绝覆盖；请先构建到干净的暂存目录，再只替换程序文件。开发运行、测试及容器构建见 [开发指南](docs/development.md)。

## 文档

- [使用指南](docs/usage.md)：会话、终端、文件管理和数据存储。
- [配置同步](docs/shared-configuration.md)：与 macOS OShell 接入同一同步目标。
- [更新与发布](docs/updates-portable.md)：升级、回滚和发布包要求。
- [开发指南](docs/development.md) / [CI 说明](docs/ci-performance.md)：构建、测试与缓存。
- [共享配置协议 v2](docs/configuration-protocol-v2.md)：开发者字段及兼容规则。

许可见 [LICENSE](LICENSE) 和 [第三方声明](THIRD_PARTY_NOTICES.md)。
