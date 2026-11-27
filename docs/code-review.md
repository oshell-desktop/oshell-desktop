# 代码审查与性能验证

2026-11-27。审查当前工作树，保留任务开始前已有的终端传输浮层改动。审查以现有功能为边界，检查连接取消与退出、文件传输、配置与凭据读写、共享同步、更新验签、输入资源限制和高频 UI 路径，并执行主模块及所用依赖的验证。有限检查不能证明所有输入和部署环境都没有缺陷。

## 已修复的问题

| 问题 | 影响与修复 | 回归入口 |
| --- | --- | --- |
| FTP 数据连接捕获首次目录读取的上下文 | 首次浏览结束取消上下文后，后续刷新和下载无法建立被动数据连接。将数据连接绑定到文件系统生命周期；取消打开仍会关闭连接，关闭文件系统也取消尚未完成的拨号。 | `TestFTPFilesOutliveOpeningContext`、`TestFTPOpenRejectsCanceledContext` |
| 代理握手回复与取消同时发生 | 已收到响应时可能把被取消、已经关闭的连接作为成功结果交给 SSH。交付前停止取消回调并核对上下文；SOCKS4 DNS 使用同一握手超时。 | `TestProxyHandshakeCancellationAtReply`、`TestProxyHandshakeDetachesSuccessfulContext` |
| Windows MCP 停止后无法重新监听 | 客户端仍持有旧管道句柄时，Windows 保留同名管道。服务每次开启发布新的本机端点，并用持有期文件锁拒绝重复服务；保留用户 SID、凭据验证和只读请求重连规则。 | `TestMCPV067BridgeRestartWaitAndReceipt`、`TestMCPWindowsEndpointExclusiveAndRestartable` |
| 已取消 MCP 请求仍可能写入通道 | 请求进入及取得串行锁后检查取消状态，避免已经撤回的请求继续发送。 | `TestMCPBridgeCanceledRequestDoesNotWrite` |
| SFTP 依赖的 Windows 路径规范化不一致 | 服务端辅助函数将 `//` 保留为 UNC 前缀，与其 POSIX 路径约定不符；补充 POSIX 清理并通过原有失败用例。Desktop 使用客户端，这一项没有证明存在客户端可利用路径。 | 依赖包 `TestCleanPath` |
| 构建工具链含已公告标准库风险 | 最低 Go 版本提高到 1.27.4，便携包使用补丁工具链重新构建。 | `govulncheck v1.10.0` |

标准库公告：[GO-2026-6859](https://pkg.go.dev/vuln/GO-2026-6859)、[GO-2026-6809](https://pkg.go.dev/vuln/GO-2026-6809)。前者的 HTTP 服务端竞态有特定触发条件，符号可达扫描本身不证明本应用可被利用；升级同时覆盖这项风险。后者涉及证书链校验。Windows 命名管道生命周期依据 [Microsoft 文档](https://learn.microsoft.com/en-us/windows/win32/api/namedpipeapi/nf-namedpipeapi-disconnectnamedpipe)。

Windows MCP 的请求帧、工具定义和配对数据不变。本机端点发现新增 `agent/mcp-control.endpoint` 和 `.mcp-server.lock`；它们不进入共享配置。新版桥接进程可以连接旧版服务，已经运行的旧版桥接进程需在更新后重启一次，无需重新配对。

## 性能变更

- ASCII 会话元数据直接做大小写转换，避免每次执行宽度折叠、分解、去音调和重组；非 ASCII 保留原 Unicode 规则，并增加等价性回归。
- 会话列表预分配结果容量；无快捷引用时不构建源会话索引，有引用时索引只保存位置，减少大结构复制。
- 采用已有千会话列表、终端吞吐、帧绘制和双向文字基准，并新增搜索规范化基准。终端渲染本轮未改动。

Windows amd64 / Ryzen 7 8745H / Go 1.27.4，原实现通过源码 overlay 对照；以下为顺序运行、每项五次、每次 500 ms 的中位数，不能直接等同于整机响应速度。

| 路径 | 修改前 → 修改后 | 分配变化 |
| --- | --- | --- |
| ASCII 搜索规范化 | 740.8 → 107.4 ns/op，耗时下降 85.5% | 576 → 48 B/op；4 → 1 次 |
| 1,000 会话列表重建 | 2.205 → 1.271 ms/op，耗时下降 42.4% | 4.79 → 2.32 MB/op，下降 51.6%；9,057 → 5,023 次 |
| 已缓存列表 | 0.408 → 0.414 ms/op，未见收益 | 1,408 B/op、2 次不变 |

混合 Unicode 首轮中位数 1.386 → 1.632 μs/op；固定 `GOMAXPROCS=1`、按新/旧/旧/新交替复核六个样本后为 1.070 → 1.029 μs/op，未复现首轮变慢。两轮均为 1,344 B/op、10 次分配，不宣称稳定的 Unicode 提速，保留原始日志供复核。

## 最终验证

- 主模块所有包分别取得完整通过结果。全量运行中核心包通过（409.9 秒）；Desktop 四个带 3 秒等待的测试在并行负载下超时，完整 Desktop 套件单独重跑通过（81.2 秒），未放宽超时或删除测试。
- `go vet ./...`、Linux amd64 交叉编译、FTP/SFTP、SSH 相关依赖、mygo UI 与 Windows 底层测试通过；构建、打包、合同与分片脚本共 20 个用例通过。
- FTP、代理和 MCP 的定向 race 验证通过。Zig 外部链接的 Windows 测试程序需要关闭测试 EXE 的动态基址标志以运行 TSan；仅处理临时测试程序，未改变正式 EXE 的安全标志。
- 图像输入与双向文字两项 15 秒 fuzz 验证通过，分别执行 48,028 与 656,313 次。
- `govulncheck v1.10.0` 对更新后的源码和最终 EXE 均报告未发现已知漏洞；该结果不代表未知漏洞不存在。
- Windows 单文件包完成独立空白目录启动、GUI 循环、嵌入运行库哈希、资源许可和数据目录隔离检查。测试未覆盖人工交互与正常退出；隐藏的空白测试进程验证后已清理。
- 固定 `dist` 的 EXE/ZIP 已替换；ZIP CRC、唯一 EXE 条目和两份程序哈希一致，原有 14 个数据文件哈希完全不变。

完整证据保存在固定 `validation/deep-review/`，详细命令和产物哈希见该目录的 `results.md`。

## 验证边界

- Windows 使用真实本机工具链和固定补丁终端 DLL，测试数据为临时合成数据。
- Linux 交叉编译只能证明编译兼容；本机没有 Linux 图形运行环境。
- Go 侧共享配置样本和合同脚本属于兼容回归，不能替代 macOS 真实 Swift ↔ Go 双端同步。本轮未修改共享字段。
- 真实堡垒机、远端 FTP/WebDAV 环境和 macOS 原生验收仍按已有清单保留。
- `validation/` 中历史探针与参考源码用独立本地 `go.mod` 隔离，避免混入主模块的 `./...`，保留这些证据。

审查阶段仅本地修改、验证与更新固定便携包。后续 v0.5.8 的提交、推送与手动发布依据用户另行授权执行。
