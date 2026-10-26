# P3 三项修复与验证记录

日期：2026-10-25。范围仅为[修复清单](oshell-desktop-remediation-plan.md)的 SF-S02、SF-T07、SF-I08，仍属于 `OShell-0.2.92-scope-1` 的原 161 项。本批三项代码修复和 Windows 本机有限回归完成；缺少的平台环境保留待验收。不新增功能编号，不关闭 P1/P2 已记录的缺口或待验项。

参考源码为 `validation/oshell-reference` 中 OShell 0.2.92，HEAD 核对为 `3f4efd9c70c82323f4e20180117c928227000580`。本次所查 SessionEditor.swift、TerminalPane.swift 与该固定提交无差异。Desktop 为当前未提交工作树；HEAD `62a620541680b687cedf89f8e48adcdb77576e33` 不代表全部代码快照。未提交、推送或触发 Actions。

## 逐项结果

| 固定条目 | 原版依据与修复 | 有限验收结果 |
| --- | --- | --- |
| **SF-S02 协议相关属性** | `Sources/OShell/SessionEditor.swift:181–182,205–206`：SFTP 禁用空闲发送开关与输入；SSH 两个输入还依赖开关启用。Desktop 在 [connection_editor.go](../internal/desktop/connection_editor.go) 对两个 Field 施加禁用状态。协议切换和保存沿用既有规则，不清除隐藏字段。 | `TestP3S02IdleControlsAndProtocolDraft` 使用真实 connectionEditor 的 UI 事件：SSH 间隔和字符串可编辑；切 SFTP 后三控件不可改；切回恢复草稿；SSH 开关关闭后输入不可改、开启恢复；取消不改变配置。另验证 SFTP 保存时仅关闭 IdleEnabled，保留间隔与字符串。既有 FTP 清理、协议默认值回归通过。 |
| **SF-T07 分屏聚合标题** | `Sources/OShell/TerminalPane.swift:333–335,1083–1112`：名称模式先清控制/格式字符，保留前 256 个 Unicode 标量，空名回退；单分屏显示自身标题。多分屏先按完整显示名去重计数，再逐名截取前 32 个扩展字素簇并加 `…`。Desktop 在 [session_titles.go](../internal/desktop/session_titles.go) 落实，保留既有布局顺序、分隔样式及详情。 | `TestP3T07CharacterLimitsAndAggregation` 覆盖 ASCII、中文、组合附加符、ZWJ emoji、国旗各 31/32/33 字符；控制字符过滤、空名和 256 标量上限；同前缀的两个不同长名仍分别显示；同名计数、第三类分屏的按屏计数、焦点切换不重排；详情保留完整分屏标题及当前标记，单分屏不套用 32 字符聚合规则；源会话名称未改。 |
| **SF-I08 本机工具入口** | `Sources/OShellCore/LocalPromptInput.swift:280`、`EndedSessionInput.swift:61`：`help` 和 `?` 调用同一帮助。Desktop 在 [tools_console.go](../internal/terminal/tools_console.go) 将 `?` 并入现有 `help` 分支。 | `TestP3I08HelpAliasReturnsToPrompt` 驱动实际 ToolsConsole 输入输出，完整帮助内容与返回提示符一致，未知命令仍拒绝。读取等待命令结果后的提示符，不将行编辑重绘误当完成。终端完整回归保留输入编辑、中断、PTY/参数边界等既有行为。工具白名单和参数解析未改。 |

桌面用例位于 [p3_parity_test.go](../internal/desktop/p3_parity_test.go)，工具用例位于 [tools_console_test.go](../internal/terminal/tools_console_test.go)。未增加共享字段或更改配置协议。

标题不能以字节或 rune 数代替 Swift Character。定向测试暴露当前 go-text 分段器对连续 ZWJ emoji 的分段不符合本例预期，因此仅标题入口使用固定 `github.com/clipperhouse/uax29/v2 v2.7.0`。未升级原终端分段器或改变终端输入规则；依赖及校验和写入 go.mod/go.sum，MIT 许可证和既有 Unicode 许可证随包内嵌，构建测试检查 `licenses/uax29-LICENSE` 存在。该依赖用于已有标题验收要求，不形成新的产品能力。

## 本机验证与待验收

| 检查 | 结果 / 本地记录 |
| --- | --- |
| P3 三项定向回归 | 通过；随后纳入下列完整回归。 |
| `go test ./internal/desktop ./internal/terminal -count=1` | desktop 67.603 s、terminal 18.044 s，均通过；`validation/p3/desktop-terminal-tests.log`。 |
| AGENTS 规定的四组 Go 共享配置样本 | `Test(NativeSwiftSharedFixture\|SharedQuickSendScope\|LegacySharedProtocol\|SharedEncodingNormalization)`，4.307 s，通过；`validation/p3/shared-fixtures.log`。 |
| `go vet ./internal/desktop ./internal/terminal ./internal/core` | 通过，无诊断；`validation/p3/go-vet.log`。 |
| Windows 单文件构建及嵌入资源测试 | 通过；`validation/p3/portable-build.log`，包括真实嵌入 DLL 加载与单文件存储激活。 |
| ZIP CRC、条目与 EXE 摘要 | 通过，ZIP 仅含一个 EXE，内容摘要一致；`validation/p3/package-integrity.json`。 |
| 最终 EXE 独立目录启动 | GUI 就绪、内嵌 DLL 摘要一致、零窗口进程存活、外部入口恢复一窗且无重复常驻进程、退出码 0；`validation/p3/portable-smoke.json`。 |
| 签名密钥与包资源 | 沿用已授权生成的密钥，公钥已内嵌，私钥未进入 EXE；`validation/p3/signing-key-verification.json`。 |
| 固定范围与 CSV | 基线和历史差异 CSV 哈希不变；43 行中仅三条 P3 的“验收状态”变化；`validation/p3/scope-verification.json`。 |

**待验收：**本机只有 Windows 运行证据。SF-S02 的 Linux/macOS 原生控件交互、SF-T07 的 macOS 原版实机显示对照、SF-I08 的 Linux/macOS 原生运行仍待对应环境。没有运行真实 Swift ↔ Go 互通脚本，Go 兼容样本通过不等于真实双端互通通过。本批未修改共享协议。

UI 控件验收使用 Windows 上运行的 MyGo 无窗口事件测试器；最终 EXE 的原生烟雾测试验证启动与生命周期，不假称自动点击了三项功能。烟雾脚本以 WM_CLOSE/WM_QUIT 正常退出隔离的合成测试进程；完整生产 App.Quit 路径由桌面回归中的现有原生生命周期测试覆盖，未强制结束用户会话。

## 固定便携包

以下为 P3 完成时的产物历史摘要；固定 dist 已由后续[保留项核查便携包](oshell-desktop-retained-validation.md)替换。P3 版本维持 0.3.3，包含当时工作树。先构建到固定暂存目录 `validation/portable-build`，验包、独立目录启动后，再替换固定 dist。P3 文件：

| 文件 | 字节数 | SHA256 |
| --- | ---: | --- |
| [OShell Desktop.exe](../dist/windows-amd64-single-file/OShell%20Desktop.exe) | 30752256 | `742464d8f74030a52719df0a940ed16dfd61ba2b1a26eed9e7292a7b58b491f5` |
| [Windows 单文件 ZIP](../dist/OShell-Desktop-windows-amd64-single-file.zip) | 14238474 | `1ac572b0d9e947cc456ba8d80b8b1c4ce7930b41bfce3b08059f75defd59facb` |

校验和同步写入 `dist/SHA256SUMS.txt`。发布前后对原有 **11 个配置/数据文件**逐一核对 SHA256 和文件集合，均未改变；见 `validation/p3/dist-replacement.json`。没有按日期新增生产输出目录。

原基线 SHA256 仍为 `0FE41373FACCF99453302FA6242AB5AAA27E7E084D03462EAE3683AE810A30B1`；历史差异 CSV 仍为 `B580DC8B3071F60C5D0F1BA30D42F5D8C527777CB9DE76AC904F4F4D1D345EDC`。优先级集合仍为 P1/P2/P3 = 9/18/3、保留 13，不以新增用例数量计算功能进度。
