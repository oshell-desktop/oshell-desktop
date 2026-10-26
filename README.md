# OShell Desktop

面向 Windows 和 Linux 的 SSH 终端与文件管理工具，使用 Go、mygo 原生 UI 和 Ghostty VT。支持与 macOS OShell 同步配置，本项目不构建 macOS 版本。

## 下载与运行

从 [GitHub Releases](https://github.com/oshell-desktop/oshell-desktop/releases/latest) 下载：

| 平台 | 安装包 | 运行方式 |
| --- | --- | --- |
| Windows amd64 | `OShell-Desktop-windows-amd64-single-file.zip` | 解压后运行 `OShell Desktop.exe` |
| Linux amd64 | `OShell-Desktop-linux-amd64.tar.gz` | 完整解压后运行 `oshell-desktop` |

Windows 单文件版会自动释放内嵌运行库，无需另放 DLL。配置保存在 EXE 同级目录；迁移时复制整个目录，手动升级时退出程序并只替换 EXE。

Linux 需要图形桌面、GTK 3 和 WebKitGTK。Debian / Ubuntu 可安装 `libwebkit2gtk-4.1-0`；请保留包内运行库和资源文件。

## 主要功能

- SSH、本地终端、分屏、标签组、多窗口和快速命令。
- 会话目录树、搜索、复制粘贴、导入导出、代理与跳板机。
- SFTP / FTP / SCP / ZMODEM 文件传输、独立限速和传输记录。
- SFTP 普通文件暂停、续传和失败重试。
- 主密码保护、共享目录 / WebDAV 同步、GitHub Release 自更新。
- 深浅主题、自动对比光标、自定义快捷键、选词方式和双宽字符。
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
