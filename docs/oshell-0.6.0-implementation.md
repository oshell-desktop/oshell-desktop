# OShell 0.6.0 实施与收尾清单

> 2026-10-30 后续样式更新：终端鼠标已改为系统 I 形文本选择指针（I01/I02），62 项相关回归及便携包启动/退出验证通过；固定 dist 已替换为该版本，原 11 个数据文件未变，暂存已清理。最新产物摘要见 `validation/terminal-pointer/package-integrity.json`；下文 MCP/剩余功能构建摘要保留为该次快照。人工指针外观待验，176 项验收状态不变。

2026-10-30。参考固定 v0.6.0 / `18770fee58770c54aa33db574b646e49ae18e208`，用户已授权完整复刻原 176 条；没有新增编号或引入另一个版本的能力。沿用 Desktop 的平台替换、共享配置 v2 和既有边界，不提交、推送或触发 Actions。

**逐项结果：[176 项最终验收 CSV](oshell-desktop-0.6.0-final-acceptance.csv)。81 项已通过、94 项待验收、1 项平台专属排除。** 本轮列明的 34 项实现增量已补齐；“待验收”包含新增能力尚缺真实环境证据及原有待验，不能解释为 94 项缺代码，也不能宣称 176 项全部验收通过。F14 持续卡顿按用户要求保留待复现/验收。原差异调查的 53/16 数量保留为历史快照，不作为当前剩余数量。

## 本轮实现与验证

| 原条目 | 落地行为与已验证内容 | 仍待验收 |
| --- | --- | --- |
| C11、F07/F08/F09/F10/F13 | rsync 选择、独立目录双向传输、增量确认；原 SSH 连接租约及私有 STDIO 桥接；同名更新、保留目标额外文件、时间/安全链接、partial-dir、启动限速、字节进度与记录。真实回环 SSH 验证单次认证/二进制流/取消/连接保留；参数、路径、进度和无工具失败提示通过。 | 两端真实 rsync、不同平台本机路径及实际增量/片段复用；未把 SSH 流桥接测试当作 rsync 文件传输完成证据。 |
| F01 | 同一文件工作区切换窗口/左/右/底部；保留标签、客户端、租约及在途操作。停靠尺寸可调，隐藏后可恢复；底部紧凑布局和记录弹出。软件 UI 与真实 SFTP 连接状态检查通过。 | 原生窗口切换/选择器父窗口/用户交互与跨平台生命周期。 |
| F02 | 本机内置/TUI 模式；预设 yazi/ranger/lf/nnn/mc、自定义命令、空闲确认，保持当前终端及目录。只发目标会话，不广播；重连、认证、交互程序、传输、MCP 占用保护通过。 | 装有真实 TUI 工具的 SSH/本地 Shell 和人工交互。 |
| W05 | 按窗口内所有标签重新排列并清除自定义布局，继承活动标签组元数据；原单标签左右/上下分屏及放大入口保留。排列与 880–1440 DIP 工具栏回归通过。 | 人工拖拽、布局切换与原版外观对照。 |
| A08 | 时钟提升到工作区终端区域；多标签平铺/多 pane 只有一个时钟，文件停靠不重复。像素与定时器/设置回归通过。 | 多原生窗口实际显示。 |
| C04、I18 | 本地 Ctrl+Alt+] 入口、MCP 本机快捷键、Finder 多目录服务；目录仅作为本地运行 cwd，不改共享资料。目录整批校验及快捷键目标测试通过。 | macOS Finder 服务及平台按键冲突。Windows 默认 nextPane 已占 Ctrl+Alt+]，按既有绑定优先保留，需用户在设置改绑后使用新本地键。MCP 默认 Ctrl+Alt+A。 |
| W10 | macOS 原生全屏 will-enter/will-exit 预处理及 Dock/菜单栏自动隐藏，保留聚焦全屏状态恢复。Go 状态回归、Darwin arm64 编译通过。 | macOS 原生绿按钮、动画、失败恢复和聚焦快捷键。 |
| E01 | 默认不选的字体附件归档/恢复；原 JSON 不变，旁路 `<JSON>.assets/fonts`，恢复合并而不替换整个库。合法字体、取消、坏数据、无附件及 JSON 不变测试通过。 | 真实 Swift ↔ Go 双端归档及窗口确认。 |
| G01–G15；I06/I19/I26/I27/A10 | MCP 控制中心、配对/撤销、逐终端授权、三档审批、OSC779、七个工具、现有 PTY 闸门、幂等收据、接管、敏感输入、加密审计、搜索预览、只读回放、密码导出与手动清理。真实私有 IPC、真实本机 Bash PTY 执行/审批/撤销、软件面板与只读回放通过；最终 EXE 的 initialize/tools/list 无 UI 或 stdout 日志污染。 | 实际 Codex/OpenCode、远端 Bash/Zsh、真实窗口（含窄窗自动独立窗）、跨平台客户端。 |
| M01；关联 I21/I22 | 修复新增字体解析依赖先注册标准 TIFF 后，使灰度 Alpha/平面 TIFF 误用另一解码器的问题；图片粘贴和终端图形统一显式选择既有 OShell TIFF。 | 原有跨平台图形联验继续保留；本机 TIFF 矩阵及图形回归通过。 |

文件位置、TUI 命令、MCP 快捷键/配对/审计均为本机状态；未给共享配置合同增加字段。Go 原版样本、旧版本兼容、规范化及字体/目录/本地 HTTP WebDAV 回归通过；真实 Swift/WebDAV 环境仍缺。MCP 在 Windows 使用同用户私有命名管道、Unix 使用私有 socket；helper 为同一 EXE 的 STDIO 模式，不开 TCP，不调用模型。审计压缩采用 gzip 平台格式与独立 magic，不声称可被原版 LZFSE 直接读取。

## 剩余有限验收步骤

1. **rsync 六条**：准备兼容本机的 rsync（加入 PATH）与远端 rsync，不由程序安装。使用合成目录分别上/下载中文、空格、同名修改文件、额外目标及安全相对链接；检查时间及保留额外文件。大文件中途取消，再次运行同方向复用 `.oshell-rsync-partial`；下一次限速生效；无 SFTP 服务的 SSH 仍能打开独立目录传输。确认只有既有 SSH 认证、代理路线不变，没有 `--delete`/`--inplace` 或伪装 SFTP 续传。
2. **文件停靠/TUI/排列/时钟**：真实传输时依次四位置，调整宽高、隐藏/重开，连接/标签/进度保持；停靠时选择器归属正确。TUI 取消不发送、确认只执行当前会话；多组排列保留内容、清理旧布局；每个工作区一个时钟。按用户要求本轮不操作桌面，这些步骤未计通过。
3. **macOS**：实际构建应用验证 Finder 多目录服务及 cwd、绿按钮和聚焦全屏进出/动画反向/失败恢复；交叉编译仅说明编译可行。Linux 的原生终端、文件窗口、字体与图形条件继续按 CSV。
4. **MCP 接入与控制**：真实客户端中手工合并控制中心生成的配置（不将凭据发进日志）；默认关闭→开启→配对→仅授权合成终端。分别验证请求审批/简单查询/完全访问，拒绝和改权不执行；空闲 Bash/Zsh 安装临时集成，同一 Shell 的 cd 状态保留，幂等重试不重复写。任务断开、Ctrl+C/双 Esc、撤权和连接变化后自动输入停止，人工 SSH 保留；超时不能显示成功。
5. **MCP 审计**：确认基线不含授权前滚动历史；输入、输出、大小和审批事件可检索，敏感值跨块脱敏。完整/未封口/损坏分别提示，按任务筛选并显示命中预览；回放播放/倍速/定位不能执行链接、剪贴板、文件协议。完整封口后加密导出，短密码/活跃记录拒绝；确认删除只删所选审计，不删幂等收据。实际外部 Agent 工具不可由 OShell 承诺控制。
6. **共享和归档**：有 macOS 后运行 `python scripts/test-sync-interop.py --native-source /path/to/OShell`，再用许可测试字体补目录、真实 WebDAV 和归档双端往返。既有默认关闭、明文附件、强 ETag、冲突拒绝、身份重绑定和无变化不写约束保持。F14 仍仅在原下载目录/真实 SSH/原生选择器条件补验，不因本轮成功关闭。

## 测试与产物证据

当前生产代码 12 个有测试的包全部通过，5420 条测试结果（含子用例），52 条跳过未计通过。主要记录：`validation/mcp/production-regression.jsonl`、`test-summary.json`、`remaining-final.log`、`rsync-tests.log`、`font-archive-tests.log`、`terminal-tests.log`、`vet.log`、`darwin-build.log`。真实 Bash PTY 设置了 `OSHELL_TEST_BASH`；没有提供真实 rsync、Swift、WebDAV 或外部 Agent 客户端。

本轮广域 `go test -json ./...` 同时选中了 `validation` 历史源码快照，命令退出 1；失败来自旧快照/验证探针，不是当前生产包。完整原日志压缩保留为 `validation/mcp/final-regression.jsonl.gz`，生产结果单独提取且没有失败。后续应使用仓库生产范围 `go test ./internal/... .`，不修改或删除历史证据来制造测试成功。最后面板调整另跑 `remaining-final.log` 通过；`go vet ./internal/... .` 通过。

固定 `dist/windows-amd64-single-file/OShell Desktop.exe` 和 `dist/OShell-Desktop-windows-amd64-single-file.zip` 已更新。ZIP 仅含程序、解压内容一致、单文件运行库自解包、隔离空目录启动与正常 WM_CLOSE 退出通过；未操作用户会话或进行人工窗口验收。原 11 个配置/数据文件集合和 SHA-256 未改变。

- EXE SHA-256：`FBB534FAE2E4E243C9FC863BCB7C55311229CC604E0E9BF0B7374702E8867905`
- ZIP SHA-256：`D3B735335EB62565BD2AB79C3B9D63781885E2D94D9EE97FD439C742B8EEE207`
- 产物证据：`validation/mcp/package-integrity.json`、`packaged-stdio.json`、`portable-smoke.json`、`dist-replacement.json`；清理及占用以 `cleanup.json` 为准。保留当前 Windows/Linux 运行库及工具链、参考源码和精简验收证据，删除本轮暂存程序/ZIP/测试二进制/独立缓存及重复截图。

清理完成：本轮移除约 126.8 MiB 暂存（包括测试 EXE/ZIP、Darwin 测试二进制及重复截图），全量测试原始流压缩保留。`validation/` 当前约 1.22 GiB；当天新增大于 20 MiB 文件为零。Windows `792636f81613`、Linux `1adbe1f14a5f` 均为当前源码与各自目标计算出的有效指纹，非旧版本重复缓存。汇总结果见 `validation/mcp/results.json`。
