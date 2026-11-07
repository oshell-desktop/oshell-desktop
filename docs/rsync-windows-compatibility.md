# Windows rsync 桥接修复

2026-11-03，归入已授权的 OShell 0.6.0 rsync 目录传输范围，关联 SF-F07/F10。修复用户报告的 cwRsync 3.5.1 上传 `EOF / code 12`，不新增协议或共享配置字段。

## 原因与修复

- cwRsync 把续传选项传成两个参数：`--partial-dir`、`.oshell-rsync-partial`。旧校验要求每项以 `-` 开头，错误拒绝目录名。现在支持分离及等号形式，仅允许程序固定的相对暂存目录；缺值、越界目录、额外位置参数、删除和原地写入仍拒绝。
- 修复参数后，真实客户端回归又复现标准输入 `The parameter is incorrect`。cwRsync/Cygwin 传入异步管道，而当前 Go 运行时为避免初始化阻塞，跳过原始 stdin 句柄的异步检测。仅在 rsync helper 中复制继承的 stdin 管道句柄，让 `os.NewFile` 正常检测 I/O 模式；返回时关闭私有副本，不关闭进程原始 stdin。非 Windows 和非标准输入保持原路径。相关 Go 背景：[issue 75949](https://go.dev/issue/75949)。
- 旧桥接把非 EOF 读取错误也当作正常输入结束；主调函数又只显示子进程的 code 12，丢失桥接原因。现在保留标准输入错误和桥接错误链，同时忽略由失败清理产生的取消/已关闭噪声。
- 后续截图中的 code 11、`/root/./ /data/...` 对应远程路径前导空格：表单原先只用 `TrimSpace` 判断非空，却把原始值交给传输层，使绝对路径变成相对的 `./ /data/...`。真实 cwRsync 已复现相同的 `mkdir ... No such file or directory`；去除首尾空白后，绝对路径上传和下载成功。现在只在手工输入的 rsync 表单边界清理远程路径首尾空白，校验、记录及执行使用同一值；目录名内部空格、引号、中文及来自文件浏览器的真实名称保持原语义。此修复不创建额外父目录、不更改远端权限。

## 验证边界

使用用户已安装的 cwRsync 3.5.1，通过本机合成 SSH 服务和同一条经过认证的连接运行真实 rsync sender/receiver：相对及绝对路径上传、下载、同名增量更新、目标额外文件保留、中文/空格/引号路径、取消保留非空片段及再次传输内容一致，均通过。绝对路径使用隔离临时目录的 Cygwin 盘符挂载，不映射或改写传给 receiver 的参数。显式坏参数回归能看到桥接原因，输入读取失败回归保留原始错误。

测试端的 SSH 服务在隔离目录直接启动真实 rsync，对生成的命令解码为 argv；它不是用户 Linux 服务器，不代表已验证用户服务器的 shell、权限或 rsync 安装。原路径引用及目标类型保护仍有独立单元回归。测试不读取真实凭据、不连接用户服务器。

证据位于 `validation/rsync-compat/`：`before.log`、`focused.log` 记录前轮桥接修复；`path-before.log` 记录本轮修复前表单回归失败，`path-real.log` 记录真实 cwRsync 路径错误复现及修正后上传/下载成功。本轮核心相关回归 24.265 秒、桌面传输相关回归 18.471 秒、`go vet` 通过，日志为 `core-regression.log`、`desktop-regression.log`、`vet.log`。包含规定的 Go 共享配置兼容样本；真实 Swift 互通未执行。

前轮 Linux 核心测试交叉编译通过，未计为 Linux 实机运行；本轮本机无可用 WSL 发行版，未补充 Linux 实机结果。本轮 Windows 单文件内嵌资源测试、ZIP CRC 与包内 EXE 哈希、独立空目录启动和内嵌 DLL 校验通过，隐藏测试窗口正常退出码 0。固定 `dist` 的 EXE/ZIP 已更新，11 个原配置/数据文件哈希不变。本轮暂存包和启动缓存已清理，保留精简证据；未提交、推送或触发 Actions。

本轮路径修复 EXE SHA-256：`d69726ef56fec8a427c2de7bfd74abac5aef90d2554f931f8d1eb6f80ae5d378`。
