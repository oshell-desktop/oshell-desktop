# Windows 原 37 项缺证据验收记录

日期：2026-10-25。只处理 [收尾清单](oshell-desktop-final-acceptance.md)原有的 37 项 Windows 缺证据条目；范围仍为 [固定基线](oshell-feature-inventory.md) `OShell-0.2.92-scope-1` 的 13 域、161 项，OShell 0.2.92 固定提交 `3f4efd9c70c82323f4e20180117c928227000580`。

**本轮 12 项有限验收通过，25 项真实窗口验收仍待完成。** 发现并修复 SF-C10 的一个关闭失败问题，修复后验证通过。全表现为 **94 项已通过、0 项已确认实现缺口、41 项缺环境待验、25 项缺证据待验、1 项平台排除**。0 缺口仅指已知问题闭合，不能推断未验部分没有缺陷。

Desktop 为包含既有未提交修改的工作树，HEAD `62a620541680b687cedf89f8e48adcdb77576e33` 不是完整快照。系统为 Windows 11 企业版 10.0.26300 / amd64，Go 1.27.2。没有提交、推送或触发 Actions；没有修改共享配置合同、增加协议或升级固定参考版本。原 43 项历史处置分类及 CSV 不变，C10 不被添加为第 44 项。

## 12 项通过的有限用例

所有资料、端点和凭据均为合成数据。真实 SSH/Agent/SFTP 指实际生产网络/命名管道路径与回环测试对端；两窗口模型及 UI.Tester 只证明状态传播和生产事件处理，不表示物理窗口交互。每项原验收要求保留在 [161 项 CSV](oshell-desktop-final-acceptance.csv)。

| 原条目 | 固定输入与实际结果 | 本轮通过测试 | 日志（validation/windows37/） |
| --- | --- | --- | --- |
| SF-S03 会话默认属性 | 默认设置保存/取消、新建采用、既有/复制/原生归档导入保留原值均通过。 | `internal/desktop::TestEvidenceS03DefaultsOnlyAffectNewProfiles` | `logic-final.log` |
| SF-C01 SSH 连接 | Windows 合成 Agent 正确密钥成功、错误密钥/空 Agent 拒绝；生产重连取消保留原连接和终端，后续输入仍送达原连接。 | `internal/core::TestEvidenceC01WindowsAgentAuthentication`<br>`internal/desktop::TestEvidenceC01ReconnectCancellationKeepsLivePane` | `agent-evidence.log`<br>`logic-final.log` |
| SF-C02 主机身份 | 真实回环 SSH 握手首次批准写 known_hosts、同钥免询问、换钥拒绝不覆盖及拒绝/取消不写入均通过。 | `internal/core::TestEvidenceC02RealHandshakeTrustLifecycle` | `core-evidence.log` |
| SF-C10 隧道占用 | 两窗口模型同 UUID 在认证中/已连接时仅首个持有隧道，克隆共享、释放后重建通过。复现并修复监听 Close 失败仍提前释放占用的问题；实际端口未释放时继续拒绝重复占用，确认关闭后释放。 | `internal/core::TestEvidenceC10CloseFailureKeepsOwnership`<br>`internal/core::TestEvidenceC10PendingAuthenticationAndReopen`<br>`internal/core::TestTunnelsFollowTransportAcrossClonedChannels`<br>`internal/desktop::TestEvidenceC10TwoWindowModelsSharePendingTunnelOwner` | `tunnel-after.log`<br>`logic-final.log` |
| SF-T01 标题方式 | 生产标题设置保存/取消、四种模式覆盖、详情/稳定名称、探测开停以及两窗口模型刷新均通过；共享会话内容不被标题设置改写。 | `internal/desktop::TestEvidenceT01T02TitleSettingsPropagation` | `title-evidence.log`<br>`logic-final.log` |
| SF-T02 默认与覆盖 | 默认标题模式仅作用新建；既有、复制继承及覆盖全部、跨窗口模型传播通过，取消不改配置。 | `internal/desktop::TestEvidenceT01T02TitleSettingsPropagation` | `title-evidence.log`<br>`logic-final.log` |
| SF-I11 提示符历史 | 100 条上限及第 101 条淘汰、相邻去重/非相邻保留、空历史/空查询/失败搜索、上下翻阅草稿与搜索取消字素光标恢复通过；交互输入暂停不混入历史通过。 | `internal/terminal::TestEvidenceI11PromptHistoryFiniteMatrix`<br>`internal/terminal::TestPromptSuspensionDiscardsInteractiveInput` | `logic-final.log` |
| SF-M02 用户确认 | 文字确认取消不写文件/偏好、不发送；本机缓存失败后可重试，未知偏好版本拒绝覆盖通过。 | `internal/desktop::TestEvidenceM02M06ImageConfirmationAndMemory` | `logic-final.log` |
| SF-M06 提醒记忆 | 本机及真实回环 SSH/SFTP 上传均仅成功后记忆；失败后重试、取消不新增规则、外层设置取消保留已成功规则、复制/改路由不沿用旧确认及未知版本拒写通过。 | `internal/desktop::TestEvidenceM02M06ImageConfirmationAndMemory`<br>`internal/desktop::TestEvidenceM06RemoteReminderOnlyAfterSuccessfulUpload` | `logic-final.log`<br>`remote-image-final.log` |
| SF-M07 资源边界 | 单图 20 MiB、缓存 200 MiB、解码输入 160 MiB、单边 20000 像素与 4000 万像素边界及超限拒绝通过；实际编码输出跨 20 MiB、过期清理不删无关文件通过。 | `internal/core::TestEvidenceM07ImageStorageBoundaries`<br>`internal/core::TestEvidenceM07DecodeAndOutputBoundaries` | `core-evidence.log`<br>`image-boundaries-final.log` |
| SF-A05 字符宽度 | 宽度设置仅影响新建终端；同一歧义字符在旧/新终端的窄/宽格位置不同且复制文本一致，旧终端不被重建。 | `internal/desktop::TestEvidenceA05WidthSettingOnlyNewTerminal` | `logic-final.log` |
| SF-E05 补充密码 | 唯一/无匹配/歧义/重复来源/已有密码/目录限定/后续解密失败七组通过；ID、其他属性与原凭据库不被预处理改写，失败不返回半套密码更新。 | `internal/core::TestEvidenceE05MissingPasswordMatrix` | `core-evidence.log` |

新增用例分别位于 `internal/core/windows_evidence_test.go`、`internal/core/agent_evidence_windows_test.go`、`internal/desktop/windows_evidence_test.go`、`internal/terminal/windows_evidence_test.go`。Agent 用例显式 opt-in，尝试以排他创建方式建立合成 Windows Agent 管道；已有 Agent 时跳过，不能接管或读取用户 Agent。本次 allowed/wrong-key/empty 三组实际通过。

图片远端用例经真实 SSH/SFTP 读写；测试对端使用 `internal/desktop/image_permission_evidence_test.go` 的 POSIX mode 模型，避免将 Windows 文件系统权限冒充 Linux 原生 chmod。该结果只关闭 M06 成功/失败提醒，不关闭 M03 的 Linux 原生权限验收。M07 的 20/200 MiB 存储边界、160 MiB 输入、20000 单边和 4000 万像素采用恰好边界与超一单位；实际 PNG 编码输出另取确定性噪声图跨越 20 MiB，不声称编码图恰好等于 20 MiB。

M06 外层设置取消的预期是保留已经成功的粘贴规则；失败/取消本次粘贴不得新增规则。旧收尾批次中的“外层取消不记”表述过宽，本轮按生产入口、成功时机及原基线明确这一点，不扩大功能。

## SF-C10 修复与前后证据

原生产关闭路径忽略 listener.Close 的错误，并直接释放同 UUID 隧道占用。复现使用实际绑定回环端口的监听器，仅注入 Close 失败：端口仍占用，第二个 transport 却能取得同 UUID 的所有权。`tunnel-before.log` 保留修复前失败，不以静态判断代替复现。

修改仅涉及 `internal/core/transport.go`、`internal/core/tunnel_ownership.go`、`internal/core/tunnels.go`：关闭流程完成且所有监听确认关闭（成功或 net.ErrClosed）后才释放占用；关闭错误仍保留跟踪和占用。后续确认关闭可释放。没有新增隧道种类、跨进程占用协调或新 UI。

`tunnel-after.log` 记录失败关闭保持占用、实际释放后可再持有、认证中/连接后的重复打开、正常关闭重开及克隆共享测试通过。`logic-final.log` 另验证两个应用窗口模型使用相同 UUID 时共享这一所有权结果。原始窗口拖动/输入不属于本条关闭的证据。

## 25 项真实窗口待验收

窗口自动化先遇到 `IGraphicsCaptureItemInterop.CreateForMonitor ... 0x80070057` 及 `failed to activate captured window`。用户恢复会话后短暂捕获隔离程序并打开 OShell 菜单，随后再次无法捕获/激活；没有成功完成关于页等条目的有限用例，也没有保存可用于逐项验收的截图。用户明确回复：**“目前无法保持，记录窗口项待验收”**。因此停止真实 UI 重试，以下各项均未完成，保留原步骤；不是逐项验证失败，也不将已有 Windows 重新计为缺少平台环境。

| 原条目 | 本轮结果 | 会话稳定后的原有限验收步骤 |
| --- | --- | --- |
| SF-S06 列表交互 | 未完成，按用户要求待验收 | 键鼠多选、批量连接、改名提交/取消一致；目录打开与会话连接正确分派；编辑输入不误触连接 |
| SF-P06 输入与回显 | 未完成，按用户要求待验收 | 显隐保留 Unicode/选择/焦点；重开不回显旧密文，退出清理输入 |
| SF-W05 标签排列 | 未完成，按用户要求待验收 | 排列/合组不重建连接，窄窗可滚动访问；Option滚轮、横向手势及历史边缘让出滚动按规则验；不承诺重排后旧组比例不变 |
| SF-W07 跨窗口迁移 | 未完成，按用户要求待验收 | 真鼠标覆盖内容、标签栏、空组/空窗、滚动插入、拒收和Esc；拖出须在源窗外12px扩展边界外且有效屏幕内 |
| SF-W09 快速切换/连接 | 未完成，按用户要求待验收 | 身份稳定、隐藏组展开且定位对应分屏；200项截断提示、无结果/超长查询、IME/回车/方向键及取消恢复焦点正确 |
| SF-W10 专注全屏 | 未完成，按用户要求待验收 | 连续切换与进入/退出失败状态正确；输入法/搜索/弹窗Esc各自处理；不重建连接、标签或草稿 |
| SF-W11 全心专注 | 未完成，按用户要求待验收 | 开关保留草稿/连接/布局；进入失败回退、退出全屏清状态；快捷键与已有配置冲突时不抢占 |
| SF-T06 本机与嵌套主机 | 未完成，按用户要求待验收 | 本机与嵌套进入/返回正确更新；主机信息、配置目标、本机实际 TCP 对端三者区分 |
| SF-T09 信息卡生命周期 | 未完成，按用户要求待验收 | 外部输入事件关闭且继续传递；卡片内部复制不关闭；连接字段变化后旧身份卡消失 |
| SF-Y16 本机配置变化 | 未完成，按用户要求待验收 | 外部修改不被旧草稿覆盖；本机冲突草稿阻止继续保存/远端同步；解决时再变化拒写，放弃仅删草稿 |
| SF-I01 常规交互 | 未完成，按用户要求待验收 | 中文/组合字符提交一次；焦点切换、关闭及取消不误写目标。原版未显式清理组字的生命周期须实机核验 |
| SF-I02 选择 | 未完成，按用户要求待验收 | 软换行/宽字/双击反向拖选正确；失焦/隐藏取消拖动仍是验收要求，原版未见主动取消路径，须记录差异 |
| SF-I18 键盘协议 | 未完成，按用户要求待验收 | 按下/重复/释放逐字节正确，IME/AltGr和快捷键消费不泄漏或重发；已知缺口须记录修正/保留预期，不把合成事件当物理布局验收 |
| SF-I19 鼠标与焦点 | 未完成，按用户要求待验收 | 跟踪与本地选择优先级正确，隐藏/迁移/释放无旧目标输入；已定位源码差异分别验证，不能以模式枚举或关闭闸门代替完整事件验收 |
| SF-I20 查询与状态 | 未完成，按用户要求待验收 | 按具体回复表核验字节、参数、宿主回退和固定报告；已知误报/序列化/栈差异须明确修正或保留预期。响铃、进度钳位/状态/超时正确；源码回复不等于宣告能力全部可用 |
| SF-I21 刷新与重置 | 未完成，按用户要求待验收 | 逐组核验保留/清理、解除同步及有限时序场景；已知差异明确处理，不用“完全重置”名称代替结果；同步期画面与恢复须有实际渲染证据 |
| SF-I23 Unicode 图形占位 | 未完成，按用户要求待验收 | 核对附加字符省略/继承、图像与位置身份、缺失/删除、单格裁剪、缩放及同 ID 替换；不将未调用辅助函数或完整图形介质算作本项 |
| SF-I25 特殊行显示 | 未完成，按用户要求待验收 | 固定两路径画面与逻辑坐标、切回普通行、擦除/滚动/回收/缩放和校准场景；已知差异明确修正或保留预期，不能用模式字段存在代替显示与命中验收 |
| SF-F01 文件窗口 | 未完成，按用户要求待验收 | 切换/复制/排序/最后页关闭正确；忙时取消关闭保留任务；输入法组字优先于 Esc 最小化 |
| SF-O04 七种发送范围 | 未完成，按用户要求待验收 | 七种范围分别命中；重开恢复模式但不恢复旧对象选择，空选择不回退全部；其他窗口配置更新不冒称即时切换当前窗口运行模式 |
| SF-O09 焦点与弹窗 | 未完成，按用户要求待验收 | 菜单跟踪优先，Esc 取消弹窗不触发唯一肯定按钮；文件窗 Esc 最小化、Cmd+W 关闭；密码 Ctrl+A 仅密码编辑器全选；一次事件不双发 |
| SF-A01 界面主题 | 未完成，按用户要求待验收 | 设置窗预览/颜色面板、取消和应用正确；应用不重建连接；跟随系统与强制浅深分别验 |
| SF-A04 图标与 Emoji | 未完成，按用户要求待验收 | PUA 有/无字形、四分隔符接缝、Emoji 开关/VS15/VS16/组合样例正确，显示处理不改复制字符；不要求跨系统像素相同 |
| SF-A09 未读状态 | 未完成，按用户要求待验收 | 固定正文/BEL/标题/分块控制/ASCII空白样例；应用激活、窗口焦点、最小化及放大分屏分别核对已读范围，不泛称全部 Unicode 空白均忽略 |
| SF-U07 关于与许可 | 未完成，按用户要求待验收 | 显示与实际构建一致，链接/许可证可访问，离线基本信息可见 |

继续验收时使用最终 EXE 的隔离合成目录、持续连接且未锁屏的 Windows 会话；逐项记录真实键鼠/IME 输入、屏幕与必要字节结果。不得用单纯启动、UI.Tester 或菜单截图替代整项通过。I22/I24/A06 等原本同时缺其他平台环境的条目仍按原分类保留，不计入这 25 项，也没有在本轮重开新的功能范围。

## 回归、失败记录与复验入口

`validation/windows37/regression.jsonl` 的 7 包完整回归没有失败：

| 包 | 结果 | 耗时（秒） |
| --- | --- | ---: |
| `oshell-desktop/internal/terminal/internal/bidi` | pass | 1.834 |
| `oshell-desktop/internal/terminal/internal/bidilayout` | pass | 2.178 |
| `oshell-desktop/internal/terminal/internal/pty` | pass | 4.378 |
| `oshell-desktop/internal/terminal/internal/vt` | pass | 8.771 |
| `oshell-desktop/internal/terminal` | pass | 19.535 |
| `oshell-desktop/internal/desktop` | pass | 81.464 |
| `oshell-desktop/internal/core` | pass | 261.397 |

该次日志有 47 条 skip 事件，逐条保留原义；不将 skip 视为 pass。共享 Go 兼容样本包含在 core 回归，真实 Swift 互通仍缺 macOS 环境。Agent、内嵌资源及实际 ZIP 签名等 opt-in 项由专项日志补证；其他原待验环境不因包级 pass 关闭。

`go vet ./internal/core ./internal/desktop ./internal/terminal/...` 通过（`vet.log`）。整包回归编译后新增的 M06 远端提醒、M07 解码/输出边界分别定向执行通过：`remote-image-final.log` 2.596 s，`image-boundaries-final.log` 3.318 s；这两组不冒称已包含在先前的整包日志。

除 C10 生产失败外，初次标题测试遗漏 Profile 的完整默认值，导致 fixture 比较失败；改用完整 NewProfile 默认，不改生产行为。远端图片初次 fixture 缺目录及 POSIX mode 支持，修正测试对端后通过。`desktop-evidence.log`、`logic-evidence.log`、`remote-image-evidence.log` 保留原始失败；最终结果分别以 `title-evidence.log`、`logic-final.log`、`remote-image-final.log` 为准。没有删除失败日志或降低产品断言。

下列命令供重验；运行库采用项目现有固定构建入口。本轮使用与最终包相同的原生 DLL，SHA256 `2aeefc8a283d1738ecbfecb83abb3d509810983c29591d9f4dc32d41c9629a1c`，ABI 62。不得将其他版本运行库替代该证据。

```powershell
# 完整 7 包回归
python scripts/terminal_runtime.py --target windows/amd64 -- go test ./internal/core ./internal/desktop ./internal/terminal/... -count=1 -timeout=8m
# 本轮有限测试（先通过上述入口准备运行库，或按开发指南配置 MYGO_GHOSTTY_VT）
go test ./internal/core ./internal/desktop ./internal/terminal -run 'TestEvidence|TestTunnelsFollowTransportAcrossClonedChannels|TestPromptSuspensionDiscardsInteractiveInput' -count=1 -timeout=8m
# 仅空闲合成 Agent 管道可用时执行；用后移除该环境变量
$env:OSHELL_ACCEPTANCE_AGENT_PIPE='1'
go test ./internal/core -run '^TestEvidenceC01WindowsAgentAuthentication$' -count=1
Remove-Item Env:OSHELL_ACCEPTANCE_AGENT_PIPE
```

这些命令不能替代上表 25 项真实交互步骤。运行日志、空白测试目录及完整性 JSON 存在本机 `validation/windows37/`（不入库），报告保存测试入口和有限用例供复验；无真实配置、密码、私钥或 WebDAV 凭据写入报告。

## 固定便携包

已在本机构建 0.3.3 Windows amd64 单文件包；构建日志 `portable-build.log`，构建中的内嵌终端/存储验证分别通过（2.605 s / 0.896 s）。最终更新固定 `dist/windows-amd64-single-file/OShell Desktop.exe`、`dist/OShell-Desktop-windows-amd64-single-file.zip` 和 `dist/SHA256SUMS.txt`。

| 检查 | 结果与限制 |
| --- | --- |
| ZIP | CRC 完整、仅一个 EXE，解压 EXE 与最终程序一致；`package-integrity.json` |
| 独立目录启动 | 新空白目录中进程存活、建立数据锁、从隔离缓存加载内嵌 DLL，外部 DLL/数据环境变量未绕过便携模式；`portable-smoke.json` |
| 真实窗口 | 最初窗口就绪检查失败；后续验证仅为进程/运行库/数据目录启动，`GuiReady=false`、`InteractiveAcceptance=false`、`GracefulClose=false`。显示、正常退出、零窗口/单实例恢复本轮未验，不使用上轮结果冒充 |
| 测试清理 | 仅终止本轮新启动、无资料/无连接且路径确认的空白测试进程；没有终止用户会话 |
| 签名 | 存储密钥对验证、公钥内嵌、私钥排除通过；实际新 ZIP 的合成签名更新准备通过，`signing-key-verification.json` / `signed-package.log`（1.155 s）。不是 Windows Authenticode 证书验证 |
| 配置保留 | 替换前后原固定目录 11 个数据/配置文件集合与 SHA256 全部不变；只替换程序及发行产物，`dist-replacement.json` |

EXE：30,764,544 字节，SHA256 `318ae8cbc1fe026aac0e58fcba8031f00d7c8e1c88fc4dc93740c69997f4610f`。

ZIP：14,246,706 字节，SHA256 `2571a71b024b52507bd4893759d68beb2e3faabda7106ae27655a7e275e13ce8`。

## 收尾清单一致性

`checklist-verification.json` 核对原 161 行的 ID/域/名称/基线验收要求不变、恰好原 37 行更新、其余 124 行不变；43 项处置 CSV 不变；固定基线及历史差异 CSV 摘要不变。当前计数 94 + 0 + 41 + 25 + 1 = 161，不用测试数量或新增记录计算完成率。后续仅继续原表剩余有限验收，出现可复现问题仍归原 SF。
