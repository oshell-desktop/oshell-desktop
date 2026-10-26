# 13 项保留条件核查与验证记录

> 后续更新：本文保留该批次的历史结果及包摘要。SF-C03 的两种 CBC 与 SF-I22 的 Windows 命名共享内存实现缺口已在[后续修复](oshell-desktop-gap-closure-validation.md)补齐；当前状态/待验环境见[收尾清单](oshell-desktop-final-acceptance.md)，最新便携包见后续修复报告。

日期：2026-10-25。范围仅为[修复清单](oshell-desktop-remediation-plan.md)中 7 项平台替换、6 项已有增强，仍属于 `OShell-0.2.92-scope-1` 的原 161 项。参考版本保持 OShell 0.2.92 / `3f4efd9c70c82323f4e20180117c928227000580`。逐项核查完成；本机有限用例通过，缺少的必要平台证据逐项保留待验收，不能将“可保留机制”理解为全平台验收豁免。

Desktop 是包含前几批修改的未提交工作树，HEAD `62a620541680b687cedf89f8e48adcdb77576e33` 不是全部内容快照。本轮没有提交、推送、触发 Actions 或更改另一个项目。

## 本轮发现及修复

**SF-C08 路由解析，按原清单 P1 严重性处理。** 原 `ResolveSSH` 在找不到 ssh 时直接沿用资料，不能证明别名或 `Host *` 代理规则没有被绕过。[ssh_config.go](../internal/core/ssh_config.go) 现在仅允许“字面 IP 且标准用户/系统配置均不存在”的缺 ssh 回退；其他解析器不可用、配置存在/不可读、别名情况明确失败。既有共享链缺失和 helper 失败阻断保持。未添加 ssh_config 解析器、协议或平台；无 ssh 时普通域名也会拒绝，因为无法可靠区分别名与直接域名。安装可用的 OpenSSH 后沿用原解析路径。

**SF-F06 FTP 列表。** 原列表没有总字节上限，且 LIST 解析器会推测年份/时区。[FTP 库](../third_party/ftp/ftp.go) 与 [Desktop FTP 入口](../internal/core/ftp.go) 补齐 MLSD/LIST/NLST 每响应 4 MiB 上限；超限及非 UTF-8 为终止错误，不能以降级绕过。严格 LIST 路径只保留 RFC3659 明确 UTC 的时间；Unix/DOS 缺时区或年份的日期改为未知，名称/类型/权限等仍保留。既有通用解析器接口及其历史测试保持；应用生产路径使用严格列表方法。错误不返回部分目录，不覆盖已显示目录。

其余 11 项未发现本轮有限样例中的新生产缺陷，保留既有实现；新增的是约定条件的回归证据。未改共享字段、默认值或配置协议。依赖本地补丁说明已更新 [第三方声明](../THIRD_PARTY_NOTICES.md)。

## 逐项验收

| 固定条目 | 已执行的有限条件及证据 | 结论 / 剩余环境 |
| --- | --- | --- |
| **SF-P02 本机加密保存** | `TestRetainedP02LocalKeyFailureAndMigration`：重开解密；缺钥、短密钥、错误的 32 字节密钥均失败且库不变；缺钥不重新生成；合成原版 UUID JSON 密钥和 local-key envelope 可迁移，身份改变拒绝。完整核心回归覆盖主密码、共享密码及 v1/v2 样本。 | 本机格式替换可保留。Windows 样本通过；真实 macOS Swift 密码/迁移互通待验收，不把 Go 合成原版数据视为原版实机输出。 |
| **SF-C08 路由解析** | `TestRetainedC08MissingResolverAndAliasFields`、`RoutingFailuresNeverUseDirectTarget`：别名/字面 IP、缺解析器、配置存在、失败 `ssh -G`、缺共享链、helper 失败；目标未发生认证。`TestRetainedC08NativeOpenSSHConfiguration` 在临时配置上调用真实 `ssh -G -F`，核对主机/用户/端口/带空格身份文件、ProxyJump/ProxyCommand。`TestSSHConfiguredNestedRouteRoundtrip` 与混合 SOCKS 路由用真实本地 SSH 验证多跳和显式覆盖。 | 缺解析器回退已修复；Windows 有限路由矩阵通过，保留系统解析加 Go 执行。不承诺全部 ssh_config 语法；POSIX helper 引号用例在 Windows 跳过，Linux 原生部分仍待对应环境。 |
| **SF-I24 双向文字** | `TestRetainedI24SixModesMixedWideAndWrap` 对六模式使用同一拉丁/希伯来/阿拉伯/宽字、软换行样例，核对逻辑文本、复制及方向键；既有 `TestBidi*`、`TestSixPresentationModes`、选择拖拽测试覆盖视觉顺序、宽字命中、光标/输入法位置、鼠标及段落映射。生成六张 Windows 字体软件渲染图。 | 保留 Go UAX#9/字形布局。生产默认启用；当前 README 已无“关闭”说明，旧历史记录不代表当前行为。Windows 真实窗口交互与 Linux 实际显示仍待验收；无窗口软件图不冒称屏幕截图或 CoreText 实机对照。 |
| **SF-F06 FTP 列表** | 合成服务覆盖 MLSD、LIST、NLST、部分坏行降级、中文名和未知元数据；`TestRetainedF06ListingLimitsAndMetadata` 三路径分别验证恰好 4 MiB/多 1 字节，非法名过滤、可靠 UTC 与歧义时间；非 UTF-8 拒绝。`TestRetainedF06ListingFailureKeepsDisplayedDirectory` 验证超限/编码失败保留原目录和条目。原未知类型 UI、文件属性与 FTP 库回归通过。 | 本轮缺口已修复，本机有限条件通过；保留可靠属性增强，无 FTPS 或新列表格式。 |
| **SF-F07 传输方式** | `TestRetainedF07SCPWithoutSFTPAndFTPFailure`：服务端明确拒绝 SFTP 后仍可通过独立 SSH channel SCP 上传/下载，中文内容和带空格/引号路径一致；进度不倒退/超过数据量，取消和失败返回错误。FTP 正常下载一致；发送完数据后返回失败仍保留原文件并清理暂存目录。既有 SCP 递归、FTP 上传/下载和独立 SCP 工作流回归通过。 | 保留 Go FTP/SCP 与字节进度，本机有限条件通过。测试端使用系统 scp 实现合成远端，Desktop SCP 客户端仍不依赖本机 scp；不增加续传能力声明。 |
| **SF-A06 渲染** | MyGo `TestGPUComesBack`、初次失败/软件回退、运行时切换及 `TestRetainedRendererRetryBoundAndStableReset` 验证故障、恢复、1–30 秒上限、稳定一分钟重置。Windows D3D11 原生隐藏窗口真实设备、像素回读与 CPU 对照、图像区域、resize、composition 均通过，设备 `software (WARP): false`。桌面设置切换不重建终端/连接或丢历史。 | 保留现有后端及退避策略。故障注入与 D3D11 分别有证据；真实终端窗口设备丢失后输入/选择/图像联验、Linux GL 仍待验收，不用分层测试合称整条链路已完成。 |
| **SF-A10 生命周期与性能** | `TestRetainedA10ClosingOneWindowReleasesTerminal`：双窗口关闭一方终端释放连接，另一方历史和输入继续；全部空闲后延迟回收，再开终端可用。`TestRetainedA10CloseReleasesNativeImagesAndLog`：图像缓存/原生 VT/render state 清理、定时器停止、日志排空。既有弱引用可回收、迁移和生产零窗口生命周期回归通过。 | 保留进程级 FreeOSMemory 和所有窗口空闲条件；Windows 有限资源用例通过，未以固定 RSS 数值代替资源释放。 |
| **SF-I04 文本粘贴** | 既有预览编辑及拒绝用例补上 1 MiB+1；NUL、超限、取消、空内容、目标关闭/缺失均不写入，队列屏障核对避免异步误判；有效编辑可发。 | 保留再校验增强，本机通过。与 O06 共用测试，不另计功能。 |
| **SF-O06 同步输入** | `TestRetainedI04O06PerTargetPasteMode`：两目标 bracketed paste 模式不同，分别得到带标记的 LF 和普通 CR；预览后选择第三目标也不转移，第三目标无写入。I04 的双目标 NUL/超限/失效拒绝共同通过。 | 保留再校验与原目标快照，本机通过；未将图片/协议回复加入广播。 |
| **SF-I14 字符与解码** | 固定 `TestOShellUTF8FallbackAndFragments`、跨控制符 continuation、ControlEncoding 和 `TestOShellExtendedOSCCancelAndOverflow` 覆盖有效/非法 UTF-8、C1、1 字节等分块、未完成控制串取消、超过 8 MiB 的 OSC 不发布部分标题并恢复普通输出。期望源于固定 SwiftTerm 路径；本机运行使用现有 ABI 62 DLL。 | 保留预算增强，Windows 有限回归通过；真实 Swift 逐字节对照未运行，C1/SUB/ST/DCS 等历史差异不能据此一概宣称全等，仍按 I14 待验收。 |
| **SF-I17 颜色与属性** | `TestRetainedI17InvisiblePresentationCopyAndLog`：同一行普通/隐藏/重置后文字，隐藏段像素与空白等价；选择复制和日志均保留隐藏文字。既有 SGR/配色回归通过。 | 保留 Invisible 的显示修正，本机通过；不复制原版显示缺陷。 |
| **SF-A02 终端配色** | 自动光标黑/白/中灰和远端背景、手动颜色测试；设置 UI 切换、保存重开与取消；`TestRetainedA02SharedAutoCursorRoundtrip` 对 false/true 和手动颜色经共享 v2 encode/decode 往返核对。 | 保留 AutoCursor 增强，Windows 本机及 Go 合同样本通过；真实 Swift 互通仍待环境。 |
| **SF-U01 更新源设置** | 合成 token 本机加密、来源大小写归一及不同仓库隔离、忙时禁止切换/修改、未应用来源禁止保存 token、来源变化清除 UI 草稿、错误不回显秘密。私有更新合法签名通过，缺签名/错误资产/重复资产拒绝，下载到外站不携带 Authorization。 | 保留既有 token 增强，本机通过；私有入口仍验签。不新增登录/OAuth，不请求真实 token，不访问或部署线上更新。 |

新增核心、桌面、终端用例分别位于 [core/retained_conditions_test.go](../internal/core/retained_conditions_test.go)、[desktop/retained_conditions_test.go](../internal/desktop/retained_conditions_test.go)、[terminal/retained_conditions_test.go](../internal/terminal/retained_conditions_test.go)。更新 UI 补例在 [updater_other_test.go](../internal/desktop/updater_other_test.go)，退避补例在 [retained_renderer_test.go](../third_party/mygo/ui/retained_renderer_test.go)。

## 验证记录与限制

| 检查 | 结果 / 本地日志 |
| --- | --- |
| 核心、桌面、终端及其子包完整回归 | 7 个有测试包全部通过；core 299.285 s、desktop 83.067 s、terminal 21.168 s、VT 9.643 s；`validation/retained/regression.jsonl`、`test-summary.json`。 |
| 新增有限网络/密钥/FTP/SCP/更新签名 | 通过；`core-focused.log`。 |
| Windows 原生 ssh_config | 通过；`native-ssh-config.log`，真实 -G 解析临时文件，无用户配置修改。 |
| 资源释放与 FTP 原目录保护补验 | 通过；`resource-listing-extra.log`。补例位于完整回归之后，单独执行通过。 |
| 共享兼容 | 完整核心回归中 AGENTS 规定的四组 Go 样本通过；共享字段未改变。 |
| 静态检查 | `go vet ./internal/core ./internal/desktop ./internal/terminal/...` 通过，`vet.log` 无诊断。 |
| FTP 库完整回归 | 通过，1.159 s；`ftp-library.log`。 |
| GPU 故障与退避 | `renderer-faults.log`、`renderer-bounds.log` 通过。 |
| Windows 硬件 D3D11 | 原生 renderer 1.912 s、device 0.513 s，全通过无跳过；`d3d11.log`。 |
| 软件显示证据 | `validation/retained/images/retained-bidi-mode-1.png` 至 `mode-6.png`、`retained-invisible.png`；其余已有颜色/选择回归也输出图像。 |

完整回归包含 **42 条跳过事件**，明细保存在 `test-summary.json`：主要是 Swift/macOS、POSIX shell/helper、可选 lrzsz、需要显式启动的 native fixture 和嵌入包用例。跳过不算验收通过；本批相关 SCP、原生 D3D11 以及新增保留条件用例未跳过。嵌入资源/存储用例另由正式构建执行，实际新 ZIP 的签名准备另验。原始全量日志保留跳过记录，不能将其抹为“所有平台测试通过”。

本机 `wsl --list --quiet` 无已安装发行版，未找到 Swift 工具链；Linux GL/原生交互和 macOS 真实 OShell ↔ Go 互通无法执行。Windows 当前能进行原生窗口生命周期、D3D11 设备/回读和无窗口 UI 事件验证；上述表中要求的真实终端窗口视觉/设备丢失联验仍明确待验。P1 的算法缺口、P2 的 Windows POSIX 共享内存缺口及其他批次待验项均未被本轮关闭。

## 便携包与范围保护

版本维持 0.3.3。构建、验包、独立目录启动后仅更新固定 `dist` EXE、ZIP 和 SHA256SUMS。最终产物：

| 文件 | 字节数 | SHA256 |
| --- | ---: | --- |
| [OShell Desktop.exe](../dist/windows-amd64-single-file/OShell%20Desktop.exe) | 30756864 | `5acf95d19192963590f9b22d0ffa7bec637d1a42519eefc017e20f5544ff58cf` |
| [Windows 单文件 ZIP](../dist/OShell-Desktop-windows-amd64-single-file.zip) | 14241191 | `06f35902805159a01133c0a5654672d116b8750f0a4b99a828eb013176b7671a` |

`portable-build.log` 的内嵌 DLL/存储激活通过。`package-integrity.json` 确认 ZIP CRC、单 EXE 条目及字节摘要一致；`signing-key-verification.json` 确认沿用已授权密钥、公钥内嵌和私钥未入包；`signed-package.log` 对实际新 ZIP 的生产签名验证/准备及篡改拒绝通过。没有安装在线更新。

最终 EXE 独立目录启动 GUI、内嵌运行库摘要匹配、零窗口存活、外部入口恢复一窗且只有一个常驻进程，退出码 0。测试用 WM_CLOSE/WM_QUIT 正常结束合成进程，不冒称真实键鼠自动化，也未强制结束用户会话；见 `portable-smoke.json`。

发布前后 **11 个原配置/数据文件**的文件集合和逐文件 SHA256 均未改变；`dist-replacement.json` 确认 EXE/ZIP 已替换。未按日期或构建次数增加生产目录。上述本地记录均位于 `validation/retained/`。

基线 SHA256 保持 `0FE41373FACCF99453302FA6242AB5AAA27E7E084D03462EAE3683AE810A30B1`，历史差异 CSV 保持 `B580DC8B3071F60C5D0F1BA30D42F5D8C527777CB9DE76AC904F4F4D1D345EDC`。43 行差异、P1/P2/P3=9/18/3、保留项 13 均不变；只更新本批 13 个既有 SF 的“验收状态”，其他列及前批状态保持原样。核对结果见 `validation/retained/scope-verification.json`，不以测试数量计算功能完成率。
