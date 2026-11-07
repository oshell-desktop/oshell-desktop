# 应用异常退出诊断（SF-W01）

2026-10-27 起本机版本默认记录应用运行与退出证据。诊断能力归原 SF-W01，关联 W12 的关闭保护和 U04 的更新退出；增加日志本身不代表修复未知退出原因。

2026-10-28 已定位本次实际崩溃：北京时间 15:17:24 关闭标签，`closeTab → pruneGroups → setCurrentTab → ui.Handle.Focus` 在 `ref.go:77` 触发多窗口绑定歧义 panic，15:17:31 诊断助手记录退出码 2。末次心跳为两个窗口、18 个标签、16 个连接，UI 队列延迟 0 ms。证据来自用户本机 `run-20261027T154740.289413900-37228.jsonl` 及其 `.crash.log` / `.termination.json`，未将原始用户日志加入仓库。

缺陷主归 SF-W08，关联 W01/W03/W07：跨窗口迁移沿用了源窗口焦点句柄。修复在迁移每个分屏时取消旧请求、创建目标窗口专属句柄，保留终端实例、连接及内容。软件双窗口回归已复现同一 panic 并验证修复，同时覆盖首次焦点、关闭/切换标签、关闭搜索、往返迁移、命令编辑焦点保持、分屏切换和键盘输入。证据在 `validation/multiwindow-focus/before-fix.log` 与 `regression.log`；真实鼠标拖拽边界仍按 SF-W07 待验，不将本次结论推广到所有未知退出。

## 2026-10-30 文件窗口关闭崩溃（SF-W12，关联 W01/F01）

本机 `run-20261030T040158.946168600-53104.jsonl` 记录北京时间 15:22:26 请求关闭文件窗口、15:22:27 确认并完成原生窗口关闭及两个分屏释放；随后 `.crash.log` 首帧为 `App.fileWindowView file_window.go:105`，诊断助手于 15:22:32 记录退出码 2。崩溃前仍有两个终端工作区、15 个连接，末次 UI 队列延迟 0 ms。原始日志留在用户目录，不复制配置或会话数据到仓库。

原因是关闭确认通过 `post → drain` 在绘制帧内执行。Windows 同步销毁窗口使 `ui.Context` 失效，文件窗口仍继续解引用 `c.Theme().Background`。合成真实 Windows 窗口已复现同一行的空指针 panic；这与 10 月 28 日的迁移焦点 panic 是不同缺陷。

文件窗口、主工作区和会话管理/设置/同步状态/MCP 辅助窗口在处理队列后检查 `Context.Valid()`，失效则结束当前视图，保留正常关闭确认和资源清理。`TestNativeCloseDuringFrameDrain` 验证前五类视图在绘制中真实销毁后返回、其他窗口存活及最终正常退出；既有原生最后窗口退出、文件停靠及跨窗口焦点回归一并执行。证据存于 `validation/frame-close/`；Linux/macOS 原生关闭路径未验收。

## 日志位置

- Windows 单文件便携版：EXE 所在目录的 `logs/diagnostics/`。
- 非便携版：`OSHELL_DATA_DIR/logs/diagnostics/`；未配置时使用系统用户配置目录下的 `oshell-desktop/logs/diagnostics/`。这是启动目录，不跟随共享配置同步。
- 首选目录不能写入时回退到用户缓存目录的 `oshell-desktop/diagnostics/`，Windows 通常为 `%LOCALAPPDATA%/oshell-desktop/diagnostics/`。

每次启动生成独立的时间戳/PID 文件组；日志时间是 UTC，同时记录本次启动后的毫秒数。启动阶段记录版本、Go/系统/架构、Git 构建状态与 EXE SHA256，可区分同版本号的本地构建。

| 文件 | 内容与判读 |
| --- | --- |
| `run-*.jsonl`、`.jsonl.1`、`.jsonl.2` | 按行 JSON 事件；`.2` 最旧。记录启动、所有原生窗口的创建/关闭请求/实际关闭、关闭确认和阻止原因、退出请求与调用栈、清理和事件循环返回。 |
| `.jsonl.exit.json` | 程序走到了显式结束记录，含原因和退出码。它表示执行了结束流程，不单独证明退出是用户主动触发；还需看 `quit_requested` 和 `event_loop_returned.quit_authorized`。 |
| `.jsonl.crash.log` | Go runtime 能输出的 panic/fatal/信号调用栈，包含函数和源文件行号。panic 值、函数参数、完整源路径和寄存器内容被过滤。 |
| `.jsonl.termination.json` | 诊断助手观察到主进程管道结束，且没有显式结束标记；记录是否取得崩溃栈和能获取的 Windows 退出码。不能仅据此区分外部结束、杀毒软件、断电或底层崩溃。 |

每 30 秒记录 Go 堆、goroutine 数、GC 次数，以及 Windows 工作集、私有内存、句柄数；另记录 UI 响应延迟、窗口/标签/分屏/活动连接/传输数量。UI 长时间不处理回调时记录 `ui_heartbeat_delayed`，最多保留一个待执行探针，不不断堆积任务。

Windows 消息循环退出记录 GetMessage 结果、WM_QUIT 代码及数值错误；会话结束通知、图形设备移除、系统休眠/恢复也记录。后台终端退出、传输开始/完成、分屏释放、闲置内存回收和更新退出各有事件。原始框架错误可能包含用户信息，因此只保留调用位置和明确允许的数值事件，重复框架日志采样。

## 保留和隐私

事件日志每份上限 2 MiB，每次运行最多当前文件加两份轮转；崩溃输出最多约 2 MiB。启动时清理旧的已结束运行，保留最新 10 次的文件组；运行中或没有结束标记的文件不会误删。日志写入失败不阻止启动或强制结束应用。

不记录密码、私钥、原始错误文本、会话名称/服务器地址、启动参数、用户命令、文件内容或终端输出；不采集完整内存转储。数据只留本机。崩溃捕获使用同一 EXE 的无界面助手，正常关闭时等待助手结束，异常退出时助手写完证据自行结束。

如果操作系统同时结束整个进程树、掉电或磁盘无法写入，可能只有末次心跳、没有崩溃栈/结束标记；启动诊断初始化之前的故障也可能无记录。Linux/macOS 的真实桌面与崩溃行为尚未验收，不能用 Windows 测试代替。

## 下次异常后如何取证

1. 记下消失的大致本地时间，以及当时是否传输、同步、休眠恢复或关闭某个窗口。
2. 将 EXE 同目录下的整个 `logs/diagnostics/` 目录打包，保留同一前缀的事件、轮转、崩溃和结束记录。无需开启终端输出录制或提供配置/密码库。
3. 优先对比 `window_close_requested → workspace_close_decision → window_closed → quit_requested → application_quit_cleanup_* → event_loop_returned → process_exit`。出现 `event_loop_exit_without_quit_approval` 时检查消息循环的数值结果和此前原生事件。
4. 缺少结束流程时查看 `.termination.json` 和 `.crash.log`，结合末次进程/UI 心跳、资源增长以及同时间的 Windows“应用程序”事件判断下一步；本轮不自动收集用户系统事件日志。

## 本机验证

`internal/diagnostics` 用独立合成进程覆盖正常退出、主线程 panic、后台 goroutine panic、`os.Exit(17)`、外部终止、敏感值过滤、轮转和保留。Desktop 的真实 Windows 多窗口/最后窗口退出、关闭确认、同步关闭保护及空闲回收回归通过。实际便携包另验证持续运行超过一个心跳周期、日志完整链、内嵌运行库和 WM_CLOSE 正常退出后助手无残留。初次证据存于 `validation/exit-diagnostics/`；本次已定位并修复的焦点崩溃见上文。
