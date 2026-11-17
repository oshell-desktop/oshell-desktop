# Mac OShell v0.6.7 增量同步与收尾

2026-11-17。用户明确选择 **v0.6.7 标签源码，包含 MCP 新能力**。起点为上次同步的 v0.6.5（`0d10f5de10edf8293d18bfa694a19e50f973dbe4`），终点为 v0.6.7（`dd69495cce674e4eb5d106aa7d987120ceb2e4d8`）。核查时正式 Release 到 v0.6.6，未将标签说成正式 Release。

依据：[官方源码差异](https://github.com/oshell-desktop/oshell/compare/v0.6.5...v0.6.7)、[v0.6.7 MCP 设计](https://github.com/oshell-desktop/oshell/blob/v0.6.7/docs/agent-control.md)。本轮归入原 176 项，新增的是已授权版本的验收条件，不新增编号。共享配置 v2 字段、身份、连接路线和原快捷链接栏手动顺序不变；没有修改 Actions 或另一项目。

176 条身份保持不变。更新后的验收总表为 **78 已通过、97 待验收、1 平台专属排除**；其中 C01/O02/S11 的原证据保留，新版本新增交互转待验。97 并非 97 项缺代码；G09 另明确记录已复现的平台差异。

| 原条目 | 同步结果 | 验证与剩余条件 |
| --- | --- | --- |
| SF-O02 | 快速发送历史改为本机持久化、多窗口共用；默认 500 条，可设 1–100000；文本与追加回车联合去重并提到最新，支持清空和分页。每条最多 64 KiB，每页最多 50 条/64 KiB。选择只填入，多行转撰写框，上下键保留草稿及回车选项。 | 重启、去重、边界、分页、草稿和发送成功/失败回归；设置页、清理确认和多原生窗口交互待验。采用现有文件存储，不引入 SQLite 依赖；历史不参与同步和归档。 |
| SF-C01，关联 SF-I06/C11 | 同一 SSH 身份重连保留主屏和滚动历史、颜色及中文；新解析器不继承备用屏幕/键盘模式。已结束 SSH 的空输入 Enter 请求重连，本地终端及非空工具命令保持原行为。 | 真实终端运行库验证主/备用屏、模式、颜色和新屏清除；原连接取消/认证回归保留。真实 SSH 窗口断开→Enter→重新认证→历史/输入检查待验。 |
| SF-Y03，关联 Y06/Y10/Y12 | WebDAV 本机认证可选 Basic/Digest，旧配置仍为 Basic；支持 MD5、MD5-sess、SHA-256、SHA-256-sess，auth qop；新 nonce 的 stale 挑战有限重试，拒绝降级 Basic。配置和字体使用同一 transport，条件写和 TLS/重定向规则保留。 | RFC 已知 MD5/SHA-256 向量、真实 HTTP 回环挑战/正文/If-Match、错误算法/无降级/重试上限、加密设置和 Go 互通样本。真实 WebDAV 及 Swift↔Go 双端待验。 |
| SF-S11，关联 S14/W09 | 快捷连接包含显式快捷引用，即使源会话未开启快捷连接；显示别名和 Links 目录、协议/引用图标，始终连接原会话身份，沿用会话管理排序。 | 原生菜单数据、同名/失效选择、引用别名及原身份回归；浅深色实机菜单待验。快捷链接顶栏手动位置不改。 |
| V060-G01/G02/G03/G10 | 标签右键“交给 Agent…”提供会话专用确认、客户端与权限选择，记住上次客户端；授权即准备 Shell。终端显示接入状态及“终止接入”；无终端输出时，审批等控制状态变化也主动刷新窗口。任务释放和接入授权独立，人工接管暂停，撤权结束接入。 | 软件 UI 浅深色取消、私有通道、真实本机 Bash 授权/审批/撤权回归；外部客户端和真实窗口待验。 |
| V060-G04/G05 | 临时 Bash/Zsh 钩子保留已有提示符钩子；分片加载，取消恢复 tty，精确过滤自身回显。命令在当前 Shell 明文执行；增加 free/docker ps 有限查询自动批准。 | 真实 Windows Git Bash 临时集成、执行、释放、撤销；分片/回显/查询策略回归。远端 Bash（含旧版本）和 Zsh 钩子兼容待验；不修改启动文件。 |
| V060-G06/G07/G08 | sessions_list/session_state 返回进度、可执行状态和待审批信息；支持 include_screen=false。新增 session_exec 自动建立任务并保留幂等回执；任务结束/停止/连接断开保留接入，已运行命令仍等实际结束。 | 协调器审批、取消、任务释放、同键回执、真实 Bash/私有 IPC 回归。真实客户端循环待验。 |
| V060-G09，关联 G12 | operation_read 增加 0–10000 ms 等待、output/completion/approval、compact、raw/text、tail_lines/contains；文本去回显/控制码/提示符，游标仍按原始字节。查询最多扫描当前页 1 MiB，保留最近 128 KiB 文本窗口；普通输出 1–65536 字节。 | 分片 Unicode/OSC/CSI、Zsh 提示符间隔、审批/完成等待、取消、过滤及原始游标回归。**已知平台差异：Windows 本地 ConPTY 不支持可靠 text 视图**，详见下节；真实 SSH/Zsh/外部客户端待验。 |
| V060-G01/G07/G08/G09 | helper 不因服务未开/重启而退出；只读传输失败最多重试一次，长等待使用独立连接；写操作不自动重发。session_exec 后续读失败保留 operation_id、task_id、state 和 read_error。 | 实际私有 IPC 服务重启、长读与控制并行、读取失败仍返回回执；实际 Codex/OpenCode 重启联验待验。 |

## 已知差异：Windows 本地 ConPTY

真实 Git Bash 回归捕获到：ConPTY 可能先转发 OSC779 的结束标记，再输出命令正文。按 start/end 截取会得到空文本，不能据此称为成功或补一个猜测延迟。Desktop 对此类本地会话返回 `text_view_available=false`，`view=text` 明确报错；可用 `view=raw` 继续读取或通过 `session_state` 读取屏幕。命令已完成不等于 ConPTY 的全部显示字节已抵达。

SSH 使用原始通道，未经过本机 ConPTY，保留完整文本查询实现；有序 PTY 分片合同已测，真实服务器待验。该差异归 V060-G09，属于明确已知限制，**不是缺少环境，也没有计为完整复刻通过**。本轮未引入额外终端依赖或改变本地 Shell 启动方式。

## 有限人工验收

1. 快速发送：两个窗口各发送一条合成命令，关闭重开，确认跨窗与持久化；改变保留数后应用，取消设置不改变上限；多页选择只填入，清空取消/确认各一次。
2. SSH：输出彩色中文和多屏历史，断开后空 Enter 重连，核对历史、正文输入和新 Shell 的编辑模式；非空 `help` 不重连。
3. WebDAV：专用测试目录分别验证 Basic 与 Digest、改错密码、取消、冲突和无变化不重复写；开启字体后验证正文与附件共用所选认证。Mac 可用后按配置协议 v2 执行真实 Swift↔Go 测试。
4. MCP：在测试终端确认空闲后授权；分别用 Bash/Zsh 验证原钩子、cd 状态、多行和中文输出、三档审批；session_exec 使用同一幂等键重试不重复执行；任务结束/停止后接入保留，终止接入后禁止自动输入。
5. MCP 输出与重启：真实 Codex/OpenCode 验证 raw/text、tail/contains、分页及等待取消；服务关闭→重开后继续查询，执行的连接错误不得换键重发。Windows 本地 ConPTY 应明确拒绝 text，保留原始输出和屏幕可读。

## 自动验证与本地产物

证据固定复用 `validation/mac-latest/`：`delta.json` 与所需源码差异、`tests.log`（核心/终端/桌面整套回归）、`final-regression.log`（最终增量与兼容样本）、便携包校验和清理记录。跳过与真实环境待验没有计为通过。同步开发阶段使用 Desktop 0.5.4 验证，未提交、推送或触发 Actions。随后用户明确授权将这批变更发布为 v0.5.5，提交代码和标签，并手动触发既有直接发布流程；共享配置协议与工作流不变。

完整 Windows 回归通过：核心 261.878 秒、终端 22.273 秒、桌面 77.008 秒及相关终端子包。最终增量回归通过；RFC 向量、Digest 加密设置/拒绝降级和兼容样本通过；`go vet` 通过。浅/深色 MCP 快捷确认离屏检查通过，仍未操作用户桌面。

同步开发阶段的固定产物（0.5.4 验证快照）：`dist/windows-amd64-single-file/OShell Desktop.exe` 与 `dist/OShell-Desktop-windows-amd64-single-file.zip`。ZIP 只有程序、内容与 EXE 一致，最终 EXE 的 8 个 MCP 工具发现及干净 STDIO 退出通过；空目录隐藏启动、内置运行库加载和正常关闭通过。原 11 个数据文件集合与 SHA-256 未变，未替换安装目录。

- EXE SHA-256：`480CB9C467A5A5B339740DA518D81B3DAADF27784E7E9B27837D1C4C361058FD`
- ZIP SHA-256：`6FBDBEEB0B746DE86947AAA5ACBE56A0E0BB6B315A09AEA5F83517CD28AB1539`

已清理本轮暂存 EXE/ZIP、隔离启动副本/缓存及一次性调研副本；最后收尾移除 77.5 MiB，验证目录现为 1.042 GiB，保留源码差异、两张离屏图和精简日志。当天新增大于 20 MiB 的验证文件为零；仍只有当前 Windows/Linux 指纹与所用 Zig 工具链。详见 `validation/mac-latest/cleanup.json`。

## v0.5.5 发布准备

2026-11-17 按用户授权将同批同步变更设为 Desktop v0.5.5。功能回归沿用上述通过记录；版本变更后重新构建并验证单文件资源、便携存储、ZIP 唯一程序与内容一致、MCP 报告版本 0.5.5 及 8 个工具、隔离隐藏启动和正常退出。固定 dist 已更新，原 11 个配置/数据文件哈希不变。

发布沿用 `Build and release` 手动标签流程：容器打包 Windows/Linux，生成并验证签名清单后创建 Release；不新增 CI 测试步骤。标签说明包含简短更新内容和 Windows ConPTY 已知限制。真实环境待验条件继续保留。

- v0.5.5 EXE SHA-256：`9E204330CC4A3D7F4FEE977B491817B63838D958C48C66ACBCF25D99B891120C`
- v0.5.5 ZIP SHA-256：`AA38207C14E2CAE678F0632B7A64673E9739F7ABE24E0C229D2E17DEEC663F65`

证据：`validation/mac-latest/release-build.log`、`release-package.json`、`release-smoke.json`、`release-dist.json`、`release-cleanup.json`。暂存程序、ZIP 和隔离缓存已清理 77.2 MiB；validation 当前 1.042 GiB，新增大于 20 MiB 文件为零。
