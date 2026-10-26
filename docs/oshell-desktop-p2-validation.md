# P2 十八项修复与本机验证记录

> 后续更新：本文保留该批次的历史结果及包摘要。SF-C03 的两种 CBC 与 SF-I22 的 Windows 命名共享内存实现缺口已在[后续修复](oshell-desktop-gap-closure-validation.md)补齐；当前状态/待验环境见[收尾清单](oshell-desktop-final-acceptance.md)，最新便携包见后续修复报告。

日期：2026-10-25。仅处理 [修复清单](oshell-desktop-remediation-plan.md) 的 18 个 P2 条目，仍归 [固定 161 项基线](oshell-feature-inventory.md) `OShell-0.2.92-scope-1`，参考 OShell 0.2.92 提交 `3f4efd9c70c82323f4e20180117c928227000580`。工作对象包含此前未提交改动，不把 HEAD 当作完整内容快照。没有提交、推送、触发 Actions 或发布更新，也未修改原版项目。

十八项均已逐项修复和记录，本机可执行的有限回归通过。**不能将本批写成十八项全部闭合：SF-I22 的 Windows POSIX 共享内存仍缺失，明确返回 ENOTSUP；Linux/macOS 原生验收及真实 Swift 双端互通缺环境。** 这与“实现已有、只缺验收环境”分别记录，不能互相替代。

## 逐项修复与证据

通用测试入口：[核心 P2 回归](../internal/core/p2_regression_test.go)、[桌面 P2 回归](../internal/desktop/p2_regression_test.go)、[终端 P2 回归](../internal/terminal/p2_regression_test.go)。以下编号均为原 SF。

| 条目 | 实现与有限验证 | 状态 |
| --- | --- | --- |
| SF-P08 | `workspaceAuthorized/workspaceLocked` 区分启动授权和秘密缓存。清缓存取消同步/探测、清解密缓存，已连接终端继续输入；配置保存、取凭据、新连接和同步仍保留秘密门禁。启动未授权、失败解锁及已有密码保护回归保留。 | Windows 本机通过。 |
| SF-M05 | 图片仍绑定捕获的 pane/term/conn、owner、宿主代次和上下文令牌。合成 PNG 在清缓存及切换活动标签后只向原终端插入路径；原目标关闭、移走、宿主/终端变化、完整启动锁和关闭应用均拒绝。 | Windows 本机通过。 |
| SF-S05 | 引用索引别名和源会话名，保留多词 AND。侧栏明确展示 4096 UTF-8 字节/32 词限制，边界及越界各一例，清空/改正恢复结果。源名命中引用后展开树的旧断言同步修正。 | 本机通过。 |
| SF-S14 | 原生不可拖菜单改为已有 UI 弹出层；目录会话/引用经真实 UI 拖动事件回到根栏，保留原目录。拖动携带配置快照、Store 本机代次和窗口 owner，过期/外窗载荷拒绝；根排序和保存回归通过。补验发现子节点 hover 抢占导致边缘不滚动，已修正并验证 A→B→A 代次、外窗/无快照拒绝与边缘滚动。 | Windows UI 回归通过；其他系统原生交互待验收。 |
| SF-W01 | 最后窗口关闭不触发默认 Quit。原生生产 `runDesktop` 测试建立两窗、关闭到零窗、命名管道恢复一窗、显式 App.Quit 完成；最终便携 EXE 独立启动并用实际外部启动参数验证只保留一个常驻进程。 | Windows 原生通过；Linux 待验收。 |
| SF-W02 | 前后标签遍历同窗所有非隐藏组，跨组循环；数字跳转仍限定当前组。跨组、隐藏跳过、回绕及组内编号测试通过，原跨窗迁移回归通过。 | 本机通过。 |
| SF-I05 | 软换行 regex 上限按终端格计数。多字节文本超过 65536 字节但只有 46006 格仍匹配；超过 65536 格受控报错。既有 Unicode、超 1000 结果导航、超时/关闭与重排回归通过。 | 本机通过；保留单次 10 ms、总扫描约 1 s 的检查预算差异，不称预算机制相同。 |
| SF-I06 | 使用固定 PlainTextFilter 状态机过滤 ANSI/OSC，保留 UTF-8、CR/LF/TAB；后台写盘，待写加进行中数据合计上限 4 MiB。分块控制序列、阻塞写端、积压停止、错误反馈、关闭后刷新释放测试通过。 | 本机通过；日志是过滤后的输出流，不是屏幕转录。 |
| SF-I10 | 编辑与解析统一 8192 UTF-8 字节；允许工具名单和无 Shell 执行路径不变。独立 `|`、`&&`、`;`、含 `>` 参数通过生产 ToolsConsole→PTY→临时合成 ssh.exe 收到原样 argv；多字节越界拒绝，原编辑/取消回归通过。 | Windows 生产 PTY 通过。 |
| SF-I20 | 生产 Options 接上 BEL 宿主响铃回调；OSC9;4 五态进度绘制、数值钳位、15 秒过期与会话结束清理。测试实际喂给终端，检查回调、状态及失效 owner 不响应；未读标记没有替代响铃。 | Windows 回调和渲染状态通过；未将听感或其他平台反馈宣称已验。 |
| SF-I22 | 新 Windows 原生库支持 Kitty 文件/临时文件，保留打开前后路径、范围、文件类型及临时清理规则。直接/文件/临时文件实际像素绘制、缺失/越界拒绝及最终内嵌 DLL 自检通过；共享内存明确 ENOTSUP，无虚假 OK。 | **部分完成：Windows POSIX 共享内存能力缺口保留。Linux 四介质端到端待环境。** |
| SF-M01 | 按 PNG→TIFF→JPEG 读取及解码回退；文件 URL 优先路径和混合文件拒绝保持。Windows 补 CF_TIFF 与 JPEG/JFIF 的 MIME 映射，原生四表示含 DIB 转 PNG 实测通过，结束恢复原剪贴板；核心 TIFF/JPEG/PNG 转码回归通过。 | Windows 原生通过；macOS TIFF 原有框架转换、Linux 原生剪贴板待验收。 |
| SF-F04 | SFTP/FTP 深度恢复 32，项目上限 10000 保留。真实本地 SFTP 端点验证顶层链接 STAT 下载、目录正常项完成/子链接跳过、本地链接上传拒绝；32/33 和 10000/10001、取消及已有 FTP 回归通过。 | Windows 本机通过，未使用远端真实数据。 |
| SF-O02 | 历史最多 50 条、总量 64 KiB、单条超过 8 KiB 不入史；上下恢复原草稿。成功发送清稿并重置追加回车，失败保稿；原 IME/发送目标隔离测试通过。 | 本机通过。 |
| SF-O07 | 保存 33 条及以上已有模型数据，仅前 32 条参加渲染；重叠首规则优先，行预算 4096 UTF-16 单位。33 条往返、被禁用前 32 条不让第 33 条递补、非 BMP 行预算及原高亮回归通过。 | 本机通过；共享字段/schema 未增改，真实 Swift 往返待 macOS。 |
| SF-O08 | 仅对变更绑定查已知系统冲突。Windows 保留组合及 RegisterHotKey 探测，探测后立即释放；读不到系统信息明确显示未完成系统检查。内部冲突、取消、录入隔离、正常绑定回归与 Windows F12 保留冲突通过。 | Windows 通过；Linux GNOME 检查原生待验，macOS 当前明确不可读，系统检查待验收。 |
| SF-E03 | SecureCRT 遍历保留物理空目录，预览显示目录数，Archive.Merge 保留到目标目录下。单级/两级空目录、含有效会话混合、最终合并通过；取消、链接不跟随及 E06 数量/容器回归保留。 | 本机通过。8000 遍历项测试改用 Xshell，避免先触发 SecureCRT 的 4000 个保留目录限制，两个限制均保留。 |
| SF-U04 | 无活动终端/文件任务直接安装，有模态记录待安装并在关闭后的帧末恢复一次；取消确认保留已准备包，失败不自动重试。跨窗模态/单次确认及真实临时更新进程成功重启、崩溃、替换失败、恢复重试与数据不变回归通过。 | Windows 本机通过；macOS 应用包安装/原生确认待环境，不安装在线更新。 |

## 验证记录

| 检查 | 结果与本机日志 |
| --- | --- |
| core 全量回归 | 通过，253.618 s；`validation/p2/full-regression.log` 的 core 结果。该轮桌面旧断言失败已在下一轮修复。 |
| desktop 最终全量回归 | 通过，73.787 s；`validation/p2/desktop-final.log`。后增验收测试单独通过。 |
| terminal / internal/vt 全量回归 | 通过，16.759 s / 7.146 s；`full-regression.log`。 |
| 新增 P2 定向、真实 SFTP、实际图片绘制 | `focused-final.log`、`acceptance-extra.log`、`link-native-shortcut.log`、`link-generation-scroll.log`、`tool-argv.log` 均通过。 |
| Windows 原生剪贴板 | `native-clipboard.log` 四种表示全部通过，未跳过，恢复原内容。 |
| Windows 原生生产窗口生命周期 | `native-lifecycle.log` 两窗/零窗/恢复/显式退出全部通过。 |
| AGENTS 要求的四组共享配置样本 | `compatibility.log` 通过；不是 Swift 实机互通。 |
| go vet core/desktop/terminal | 通过；`vet.log` 无诊断。 |
| Windows 原生运行库 | ABI 62；规范化补丁 41 个既有文件与已应用源码一致，语义修改仅 Kitty 的两个文件。`native-patch-verification.json`；新 DLL SHA256 `84eb819aced52329f909319ad4d35f6420ef6bed40565ff01e7dc1b4cca4cb60`。 |
| 单文件最终构建 | `portable-build-final.log` 通过，包含独立内嵌 DLL 自检。 |

Linux：本机 `wsl --list --quiet` 没有已安装发行版，未执行 Linux 原生矩阵。macOS/真实 OShell Swift 环境不可用，`scripts/test-sync-interop.py --native-source ...` 未执行，明确待验收。没有用交叉编译或跳过测试替代这些结果。

## 便携包与数据保护

以下为 P2 完成时的产物历史摘要；固定 dist 已由后续 [P3 便携包](oshell-desktop-p3-validation.md)替换。版本维持 0.3.3，包含当时工作树。P2 产物：

| 文件 | 字节数 | SHA256 |
| --- | ---: | --- |
| [OShell Desktop.exe](../dist/windows-amd64-single-file/OShell%20Desktop.exe) | 30671872 | `3405af2376de23336589404c11f3c727af41994a24b3421a1d184e7d6c98664d` |
| [Windows 单文件 ZIP](../dist/OShell-Desktop-windows-amd64-single-file.zip) | 14229594 | `4dde855a1fe0e1776e6f9b400f46a9fa4aaec9ac58dfc823cb86f0d2f64e2d9f` |

ZIP CRC/完整性通过且仅含一个 EXE，解包摘要一致。最终 EXE 在独立目录启动 GUI，使用内嵌且摘要一致的 DLL；不存在旁置 DLL 依赖，忽略外部数据目录覆盖。外部入口恢复一窗且无重复常驻进程，退出码 0。最终 EXE 的非交互烟雾脚本用原生 WM_CLOSE/WM_QUIT 清理合成进程；完整显式 App.Quit 路径由上述生产入口原生测试验证，两者证据分开，不假称按键自动化通过。

沿用 P1 公钥，验证密钥配对、公钥已内嵌、私钥未入包；没有重新生成或上传签名私钥。实际新 ZIP 的签名准备回归见 `signed-package.log`。

固定 dist 替换结果见 `validation/p2/dist-replacement.json`；仅替换 EXE、ZIP、SHA256SUMS，原有 11 个配置/数据文件逐文件 SHA256 均未改变，未强制结束用户会话。原 161 项基线及历史差异 CSV 的 SHA256 未改变，仍为 43 项差异、P1/P2/P3 = 9/18/3、保留项 13；只更新本批验收状态，不用测试数量计算功能完成率。
