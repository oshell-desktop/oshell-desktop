# Desktop 固定细分功能差异清单

当前验收状态见 [原 161 项最终验收汇总与收尾清单](oshell-desktop-final-acceptance.md)（[逐项 CSV](oshell-desktop-final-acceptance.csv)）。本表保留首次静态核查的“已实现 / 存在差异”结论，不作为修复后的最终验收状态。

核查日期：2026-10-24。范围唯一入口：[固定功能基线](oshell-feature-inventory.md)，标识 `OShell-0.2.92-scope-1`，13 域、161 个 SF 条目。原版固定提交 `3f4efd9c70c82323f4e20180117c928227000580`，逐项原版证据见[源码分析记录](oshell-source-analysis.md)。

Desktop 核查对象是当前工作树（包含已有未提交修改），HEAD 为 `62a620541680b687cedf89f8e48adcdb77576e33`。HEAD 不是这些工作树内容的完整快照；本清单也不认证现有 dist 包与工作树一致。核查沿应用入口、处理函数、数据/协议路径及本地第三方适配源码展开，历史 checklist/parity 只用来定位，未沿用其“已有回归”作为本轮通过证据。

本轮只形成清单，没有修改程序、协议、原版源码或构建产物，没有提交、推送或触发 Actions。[同内容 CSV](oshell-desktop-gap-analysis.csv) 可按 SF/状态筛选。

后续处置见 [43 项差异分类与修复优先级](oshell-desktop-remediation-plan.md)：在原 SF 下区分功能缺口、行为差异、平台替换与已有增强。2026-10-25 的九项 P1 修复结果已记录在 [P1 验证记录](oshell-desktop-p1-validation.md)及修复清单 CSV。本核查表保留首次核查状态作为历史输入，当前修复状态以修复清单为准，不用历史 43 项数字推算剩余缺陷或完成率。

## 结论口径

- **已实现**：已找到并静态核查主要生产路径，本轮未定位明确功能差异。不是“该条所有参数和边界均已证明等价”，也不是“已验收”；表内仍列出具体待验内容。
- **存在差异**：主要功能存在，但有部分缺失、行为/限制不同、有意加强或不能直接视作等价的平台替换。不能将这 43 项全部当成待修复缺陷。
- **缺失**：整个固定条目没有可达生产实现。本轮未发现这种整项空白；部分能力缺失全部归回原 SF 的“存在差异”，没有另建编号。
- **平台专属排除**：只有 SF-P10，按原基线排除 Touch ID；不是缺失，不追加 Windows Hello。

实现状态与验收状态分开。本清单所有条目均未由本轮升级为“已验收”；静态差异是源码结论，未执行的运行场景不写成已复现。未发现差异也不能替代真实 Windows/Linux 输入、渲染、网络及 Swift 双端验证。终端底层 I14–I26 已检查兼容入口与宿主交接，但本轮不是 Ghostty 全部内部代码或全部协议参数的逐字节审计。

| 功能域 | 固定条目 | 已实现 | 存在差异 | 整项缺失 | 平台排除 |
| --- | ---: | ---: | ---: | ---: | ---: |
| 会话资料与管理 | 14 | 10 | 4 | 0 | 0 |
| 密码与本机保护 | 10 | 5 | 4 | 0 | 1 |
| 连接与代理 | 14 | 11 | 3 | 0 | 0 |
| 窗口与导航 | 12 | 10 | 2 | 0 | 0 |
| 主机信息与标题 | 9 | 8 | 1 | 0 | 0 |
| 配置与同步 | 17 | 17 | 0 | 0 | 0 |
| 终端 | 27 | 16 | 11 | 0 | 0 |
| 图片粘贴 | 8 | 6 | 2 | 0 | 0 |
| 文件管理与传输 | 15 | 12 | 3 | 0 | 0 |
| 快捷操作 | 9 | 5 | 4 | 0 | 0 |
| 外观与资源 | 10 | 7 | 3 | 0 | 0 |
| 外部互通与迁移 | 9 | 7 | 2 | 0 | 0 |
| 更新与分发 | 7 | 3 | 4 | 0 | 0 |
| 合计 | 161 | 117 | 43 | 0 | 1 |

这些数字是本次分类统计，**不计算完成率**，不以源码/测试文件数量或新增缺陷记录增加功能数量。

## 优先关注的具体缺口

以下是主表中值得先处理的部分，不是新增任务编号，也不表示本轮开始修复。

| 固定归属 | 当前差异/缺失 | 后续有限验证输入与预期 |
| --- | --- | --- |
| SF-I27 | OSC52 写入仍接系统剪贴板，与原版禁用相反 | 设置合成剪贴板文本，再发送 OSC52 写入/查询；剪贴板不变且查询无回复 |
| SF-U03 | Windows/Linux 更新入口只有 SHA256，未走 Ed25519 签名验证 | 仅使用本地合成更新服务/密钥；坏签名和非签名元数据拒绝，合法包按确认流程处理；不访问真实发布或安装 |
| SF-I20 | 进度报告没有生产 UI 回调；默认响铃未接宿主 | 合成 BEL 与 OSC9;4 五状态，检查响铃、数值钳位和 15 秒消失 |
| SF-I06 | 日志写 ANSI 原始字节、同步文件写，无过滤和有界异步队列 | 分块 CSI/OSC、普通文本及模拟写入阻塞/错误；验证基线过滤和 4 MiB 停止语义 |
| SF-S14 | 子菜单项拖回根栏、拖动代次检查未接 | 一个子目录引用拖出、一个拖动中配置变化样例；不错误移动新对象 |
| SF-O02 | 30 条历史代替 50 条/字节限额，发送后回车选项未重置 | 51 条短命令、单条 >8 KiB、合计 >64 KiB、上下键草稿恢复及一次成功/失败发送 |
| SF-O08 | 未找到系统热键读取和读取失败提示 | 内部重复、一个已知系统冲突、系统读取失败；不能把后两项当“无冲突” |
| SF-E03、SF-E06 | 空目录保留缺口；容器限制和验证不同 | 空目录、4001 项 ZIP、单项 >2 MiB、ZIP64、头不一致/目录 CRC 错误各一合成样例 |
| SF-W01、SF-W02 | 最后窗关闭退出；前后导航仅当前组 | 两组加一个隐藏组检查编号/前后/MRU；最后窗关闭后应用生命周期按目标平台决议验收 |
| SF-S07、SF-P07、SF-P08 | 剪贴板代次、认证提示和缓存锁语义不同 | 令牌换出再恢复、通用/身份化密码提示、已连接会话锁缓存后输入与图片插入 |
| SF-F04、SF-C06 | 递归深度/链接行为及代理握手限制不同 | 深度 32/33、顶层/子链接、带密码 SOCKS5 返回无认证、CONNECT 超长头 |
| SF-T07、SF-O07 | 标题截断缺少；高亮优先级/限制不同 | 33 字符标题、同名多分屏、两条重叠高亮及 UTF-16 边界样例 |

原版本身已存在的缺口不要求盲目复现：例如 I04/O06 编辑后再次校验、I17 隐藏属性实际绘制。P02/C01/C04/C11/E08 等本机文件、SSH、PTY 和 IPC 替换以用户结果及兼容合同验收；已有额外能力不自动转为复刻要求。是否保留具体行为差异应在原 SF 下写明决定，不能悄悄删掉差异或放宽原保护约束。

## 161 项逐项对照

“已有实现”列只描述本轮读到的生产路径；“差异/缺失与验收边界”列区分具体偏差和未运行的验证。每行名称直接继承固定基线。源码链接指向本次工作树，后续代码变更须更新对应行。

### 会话资料与管理（14 项）

| 固定编号 | 细分功能 | 状态 | Desktop 已有实现 | 差异/缺失与验收边界 | Desktop 源码证据 |
| --- | --- | --- | --- | --- | --- |
| SF-S01 | 新建与编辑 | 已实现 | SSH/SFTP/FTP 新建、编辑、保存/连接及本地入口已接入。 | 本轮未定位固定字段的缺失；字段保存和取消仍需桌面验收。 | [internal/desktop/connection_editor.go](../internal/desktop/connection_editor.go)<br>[internal/desktop/app.go](../internal/desktop/app.go) |
| SF-S02 | 协议相关属性 | 存在差异 | 按协议切换属性页，FTP 保存清理 SSH 专用项。 | SFTP 仅禁用空闲发送开关，空闲间隔/字符串输入仍可编辑；与整组禁用要求不同。 | [internal/desktop/connection_editor.go](../internal/desktop/connection_editor.go)<br>[internal/desktop/editor_protocol.go](../internal/desktop/editor_protocol.go) |
| SF-S03 | 会话默认属性 | 已实现 | 独立会话默认草稿，新建资料时应用；复制走源资料。 | 本轮未定位覆盖既有资料的路径。 | [internal/core/defaults.go](../internal/core/defaults.go)<br>[internal/desktop/defaults.go](../internal/desktop/defaults.go)<br>[internal/desktop/editor_protocol.go](../internal/desktop/editor_protocol.go) |
| SF-S04 | 目录树 | 已实现 | 目录增改移、递归层级、同名/自包含检查、父目录保留。 | 普通目录移入 Links 转引用；不作为新增功能。 | [internal/core/session_moves.go](../internal/core/session_moves.go)<br>[internal/desktop/manager_drag.go](../internal/desktop/manager_drag.go)<br>[internal/desktop/manager_rename.go](../internal/desktop/manager_rename.go) |
| SF-S05 | 搜索 | 存在差异 | 字面多词 AND、Unicode 折叠、4096 字节/32 词限制及目录/协议过滤。 | 匹配/限额检查已有；侧栏查询无效时直接返回空列表，未展示超限错误；引用检索用显示名覆盖源名，未同时保留源会话名称词。 | [internal/core/session_search.go](../internal/core/session_search.go)<br>[internal/desktop/manager.go](../internal/desktop/manager.go)<br>[internal/desktop/sidebar.go](../internal/desktop/sidebar.go) |
| SF-S06 | 列表交互 | 已实现 | 表格选择、批量操作、双击/回车、F2 行内改名及失焦取消已接入。 | 键鼠多选和列调整尚未在本轮实机复测。 | [internal/desktop/manager.go](../internal/desktop/manager.go)<br>[internal/desktop/manager_rename.go](../internal/desktop/manager_rename.go)<br>[internal/desktop/manager_actions.go](../internal/desktop/manager_actions.go) |
| SF-S07 | 配置复制 | 存在差异 | 进程内配置快照、剪贴板令牌、凭据重绑定及事务复制。 | 缺少系统剪贴板 changeCount 校验：令牌被替换再恢复且中间未检查时，旧快照仍可有效；额外以 vault revision/代理快照限制粘贴。 | [internal/desktop/manager_clipboard.go](../internal/desktop/manager_clipboard.go)<br>[internal/desktop/manager_actions.go](../internal/desktop/manager_actions.go) |
| SF-S08 | 删除 | 已实现 | 删除规划、父子去重、关联引用清理、确认后提交。 | 本轮未定位整项缺失；并发变更/取消需固定用例验收。 | [internal/core/session_delete.go](../internal/core/session_delete.go)<br>[internal/desktop/manager_actions.go](../internal/desktop/manager_actions.go) |
| SF-S09 | 左侧会话栏 | 已实现 | 侧栏目录树、搜索、连接及管理操作接入共享资料。 | 侧栏改名弹窗与管理器行内改名分别保留。 | [internal/desktop/sidebar.go](../internal/desktop/sidebar.go)<br>[internal/desktop/session_catalog.go](../internal/desktop/session_catalog.go)<br>[internal/desktop/manager_actions.go](../internal/desktop/manager_actions.go) |
| SF-S10 | 根目录置顶 | 已实现 | 根会话/目录置顶、本机持久化、改名与删除后整理。 | 不对 Links 或任意子目录扩展置顶范围。 | [internal/desktop/session_pins.go](../internal/desktop/session_pins.go)<br>[internal/desktop/sidebar.go](../internal/desktop/sidebar.go) |
| SF-S11 | 快捷连接 | 已实现 | 默认本机项按工厂值、引用和凭据条件在展示投影中过滤。 | 过滤不删除配置；快捷连接按已有开关筛选。 | [internal/desktop/session_catalog.go](../internal/desktop/session_catalog.go)<br>[internal/desktop/quick_connection_menu.go](../internal/desktop/quick_connection_menu.go) |
| SF-S12 | `/Links` | 已实现 | 创建引用、添加当前会话、目录移动转引用、源身份保持。 | 不将引用编辑改为复制源资料。 | [internal/core/link_edit.go](../internal/core/link_edit.go)<br>[internal/core/session_moves.go](../internal/core/session_moves.go)<br>[internal/desktop/link_editor.go](../internal/desktop/link_editor.go) |
| SF-S13 | 引用完整性 | 已实现 | 引用名称/目录编辑、删除与目录保护、实际会话另行管理。 | 根目录及包含实际资料的目录保护已在操作路径区分。 | [internal/core/link_edit.go](../internal/core/link_edit.go)<br>[internal/desktop/link_menu.go](../internal/desktop/link_menu.go) |
| SF-S14 | 链接栏 | 存在差异 | 根栏排序、插入提示、移入目录及滚动容器已有。 | 子目录使用原生菜单，未找到菜单项拖回根栏的拖动源；根栏拖动载荷只有 Key/Owner，未见基线要求的配置代次校验。 | [internal/desktop/workspace.go](../internal/desktop/workspace.go)<br>[internal/desktop/link_menu.go](../internal/desktop/link_menu.go) |

### 密码与本机保护（10 项）

| 固定编号 | 细分功能 | 状态 | Desktop 已有实现 | 差异/缺失与验收边界 | Desktop 源码证据 |
| --- | --- | --- | --- | --- | --- |
| SF-P01 | 主密码 | 已实现 | 主密码设置/修改/移除、配置与密码库联合事务、解锁及 revision 检查。 | 源码存在完整主路径；密码错误/事务恢复及双端结果未因静态核查而通过。 | [internal/core/master_transaction.go](../internal/core/master_transaction.go)<br>[internal/core/vault.go](../internal/core/vault.go)<br>[internal/desktop/features.go](../internal/desktop/features.go) |
| SF-P02 | 本机加密保存 | 存在差异 | 本机密钥保护与原版密钥迁移读取均存在。 | 本机存储采用 credential-key 原始密钥及独立 vault，不是原版带 UUID 的 JSON 密钥格式；共享交换仍走约定协议，不能把本机文件直接互换。 | [internal/core/local_credential_key.go](../internal/core/local_credential_key.go)<br>[internal/core/vault.go](../internal/core/vault.go) |
| SF-P03 | 身份绑定 | 已实现 | 密码封装、身份绑定、复制/导入后解密再绑定、错误拒绝。 | 真实 Swift ↔ Go 往返另待 macOS；本轮 Go 合同样本尝试未完成，不能计为通过。 | [internal/core/mac_cipher.go](../internal/core/mac_cipher.go)<br>[internal/core/portable_archive.go](../internal/core/portable_archive.go)<br>[internal/core/vault.go](../internal/core/vault.go) |
| SF-P04 | 认证队列 | 已实现 | 应用级认证排队、问题/回显展示、取消、对象失效清理。 | Go SSH 使用进程内挑战队列替代 askpass/socket；不据此新增 helper 协议开发要求。 | [internal/desktop/authentication.go](../internal/desktop/authentication.go)<br>[internal/core/transport.go](../internal/core/transport.go) |
| SF-P05 | 成功后保存 | 存在差异 | 认证成功后保存，检查资料/密码库 revision 和真实身份。 | 保存调用也接入 FTP 登录及 SFTP Dial 成功路径；固定原版的 SSH 保存闸门不能概括为 Desktop 的同一规则。一次性入口仍需单独验证不落库。 | [internal/desktop/authenticated_password.go](../internal/desktop/authenticated_password.go)<br>[internal/desktop/app.go](../internal/desktop/app.go) |
| SF-P06 | 输入与回显 | 已实现 | 密码输入遮罩/显示切换及本机保护选择已有入口。 | 切换焦点、选区和不主动回填解密密码需交互验收。 | [internal/desktop/password_input.go](../internal/desktop/password_input.go)<br>[internal/desktop/password_protection.go](../internal/desktop/password_protection.go) |
| SF-P07 | 一次性凭据 | 存在差异 | 密码只自动提供一次；交互式认证区分密码问题与其他问题。 | 识别额外接受通用 password、password: 和中文密码提示，且允许多问题中填唯一识别项；比固定原版三个身份化提示更宽。 | [internal/core/password_prompt.go](../internal/core/password_prompt.go)<br>[internal/core/transport.go](../internal/core/transport.go) |
| SF-P08 | 安全状态保护 | 存在差异 | 可清除主密码缓存并要求重新解锁。 | Desktop 同时锁配置/工作区，普通输入与图片粘贴被锁定门禁阻止；原版缓存锁不等于启动锁，也不阻断已连接终端同类操作。 | [internal/desktop/configuration_unlock.go](../internal/desktop/configuration_unlock.go)<br>[internal/desktop/application.go](../internal/desktop/application.go)<br>[internal/desktop/security_view.go](../internal/desktop/security_view.go)<br>[internal/desktop/image_paste.go](../internal/desktop/image_paste.go) |
| SF-P09 | 风险提示 | 已实现 | 未设主密码提醒、确认记忆、专注模式隐藏。 | 同步投影排除 masterWarningAcknowledged，未作为跨设备提醒状态。 | [internal/desktop/master_warning.go](../internal/desktop/master_warning.go)<br>[internal/core/session_sync.go](../internal/core/session_sync.go) |
| SF-P10 | Touch ID | 平台专属排除 | Touch ID/苹果授权机制按固定基线排除。 | 不计缺失，不追加 Windows Hello。通用保护仍由 P01–P09 约束。 | [docs/oshell-feature-inventory.md](oshell-feature-inventory.md) |

### 连接与代理（14 项）

| 固定编号 | 细分功能 | 状态 | Desktop 已有实现 | 差异/缺失与验收边界 | Desktop 源码证据 |
| --- | --- | --- | --- | --- | --- |
| SF-C01 | SSH 连接 | 已实现 | SSH 密码/私钥/agent、PTY 尺寸和连接生命周期接入。 | Go SSH 替代系统 ssh；配置兼容范围差异集中记 C08，认证超时为 2 分钟。 | [internal/core/transport.go](../internal/core/transport.go)<br>[internal/desktop/app.go](../internal/desktop/app.go) |
| SF-C02 | 主机身份 | 已实现 | 未知主机确认、known_hosts 保存、已知指纹不匹配拒绝。 | 真实服务端首次/变更指纹用例待验收，不因回调存在宣称握手通过。 | [internal/core/transport.go](../internal/core/transport.go)<br>[internal/core/ssh.go](../internal/core/ssh.go) |
| SF-C03 | 旧 SSH 兼容 | 存在差异 | 旧 SSH 兼容开关会扩展算法。 | 直接合并 x/crypto 的 SupportedAlgorithms 与 InsecureAlgorithms；不是固定原版逐项算法表，受当前依赖支持集约束。 | [internal/core/transport.go](../internal/core/transport.go)<br>[go.mod](../go.mod) |
| SF-C04 | 本地终端 | 已实现 | 本地终端使用平台 PTY，Windows ConPTY、Unix PTY。 | 平台机制替换；不追加原版范围外终端类型。 | [internal/terminal/internal/pty/pty_windows.go](../internal/terminal/internal/pty/pty_windows.go)<br>[internal/terminal/internal/pty/pty_unix.go](../internal/terminal/internal/pty/pty_unix.go)<br>[internal/desktop/app.go](../internal/desktop/app.go) |
| SF-C05 | 共享代理管理 | 已实现 | 共享代理目录、创建编辑删除、会话引用与凭据绑定。 | 2000 项限制、被引用及链路有效性检查已有。 | [internal/core/proxy_catalog.go](../internal/core/proxy_catalog.go)<br>[internal/desktop/proxy_manager.go](../internal/desktop/proxy_manager.go) |
| SF-C06 | 代理类型 | 存在差异 | HTTP CONNECT、SOCKS4/4a/5 的拨号与认证路径存在。 | SOCKS5 带凭据时仍同时宣告无认证；HTTP ReadResponse 路径未见原版 16 KiB 响应头硬限。 | [internal/core/proxy.go](../internal/core/proxy.go) |
| SF-C07 | 多级路由 | 已实现 | 代理链解析、缺失/循环拒绝、最多 8 层及 SSH 跳板。 | 按上游至下游建立；不扩为任意代理协议。 | [internal/core/proxy_catalog.go](../internal/core/proxy_catalog.go)<br>[internal/core/ssh_jump.go](../internal/core/ssh_jump.go)<br>[internal/core/proxy.go](../internal/core/proxy.go) |
| SF-C08 | 路由解析 | 存在差异 | 调用 ssh -G 解析别名、user/port/identityfile/ProxyJump/ProxyCommand。 | 仅消费列出的字段；缺系统 ssh 时回退原资料。不能等价承诺系统 OpenSSH 实际执行时的其余配置语义。 | [internal/core/ssh_config.go](../internal/core/ssh_config.go)<br>[internal/core/transport.go](../internal/core/transport.go) |
| SF-C09 | 隧道 | 已实现 | 本地/远程端口转发及动态代理由连接上的转发规则启动。 | 真实监听、远端许可及失败回收待网络验收。 | [internal/core/transport.go](../internal/core/transport.go)<br>[internal/core/tunnels.go](../internal/core/tunnels.go)<br>[internal/desktop/connection_editor.go](../internal/desktop/connection_editor.go) |
| SF-C10 | 隧道占用 | 已实现 | 以会话 UUID 原子占用隧道所有权，重复连接跳过已有隧道。 | 进程内所有权替代控制 socket 锁；需真实并发连接验证释放时机。 | [internal/core/tunnel_ownership.go](../internal/core/tunnel_ownership.go)<br>[internal/core/transport.go](../internal/core/transport.go) |
| SF-C11 | SSH 连接复用 | 已实现 | SSH transport 引用计数、Clone/CloneFiles、最终持有者释放。 | 复用进程内已认证连接，不生成 OpenSSH ControlMaster 文件；复用失败不自动新认证。 | [internal/core/transport.go](../internal/core/transport.go)<br>[internal/desktop/app.go](../internal/desktop/app.go) |
| SF-C12 | 保活 | 已实现 | SSH 请求保活、TCP KeepAlive、间隔和最大未响应次数设置。 | 计时及断网行为需网络验收；平台套接字实现不同。 | [internal/core/transport.go](../internal/core/transport.go)<br>[internal/core/options.go](../internal/core/options.go)<br>[internal/desktop/defaults.go](../internal/desktop/defaults.go) |
| SF-C13 | 空闲字符串 | 已实现 | 空闲发送、转义解析、输入/传输门禁与运行期计时重置。 | 不将保活字符串广播给其他分屏。 | [internal/core/transport.go](../internal/core/transport.go)<br>[internal/terminal/idle_input.go](../internal/terminal/idle_input.go) |
| SF-C14 | 连接身份展示 | 已实现 | 从实际连接记录 TCP 对端，主机详情独立展示。 | 代理链时可为第一跳；不将配置域名或 DNS 结果冒充已连接对端。 | [internal/core/transport.go](../internal/core/transport.go)<br>[internal/desktop/tab_details.go](../internal/desktop/tab_details.go) |

### 窗口与导航（12 项）

| 固定编号 | 细分功能 | 状态 | Desktop 已有实现 | 差异/缺失与验收边界 | Desktop 源码证据 |
| --- | --- | --- | --- | --- | --- |
| SF-W01 | 多窗口 | 存在差异 | 多窗口和当前窗菜单、共享 hub，窗口各自持有标签与发送目标。 | 仅 darwin 注册空 OnWindowAllClosed；Windows/Linux 走 mygo.lastWindowClosed 默认 Quit，最后窗口关闭退出应用，与固定基线不同。 | [internal/desktop/application.go](../internal/desktop/application.go)<br>[internal/desktop/layout.go](../internal/desktop/layout.go)<br>[third_party/mygo/app.go](../third_party/mygo/app.go) |
| SF-W02 | 标签导航 | 存在差异 | 组内编号跳转、最近使用切换及前后动作已有。 | nextTab/previousTab 调用 navigateTab，只遍历当前组；基线要求前后遍历当前窗口全部非隐藏标签。 | [internal/desktop/groups.go](../internal/desktop/groups.go)<br>[internal/desktop/shortcuts.go](../internal/desktop/shortcuts.go) |
| SF-W03 | 标签操作 | 已实现 | 空白/本地入口、活跃分屏复制、复用连接、重连、排序和拖动滚动。 | 不复制历史或整棵分屏树；双击门禁与右键入口分别存在。 | [internal/desktop/tab_duplicate.go](../internal/desktop/tab_duplicate.go)<br>[internal/desktop/app.go](../internal/desktop/app.go)<br>[internal/desktop/tab_drag_scroll.go](../internal/desktop/tab_drag_scroll.go) |
| SF-W04 | 终端分屏 | 已实现 | 嵌套分屏、深度优先切换、单叶关闭、放大与尺寸状态。 | 分隔比例及关闭后的焦点保持待键鼠验收。 | [internal/desktop/workspace.go](../internal/desktop/workspace.go)<br>[internal/desktop/navigation.go](../internal/desktop/navigation.go)<br>[internal/desktop/pane_sizing.go](../internal/desktop/pane_sizing.go) |
| SF-W05 | 标签排列 | 已实现 | 水平/垂直/网格/单组排列、中心合组和边缘分组。 | 已有命名组按组排列，合回单组移除组结构。 | [internal/desktop/groups.go](../internal/desktop/groups.go)<br>[internal/desktop/group_sizing.go](../internal/desktop/group_sizing.go) |
| SF-W06 | 自定义标签组 | 已实现 | 命名空组、改名、显隐、移入、解散保留会话和名称限制。 | 运行期窗口状态，不追加持久布局模板。 | [internal/desktop/groups.go](../internal/desktop/groups.go) |
| SF-W07 | 跨窗口迁移 | 已实现 | 同进程跨窗拖放、指定位置插入、拖出新窗和取消/模态门禁。 | Windows/Linux 原生数据拖动替代 AppKit；12px 边界、屏幕范围和拒收窗场景仍待真鼠标验收。 | [internal/desktop/tab_window_drag.go](../internal/desktop/tab_window_drag.go)<br>[internal/desktop/tab_move_guard.go](../internal/desktop/tab_move_guard.go)<br>[internal/desktop/application.go](../internal/desktop/application.go) |
| SF-W08 | 迁移连续性 | 已实现 | 原 tab/pane 迁移、owner 重绑、源发送/同步/预览目标清理。 | Desktop 文件窗由 hub 统一持有，生命周期组织与原版附属窗不同；迁移不重建连接。 | [internal/desktop/application.go](../internal/desktop/application.go)<br>[internal/desktop/input_target_detach.go](../internal/desktop/input_target_detach.go)<br>[internal/desktop/file_window.go](../internal/desktop/file_window.go) |
| SF-W09 | 快速切换/连接 | 已实现 | 全部/已打开/已保存检索，定位分屏、隐藏组及保存资料连接。 | 前 200 项与查询错误提示应以固定交互用例验收。 | [internal/desktop/navigation.go](../internal/desktop/navigation.go)<br>[internal/core/session_search.go](../internal/core/session_search.go) |
| SF-W10 | 专注全屏 | 已实现 | 专注全屏状态机、外观隐藏、失败/转换状态回收。 | 目标平台全屏替代 macOS Spaces，不能把 headless 状态变化当真实全屏通过。 | [internal/desktop/focus_fullscreen.go](../internal/desktop/focus_fullscreen.go)<br>[internal/desktop/workspace.go](../internal/desktop/workspace.go) |
| SF-W11 | 全心专注 | 已实现 | 全心专注自动进入全屏、隐藏附属标题/发送栏、退出保留普通专注。 | 本机快捷键和当前草稿独立于共享偏好。 | [internal/desktop/focus_fullscreen.go](../internal/desktop/focus_fullscreen.go)<br>[internal/desktop/workspace.go](../internal/desktop/workspace.go)<br>[internal/desktop/shortcuts.go](../internal/desktop/shortcuts.go) |
| SF-W12 | 关闭确认 | 已实现 | 标签/分屏本机确认、窗口及退出活动计数、重连确认。 | 忙碌文件页和运行工具分别计入；取消与跨窗退出待交互验收。 | [internal/desktop/session_close.go](../internal/desktop/session_close.go)<br>[internal/desktop/workspace_close.go](../internal/desktop/workspace_close.go)<br>[internal/desktop/application.go](../internal/desktop/application.go) |

### 主机信息与标题（9 项）

| 固定编号 | 细分功能 | 状态 | Desktop 已有实现 | 差异/缺失与验收边界 | Desktop 源码证据 |
| --- | --- | --- | --- | --- | --- |
| SF-T01 | 标题方式 | 已实现 | 四种标题模式及主机观察策略分离，名称模式保留详情身份。 | 探测/集成模式的变化不会更换连接身份。 | [internal/desktop/session_titles.go](../internal/desktop/session_titles.go)<br>[internal/core/host_identity.go](../internal/core/host_identity.go) |
| SF-T02 | 默认与覆盖 | 已实现 | 本机默认/逐资料覆盖、共享 titleMode 回退、应用与复制继承。 | 本机设置 revision 冲突与共享保存失败分开处理。 | [internal/desktop/session_titles.go](../internal/desktop/session_titles.go)<br>[internal/desktop/sidebar.go](../internal/desktop/sidebar.go)<br>[internal/desktop/manager_actions.go](../internal/desktop/manager_actions.go) |
| SF-T03 | Shell 集成 | 已实现 | 随包 Bash/Zsh 集成脚本及 OSC 777 身份上报解析接入。 | 不自动改远端启动文件；真实两种 shell 钩子共存待验收。 | [internal/terminal/shell_integration.go](../internal/terminal/shell_integration.go)<br>[internal/core/host_identity.go](../internal/core/host_identity.go)<br>[internal/desktop/host_identity.go](../internal/desktop/host_identity.go) |
| SF-T04 | 被动识别 | 已实现 | 固定提示符/OSC 标题/目录线索解析，超限和认证提示门禁。 | 未识别值显示未知；不通过 DNS 猜测嵌套主机身份。 | [internal/core/host_identity.go](../internal/core/host_identity.go)<br>[internal/desktop/host_identity.go](../internal/desktop/host_identity.go) |
| SF-T05 | 主动探测 | 已实现 | 80ms 扫描、300ms 稳定等待、4s 探测超时及代次/输入状态检查。 | 需要远端真实 shell 验证回显和 TUI 门禁，不能声明识别所有主题。 | [internal/desktop/host_identity.go](../internal/desktop/host_identity.go)<br>[internal/core/host_probe.go](../internal/core/host_probe.go) |
| SF-T06 | 本机与嵌套主机 | 已实现 | 本机身份快照、别名归一、嵌套切换命令观察及身份重置。 | 主机展示和实际 TCP 对端分开；多网卡选择仍待平台验收。 | [internal/core/host_identity.go](../internal/core/host_identity.go)<br>[internal/desktop/host_identity.go](../internal/desktop/host_identity.go) |
| SF-T07 | 分屏聚合标题 | 存在差异 | 按布局首次出现聚合、同名计数、最多两名与剩余分屏计数。 | displayTabTitle 未对每个标题做 32 字符截断，paneDisplayTitle 直接使用资料名；整体控件截宽不能替代基线的逐名规则。 | [internal/desktop/session_titles.go](../internal/desktop/session_titles.go) |
| SF-T08 | 悬停信息卡 | 已实现 | 当前分屏字段、实际对端、逐项/全部复制及完整连接详情。 | 复制按钮 KeepFocusOnClick；缺失字段不提供伪值复制。 | [internal/desktop/tab_details.go](../internal/desktop/tab_details.go)<br>[internal/desktop/host_identity.go](../internal/desktop/host_identity.go) |
| SF-T09 | 信息卡生命周期 | 已实现 | 500ms 展示/250ms 离开关闭、外部交互/失焦/身份变化关闭。 | 平台原生事件和卡片内部复制需实机验收。 | [internal/desktop/tab_details.go](../internal/desktop/tab_details.go)<br>[internal/desktop/application.go](../internal/desktop/application.go) |

### 配置与同步（17 项）

| 固定编号 | 细分功能 | 状态 | Desktop 已有实现 | 差异/缺失与验收边界 | Desktop 源码证据 |
| --- | --- | --- | --- | --- | --- |
| SF-Y01 | 本机工作副本 | 已实现 | 本机配置/密码库工作副本、原版迁移及备份路径独立于同步目录。 | 不共享本机密钥文件；真实旧数据迁移仅用合成资料验收。 | [internal/desktop/native_migration.go](../internal/desktop/native_migration.go)<br>[internal/core/sync_location.go](../internal/core/sync_location.go)<br>[internal/core/shared_sync.go](../internal/core/shared_sync.go) |
| SF-Y02 | 同步目录 | 已实现 | 新建/已有同步目录选择、重叠检查、空目录条件和条件写入。 | 目录与 WebDAV 通过单一后端配置互斥；云占位/文件冲突需平台样例验收。 | [internal/core/sync_location.go](../internal/core/sync_location.go)<br>[internal/core/sync_transport.go](../internal/core/sync_transport.go)<br>[internal/desktop/sync_location.go](../internal/desktop/sync_location.go) |
| SF-Y03 | WebDAV | 已实现 | GET/PROPFIND/MKCOL/PUT、Basic Auth、拒重定向、强 ETag、体积限制和目录发现。 | 无 WebDAV 文件管理扩张；HTTP 只按显式配置使用。 | [internal/core/sync_transport.go](../internal/core/sync_transport.go)<br>[internal/core/webdav_directory.go](../internal/core/webdav_directory.go) |
| SF-Y04 | 同步前提 | 已实现 | 同步要求已解锁主密码，WebDAV 凭据受本机保护，保护变更走事务。 | 取消同步设置不自动撤销已完成的主密码事务。 | [internal/core/sync_settings.go](../internal/core/sync_settings.go)<br>[internal/desktop/sync_options_protection.go](../internal/desktop/sync_options_protection.go)<br>[internal/core/master_transaction.go](../internal/core/master_transaction.go) |
| SF-Y05 | 默认范围 | 已实现 | 会话类默认选中，包含会话/目录/引用/代理及必要凭据。 | 未选类别投影不进入本轮比较，写入时保留远端未选类别。 | [internal/core/session_sync.go](../internal/core/session_sync.go)<br>[internal/core/sync_policy.go](../internal/core/sync_policy.go) |
| SF-Y06 | 可选范围 | 已实现 | 四类内容独立选择，默认其余关闭；排除更新偏好和本机提醒。 | 先完整解析/验证再投影；不以未选为理由丢未知字段。 | [internal/core/session_sync.go](../internal/core/session_sync.go)<br>[internal/core/shared_sync.go](../internal/core/shared_sync.go) |
| SF-Y07 | 策略 | 已实现 | 自动/启动/变更调度，2 秒合并、60 秒检查/重试默认及暂停状态。 | 共享 hub 管理定时器；实际休眠/唤醒时序待平台验收。 | [internal/core/sync_policy.go](../internal/core/sync_policy.go)<br>[internal/desktop/sync_policy.go](../internal/desktop/sync_policy.go)<br>[internal/desktop/shared_sync.go](../internal/desktop/shared_sync.go) |
| SF-Y08 | 首次接入 | 已实现 | 首次采用共享/合并选择、冲突草稿和提交前备份，相同内容省去选择。 | 取消不应用未确认计划；真实双端首次同步待验收。 | [internal/core/shared_sync.go](../internal/core/shared_sync.go)<br>[internal/desktop/shared_sync.go](../internal/desktop/shared_sync.go) |
| SF-Y09 | 多节点合并 | 已实现 | 按会话 UUID 三方合并，整会话冲突与其他顶层原子字段选择。 | 不扩为任意字段级自动融合。 | [internal/core/shared_merge.go](../internal/core/shared_merge.go)<br>[internal/core/session_sync.go](../internal/core/session_sync.go) |
| SF-Y10 | 并发写保护 | 已实现 | If-Match/If-None-Match、412/远端删除保护、上传后本地再变的延续状态。 | 本轮静态读到提交前后 revision/ETag 检查；网络并发用例另验。 | [internal/core/sync_transport.go](../internal/core/sync_transport.go)<br>[internal/core/shared_sync.go](../internal/core/shared_sync.go) |
| SF-Y11 | 无变化处理 | 已实现 | UUID 大小写、默认值与空集合规范化；语义无变化不重复 PUT。 | v1→v2 协议升级可有一次必要写入，不算无变化重复上传。 | [internal/core/shared_merge.go](../internal/core/shared_merge.go)<br>[internal/core/session_sync.go](../internal/core/session_sync.go)<br>[internal/core/shared_sync.go](../internal/core/shared_sync.go) |
| SF-Y12 | 错误保护 | 已实现 | 密码/数据/版本/冲突错误拒绝应用，保留旧配置和冲突草稿。 | 未知字段更严格拒绝符合项目约定，不能为复刻放宽覆盖保护。 | [internal/core/shared_sync.go](../internal/core/shared_sync.go)<br>[internal/core/shared_merge.go](../internal/core/shared_merge.go)<br>[internal/core/shared_protocol.go](../internal/core/shared_protocol.go) |
| SF-Y13 | 证书 | 已实现 | WebDAV 证书按 host/port/证书精确绑定，验证有效期并本机加密保存。 | 只适用显式确认的 WebDAV 例外；不改变其他网络 TLS。 | [internal/core/webdav_certificate.go](../internal/core/webdav_certificate.go)<br>[internal/desktop/webdav_certificate.go](../internal/desktop/webdav_certificate.go) |
| SF-Y14 | 状态窗口 | 已实现 | 共享同步状态、独立非模态状态窗、工具栏与菜单展示。 | 关闭状态窗与取消同步分别处理。 | [internal/desktop/sync_status_window.go](../internal/desktop/sync_status_window.go)<br>[internal/desktop/sync_presentation.go](../internal/desktop/sync_presentation.go)<br>[internal/desktop/shared_sync.go](../internal/desktop/shared_sync.go) |
| SF-Y15 | 诊断 | 已实现 | 安全事件/失败分类、7 天/2000 条/大小裁剪、状态摘要及日志导出。 | 导出不带本机指纹；不将原始凭据/响应写作诊断正文。 | [internal/core/sync_diagnostics.go](../internal/core/sync_diagnostics.go)<br>[internal/desktop/sync_diagnostics.go](../internal/desktop/sync_diagnostics.go) |
| SF-Y16 | 本机配置变化 | 已实现 | 本机外部变更检测、持久冲突草稿及明确解决/放弃路径。 | 并发变更不直接覆盖；激活触发与实际多进程冲突待验收。 | [internal/desktop/local_conflict.go](../internal/desktop/local_conflict.go)<br>[internal/core/shared_sync.go](../internal/core/shared_sync.go)<br>[internal/core/sync_connection.go](../internal/core/sync_connection.go) |
| SF-Y17 | 格式兼容 | 已实现 | 读 v1/v2、写 v2，JSON 合同及生成的 Swift 合同约束字段。 | Go 样本不代替真实 Swift 双端；固定原版较宽的信封解析不作为降低校验的理由。 | [internal/core/shared_configuration_schema.json](../internal/core/shared_configuration_schema.json)<br>[internal/core/shared_protocol.go](../internal/core/shared_protocol.go)<br>[docs/configuration-protocol-v2.md](configuration-protocol-v2.md) |

### 终端（27 项）

| 固定编号 | 细分功能 | 状态 | Desktop 已有实现 | 差异/缺失与验收边界 | Desktop 源码证据 |
| --- | --- | --- | --- | --- | --- |
| SF-I01 | 常规交互 | 已实现 | 键盘、组字覆盖/提交/取消、焦点和终端常规交互路径接通。 | Windows/Linux 事件适配不同；真实中文 IME、AltGr 和窗口生命周期待验收。 | [internal/terminal/view.go](../internal/terminal/view.go)<br>[internal/terminal/preedit.go](../internal/terminal/preedit.go)<br>[internal/terminal/keys.go](../internal/terminal/keys.go) |
| SF-I02 | 选择 | 已实现 | 字符/词/行/Shift 选择、连续双击、宽字及越界滚动实现。 | 失焦取消与历史裁剪属于固定验收；不能以选择状态结构存在判通过。 | [internal/terminal/word_click.go](../internal/terminal/word_click.go)<br>[internal/terminal/selection_autoscroll.go](../internal/terminal/selection_autoscroll.go)<br>[internal/terminal/internal/vt/selection.go](../internal/terminal/internal/vt/selection.go)<br>[third_party/ghostty/oshell_selection.zig](../third_party/ghostty/oshell_selection.zig) |
| SF-I03 | 复制 | 已实现 | 手动/自动复制、首尾清理、逻辑行提取和宽字占位处理。 | 不导出原始控制流或富文本。 | [internal/terminal/oshell.go](../internal/terminal/oshell.go)<br>[internal/terminal/view.go](../internal/terminal/view.go)<br>[internal/terminal/internal/vt/selection.go](../internal/terminal/internal/vt/selection.go) |
| SF-I04 | 文本粘贴 | 存在差异 | 多入口粘贴、可编辑预览、目标快照、换行及 bracketed paste。 | Desktop 在确认及最终分发时再次检查 1 MiB/NUL；修正原版预览编辑后的校验缺口。应保留为有意加强，非缺失。 | [internal/desktop/terminal_features.go](../internal/desktop/terminal_features.go)<br>[internal/terminal/terminal.go](../internal/terminal/terminal.go) |
| SF-I05 | 终端搜索 | 存在差异 | 每分屏搜索、正则/整词、循环、1000+ 统计与继续导航已有。 | Desktop 正则单行按 65536 字节限制，原版按格；单次正则 10ms/总扫描约 1s，与原版约 0.25s 检查预算不同。 | [internal/terminal/search.go](../internal/terminal/search.go)<br>[internal/desktop/search_navigation.go](../internal/desktop/search_navigation.go) |
| SF-I06 | 输出保留 | 存在差异 | 历史调整、日志启停/错误及结束保屏已有。 | 日志菜单明确为 ANSI 原始追加日志，observeOutput 同步写原始字节；缺少原版控制序列过滤和 4 MiB 待写队列超限停止机制。 | [internal/desktop/features.go](../internal/desktop/features.go)<br>[internal/terminal/oshell.go](../internal/terminal/oshell.go)<br>[internal/terminal/session_fallback.go](../internal/terminal/session_fallback.go) |
| SF-I07 | 链接 | 已实现 | OSC8/文本链接、悬停点击与方案白名单接入。 | 显式 URI 分号截断专门保留原版行为；非 ASCII/软换行及命中按固定样例验收。 | [internal/terminal/links.go](../internal/terminal/links.go)<br>[internal/terminal/link_discovery.go](../internal/terminal/link_discovery.go)<br>[internal/terminal/link_hover.go](../internal/terminal/link_hover.go)<br>[internal/terminal/internal/vt/hyperlink.go](../internal/terminal/internal/vt/hyperlink.go) |
| SF-I08 | 本机工具入口 | 存在差异 | 本机提示符、help/tools、独立工具 PTY、尺寸/退出/中断路径已有。 | 命令分派仅识别 help，未接入原版 ? 帮助别名；? 会进入工具白名单拒绝。 | [internal/terminal/tools_console.go](../internal/terminal/tools_console.go)<br>[internal/core/local_tools.go](../internal/core/local_tools.go) |
| SF-I09 | 工具分类 | 已实现 | 固定八类工具名称均在白名单，路径查询与交互工具单目标限制已有。 | tracert/pathping/ss/ip/ncat 是 Desktop 既有平台补充，不转为新增复刻要求；展示为平面名单。 | [internal/core/local_tools.go](../internal/core/local_tools.go)<br>[internal/terminal/tools_console.go](../internal/terminal/tools_console.go) |
| SF-I10 | 提示符编辑 | 存在差异 | 提示符编辑、kill/yank、补全、引号解析与直接 argv 执行。 | 编辑缓冲上限为 64 KiB，执行解析才限制 8192 字节；独立参数中的管道等符号被拒，原版按普通 argv 字面传入。 | [internal/terminal/prompt_editor.go](../internal/terminal/prompt_editor.go)<br>[internal/core/local_tools.go](../internal/core/local_tools.go) |
| SF-I11 | 提示符历史 | 已实现 | 100 条内存历史、相邻去重、上下草稿及反向/正向搜索。 | 工具运行输入不进入提示符历史；启动参数不脱敏与原版边界一致。 | [internal/terminal/prompt_editor.go](../internal/terminal/prompt_editor.go)<br>[internal/terminal/tools_console.go](../internal/terminal/tools_console.go) |
| SF-I12 | 结束页 | 已实现 | 结束保屏、切本机提示符、工具退出排空与当前分屏关闭。 | 不自动重连；原版特殊终端状态恢复需有限字节序列对照。 | [internal/terminal/session_fallback.go](../internal/terminal/session_fallback.go)<br>[internal/terminal/tool_process.go](../internal/terminal/tool_process.go)<br>[internal/desktop/app.go](../internal/desktop/app.go) |
| SF-I13 | 传输中的交互 | 已实现 | 传输占用输入通道，阻止常规/托管发送，保留本地选择与滚动。 | 取消编码与增强键盘组合仍需固定用例，不扩传输协议。 | [internal/terminal/transfer.go](../internal/terminal/transfer.go)<br>[internal/terminal/view.go](../internal/terminal/view.go)<br>[internal/terminal/keys.go](../internal/terminal/keys.go) |
| SF-I14 | 字符与解码 | 存在差异 | 定制 Ghostty 解析与 OShell 兼容分支、分块解码和字符集处理已有。 | 定制兼容路径已接通，但 OSC 使用 8 MiB 总长预算，原版没有该固定总长限额；C1/非法 UTF-8/控制串异常尚待固定逐字节对照，不能统称一致。 | [third_party/ghostty/oshell.patch](../third_party/ghostty/oshell.patch)<br>[internal/terminal/internal/vt/terminal.go](../internal/terminal/internal/vt/terminal.go)<br>[internal/terminal/internal/vt/osc_extended_test.go](../internal/terminal/internal/vt/osc_extended_test.go) |
| SF-I15 | 屏幕与历史 | 已实现 | 双屏/历史/边界/重排及光标恢复的兼容分支已接入。 | 原版已记录的裁剪/重排缺口不能自动认定 Desktop 复现或修正；按固定场景验收。 | [third_party/ghostty/oshell.patch](../third_party/ghostty/oshell.patch)<br>[internal/terminal/internal/vt/terminal.go](../internal/terminal/internal/vt/terminal.go)<br>[internal/terminal/scrollbar.go](../internal/terminal/scrollbar.go) |
| SF-I16 | 编辑与擦除 | 已实现 | 光标编辑、擦除、矩形与单元格元数据搬移的定制路径已有。 | 重叠/宽字/边界安全结果仍待逐字节用例；不要求复制原版越界损坏。 | [third_party/ghostty/oshell.patch](../third_party/ghostty/oshell.patch)<br>[internal/terminal/internal/vt/terminal.go](../internal/terminal/internal/vt/terminal.go) |
| SF-I17 | 颜色与属性 | 存在差异 | SGR、调色板、查询、下划线/删除线/闪烁及光标绘制已有。 | Desktop paint.resolve 消费 Invisible 属性；与原版已定位的隐藏属性显示交接缺口不同。其余属性不能因此统称完全等价。 | [internal/terminal/paint.go](../internal/terminal/paint.go)<br>[internal/terminal/palette.go](../internal/terminal/palette.go)<br>[third_party/ghostty/oshell.patch](../third_party/ghostty/oshell.patch) |
| SF-I18 | 键盘协议 | 已实现 | 传统/Kitty 编码、按下重复释放、文本与 IME 分流、平台备用键。 | Windows 物理键位/AltGr 与 Mac 映射不能以合成测试等价；固定编码用例及原版已知交接缺口另验。 | [internal/terminal/keys.go](../internal/terminal/keys.go)<br>[internal/terminal/function_keys.go](../internal/terminal/function_keys.go)<br>[third_party/ghostty/oshell.patch](../third_party/ghostty/oshell.patch) |
| SF-I19 | 鼠标与焦点 | 已实现 | 鼠标模式、坐标编码、Shift 本地选择、焦点及窗口输入路由接通。 | 显示/隐藏/迁移、捕获释放及多按钮必须真鼠标验收。 | [internal/terminal/view.go](../internal/terminal/view.go)<br>[internal/terminal/internal/vt/focus.go](../internal/terminal/internal/vt/focus.go) |
| SF-I20 | 查询与状态 | 存在差异 | DA/DSR/状态查询、标题栈等底层回复已接入。 | 生产 Terminal.New 未传 Progress 回调，未见 OSC9;4 终端进度条及 15s 超时；Desktop 未设置 OnBell，默认响铃宿主路径缺失。未读 BEL 标记不等于响铃。 | [internal/terminal/terminal.go](../internal/terminal/terminal.go)<br>[internal/terminal/internal/vt/terminal.go](../internal/terminal/internal/vt/terminal.go)<br>[internal/desktop/app.go](../internal/desktop/app.go) |
| SF-I21 | 刷新与重置 | 已实现 | 合并刷新、同步输出与 DECSTR/RIS 定制状态处理存在。 | 保留/清理表、同步期间实际画面和恢复时序仍待固定渲染用例，不宣称完全重置。 | [internal/terminal/terminal.go](../internal/terminal/terminal.go)<br>[internal/terminal/paint.go](../internal/terminal/paint.go)<br>[third_party/ghostty/oshell.patch](../third_party/ghostty/oshell.patch) |
| SF-I22 | 终端图片 | 存在差异 | Kitty 直接/分片、位置与资源管理，OSC1337、Sixel 解码/绘制已接入。 | POSIX 文件/临时文件/共享内存介质仅 Linux ABI45+ 开启；Windows 保持禁用。这是明确子能力缺口，不能用直接图片可显示替代。 | [internal/terminal/internal/vt/terminal.go](../internal/terminal/internal/vt/terminal.go)<br>[internal/terminal/internal/vt/graphics.go](../internal/terminal/internal/vt/graphics.go)<br>[internal/terminal/internal/vt/kitty_local.go](../internal/terminal/internal/vt/kitty_local.go)<br>[internal/terminal/graphics.go](../internal/terminal/graphics.go) |
| SF-I23 | Unicode 图形占位 | 已实现 | Unicode 占位符、颜色身份/附加字符继承及虚拟位置逐格绘制。 | 源码有实际 paint 调用；删除/替换、缩放与两渲染路径待固定画面对照。 | [internal/terminal/kitty_placeholder.go](../internal/terminal/kitty_placeholder.go)<br>[internal/terminal/paint.go](../internal/terminal/paint.go)<br>[internal/terminal/internal/vt/graphics.go](../internal/terminal/internal/vt/graphics.go) |
| SF-I24 | 双向文字 | 存在差异 | 六模式、Unicode17 数据、段落/字形及选择/输入映射已启用。 | 底层用 Go UAX#9/字形布局替代 CoreText 探针；复杂混排等价待实测。旧 README 写默认关闭已落后于 NewTerminal 的 ABI32+ 启用路径。 | [internal/terminal/internal/vt/bidi.go](../internal/terminal/internal/vt/bidi.go)<br>[internal/terminal/internal/vt/terminal.go](../internal/terminal/internal/vt/terminal.go)<br>[internal/terminal/internal/bidilayout/layout.go](../internal/terminal/internal/bidilayout/layout.go)<br>[internal/terminal/bidi.go](../internal/terminal/bidi.go) |
| SF-I25 | 特殊行显示 | 已实现 | 双宽/双高行元数据、行变换/裁剪与光标/命中交接已有。 | 逻辑列不缩减；原版行回收/重排缺口及 GPU/软件画面对照尚待验收。 | [internal/terminal/internal/vt/line_mode.go](../internal/terminal/internal/vt/line_mode.go)<br>[internal/terminal/line_mode.go](../internal/terminal/line_mode.go)<br>[internal/terminal/paint.go](../internal/terminal/paint.go)<br>[third_party/ghostty/oshell.patch](../third_party/ghostty/oshell.patch) |
| SF-I26 | 提示符语义 | 已实现 | OSC133 状态、单元格/行身份、延迟点击、坐标/方向键发送已接入。 | 不含跨重启命令库；原版已记录的管理发送/延迟内容变化边界需按固定样例对照。 | [internal/terminal/semantic_click.go](../internal/terminal/semantic_click.go)<br>[internal/terminal/internal/vt/semantic_input.go](../internal/terminal/internal/vt/semantic_input.go)<br>[internal/terminal/internal/vt/semantic_cell.go](../internal/terminal/internal/vt/semantic_cell.go)<br>[third_party/ghostty/oshell.patch](../third_party/ghostty/oshell.patch) |
| SF-I27 | 宿主限制 | 存在差异 | 链接方案限制、OSC777 主机扫描和非内联图像不落盘路径已有。 | OSC52 写回调仍将文本加入 clipOut，view 刷新写系统剪贴板；未见 OShell 应用层禁用闸门，违反固定原版 OSC52 禁用边界。 | [internal/terminal/terminal.go](../internal/terminal/terminal.go)<br>[internal/terminal/view.go](../internal/terminal/view.go)<br>[internal/terminal/internal/vt/terminal.go](../internal/terminal/internal/vt/terminal.go) |

### 图片粘贴（8 项）

| 固定编号 | 细分功能 | 状态 | Desktop 已有实现 | 差异/缺失与验收边界 | Desktop 源码证据 |
| --- | --- | --- | --- | --- | --- |
| SF-M01 | 识别与转换 | 存在差异 | 八种文件扩展名、单文件/混合拒绝、方向与色彩转换已有。 | 应用剪贴板入口只读 transfer.PNG；框架可转换部分平台 TIFF/DIB，但未见原版完整 PNG→TIFF→JPEG 回退链，原生仅 JPEG 表示需补验/补接。 | [internal/desktop/image_paste.go](../internal/desktop/image_paste.go)<br>[internal/core/clipboard_image.go](../internal/core/clipboard_image.go)<br>[third_party/mygo/internal/darwin/clipboard.go](../third_party/mygo/internal/darwin/clipboard.go)<br>[third_party/mygo/internal/windows/clipboard.go](../third_party/mygo/internal/windows/clipboard.go) |
| SF-M02 | 用户确认 | 已实现 | 确认窗显示尺寸/大小、远端目录及记忆选项，取消不插入。 | 无缩略图不是缺失，基线要求为文字确认。 | [internal/desktop/image_paste.go](../internal/desktop/image_paste.go) |
| SF-M03 | 本地目标 | 已实现 | 本机私有 PNG 缓存、普通文件/目录检查及目标 shell 转义。 | Windows 用本机权限实现；拒空白提示符和工具内 ssh/telnet。 | [internal/core/clipboard_image.go](../internal/core/clipboard_image.go)<br>[internal/core/clipboard_image_windows.go](../internal/core/clipboard_image_windows.go)<br>[internal/terminal/paste_context.go](../internal/terminal/paste_context.go) |
| SF-M04 | SSH 目标 | 已实现 | 复用已认证连接的 SFTP、随机私有目录、排他文件、取消与进度。 | 子系统失败无新认证回退；真实远端权限/取消需验收。 | [internal/core/clipboard_image.go](../internal/core/clipboard_image.go)<br>[internal/desktop/image_paste.go](../internal/desktop/image_paste.go)<br>[internal/core/transport.go](../internal/core/transport.go) |
| SF-M05 | 发送规则 | 存在差异 | 捕获 pane/term/conn、宿主代次和工具上下文，仅原目标插入路径。 | 最终有效性额外检查 passwordLocked，缓存锁后拒绝插入；差异主归 P08，不新增功能或重复算缺陷。 | [internal/desktop/image_paste.go](../internal/desktop/image_paste.go)<br>[internal/terminal/paste_context.go](../internal/terminal/paste_context.go) |
| SF-M06 | 提醒记忆 | 已实现 | 本地总开关及 SSH UUID/路由指纹记忆，成功插入后保存。 | 本机设置严格读取并有并发保护；不共享这类运行偏好。 | [internal/desktop/image_paste.go](../internal/desktop/image_paste.go)<br>[internal/desktop/sidebar.go](../internal/desktop/sidebar.go) |
| SF-M07 | 资源边界 | 已实现 | 20 MiB 文件/输出、160 MiB 输入、边长/像素限制和缓存 200 MiB/7 天。 | 只清自身前缀普通文件；不把所有临时文件列作可删除。 | [internal/core/clipboard_image.go](../internal/core/clipboard_image.go) |
| SF-M08 | 生命周期 | 已实现 | 失败/取消尽力清理，成功远端图片保留，生命周期回调核对原状态。 | 远端上传直接由内存 PNG 发送，无额外本地上传副本；不承诺零残留。 | [internal/core/clipboard_image.go](../internal/core/clipboard_image.go)<br>[internal/desktop/image_paste.go](../internal/desktop/image_paste.go) |

### 文件管理与传输（15 项）

| 固定编号 | 细分功能 | 状态 | Desktop 已有实现 | 差异/缺失与验收边界 | Desktop 源码证据 |
| --- | --- | --- | --- | --- | --- |
| SF-F01 | 文件窗口 | 已实现 | 独立文件窗、多标签、复制/排序/重连、最后页及忙碌关闭处理。 | 文件标签未扩成跨窗拖动；Esc/关闭与 IME 门禁待平台验收。 | [internal/desktop/file_window.go](../internal/desktop/file_window.go)<br>[internal/desktop/files.go](../internal/desktop/files.go)<br>[internal/desktop/workspace_close.go](../internal/desktop/workspace_close.go) |
| SF-F02 | 连接入口 | 已实现 | 保存资料独立连接、活动 SSH 复用、目录和拖放上传入口。 | 复用失败不新认证；文件入口保存行为差异关联 P05，一次性凭据另验。 | [internal/desktop/file_window.go](../internal/desktop/file_window.go)<br>[internal/desktop/app.go](../internal/desktop/app.go)<br>[internal/desktop/files.go](../internal/desktop/files.go) |
| SF-F03 | 目录浏览 | 已实现 | 路径/上级/刷新、目录优先、多选、当前目录过滤和未知类型探测。 | 异步失败保留既有列表；筛选后的选择恢复不追加为新需求。 | [internal/desktop/files.go](../internal/desktop/files.go)<br>[internal/desktop/file_filter.go](../internal/desktop/file_filter.go)<br>[internal/desktop/file_selection.go](../internal/desktop/file_selection.go) |
| SF-F04 | 文件操作 | 存在差异 | 建目录、改名、确认删除、递归上传下载及链接检查。 | SFTP/FTP 递归上限 64、额外限制 10000 项，原版深度 32；SFTP 顶层链接被 Lstat 拒绝，原版 STAT 跟随；递归子链接返回错误停止，原版跳过。 | [internal/core/transfer.go](../internal/core/transfer.go)<br>[internal/core/ftp.go](../internal/core/ftp.go)<br>[internal/desktop/file_actions.go](../internal/desktop/file_actions.go) |
| SF-F05 | 属性展示 | 已实现 | 七列属性、文件图标、权限/用户组和可选元数据表示。 | 服务端缺失属性/零值降级按固定样例验收，不泛称所有未知属性都能判断。 | [internal/desktop/files.go](../internal/desktop/files.go)<br>[internal/desktop/file_icons.go](../internal/desktop/file_icons.go)<br>[internal/core/file_attributes.go](../internal/core/file_attributes.go) |
| SF-F06 | FTP 列表 | 存在差异 | FTP ListWithFallback→NameList、UTF-8 和安全名称过滤接入。 | 使用 Go FTP 解析器替代 curl 输出解析；该库解析 LIST 修改时间，与原版 LIST 不转修改时间不同。降级/4 MiB 限制需库路径联验。 | [internal/core/ftp.go](../internal/core/ftp.go)<br>[third_party/ftp/ftp.go](../third_party/ftp/ftp.go)<br>[third_party/ftp/parse.go](../third_party/ftp/parse.go) |
| SF-F07 | 传输方式 | 存在差异 | SFTP、FTP、SCP 与不依赖 SFTP 的独立 SCP 入口已有。 | FTP 用 Go 客户端，SCP 经 SSH channel 自行编解码并调用远端 scp -rt；不走本地系统 scp 能力检测，且有字节进度。属于既有机制/行为差异。 | [internal/core/filesystem.go](../internal/core/filesystem.go)<br>[internal/core/ftp.go](../internal/core/ftp.go)<br>[internal/core/scp.go](../internal/core/scp.go)<br>[internal/desktop/standalone_scp.go](../internal/desktop/standalone_scp.go) |
| SF-F08 | 目标冲突 | 已实现 | 目标预列避名、排他创建、暂存及实际目标路径记录。 | FTP/独立 SCP 上传仍不能承诺并发不覆盖；Windows 名称规则更严格属于平台保护。 | [internal/desktop/transfer_names.go](../internal/desktop/transfer_names.go)<br>[internal/core/transfer.go](../internal/core/transfer.go)<br>[internal/core/scp.go](../internal/core/scp.go)<br>[internal/core/transfer_resume.go](../internal/core/transfer_resume.go) |
| SF-F09 | 限速 | 已实现 | 标签独立限速、预设/小数单位及范围校验，运行期调整/取消唤醒。 | SCP 使用开始时快照；不是网络瞬时带宽硬上限。 | [internal/desktop/file_rate.go](../internal/desktop/file_rate.go)<br>[internal/core/transfer_limit.go](../internal/core/transfer_limit.go) |
| SF-F10 | 进度与取消 | 已实现 | 串行批次、进度/速度、失败停止后续、取消及后端断开重连。 | SCP 字节进度与原版差异主归 F07；目录不冒称整体精确百分比。 | [internal/desktop/files.go](../internal/desktop/files.go)<br>[internal/desktop/transfer_records.go](../internal/desktop/transfer_records.go)<br>[internal/desktop/file_transfer_overlay.go](../internal/desktop/file_transfer_overlay.go) |
| SF-F11 | 传输记录 | 已实现 | 内存倒序记录、详情/片段、500 条已结束裁剪及清理。 | 基本身份变更清记录；暂停保留，不追加跨进程审计库。 | [internal/desktop/transfer_records.go](../internal/desktop/transfer_records.go) |
| SF-F12 | SFTP 续传 | 已实现 | SFTP 检查点、源/片段身份检查、前缀逐字节比对和排他发布。 | Windows 身份/硬链接验证采用平台实现；不承诺远端 inode 或全文件哈希。 | [internal/core/transfer_resume.go](../internal/core/transfer_resume.go)<br>[internal/core/sftp_open.go](../internal/core/sftp_open.go) |
| SF-F13 | 续传范围 | 已实现 | 仅 SFTP 顶层普通文件可暂停/继续，记录内持有检查点。 | 不支持目录/FTP/SCP/跨重启续传；关闭或清记录丢入口。 | [internal/desktop/transfer_records.go](../internal/desktop/transfer_records.go)<br>[internal/core/transfer_resume.go](../internal/core/transfer_resume.go) |
| SF-F14 | ZMODEM | 已实现 | CRC 有效握手检测、随包工具、选择/进度/取消和通道恢复路径。 | 真实对端成功/取消超时待验收，100% 不代替最终成功；不增加其他协议。 | [internal/terminal/transfer_detector.go](../internal/terminal/transfer_detector.go)<br>[internal/terminal/transfer.go](../internal/terminal/transfer.go)<br>[internal/desktop/terminal_transfer.go](../internal/desktop/terminal_transfer.go)<br>[internal/desktop/terminal_transfer_overlay.go](../internal/desktop/terminal_transfer_overlay.go) |
| SF-F15 | 资源归属 | 已实现 | 终端/文件/SCP 持有连接 lease，释放幂等，迁移重绑任务归属。 | 进程内连接池组织替代 control socket；无跨进程复用或失效自动认证。 | [internal/core/transport.go](../internal/core/transport.go)<br>[internal/desktop/file_window.go](../internal/desktop/file_window.go)<br>[internal/desktop/application.go](../internal/desktop/application.go) |

### 快捷操作（9 项）

| 固定编号 | 细分功能 | 状态 | Desktop 已有实现 | 差异/缺失与验收边界 | Desktop 源码证据 |
| --- | --- | --- | --- | --- | --- |
| SF-O01 | 快速命令管理 | 已实现 | 快速命令增改删、分组、追加回车及填入快速栏/撰写窗。 | 选择不立即执行；1 MiB/NUL/空值校验已在提交路径。 | [internal/desktop/quick_command_manager.go](../internal/desktop/quick_command_manager.go)<br>[internal/desktop/operators.go](../internal/desktop/operators.go)<br>[internal/core/operators.go](../internal/core/operators.go) |
| SF-O02 | 快速发送栏 | 存在差异 | 单行发送、保存命令/历史菜单、范围及多行转撰写窗已有。 | 历史只限 30 条，未见 50 条/64 KiB 合计/8 KiB 单条规则；send 成功未重置 noReturn；输入未见上下历史草稿恢复处理。 | [internal/desktop/operators.go](../internal/desktop/operators.go)<br>[internal/desktop/workspace.go](../internal/desktop/workspace.go) |
| SF-O03 | 撰写窗 | 已实现 | 多行编辑、独立目标快照、追加回车及无额外确认直接发送。 | 全部捕获目标可输入才分发；目标迁移/关闭拒绝，换行规范化走 ManagedInput。 | [internal/desktop/operators.go](../internal/desktop/operators.go)<br>[internal/core/operators.go](../internal/core/operators.go)<br>[internal/terminal/oshell.go](../internal/terminal/oshell.go) |
| SF-O04 | 七种发送范围 | 已实现 | 七范围选择，组含隐藏成员，模式持久化/选择身份运行期。 | 可见范围来自 visiblePanes；窗口裁剪/最小化需真实布局验收。 | [internal/desktop/operators.go](../internal/desktop/operators.go)<br>[internal/desktop/groups.go](../internal/desktop/groups.go) |
| SF-O05 | 范围动态更新 | 已实现 | 发送前重算可输入对象、接收/跳过计数及关闭/迁移选择清理。 | 快速栏跳过不可输入对象，与撰写窗整组拒绝规则区分。 | [internal/desktop/operators.go](../internal/desktop/operators.go)<br>[internal/desktop/input_target_detach.go](../internal/desktop/input_target_detach.go) |
| SF-O06 | 同步输入 | 存在差异 | 至少两目标同步、失效停止、按目标模式粘贴且不广播图片/协议回复。 | 预览编辑后再次验证大小/NUL，修正原版缺口；同一差异关联 I04，不重复形成修复工作量。 | [internal/desktop/sync_input_draft.go](../internal/desktop/sync_input_draft.go)<br>[internal/desktop/terminal_features.go](../internal/desktop/terminal_features.go) |
| SF-O07 | 高亮规则 | 存在差异 | 集合/规则编辑、主机替换、正则/颜色及渲染预算已有。 | 重叠匹配后规则覆盖前规则，原版首规则优先；4096 按 rune 而非 UTF-16，超过 32 条直接拒绝；还渲染背景/粗体和 8 位色，超出原版实际显示能力但不扩基线。 | [internal/desktop/highlights.go](../internal/desktop/highlights.go)<br>[internal/desktop/terminal_features.go](../internal/desktop/terminal_features.go)<br>[internal/terminal/oshell.go](../internal/terminal/oshell.go)<br>[internal/terminal/paint.go](../internal/terminal/paint.go) |
| SF-O08 | 快捷键配置 | 存在差异 | 动作绑定录入/手选、清除、恢复、内部冲突和菜单刷新。 | 已读生产校验只核应用内部冲突，未找到原版已知系统热键读取/读取失败提示路径；Mac Command 在 Windows/Linux 映射 Ctrl，属于平台映射。 | [internal/desktop/shortcuts.go](../internal/desktop/shortcuts.go)<br>[internal/core/shortcuts.go](../internal/core/shortcuts.go)<br>[internal/desktop/shortcut_capture_other.go](../internal/desktop/shortcut_capture_other.go) |
| SF-O09 | 焦点与弹窗 | 已实现 | 工作区快捷键、录入/模态/密码/搜索/辅助窗等输入门禁。 | 真正键盘/IME/原生菜单跟踪仍待平台验收；不因消费函数存在就认定一次事件只发一次。 | [internal/desktop/shortcuts.go](../internal/desktop/shortcuts.go)<br>[internal/desktop/password_input.go](../internal/desktop/password_input.go)<br>[internal/desktop/file_window.go](../internal/desktop/file_window.go)<br>[internal/terminal/view.go](../internal/terminal/view.go) |

### 外观与资源（10 项）

| 固定编号 | 细分功能 | 状态 | Desktop 已有实现 | 差异/缺失与验收边界 | Desktop 源码证据 |
| --- | --- | --- | --- | --- | --- |
| SF-A01 | 界面主题 | 已实现 | 系统/浅/深主题，设置草稿预览及应用到正式界面。 | 非 macOS 原生标题外观预览函数为空，框架内容主题独立处理；系统主题切换需目标平台验收。 | [internal/desktop/settings.go](../internal/desktop/settings.go)<br>[internal/desktop/appearance_other.go](../internal/desktop/appearance_other.go)<br>[internal/desktop/workspace.go](../internal/desktop/workspace.go) |
| SF-A02 | 终端配色 | 存在差异 | 九预设、自定义/复制/改名/删除、JSON/XCS 导入导出。 | Desktop 实际启用 AutoCursor 自动对比色；原版该字段没有接到显示。已有额外行为不变成新复刻要求。 | [internal/core/colors.go](../internal/core/colors.go)<br>[internal/desktop/settings.go](../internal/desktop/settings.go)<br>[internal/desktop/fonts.go](../internal/desktop/fonts.go)<br>[internal/terminal/paint.go](../internal/terminal/paint.go) |
| SF-A03 | 字体 | 已实现 | 本机字体选择、内置四样式进程注册、预览与缺失回退。 | 本轮未执行物理字体/多语言显示验收，不能以注册成功保证字形覆盖。 | [internal/desktop/font_picker.go](../internal/desktop/font_picker.go)<br>[internal/desktop/fonts.go](../internal/desktop/fonts.go)<br>[internal/core/font_catalog.go](../internal/core/font_catalog.go)<br>[internal/terminal/font.go](../internal/terminal/font.go) |
| SF-A04 | 图标与 Emoji | 已实现 | Nerd Font 回退、彩色 Emoji 偏好、VS15 与 Powerline 几何绘制。 | 跨系统字形/颜色自然差异允许；单元格接缝及复制文本仍需画面对照。 | [internal/desktop/fonts.go](../internal/desktop/fonts.go)<br>[internal/terminal/font.go](../internal/terminal/font.go)<br>[internal/terminal/paint.go](../internal/terminal/paint.go)<br>[internal/terminal/box.go](../internal/terminal/box.go) |
| SF-A05 | 字符宽度 | 已实现 | 固定宽度表、创建期歧义双宽开关与宽字占位复制。 | 当前代码调用 ABI 宽度接口；旧 terminal README 的 DLL 不支持描述不能作当前结论，打包原生库需另验。 | [third_party/ghostty/oshell_width.zig](../third_party/ghostty/oshell_width.zig)<br>[internal/terminal/internal/vt/terminal.go](../internal/terminal/internal/vt/terminal.go)<br>[internal/terminal/terminal.go](../internal/terminal/terminal.go) |
| SF-A06 | 渲染 | 存在差异 | 窗口 GPU/软件选择、失败降级和重建接入 mygo renderer。 | Windows D3D11/Linux GL 替代 Metal/CG；失败后按 1–30 秒退避重试，稳定 1 分钟重置，并非原版 30 秒内再次失败就固定回退策略。 | [internal/desktop/workspace.go](../internal/desktop/workspace.go)<br>[third_party/mygo/ui/renderer_preference.go](../third_party/mygo/ui/renderer_preference.go)<br>[third_party/mygo/ui/window.go](../third_party/mygo/ui/window.go) |
| SF-A07 | 历史容量 | 已实现 | 3000 默认、六档/500–100000 自定义、>20000 提示及运行期容量调整。 | 裁剪、搜索/选择和图像释放以固定用例验收，不承诺字节上限。 | [internal/desktop/history_settings.go](../internal/desktop/history_settings.go)<br>[internal/terminal/oshell.go](../internal/terminal/oshell.go) |
| SF-A08 | 本机时钟 | 已实现 | 九位置、字体/颜色/透明度及本机每秒时钟，共享计时器。 | 按可见窗口/终端订阅，隐藏无输入焦点；不添加远端时间服务。 | [internal/desktop/clock.go](../internal/desktop/clock.go)<br>[internal/desktop/clock_settings.go](../internal/desktop/clock_settings.go)<br>[internal/core/clock_style.go](../internal/core/clock_style.go) |
| SF-A09 | 未读状态 | 已实现 | 跨块输出活动解析、BEL/正文未读及观察标签清除。 | 非 ASCII 空白按字节可能触发，与基线口径一致；不含系统通知。 | [internal/core/output_activity.go](../internal/core/output_activity.go)<br>[internal/desktop/group_unread.go](../internal/desktop/group_unread.go)<br>[internal/desktop/app.go](../internal/desktop/app.go) |
| SF-A10 | 生命周期与性能 | 存在差异 | 隐藏继续解析/减少刷新、关闭释放、结束保屏及延迟内存回收。 | Desktop 空闲回收要求所有工作窗均无标签；原版按当前窗口关闭最后标签后 2 秒检查。回收使用 Go FreeOSMemory，不保证相同 RSS。 | [internal/terminal/oshell.go](../internal/terminal/oshell.go)<br>[internal/terminal/terminal.go](../internal/terminal/terminal.go)<br>[internal/desktop/idle_memory.go](../internal/desktop/idle_memory.go) |

### 外部互通与迁移（9 项）

| 固定编号 | 细分功能 | 状态 | Desktop 已有实现 | 差异/缺失与验收边界 | Desktop 源码证据 |
| --- | --- | --- | --- | --- | --- |
| SF-E01 | OShell 原生归档 | 已实现 | 原生 v1–v4 读取、v2/v3/v4 输出、范围导出/导入及严格格式校验。 | 归档不是偏好/私钥文件打包；真实原版往返待 macOS 验收。 | [internal/core/portable_archive.go](../internal/core/portable_archive.go)<br>[internal/desktop/archive.go](../internal/desktop/archive.go)<br>[internal/desktop/manager_context_menu.go](../internal/desktop/manager_context_menu.go) |
| SF-E02 | 归档重绑定 | 已实现 | 导入新 UUID、代理/引用映射、同名副本及密码重加密。 | 不含密码导出去凭据；错误密码/并发 revision 走原子提交拒绝。 | [internal/core/portable_archive.go](../internal/core/portable_archive.go)<br>[internal/desktop/archive.go](../internal/desktop/archive.go)<br>[internal/core/credential_edit.go](../internal/core/credential_edit.go) |
| SF-E03 | SecureCRT 导入 | 存在差异 | SecureCRT XML/INI/目录解析、预览及允许字段导入。 | 目录遍历遇目录直接返回，未记录物理空目录；与基线保留空目录的要求有缺口。容器/遍历限制关联 E06。 | [internal/core/third_party.go](../internal/core/third_party.go)<br>[internal/desktop/archive.go](../internal/desktop/archive.go) |
| SF-E04 | Xshell 导入 | 已实现 | XSH/目录/XTS/ZIP 与兼容 RC4+SHA256 密码迁移，预览后事务应用。 | 不声称所有 Xshell 密码格式可迁移；容器具体限制差异归 E06。 | [internal/core/third_party.go](../internal/core/third_party.go)<br>[internal/desktop/archive.go](../internal/desktop/archive.go) |
| SF-E05 | 补充密码 | 已实现 | 只补唯一匹配且无密码资料，匹配属性/目录范围，保留 ID。 | 多目标/重复来源及解密失败拒绝；不增加来源追踪字段。 | [internal/core/third_party.go](../internal/core/third_party.go)<br>[internal/desktop/archive.go](../internal/desktop/archive.go) |
| SF-E06 | 容器边界 | 存在差异 | 内存 ZIP 读取、路径/重复/加密/符号链接拒绝和总量限制。 | ZIP/遍历上限改成 10000；未见每项 2 MiB、显式 ZIP64 拒绝、中央/本地头一致性及目录 CRC 校验；Go zip 的接受范围不等同固定容器约束。 | [internal/core/third_party.go](../internal/core/third_party.go) |
| SF-E07 | 启动器兼容 | 已实现 | ZOC 参数、FileZilla URL/单站点解析与主程序交接。 | TITLE 校验但不赋值显示，与固定原版相同；无 FTPS 或整库导入承诺。 | [internal/core/external_launch.go](../internal/core/external_launch.go)<br>[internal/core/filezilla_launch.go](../internal/core/filezilla_launch.go)<br>[internal/core/launch_entrypoint.go](../internal/core/launch_entrypoint.go) |
| SF-E08 | 启动交接 | 已实现 | 8 槽/16 KiB/300s 等待、256 请求去重、解锁后临时页及身份校验。 | Windows 具用户 ACL/SID 校验的命名管道替代 Unix socket；不把临时凭据自动存库。 | [internal/core/launch_ipc.go](../internal/core/launch_ipc.go)<br>[internal/core/launch_ipc_windows.go](../internal/core/launch_ipc_windows.go)<br>[internal/core/launch_wire.go](../internal/core/launch_wire.go)<br>[internal/desktop/external_launch.go](../internal/desktop/external_launch.go) |
| SF-E09 | 其他客户端迁移 | 已实现 | 通用 OShell 归档入口可接收外部转换结果，内置来源仅 SecureCRT/Xshell。 | 属于范围边界项；PuTTY 等没有内置解析器不计缺失，不新建转换工具。 | [internal/core/third_party.go](../internal/core/third_party.go)<br>[internal/core/portable_archive.go](../internal/core/portable_archive.go) |

### 更新与分发（7 项）

| 固定编号 | 细分功能 | 状态 | Desktop 已有实现 | 差异/缺失与验收边界 | Desktop 源码证据 |
| --- | --- | --- | --- | --- | --- |
| SF-U01 | 更新源设置 | 存在差异 | 仓库设置、空值默认回退、自动检查开关和忙时防切换。 | Windows/Linux 还提供私有仓库 token；为既有 Desktop 额外能力，不纳入本次原版复刻范围。 | [internal/desktop/updater_other.go](../internal/desktop/updater_other.go)<br>[internal/core/update_source.go](../internal/core/update_source.go)<br>[internal/core/update_auth.go](../internal/core/update_auth.go) |
| SF-U02 | 更新检查 | 存在差异 | 版本/平台资产选择、大小/元数据校验与检查缓存。 | Windows/Linux 调用 GitHub releases/latest API；原版静态 Pages latest.json→Sparkle。不能以另一未被调用的解析器宣称生产路径一致。 | [internal/core/update_release.go](../internal/core/update_release.go)<br>[internal/core/update_cache.go](../internal/core/update_cache.go)<br>[internal/desktop/updater_other.go](../internal/desktop/updater_other.go) |
| SF-U03 | 原版更新链路 | 存在差异 | 确认下载、包大小/SHA256 检查和准备更新已有。 | Windows/Linux 生产 ReleaseUpdate 路径未使用内置 Ed25519 签名 feed/包验证；仓库虽有 signed manifest 代码，但未接此入口，等价签名验证缺失。 | [internal/core/update_release.go](../internal/core/update_release.go)<br>[internal/core/update_manifest.go](../internal/core/update_manifest.go)<br>[internal/desktop/updater_other.go](../internal/desktop/updater_other.go) |
| SF-U04 | 安装生命周期 | 存在差异 | 待安装保留、活动关闭、替换重启与失败回滚实现存在。 | Desktop 无活动连接仍弹安装确认；模态时提示先关闭后重试，不采用原版延后回调方式。回滚是既有附加机制，不以源码存在声明恢复实测通过。 | [internal/desktop/updater_other.go](../internal/desktop/updater_other.go)<br>[internal/core/update_portable.go](../internal/core/update_portable.go)<br>[internal/core/update_process_windows.go](../internal/core/update_process_windows.go) |
| SF-U05 | 平台分发 | 已实现 | Windows 单文件嵌资源/依赖及 EXE/ZIP 构建、Linux 包装入口存在。 | 平台产物替代原版 app/DMG；本轮不构建/替换 dist，不能认证当前 dist 与工作树一致。 | [scripts/build.py](../scripts/build.py)<br>[internal/terminal/library.go](../internal/terminal/library.go)<br>[README.md](../README.md) |
| SF-U06 | 配置保留 | 已实现 | 数据目录独立，更新准备/发布仅替换应用文件并保留配置。 | 本轮未执行真实升级/失败恢复，配置保留仍需包级验收。 | [internal/core/update_portable.go](../internal/core/update_portable.go)<br>[internal/core/update_process_windows.go](../internal/core/update_process_windows.go) |
| SF-U07 | 关于与许可 | 已实现 | 版本/构建/架构、项目链接、关于及第三方许可展示入口。 | 本轮未核实际分发 EXE 的嵌入版本和离线许可访问。 | [internal/desktop/about.go](../internal/desktop/about.go)<br>[README.md](../README.md) |

## 本轮验证与保留事项

- 结构校验：主表与固定基线 161 个 ID/名称逐一对应，每个只出现一次；13 域计数不变；源码链接检查存在。CSV 与主表来自同一份记录。
- Go 合同样本：尝试运行 `go test ./internal/core -run 'Test(NativeSwiftSharedFixture|SharedQuickSendScope|LegacySharedProtocol|SharedEncodingNormalization)' -count=1`；默认工具链和本机 `Go 1.27.2 windows/amd64`（`GOTOOLCHAIN=local`，加 `-v -timeout=3m`）两次均长时间没有返回测试输出，已主动中止，退出码 1。本轮结果为**未验证**，不能称通过或断言功能测试失败；停滞原因未确定。Go 安装版本已单独读取确认。
- 未执行真实 macOS Swift ↔ Go 互通、Linux 桌面、真键鼠/IME、两渲染路径画面对照、真实远端协议或更新安装。历史测试记录不冒充本轮执行。
- 文档核查没有程序修改，因此不触发本地便携包重建/替换；当前 dist 未作本轮验收。

后续按本表原 SF 逐项立有限用例并修复已确认缺口；实现存在、静态无差异、用例通过、真实平台验收是不同证据。仅出现新的具体证据时重开对应条目，不用继续猜测组合或追随新上游版本扩张收尾边界。

