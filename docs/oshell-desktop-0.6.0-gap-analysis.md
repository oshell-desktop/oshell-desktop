# Desktop 对照 OShell 0.6.0 的 176 项实现差异清单

核查日期：2026-10-29。原版基准为官方 `v0.6.0` / `18770fee58770c54aa33db574b646e49ae18e208`；逐项身份来自 [0.6.0 功能清单](oshell-0.6.0-feature-inventory.md)。Desktop 基于当前本地工作树，HEAD 为 `61c8e2ae60dd0e44841fc4bfe8d03cf5dc722211`，**包含此前已有的未提交改动**，并非仅检查 HEAD。输入文件摘要见 [核查证据](../validation/oshell-0.6.0-desktop-audit/audit-evidence.json)。

本次按用户要求只形成调查清单。**没有修改产品代码、旧 161 项范围/验收状态、便携包或发布配置，没有提交、推送或触发流水线。** `V060-*` 仍为调查编号；`SF-*` 保持旧身份。新版本能力没有自动进入 `OShell-0.2.92-scope-1`，也没有据此排列必须修复的优先级。

## 状态口径

- **已实现**：当前有相应生产入口与实现，结合既有逐项记录未定位本行新增功能缺口；不表示每个环境或 0.6.0 运行对照已通过。
- **存在差异**：条目部分已实现，但有子能力缺失、可观察行为/界面差别、既有平台替换或增强。性质单独列出，不能把所有差异都当缺陷。
- **缺失**：该调查条目没有对应产品实现。包括新增 MCP 15 项，以及旧基线已明确排除的 Touch ID 1 项；后者不是待修复承诺。
- **验证证据独立**：CSV 保留旧 161 项的状态、证据、测试入口和剩余步骤，并注明本轮补充及历史记录文件是否仍存在；它们是历史证据。本轮为源码调查，未新执行单元/集成/GUI/Swift 双端用例。真实窗口、Linux/macOS、Swift 互通等原待验项不自动转为通过。

新增/变化 49 项逐项查看产品入口、相关生产实现及缺失搜索；其余 127 项按现有生产入口与最新修复/验收记录复核，不声称全源码重新逐行审计。原版“沿用”条目也继承上轮 0.2.92 分析；旧终端细节缺陷在 0.6.0 是否仍存在，不能仅凭旧结论断言。

## 数量与重点

总计 **176 项：已实现 107，存在差异 53，缺失 16**。这是调查行数，不是独立缺陷数、开发任务数或完成率。

| 核查组 | 已实现 | 存在差异 | 缺失 | 合计 |
| --- | ---: | ---: | ---: | ---: |
| 原34项变化 | 1 | 33 | 0 | 34 |
| 新增15项MCP | 0 | 0 | 15 | 15 |
| 其余127项 | 106 | 20 | 1 | 127 |

重点变化的归属如下；关联行不重复建立任务：

| 发现 | 对应调查条目 | 解释 |
| --- | --- | --- |
| 标签隐藏滚动条及 Shift 横滚已有 | W03 | 新版变化已被既有用户授权实现覆盖；物理设备仍待验 |
| 全局代理/本地目录入口尚未覆盖 | C04、C08，关联 C01/I18 | 会话代理、local 动作已存在；不能据此算全局开关、Finder 服务或新默认快捷键已实现 |
| 两类排列的操作对象不同 | W05 | Desktop 分屏菜单重排当前标签 pane；原版该入口排列窗口标签；命名组策略也不同 |
| 缩放参数与时钟粒度不同 | I01、A08，关联 I19/A03 | Desktop 300 DIP/1 秒/10–26；时钟逐 pane；不把滚轮单位直接换算成体感比例 |
| 文件停靠/终端工具模式不完整 | F01、F02 | 生产入口为独立文件窗，旧无 hub 侧栏分支不是新版可切换停靠 |
| rsync 尚未实现 | F07，关联 C11、F08/F09/F10/F13 | 既有 SFTP/FTP/SCP 和租约已实现；相同能力在多行被引用不等于多项独立开发 |
| 用户字体库及附件通道尚未实现 | A03、Y06/Y09/Y10/Y11/Y17、E01，关联 A04/U05 | 原版字体附件默认关闭且明文独立；核心配置 v2 不因此失效；Desktop 保留内置字体 |
| MCP 15 项整体缺失 | G01–G15，关联 W01/I06/I18/I19/I26/I27/A01/A10/U05 | SSH Agent、快速发送、普通日志、崩溃诊断均不算 MCP 替代实现 |
| 旧授权差异继续保留 | W01、I02、A01/A02 等 | Windows 最后窗口退出、高对比选区、紧凑专业风等不能因参考版本升级自动撤销 |
| 旧问题/文案需要单独跟踪 | F14、O07 | 下载目录选择卡顿未确认根因；高亮 UI 仍称最多 32 条，实际限制的是渲染前 32 条 |
| 更新及包形态属于独立平台路线 | U02/U03/U05 | 不直接切到原版 macos15.json 或 DMG；不把通道命名差异自动判为缺陷 |

## 功能域汇总

| 功能域 | 已实现 | 存在差异 | 缺失 | 总计 |
| --- | ---: | ---: | ---: | ---: |
| 会话资料与管理 | 14 | 0 | 0 | 14 |
| 密码与本机保护 | 7 | 2 | 1 | 10 |
| 连接与代理 | 10 | 4 | 0 | 14 |
| 窗口与导航 | 8 | 4 | 0 | 12 |
| 主机信息与标题 | 9 | 0 | 0 | 9 |
| 配置与同步 | 12 | 5 | 0 | 17 |
| 终端 | 14 | 13 | 0 | 27 |
| 图片粘贴 | 8 | 0 | 0 | 8 |
| 文件管理与传输 | 5 | 10 | 0 | 15 |
| 快捷操作 | 7 | 2 | 0 | 9 |
| 外观与资源 | 3 | 7 | 0 | 10 |
| 外部互通与迁移 | 8 | 1 | 0 | 9 |
| 更新与分发 | 2 | 5 | 0 | 7 |
| MCP 控制与审计 | 0 | 0 | 15 | 15 |

## 逐项清单

以下每行包含状态、Desktop 当前实现、具体差异及生产源码入口。**“原34变化”/“新增MCP”标记只用于重点筛选**。完整上游范围、固定提交源码路径、历史测试与验收步骤见 [176 行 CSV](oshell-desktop-0.6.0-gap-analysis.csv)。

源码链接指向当前工作树；带行号的标签是本轮定位值。缺失项所列 Desktop 文件是已检查的相关入口/边界，不能理解为这些文件实现了缺失能力。负面证据来自 [生产源搜索与输入指纹](../validation/oshell-0.6.0-desktop-audit/audit-evidence.json)，并结合菜单/设置、协议分派、日志/归档和依赖入口判断，不只看名称。

### 会话资料与管理

| 编号 / 旧编号 | 功能与重点 | 实现状态 / 性质 | Desktop 当前实现 | 差异、缺失与边界 | 当前源码入口 |
| --- | --- | --- | --- | --- | --- |
| V060-S01 / SF-S01 | 新建与编辑 | 已实现；未定位新增功能差异 | SSH/SFTP/FTP 新建、编辑、保存/连接及本地入口已接入。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [connection_editor.go](../internal/desktop/connection_editor.go)；[app.go](../internal/desktop/app.go) |
| V060-S02 / SF-S02 | 协议相关属性 | 已实现；未定位新增功能差异 | 协议页按 SSH/SFTP/FTP 切换；SFTP 空闲控件整组禁用，FTP 保存清除 SSH 专属数据。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [connection_editor.go](../internal/desktop/connection_editor.go)；[editor_protocol.go](../internal/desktop/editor_protocol.go) |
| V060-S03 / SF-S03 | 会话默认属性 | 已实现；未定位新增功能差异 | 独立会话默认草稿，新建资料时应用；复制走源资料。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [defaults.go](../internal/core/defaults.go)；[defaults.go](../internal/desktop/defaults.go)；[editor_protocol.go](../internal/desktop/editor_protocol.go) |
| V060-S04 / SF-S04 | 目录树 | 已实现；未定位新增功能差异 | 普通/Links 目录树、递归移动/改名及父目录保留；侧栏目录双击展开/收起已补齐。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [session_moves.go](../internal/core/session_moves.go)；[manager_drag.go](../internal/desktop/manager_drag.go)；[manager_rename.go](../internal/desktop/manager_rename.go)；[sidebar.go:360](../internal/desktop/sidebar.go) |
| V060-S05 / SF-S05 | 搜索 | 已实现；未定位新增功能差异 | 多词字面搜索、范围/协议过滤、引用源元数据、超限错误及搜索展开状态隔离已有。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [session_search.go](../internal/core/session_search.go)；[manager.go](../internal/desktop/manager.go)；[sidebar.go](../internal/desktop/sidebar.go) |
| V060-S06 / SF-S06 | 列表交互 | 已实现；未定位新增功能差异 | 表格选择、批量操作、双击/回车、F2 行内改名及失焦取消已接入。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [manager.go](../internal/desktop/manager.go)；[manager_rename.go](../internal/desktop/manager_rename.go)；[manager_actions.go](../internal/desktop/manager_actions.go) |
| V060-S07 / SF-S07 | 配置复制 | 已实现；未定位新增功能差异 | 进程内快照与系统剪贴板代次绑定；复制生成新身份并重绑凭据。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [manager_clipboard.go](../internal/desktop/manager_clipboard.go)；[manager_actions.go](../internal/desktop/manager_actions.go) |
| V060-S08 / SF-S08 | 删除 | 已实现；未定位新增功能差异 | 删除规划、父子去重、关联引用清理、确认后提交。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [session_delete.go](../internal/core/session_delete.go)；[manager_actions.go](../internal/desktop/manager_actions.go) |
| V060-S09 / SF-S09 | 左侧会话栏 | 已实现；未定位新增功能差异 | 侧栏目录树、搜索、连接及管理操作接入共享资料。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [sidebar.go](../internal/desktop/sidebar.go)；[session_catalog.go](../internal/desktop/session_catalog.go)；[manager_actions.go](../internal/desktop/manager_actions.go) |
| V060-S10 / SF-S10 | 根目录置顶 | 已实现；未定位新增功能差异 | 根会话/目录置顶、本机持久化、改名与删除后整理。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [session_pins.go](../internal/desktop/session_pins.go)；[sidebar.go](../internal/desktop/sidebar.go) |
| V060-S11 / SF-S11 | 快捷连接 | 已实现；未定位新增功能差异 | 默认本机项按工厂值、引用和凭据条件在展示投影中过滤。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [session_catalog.go](../internal/desktop/session_catalog.go)；[quick_connection_menu.go](../internal/desktop/quick_connection_menu.go) |
| V060-S12 / SF-S12 | `/Links` | 已实现；未定位新增功能差异 | 创建引用、添加当前会话、目录移动转引用、源身份保持。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [link_edit.go](../internal/core/link_edit.go)；[session_moves.go](../internal/core/session_moves.go)；[link_editor.go](../internal/desktop/link_editor.go) |
| V060-S13 / SF-S13 | 引用完整性 | 已实现；未定位新增功能差异 | 引用名称/目录编辑、删除与目录保护、实际会话另行管理。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [link_edit.go](../internal/core/link_edit.go)；[link_menu.go](../internal/desktop/link_menu.go) |
| V060-S14 / SF-S14 | 链接栏 | 已实现；未定位新增功能差异 | 根栏/目录树、悬停展开、菜单项拖回根、配置代次及引用来源保护已有。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [workspace.go](../internal/desktop/workspace.go)；[link_menu.go](../internal/desktop/link_menu.go) |

### 密码与本机保护

| 编号 / 旧编号 | 功能与重点 | 实现状态 / 性质 | Desktop 当前实现 | 差异、缺失与边界 | 当前源码入口 |
| --- | --- | --- | --- | --- | --- |
| V060-P01 / SF-P01 | 主密码 | 已实现；未定位新增功能差异 | 主密码设置/修改/移除、配置与密码库联合事务、解锁及 revision 检查。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [master_transaction.go](../internal/core/master_transaction.go)；[vault.go](../internal/core/vault.go)；[features.go](../internal/desktop/features.go) |
| V060-P02 / SF-P02 | 本机加密保存 | 存在差异；平台替换 | credential-key 原始本机密钥及独立 vault 已实现。 | 原版带 UUID 的 JSON 密钥文件与 Desktop 本机格式不同，不能直接互换；共享协议及身份重绑继续独立约束。 | [local_credential_key.go](../internal/core/local_credential_key.go)；[vault.go](../internal/core/vault.go) |
| V060-P03 / SF-P03 | 身份绑定 | 已实现；未定位新增功能差异 | 密码封装、身份绑定、复制/导入后解密再绑定、错误拒绝。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [mac_cipher.go](../internal/core/mac_cipher.go)；[portable_archive.go](../internal/core/portable_archive.go)；[vault.go](../internal/core/vault.go) |
| V060-P04 / SF-P04 | 认证队列 | 已实现；未定位新增功能差异 | 应用级认证排队、问题/回显展示、取消、对象失效清理。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [authentication.go](../internal/desktop/authentication.go)；[transport.go](../internal/core/transport.go) |
| V060-P05 / SF-P05 | 成功后保存 | 存在差异；已有增强 | SSH/SFTP/FTP 成功后保存，外部禁存、缓存缺失跳过和代次校验已有。 | 保存覆盖 FTP/SFTP 成功路径，比原版 SSH 保存入口更广；不删除已完成增强，也不放宽一次性来源禁存。 | [authenticated_password.go:10](../internal/desktop/authenticated_password.go)；[app.go](../internal/desktop/app.go) |
| V060-P06 / SF-P06 | 输入与回显 | 已实现；未定位新增功能差异 | 密码输入遮罩/显示切换及本机保护选择已有入口。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [password_input.go](../internal/desktop/password_input.go)；[password_protection.go](../internal/desktop/password_protection.go) |
| V060-P07 / SF-P07 | 一次性凭据 | 已实现；未定位新增功能差异 | 只识别单个身份化固定密码挑战，一次性自动尝试及外部禁存已有。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [password_prompt.go](../internal/core/password_prompt.go)；[transport.go](../internal/core/transport.go) |
| V060-P08 / SF-P08 | 安全状态保护 | 已实现；未定位新增功能差异 | 启动工作区锁与缓存清除分离；缓存清除不阻断已有连接普通输入/图片，秘密操作仍走保护。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [configuration_unlock.go](../internal/desktop/configuration_unlock.go)；[application.go](../internal/desktop/application.go)；[security_view.go](../internal/desktop/security_view.go)；[image_paste.go](../internal/desktop/image_paste.go) |
| V060-P09 / SF-P09 | 风险提示 | 已实现；未定位新增功能差异 | 未设主密码提醒、确认记忆、专注模式隐藏。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [master_warning.go](../internal/desktop/master_warning.go)；[session_sync.go](../internal/core/session_sync.go) |
| V060-P10 / SF-P10 | Touch ID | 缺失；原基线平台专属排除 | Desktop 有主密码与本机保护，没有 Touch ID 产品入口；SSH Agent 与系统凭据文件保护不是 Touch ID。 | OShell 原版有 Touch ID；Desktop 当前源码未发现 LocalAuthentication/Touch ID 集成。旧范围明确排除此平台项，因此记“缺失＋平台专属排除”，不是新的必须修复任务，不追加 Windows Hello。 | [password_protection.go](../internal/desktop/password_protection.go)；[security_view.go](../internal/desktop/security_view.go)；[oshell-feature-inventory.md](../docs/oshell-feature-inventory.md) |

### 连接与代理

| 编号 / 旧编号 | 功能与重点 | 实现状态 / 性质 | Desktop 当前实现 | 差异、缺失与边界 | 当前源码入口 |
| --- | --- | --- | --- | --- | --- |
| V060-C01 / SF-C01 | SSH 连接；原34变化 | 存在差异；平台替换；新增提示缺口 | Go SSH 已接密码/私钥/SSH Agent、known_hosts、连接状态及重连；连接配置超时 15 秒，认证上下文 2 分钟。 | 原版运行系统 ssh，并用临时 Include 配置控制本机 UTF-8/SendEnv。Desktop 直接使用 Go SSH，未发现对应 SSH 客户端 locale 包装；已有“已连接”状态，但没有原版终端内 Ctrl+Alt+] 本地入口提示。不能把两种 SSH 执行机制当作完全一致，也不能仅凭无 locale 包装断言中文必然乱码。 | [transport.go:193](../internal/core/transport.go)；[app.go:757](../internal/desktop/app.go)；[ssh_config.go:36](../internal/core/ssh_config.go) |
| V060-C02 / SF-C02 | 主机身份 | 已实现；未定位新增功能差异 | 未知主机确认、known_hosts 保存、已知指纹不匹配拒绝。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [transport.go](../internal/core/transport.go)；[ssh.go](../internal/core/ssh.go) |
| V060-C03 / SF-C03 | 旧 SSH 兼容 | 已实现；未定位新增功能差异 | 固定旧 SSH 算法表已映射；aes192-cbc/aes256-cbc 与 CBC/MAC 通路已补齐，默认关闭。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [transport.go](../internal/core/transport.go)；[go.mod](../go.mod)；[cipher.go](../third_party/xcrypto/ssh/cipher.go)；[common.go](../third_party/xcrypto/ssh/common.go)；[cipher_cbc_etm.go](../third_party/xcrypto/ssh/cipher_cbc_etm.go) |
| V060-C04 / SF-C04 | 本地终端；原34变化 | 存在差异；部分缺失；平台入口差异 | 本地终端按钮、可配置 local 动作及 Windows ConPTY/Unix PTY 已有，打开新本地页保留原连接。 | 未发现 Finder 所选目录服务入口及原版专门的 Ctrl+Alt+] 快捷处理；通用 local 动作不等于目录服务或默认绑定完全相同。Finder 属 macOS 特定入口，本轮不追加 Windows 资源管理器扩展。 | [app.go:388](../internal/desktop/app.go)；[shortcuts-presets.json](../internal/core/shortcuts-presets.json)；[shortcuts.go:135](../internal/desktop/shortcuts.go)；[launch_compat_darwin.go](../launch_compat_darwin.go)；[pty_windows.go](../internal/terminal/internal/pty/pty_windows.go) |
| V060-C05 / SF-C05 | 共享代理管理 | 已实现；未定位新增功能差异 | 共享代理目录、创建编辑删除、会话引用与凭据绑定。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [proxy_catalog.go](../internal/core/proxy_catalog.go)；[proxy_manager.go](../internal/desktop/proxy_manager.go) |
| V060-C06 / SF-C06 | 代理类型 | 已实现；未定位新增功能差异 | 四类代理已有；SOCKS5 有凭据禁止无认证回退，HTTP CONNECT 响应头限制 16 KiB。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [proxy.go](../internal/core/proxy.go) |
| V060-C07 / SF-C07 | 多级路由 | 已实现；未定位新增功能差异 | 代理链解析、缺失/循环拒绝、最多 8 层及 SSH 跳板。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [proxy_catalog.go](../internal/core/proxy_catalog.go)；[ssh_jump.go](../internal/core/ssh_jump.go)；[proxy.go](../internal/core/proxy.go) |
| V060-C08 / SF-C08 | 路由解析；原34变化 | 存在差异；新增全局代理缺口；既有平台替换 | 共享代理管理、按会话 ProxyID 解析链、ssh -G 解析别名及 ProxyJump/ProxyCommand 已有。 | 没有本机 global-proxy 设置、工具栏开关或覆盖新 SSH/SFTP/SCP 路由的全局入口。现有 RuntimeProxyRoute 来自会话选择，不能算全局代理；ssh -G + Go SSH 的固定字段消费仍是既有替换。 | [proxy_catalog.go:112](../internal/core/proxy_catalog.go)；[ssh_config.go:36](../internal/core/ssh_config.go)；[proxy_route.go:13](../internal/core/proxy_route.go)；[unified_toolbar.go](../internal/desktop/unified_toolbar.go) |
| V060-C09 / SF-C09 | 隧道 | 已实现；未定位新增功能差异 | 本地/远程端口转发及动态代理由连接上的转发规则启动。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [transport.go](../internal/core/transport.go)；[tunnels.go](../internal/core/tunnels.go)；[connection_editor.go](../internal/desktop/connection_editor.go) |
| V060-C10 / SF-C10 | 隧道占用 | 已实现；未定位新增功能差异 | 以会话 UUID 原子占用隧道所有权，重复连接跳过已有隧道。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [tunnel_ownership.go](../internal/core/tunnel_ownership.go)；[transport.go](../internal/core/transport.go) |
| V060-C11 / SF-C11 | SSH 连接复用；原34变化 | 存在差异；关联 rsync 子能力缺失 | 进程内 SSH transport 引用计数、克隆终端/文件通道、Lease 与图片上传复用已有。 | 原版新增 rsync 连接组租约；Desktop 没有 rsync 调用者。现有 Lease 只是可复用基础设施，不证明 rsync 已接入。归 F07 同一缺口，不独立计任务。 | [transport.go:152](../internal/core/transport.go)；[file_window.go:219](../internal/desktop/file_window.go)；[image_paste.go](../internal/desktop/image_paste.go) |
| V060-C12 / SF-C12 | 保活 | 已实现；未定位新增功能差异 | SSH 请求保活、TCP KeepAlive、间隔和最大未响应次数设置。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [transport.go](../internal/core/transport.go)；[options.go](../internal/core/options.go)；[defaults.go](../internal/desktop/defaults.go) |
| V060-C13 / SF-C13 | 空闲字符串 | 已实现；未定位新增功能差异 | 空闲发送、转义解析、输入/传输门禁与运行期计时重置。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [transport.go](../internal/core/transport.go)；[idle_input.go](../internal/terminal/idle_input.go) |
| V060-C14 / SF-C14 | 连接身份展示 | 已实现；未定位新增功能差异 | 从实际连接记录 TCP 对端，主机详情独立展示。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [transport.go](../internal/core/transport.go)；[tab_details.go](../internal/desktop/tab_details.go) |

### 窗口与导航

| 编号 / 旧编号 | 功能与重点 | 实现状态 / 性质 | Desktop 当前实现 | 差异、缺失与边界 | 当前源码入口 |
| --- | --- | --- | --- | --- | --- |
| V060-W01 / SF-W01 | 多窗口；原34变化 | 存在差异；授权行为差异；关联 MCP/文件停靠缺失 | 多窗口、共享配置 hub、各窗标签/发送目标、统一头部和本机生命周期诊断已有。 | Windows 按用户明确要求最后窗口关闭即退出；非 Windows 注册零窗口存活，不能概括为全部平台都退出。原版为 macOS 生命周期；Desktop 未有 MCP 面板及新版文件停靠。现有崩溃日志不是 Agent 审计。 | [application.go:45](../internal/desktop/application.go)；[unified_toolbar.go](../internal/desktop/unified_toolbar.go)；[lifecycle_diagnostics.go](../internal/desktop/lifecycle_diagnostics.go)；[log.go](../internal/diagnostics/log.go) |
| V060-W02 / SF-W02 | 标签导航 | 已实现；未定位新增功能差异 | 前后遍历当前窗口全部非隐藏标签，编号跳转仍限当前组；最近使用切换已有。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [groups.go](../internal/desktop/groups.go)；[shortcuts.go](../internal/desktop/shortcuts.go) |
| V060-W03 / SF-W03 | 标签操作；原34变化 | 已实现；新增交互已由既有授权覆盖 | 标签新建/复制/重连/排序/拖动已实现；会话和文件标签使用 HideScrollbars，支持 Shift 纵向滚轮横移、横向手势及定位。 | 0.6.0 本项的隐藏滚动条与 Shift 横滚已覆盖；未定位新的功能缺口。真实物理滚轮和各平台事件仍待验，不能由软件交互回归推定全部设备一致。 | [workspace.go:475](../internal/desktop/workspace.go)；[file_window.go:112](../internal/desktop/file_window.go)；[input.go](../third_party/mygo/ui/input.go)；[tab_drag_scroll.go](../internal/desktop/tab_drag_scroll.go)；[tab_duplicate.go](../internal/desktop/tab_duplicate.go) |
| V060-W04 / SF-W04 | 终端分屏 | 已实现；未定位新增功能差异 | 嵌套分屏、深度优先切换、单叶关闭、放大与尺寸状态。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [workspace.go](../internal/desktop/workspace.go)；[navigation.go](../internal/desktop/navigation.go)；[pane_sizing.go](../internal/desktop/pane_sizing.go) |
| V060-W05 / SF-W05 | 标签排列；原34变化 | 存在差异；操作对象差异 | arrangeTabs 实现窗口标签/组布局；arrangeActive 实现当前标签内部 pane 重排；头部已有“分组排列/分屏排列”。 | 原版“分屏排列”清除自定义布局并排列窗口标签；Desktop 同名入口调用 arrangeActive，只重排当前标签的 pane。Desktop arrangeTabs 遇命名组时保留既有分组，而原版 arrangeGroups 按标签建组并继承活动组元数据。标签滚动已实现不抵消排列语义差异。 | [unified_toolbar.go:256](../internal/desktop/unified_toolbar.go)；[layout.go:75](../internal/desktop/layout.go)；[groups.go:328](../internal/desktop/groups.go) |
| V060-W06 / SF-W06 | 自定义标签组 | 已实现；未定位新增功能差异 | 命名空组、改名、显隐、移入、解散保留会话和名称限制。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [groups.go](../internal/desktop/groups.go) |
| V060-W07 / SF-W07 | 跨窗口迁移 | 已实现；未定位新增功能差异 | 同进程跨窗拖放、指定位置插入、拖出新窗和取消/模态门禁。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [tab_window_drag.go](../internal/desktop/tab_window_drag.go)；[tab_move_guard.go](../internal/desktop/tab_move_guard.go)；[application.go](../internal/desktop/application.go) |
| V060-W08 / SF-W08 | 迁移连续性 | 存在差异；平台组织差异 | 迁移原 tab/pane、连接与焦点 owner；多窗焦点崩溃修复已在当前源码。 | Desktop 文件窗由 applicationHub 全局持有，原版附属工作区文件窗的转移组织不同；会话连续性已有，不把对象组织差别当作全部迁移缺失。 | [application.go](../internal/desktop/application.go)；[input_target_detach.go](../internal/desktop/input_target_detach.go)；[file_window.go](../internal/desktop/file_window.go) |
| V060-W09 / SF-W09 | 快速切换/连接 | 已实现；未定位新增功能差异 | 全部/已打开/已保存检索，定位分屏、隐藏组及保存资料连接。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [navigation.go](../internal/desktop/navigation.go)；[session_search.go](../internal/core/session_search.go) |
| V060-W10 / SF-W10 | 专注全屏；原34变化 | 存在差异；平台替换；macOS 原生入口未对齐 | 专注全屏有请求/过渡/失败恢复状态机；应用入口先隐藏 chrome，再切系统全屏，退出恢复。 | Windows/Linux 全屏替代 macOS Spaces 属平台差别。Desktop darwin 绑定进入/退出完成与失败回调，未发现原版 windowWillEnterFullScreen 绿按钮预处理和相同 presentation options 链路；不能认定原生按钮路径等价。 | [focus_fullscreen.go:44](../internal/desktop/focus_fullscreen.go)；[focus_fullscreen_darwin.go:57](../internal/desktop/focus_fullscreen_darwin.go)；[application.go:228](../internal/desktop/application.go) |
| V060-W11 / SF-W11 | 全心专注 | 已实现；未定位新增功能差异 | 全心专注自动进入全屏、隐藏附属标题/发送栏、退出保留普通专注。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [focus_fullscreen.go](../internal/desktop/focus_fullscreen.go)；[workspace.go](../internal/desktop/workspace.go)；[shortcuts.go](../internal/desktop/shortcuts.go) |
| V060-W12 / SF-W12 | 关闭确认 | 已实现；未定位新增功能差异 | 标签/分屏本机确认、窗口及退出活动计数、重连确认。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [session_close.go](../internal/desktop/session_close.go)；[workspace_close.go](../internal/desktop/workspace_close.go)；[application.go](../internal/desktop/application.go) |

### 主机信息与标题

| 编号 / 旧编号 | 功能与重点 | 实现状态 / 性质 | Desktop 当前实现 | 差异、缺失与边界 | 当前源码入口 |
| --- | --- | --- | --- | --- | --- |
| V060-T01 / SF-T01 | 标题方式 | 已实现；未定位新增功能差异 | 四种标题模式及主机观察策略分离，名称模式保留详情身份。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [session_titles.go](../internal/desktop/session_titles.go)；[host_identity.go](../internal/core/host_identity.go) |
| V060-T02 / SF-T02 | 默认与覆盖 | 已实现；未定位新增功能差异 | 本机默认/逐资料覆盖、共享 titleMode 回退、应用与复制继承。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [session_titles.go](../internal/desktop/session_titles.go)；[sidebar.go](../internal/desktop/sidebar.go)；[manager_actions.go](../internal/desktop/manager_actions.go) |
| V060-T03 / SF-T03 | Shell 集成 | 已实现；未定位新增功能差异 | 随包 Bash/Zsh 集成脚本及 OSC 777 身份上报解析接入。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [shell_integration.go](../internal/terminal/shell_integration.go)；[host_identity.go](../internal/core/host_identity.go)；[host_identity.go](../internal/desktop/host_identity.go) |
| V060-T04 / SF-T04 | 被动识别 | 已实现；未定位新增功能差异 | 固定提示符/OSC 标题/目录线索解析，超限和认证提示门禁。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [host_identity.go](../internal/core/host_identity.go)；[host_identity.go](../internal/desktop/host_identity.go) |
| V060-T05 / SF-T05 | 主动探测 | 已实现；未定位新增功能差异 | 80ms 扫描、300ms 稳定等待、4s 探测超时及代次/输入状态检查。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [host_identity.go](../internal/desktop/host_identity.go)；[host_probe.go](../internal/core/host_probe.go) |
| V060-T06 / SF-T06 | 本机与嵌套主机 | 已实现；未定位新增功能差异 | 本机身份快照、别名归一、嵌套切换命令观察及身份重置。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [host_identity.go](../internal/core/host_identity.go)；[host_identity.go](../internal/desktop/host_identity.go) |
| V060-T07 / SF-T07 | 分屏聚合标题 | 已实现；未定位新增功能差异 | 分屏逐名按 32 字符规则裁剪、控制字符清理、同名聚合及顺序已有。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [session_titles.go](../internal/desktop/session_titles.go) |
| V060-T08 / SF-T08 | 悬停信息卡 | 已实现；未定位新增功能差异 | 当前分屏字段、实际对端、逐项/全部复制及完整连接详情。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [tab_details.go](../internal/desktop/tab_details.go)；[host_identity.go](../internal/desktop/host_identity.go) |
| V060-T09 / SF-T09 | 信息卡生命周期 | 已实现；未定位新增功能差异 | 悬停信息卡已修复鼠标在卡内超时关闭；移出/重入、复制和外部关闭路径已有。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [tab_details.go](../internal/desktop/tab_details.go)；[application.go](../internal/desktop/application.go) |

### 配置与同步

| 编号 / 旧编号 | 功能与重点 | 实现状态 / 性质 | Desktop 当前实现 | 差异、缺失与边界 | 当前源码入口 |
| --- | --- | --- | --- | --- | --- |
| V060-Y01 / SF-Y01 | 本机工作副本 | 已实现；未定位新增功能差异 | 本机配置/密码库工作副本、原版迁移及备份路径独立于同步目录。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [native_migration.go](../internal/desktop/native_migration.go)；[sync_location.go](../internal/core/sync_location.go)；[shared_sync.go](../internal/core/shared_sync.go) |
| V060-Y02 / SF-Y02 | 同步目录 | 已实现；未定位新增功能差异 | 新建/已有同步目录选择、重叠检查、空目录条件和条件写入。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [sync_location.go](../internal/core/sync_location.go)；[sync_transport.go](../internal/core/sync_transport.go)；[sync_location.go](../internal/desktop/sync_location.go) |
| V060-Y03 / SF-Y03 | WebDAV | 已实现；未定位新增功能差异 | GET/PROPFIND/MKCOL/PUT、Basic Auth、拒重定向、强 ETag、体积限制和目录发现。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [sync_transport.go](../internal/core/sync_transport.go)；[webdav_directory.go](../internal/core/webdav_directory.go) |
| V060-Y04 / SF-Y04 | 同步前提 | 已实现；未定位新增功能差异 | 同步要求已解锁主密码，WebDAV 凭据受本机保护，保护变更走事务。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [sync_settings.go](../internal/core/sync_settings.go)；[sync_options_protection.go](../internal/desktop/sync_options_protection.go)；[master_transaction.go](../internal/core/master_transaction.go) |
| V060-Y05 / SF-Y05 | 默认范围 | 已实现；未定位新增功能差异 | 会话类默认选中，包含会话/目录/引用/代理及必要凭据。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [session_sync.go](../internal/core/session_sync.go)；[sync_policy.go](../internal/core/sync_policy.go) |
| V060-Y06 / SF-Y06 | 可选范围；原34变化 | 存在差异；字体附件子能力缺失 | 四类同步内容选择已有，默认仅会话资料。 | 缺少默认关闭的“字体文件”选择和对应同步调度。 字体是原版新增的独立明文附件通道；不能据此判断现有加密会话 v2 已失效，也不能以 v2 样本通过认定字体互通。 | [sync_policy.go:9](../internal/core/sync_policy.go)；[sync_policy.go](../internal/desktop/sync_policy.go) |
| V060-Y07 / SF-Y07 | 策略 | 已实现；未定位新增功能差异 | 自动/启动/变更调度，2 秒合并、60 秒检查/重试默认及暂停状态。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [sync_policy.go](../internal/core/sync_policy.go)；[sync_policy.go](../internal/desktop/sync_policy.go)；[shared_sync.go](../internal/desktop/shared_sync.go) |
| V060-Y08 / SF-Y08 | 首次接入 | 已实现；未定位新增功能差异 | 首次采用共享/合并选择、冲突草稿和提交前备份，相同内容省去选择。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [shared_sync.go](../internal/core/shared_sync.go)；[shared_sync.go](../internal/desktop/shared_sync.go) |
| V060-Y09 / SF-Y09 | 多节点合并；原34变化 | 存在差异；字体附件子能力缺失 | 会话/引用/代理等三方合并及冲突选择已有。 | 缺少独立字体清单、每目标基准及新增/删除合并。 字体是原版新增的独立明文附件通道；不能据此判断现有加密会话 v2 已失效，也不能以 v2 样本通过认定字体互通。 | [shared_merge.go](../internal/core/shared_merge.go)；[session_sync.go](../internal/core/session_sync.go) |
| V060-Y10 / SF-Y10 | 并发写保护；原34变化 | 存在差异；字体附件子能力缺失 | 配置强 ETag、If-Match/If-None-Match、412 与本机 revision 保护已有。 | 缺少字体内容对象先上传、清单条件提交及本机字体快照校验。 字体是原版新增的独立明文附件通道；不能据此判断现有加密会话 v2 已失效，也不能以 v2 样本通过认定字体互通。 | [sync_transport.go](../internal/core/sync_transport.go)；[shared_sync.go](../internal/core/shared_sync.go) |
| V060-Y11 / SF-Y11 | 无变化处理；原34变化 | 存在差异；字体附件子能力缺失 | 配置 UUID/默认值/空集合规范化和无变化不重复 PUT 已有。 | 缺少字体对象/清单的语义无变化判断及 ETag 复核。 字体是原版新增的独立明文附件通道；不能据此判断现有加密会话 v2 已失效，也不能以 v2 样本通过认定字体互通。 | [session_sync.go](../internal/core/session_sync.go)；[shared_sync.go](../internal/core/shared_sync.go) |
| V060-Y12 / SF-Y12 | 错误保护 | 已实现；未定位新增功能差异 | 密码/数据/版本/冲突错误拒绝应用，保留旧配置和冲突草稿。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [shared_sync.go](../internal/core/shared_sync.go)；[shared_merge.go](../internal/core/shared_merge.go)；[shared_protocol.go](../internal/core/shared_protocol.go) |
| V060-Y13 / SF-Y13 | 证书 | 已实现；未定位新增功能差异 | WebDAV 证书按 host/port/证书精确绑定，验证有效期并本机加密保存。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [webdav_certificate.go](../internal/core/webdav_certificate.go)；[webdav_certificate.go](../internal/desktop/webdav_certificate.go) |
| V060-Y14 / SF-Y14 | 状态窗口 | 已实现；未定位新增功能差异 | 共享同步状态、独立非模态状态窗、工具栏与菜单展示。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [sync_status_window.go](../internal/desktop/sync_status_window.go)；[sync_presentation.go](../internal/desktop/sync_presentation.go)；[shared_sync.go](../internal/desktop/shared_sync.go) |
| V060-Y15 / SF-Y15 | 诊断 | 已实现；未定位新增功能差异 | 安全事件/失败分类、7 天/2000 条/大小裁剪、状态摘要及日志导出。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [sync_diagnostics.go](../internal/core/sync_diagnostics.go)；[sync_diagnostics.go](../internal/desktop/sync_diagnostics.go) |
| V060-Y16 / SF-Y16 | 本机配置变化 | 已实现；未定位新增功能差异 | 本机外部变更检测、持久冲突草稿及明确解决/放弃路径。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [local_conflict.go](../internal/desktop/local_conflict.go)；[shared_sync.go](../internal/core/shared_sync.go)；[sync_connection.go](../internal/core/sync_connection.go) |
| V060-Y17 / SF-Y17 | 格式兼容；原34变化 | 存在差异；字体附件子能力缺失 | 核心共享配置读 v1/v2、写 v2及生成合同已有；原版 0.6.0 核心合同与旧版相同。 | 缺少 OShell.font-library v1 明文清单及 fonts/ 附件格式支持。 字体是原版新增的独立明文附件通道；不能据此判断现有加密会话 v2 已失效，也不能以 v2 样本通过认定字体互通。 | [shared_configuration_schema.json](../internal/core/shared_configuration_schema.json)；[shared_protocol.go](../internal/core/shared_protocol.go)；[sync_policy.go](../internal/core/sync_policy.go) |

### 终端

| 编号 / 旧编号 | 功能与重点 | 实现状态 / 性质 | Desktop 当前实现 | 差异、缺失与边界 | 当前源码入口 |
| --- | --- | --- | --- | --- | --- |
| V060-I01 / SF-I01 | 常规交互；原34变化 | 存在差异；授权缩放参数差异 | 普通键鼠/IME 路径和 Ctrl 滚轮当前终端临时缩放已有；缩放不保存、不广播、不向远端转发滚轮。 | Desktop 阈值 300 DIP、空闲重置 1 秒、字号 10–26；原版精细/非精细阈值 120/12、0.8 秒、8–40，并显式忽略惯性与处理手势阶段。两端滚轮单位不能直接相除推断体感；Desktop 未见对应手势/惯性分支。 | [font_wheel.go:10](../internal/terminal/font_wheel.go)；[view.go](../internal/terminal/view.go)；[preedit.go](../internal/terminal/preedit.go) |
| V060-I02 / SF-I02 | 选择 | 存在差异；用户授权视觉差异 | 选择、复制、宽字和跨行选区已有；内置配色改为高对比反色。 | 与原版默认选区色不同，是按用户 Xshell 参考图批准的视觉行为；自定义已存配色继续保留。 | [colors-presets.json](../internal/core/colors-presets.json)；[paint.go](../internal/terminal/paint.go)；[selection.go](../internal/terminal/internal/vt/selection.go) |
| V060-I03 / SF-I03 | 复制 | 已实现；未定位新增功能差异 | 手动/自动复制、首尾清理、逻辑行提取和宽字占位处理。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [oshell.go](../internal/terminal/oshell.go)；[view.go](../internal/terminal/view.go)；[selection.go](../internal/terminal/internal/vt/selection.go) |
| V060-I04 / SF-I04 | 文本粘贴 | 存在差异；已有增强 | 多入口粘贴、预览、目标快照及 CRLF 单换行处理已有。 | 编辑预览后与最终发送再次检查 1 MiB/NUL，保留对原版校验缺口的修正。 | [terminal_features.go](../internal/desktop/terminal_features.go)；[terminal.go](../internal/terminal/terminal.go) |
| V060-I05 / SF-I05 | 终端搜索 | 存在差异；已有执行预算差异 | 正则/整词搜索、65536 格软换行上限及循环导航已有。 | regexp2/平台正则替代与执行预算不同；已记录约单次 10 ms/总扫描 1 秒限制，不能保证任意复杂模式与原版相同。 | [search.go](../internal/terminal/search.go)；[regex.go](../internal/core/regex.go)；[regex_native_other.go](../internal/core/regex_native_other.go) |
| V060-I06 / SF-I06 | 输出保留；原34变化 | 存在差异；关联 MCP 审计缺失 | 历史容量、结束保屏和过滤控制序列的异步文本日志已有，积压上限 4 MiB。 | 没有新增 Agent 加密审计/只读回放。普通文本日志与运行诊断不含授权、审批、命令收据和认证链，不能替代 G12–G15；这是同一新增域的关联缺口。 | [output_log.go:56](../internal/terminal/output_log.go)；[features.go](../internal/desktop/features.go)；[log.go](../internal/diagnostics/log.go) |
| V060-I07 / SF-I07 | 链接 | 已实现；未定位新增功能差异 | OSC8/文本链接、悬停点击与方案白名单接入。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [links.go](../internal/terminal/links.go)；[link_discovery.go](../internal/terminal/link_discovery.go)；[link_hover.go](../internal/terminal/link_hover.go)；[hyperlink.go](../internal/terminal/internal/vt/hyperlink.go) |
| V060-I08 / SF-I08 | 本机工具入口 | 已实现；未定位新增功能差异 | 本机提示符、help/?/tools、独立工具 PTY 与退出中断已有。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [tools_console.go](../internal/terminal/tools_console.go)；[local_tools.go](../internal/core/local_tools.go) |
| V060-I09 / SF-I09 | 工具分类 | 已实现；未定位新增功能差异 | 固定八类工具名称均在白名单，路径查询与交互工具单目标限制已有。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [local_tools.go](../internal/core/local_tools.go)；[tools_console.go](../internal/terminal/tools_console.go) |
| V060-I10 / SF-I10 | 提示符编辑 | 已实现；未定位新增功能差异 | 提示符编辑/补全/历史及 8192 UTF-8 字节上限，解析后符号按字面 argv 传工具。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [prompt_editor.go](../internal/terminal/prompt_editor.go)；[local_tools.go](../internal/core/local_tools.go) |
| V060-I11 / SF-I11 | 提示符历史 | 已实现；未定位新增功能差异 | 100 条内存历史、相邻去重、上下草稿及反向/正向搜索。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [prompt_editor.go](../internal/terminal/prompt_editor.go)；[tools_console.go](../internal/terminal/tools_console.go) |
| V060-I12 / SF-I12 | 结束页 | 已实现；未定位新增功能差异 | 结束保屏、切本机提示符、工具退出排空与当前分屏关闭。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [session_fallback.go](../internal/terminal/session_fallback.go)；[tool_process.go](../internal/terminal/tool_process.go)；[app.go](../internal/desktop/app.go) |
| V060-I13 / SF-I13 | 传输中的交互 | 已实现；未定位新增功能差异 | 传输占用输入通道，阻止常规/托管发送，保留本地选择与滚动。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [transfer.go](../internal/terminal/transfer.go)；[view.go](../internal/terminal/view.go)；[keys.go](../internal/terminal/keys.go) |
| V060-I14 / SF-I14 | 字符与解码 | 存在差异；已有增强；引擎替换 | Ghostty 兼容分支、UTF-8/字符集/控制串处理已有。 | 保留 OSC 8 MiB 总预算；不移除已有上限来复制原版缺陷。Swift 对照未运行，不能宣称底层全部边界等价。 | [oshell.patch](../third_party/ghostty/oshell.patch)；[terminal.go](../internal/terminal/internal/vt/terminal.go) |
| V060-I15 / SF-I15 | 屏幕与历史 | 已实现；未定位新增功能差异 | 双屏/历史/边界/重排及光标恢复的兼容分支已接入。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [oshell.patch](../third_party/ghostty/oshell.patch)；[terminal.go](../internal/terminal/internal/vt/terminal.go)；[scrollbar.go](../internal/terminal/scrollbar.go) |
| V060-I16 / SF-I16 | 编辑与擦除 | 已实现；未定位新增功能差异 | 光标编辑、擦除、矩形与单元格元数据搬移的定制路径已有。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [oshell.patch](../third_party/ghostty/oshell.patch)；[terminal.go](../internal/terminal/internal/vt/terminal.go) |
| V060-I17 / SF-I17 | 颜色与属性 | 存在差异；已有增强 | SGR/调色板/下划线等已有，并在绘制中消费 Invisible 属性。 | Invisible 隐藏行为是对固定原版属性交接缺口的有意修正；本轮不反向引入显示缺陷。 | [paint.go](../internal/terminal/paint.go)；[palette.go](../internal/terminal/palette.go) |
| V060-I18 / SF-I18 | 键盘协议；原34变化 | 存在差异；新增应用快捷入口缺失 | 传统/Kitty 键盘编码、IME 和 AltGr 路由已有，应用快捷键可配置。 | 没有原版专门 Ctrl+Alt+] 本地终端处理，也无 MCP 面板动作；现有 local 可配置动作不能认定默认绑定相同。缺失属于应用动作，不是新增远端键盘协议。 | [keys.go](../internal/terminal/keys.go)；[shortcuts.go:135](../internal/desktop/shortcuts.go)；[shortcuts-presets.json](../internal/core/shortcuts-presets.json) |
| V060-I19 / SF-I19 | 鼠标与焦点；原34变化 | 存在差异；缩放差异；关联 MCP 写入仲裁缺失 | 鼠标协议/焦点/选择、标签 Shift 横滚及当前终端 Ctrl 缩放已有。 | 字号参数差异归 I01；未有 MCP 任务所有权和按客户端/连接代次检查的最终 PTY 闸门。现有传输忙碌、同步输入及 managed 输入保护不能视为 Agent 授权仲裁。 | [view.go](../internal/terminal/view.go)；[font_wheel.go](../internal/terminal/font_wheel.go)；[terminal.go](../internal/terminal/terminal.go)；[operators.go](../internal/desktop/operators.go) |
| V060-I20 / SF-I20 | 查询与状态 | 已实现；未定位新增功能差异 | DA/DSR/状态回复、宿主 BEL 响铃、OSC9;4 五状态进度与超时清理已接通。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [terminal.go](../internal/terminal/terminal.go)；[terminal.go](../internal/terminal/internal/vt/terminal.go)；[app.go](../internal/desktop/app.go) |
| V060-I21 / SF-I21 | 刷新与重置 | 已实现；未定位新增功能差异 | 合并刷新、同步输出与 DECSTR/RIS 定制状态处理存在。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [terminal.go](../internal/terminal/terminal.go)；[paint.go](../internal/terminal/paint.go)；[oshell.patch](../third_party/ghostty/oshell.patch) |
| V060-I22 / SF-I22 | 终端图片 | 存在差异；平台介质替换 | Kitty 直接/文件/临时文件/共享内存、Sixel、OSC1337 等已有。 | Windows 共享内存使用 Win32 命名对象替代 POSIX 介质，命名及最后句柄释放语义不同；四介质已有实现，不能沿用修复前“Windows 不支持”的历史结论。 | [kitty_local.go](../internal/terminal/internal/vt/kitty_local.go)；[oshell.patch](../third_party/ghostty/oshell.patch)；[graphics.go](../internal/terminal/graphics.go) |
| V060-I23 / SF-I23 | Unicode 图形占位 | 已实现；未定位新增功能差异 | Unicode 占位符、颜色身份/附加字符继承及虚拟位置逐格绘制。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [kitty_placeholder.go](../internal/terminal/kitty_placeholder.go)；[paint.go](../internal/terminal/paint.go)；[graphics.go](../internal/terminal/internal/vt/graphics.go) |
| V060-I24 / SF-I24 | 双向文字 | 存在差异；平台排版替换 | Go UAX#9/字形布局、六模式、显示/选择映射已有并默认启用。 | 实现机制不同于 CoreText 探针；不能据代码存在判复杂混排、IME 或所有字形已等价。 | [bidi.go](../internal/terminal/bidi.go)；[layout.go](../internal/terminal/internal/bidilayout/layout.go)；[bidi.go](../internal/terminal/internal/vt/bidi.go) |
| V060-I25 / SF-I25 | 特殊行显示 | 已实现；未定位新增功能差异 | 双宽/双高行元数据、行变换/裁剪与光标/命中交接已有。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [line_mode.go](../internal/terminal/internal/vt/line_mode.go)；[line_mode.go](../internal/terminal/line_mode.go)；[paint.go](../internal/terminal/paint.go)；[oshell.patch](../third_party/ghostty/oshell.patch) |
| V060-I26 / SF-I26 | 提示符语义；原34变化 | 存在差异；新增 OSC779 命令生命周期缺失 | OSC133 提示符语义与点击输入已有；OSC777 主机身份上报已有。 | 没有原版带随机令牌的 OSC779 ready/start/end/退出码与 Agent 命令绑定。OSC133/777 不是相同协议，也不能用于声称 MCP 命令已完成。 | [semantic_click.go](../internal/terminal/semantic_click.go)；[shell_integration.go:17](../internal/terminal/shell_integration.go)；[host_identity.go](../internal/desktop/host_identity.go) |
| V060-I27 / SF-I27 | 宿主限制；原34变化 | 存在差异；关联 MCP 宿主能力缺失 | 生产宿主禁用远端 OSC52，用户主动复制粘贴、链接白名单与现有图形约束已有。 | 没有 Agent 屏幕观察/最终写入闸门、OSC779 分派及独立只读回放宿主。终端测试快照不是生产回放，不能算 G14。 | [terminal.go:252](../internal/terminal/terminal.go)；[view.go](../internal/terminal/view.go)；[terminal.go](../internal/terminal/internal/vt/terminal.go) |

### 图片粘贴

| 编号 / 旧编号 | 功能与重点 | 实现状态 / 性质 | Desktop 当前实现 | 差异、缺失与边界 | 当前源码入口 |
| --- | --- | --- | --- | --- | --- |
| V060-M01 / SF-M01 | 识别与转换 | 已实现；未定位新增功能差异 | 图片文件及 PNG/TIFF/JPEG 原生剪贴板回退、Windows DIB 等平台转换路径已有。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [image_paste.go](../internal/desktop/image_paste.go)；[clipboard_image.go](../internal/core/clipboard_image.go)；[clipboard.go](../third_party/mygo/internal/darwin/clipboard.go)；[clipboard.go](../third_party/mygo/internal/windows/clipboard.go) |
| V060-M02 / SF-M02 | 用户确认 | 已实现；未定位新增功能差异 | 确认窗显示尺寸/大小、远端目录及记忆选项，取消不插入。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [image_paste.go](../internal/desktop/image_paste.go) |
| V060-M03 / SF-M03 | 本地目标 | 已实现；未定位新增功能差异 | 本机私有 PNG 缓存、普通文件/目录检查及目标 shell 转义。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [clipboard_image.go](../internal/core/clipboard_image.go)；[clipboard_image_windows.go](../internal/core/clipboard_image_windows.go)；[paste_context.go](../internal/terminal/paste_context.go) |
| V060-M04 / SF-M04 | SSH 目标 | 已实现；未定位新增功能差异 | 复用已认证连接的 SFTP、随机私有目录、排他文件、取消与进度。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [clipboard_image.go](../internal/core/clipboard_image.go)；[image_paste.go](../internal/desktop/image_paste.go)；[transport.go](../internal/core/transport.go) |
| V060-M05 / SF-M05 | 发送规则 | 已实现；未定位新增功能差异 | 图片结果仅插入原目标且不回车，最终检查连接/宿主代次；缓存清除不误阻断。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [image_paste.go](../internal/desktop/image_paste.go)；[paste_context.go](../internal/terminal/paste_context.go) |
| V060-M06 / SF-M06 | 提醒记忆 | 已实现；未定位新增功能差异 | 本地总开关及 SSH UUID/路由指纹记忆，成功插入后保存。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [image_paste.go](../internal/desktop/image_paste.go)；[sidebar.go](../internal/desktop/sidebar.go) |
| V060-M07 / SF-M07 | 资源边界 | 已实现；未定位新增功能差异 | 20 MiB 文件/输出、160 MiB 输入、边长/像素限制和缓存 200 MiB/7 天。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [clipboard_image.go](../internal/core/clipboard_image.go) |
| V060-M08 / SF-M08 | 生命周期 | 已实现；未定位新增功能差异 | 失败/取消尽力清理，成功远端图片保留，生命周期回调核对原状态。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [clipboard_image.go](../internal/core/clipboard_image.go)；[image_paste.go](../internal/desktop/image_paste.go) |

### 文件管理与传输

| 编号 / 旧编号 | 功能与重点 | 实现状态 / 性质 | Desktop 当前实现 | 差异、缺失与边界 | 当前源码入口 |
| --- | --- | --- | --- | --- | --- |
| V060-F01 / SF-F01 | 文件窗口；原34变化 | 存在差异；部分缺失 | 独立文件窗、多标签、复制/重连、传输与关闭确认已有。 | 正常 hub 工具栏进入独立文件窗，没有窗口/左/右/底部位置设置及同一文件工作区重挂接。filesVisible 的旧侧栏分支只在无 hub 路径切换，不能当作正常产品的新版停靠功能。终端命令模式亦缺失。 | [file_window.go:17](../internal/desktop/file_window.go)；[workspace.go:225](../internal/desktop/workspace.go)；[workspace.go:57](../internal/desktop/workspace.go) |
| V060-F02 / SF-F02 | 连接入口；原34变化 | 存在差异；终端文件管理模式缺失 | 独立 SSH/SFTP/FTP 文件入口、复用当前 SSH 及拖放上传已有。 | 没有文件管理本机 mode/command 设置及空闲确认后向当前终端启动 yazi/ranger/lf/nnn/mc 的工具栏流程。用户手动输入这些命令不算该产品入口已实现。 | [file_window.go:17](../internal/desktop/file_window.go)；[settings.go](../internal/desktop/settings.go)；[files.go](../internal/desktop/files.go)；[terminal_drop.go](../internal/desktop/terminal_drop.go) |
| V060-F03 / SF-F03 | 目录浏览 | 已实现；未定位新增功能差异 | 路径/上级/刷新、目录优先、多选、当前目录过滤和未知类型探测。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [files.go](../internal/desktop/files.go)；[file_filter.go](../internal/desktop/file_filter.go)；[file_selection.go](../internal/desktop/file_selection.go) |
| V060-F04 / SF-F04 | 文件操作 | 存在差异；已有增强 | 建目录/改名/删除及 32 层递归、链接处理已修正。 | 保留 10000 项资源上限增强；顶层 SFTP 链接和递归子链接行为按旧修复执行，不扩展任意跟链。 | [transfer.go](../internal/core/transfer.go)；[ftp.go](../internal/core/ftp.go)；[file_actions.go](../internal/desktop/file_actions.go) |
| V060-F05 / SF-F05 | 属性展示 | 已实现；未定位新增功能差异 | 七列属性、文件图标、权限/用户组和可选元数据表示。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [files.go](../internal/desktop/files.go)；[file_icons.go](../internal/desktop/file_icons.go)；[file_attributes.go](../internal/core/file_attributes.go) |
| V060-F06 / SF-F06 | FTP 列表 | 存在差异；平台替换；已有增强 | Go FTP 三层列表回退、UTF-8/4 MiB/安全名称及未知值表示已有。 | Go FTP 解析器替代 curl，可靠 LIST 时间可显示；不能可靠解析的时间保持未知。 | [ftp.go](../internal/core/ftp.go)；[parse.go](../third_party/ftp/parse.go) |
| V060-F07 / SF-F07 | 传输方式；原34变化 | 存在差异；rsync 子能力缺失 | SFTP、Go FTP、SSH channel SCP 及无需 SFTP 的独立 SCP 已有。 | 没有 rsync 执行后端、协议选项或独立目录同步入口。Go FTP/SCP 继续是原有平台实现替换。 与 F07 是同一能力簇，不按关联行重复立项。 | [files.go:318](../internal/desktop/files.go)；[filesystem.go](../internal/core/filesystem.go)；[scp.go](../internal/core/scp.go)；[standalone_scp.go](../internal/desktop/standalone_scp.go) |
| V060-F08 / SF-F08 | 目标冲突；原34变化 | 存在差异；rsync 子能力缺失 | 原有避名、排他创建、临时文件发布与明确覆盖已有。 | 没有 rsync 同名增量更新、保留额外目标和目录同步确认语义。原有上传/下载冲突处理不能代表 rsync 路径。 与 F07 是同一能力簇，不按关联行重复立项。 | [transfer_names.go](../internal/desktop/transfer_names.go)；[transfer.go](../internal/core/transfer.go)；[scp.go](../internal/core/scp.go) |
| V060-F09 / SF-F09 | 限速；原34变化 | 存在差异；rsync 子能力缺失 | SFTP/FTP 动态限速，SCP 任务启动快照及标签限速 UI 已有。 | 没有 rsync --bwlimit 及其下一次传输生效语义。 与 F07 是同一能力簇，不按关联行重复立项。 | [file_rate.go](../internal/desktop/file_rate.go)；[transfer_limit.go](../internal/core/transfer_limit.go)；[files.go](../internal/desktop/files.go) |
| V060-F10 / SF-F10 | 进度与取消；原34变化 | 存在差异；rsync 子能力缺失 | 传输进度/速度、取消、批次失败停止及内存记录已有。 | 没有 rsync 进度解析、进程取消及记录接入。 与 F07 是同一能力簇，不按关联行重复立项。 | [files.go](../internal/desktop/files.go)；[transfer_records.go](../internal/desktop/transfer_records.go)；[file_transfer_overlay.go](../internal/desktop/file_transfer_overlay.go) |
| V060-F11 / SF-F11 | 传输记录 | 已实现；未定位新增功能差异 | 内存倒序记录、详情/片段、500 条已结束裁剪及清理。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [transfer_records.go](../internal/desktop/transfer_records.go) |
| V060-F12 / SF-F12 | SFTP 续传 | 已实现；未定位新增功能差异 | SFTP 检查点、源/片段身份检查、前缀逐字节比对和排他发布。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [transfer_resume.go](../internal/core/transfer_resume.go)；[sftp_open.go](../internal/core/sftp_open.go) |
| V060-F13 / SF-F13 | 续传范围；原34变化 | 存在差异；rsync 子能力缺失 | SFTP 顶层普通文件运行期暂停/继续、源/片段校验与失败重试已有。 | 没有 rsync .oshell-rsync-partial 片段复用。原版也不是跨重启自动恢复任务，不把这一点额外列为 Desktop 缺口。 与 F07 是同一能力簇，不按关联行重复立项。 | [transfer_resume.go:22](../internal/core/transfer_resume.go)；[transfer_records.go](../internal/desktop/transfer_records.go) |
| V060-F14 / SF-F14 | ZMODEM | 存在差异；已有授权增强；残留问题待定位 | rz/sz、选择/进度/取消、输入恢复及拖文件自动 rz 已有。 | 拖入 SSH 终端默认启动 rz 是用户授权增强。既有报告仍记录“下载”目录文件选择卡在正在处理，直接根因未确认；本轮未重现，不能宣称该问题已闭合，也不能据此认定 ZMODEM 协议缺失。 | [terminal_drop.go](../internal/desktop/terminal_drop.go)；[terminal_transfer.go](../internal/desktop/terminal_transfer.go)；[upload_picker_other.go](../internal/desktop/upload_picker_other.go)；[transfer.go](../internal/terminal/transfer.go) |
| V060-F15 / SF-F15 | 资源归属 | 已实现；未定位新增功能差异 | 终端/文件/SCP 持有连接 lease，释放幂等，迁移重绑任务归属。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [transport.go](../internal/core/transport.go)；[file_window.go](../internal/desktop/file_window.go)；[application.go](../internal/desktop/application.go) |

### 快捷操作

| 编号 / 旧编号 | 功能与重点 | 实现状态 / 性质 | Desktop 当前实现 | 差异、缺失与边界 | 当前源码入口 |
| --- | --- | --- | --- | --- | --- |
| V060-O01 / SF-O01 | 快速命令管理 | 已实现；未定位新增功能差异 | 快速命令增改删、分组、追加回车及填入快速栏/撰写窗。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [quick_command_manager.go](../internal/desktop/quick_command_manager.go)；[operators.go](../internal/desktop/operators.go)；[operators.go](../internal/core/operators.go) |
| V060-O02 / SF-O02 | 快速发送栏 | 已实现；未定位新增功能差异 | 快速发送/范围/多行撰写、上下历史与草稿恢复已有；历史 50 条/64 KiB/单条 8 KiB，成功重置追加回车。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [operators.go](../internal/desktop/operators.go)；[workspace.go](../internal/desktop/workspace.go) |
| V060-O03 / SF-O03 | 撰写窗 | 已实现；未定位新增功能差异 | 多行编辑、独立目标快照、追加回车及无额外确认直接发送。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [operators.go](../internal/desktop/operators.go)；[operators.go](../internal/core/operators.go)；[oshell.go](../internal/terminal/oshell.go) |
| V060-O04 / SF-O04 | 七种发送范围 | 已实现；未定位新增功能差异 | 七范围选择，组含隐藏成员，模式持久化/选择身份运行期。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [operators.go](../internal/desktop/operators.go)；[groups.go](../internal/desktop/groups.go) |
| V060-O05 / SF-O05 | 范围动态更新 | 已实现；未定位新增功能差异 | 发送前重算可输入对象、接收/跳过计数及关闭/迁移选择清理。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [operators.go](../internal/desktop/operators.go)；[input_target_detach.go](../internal/desktop/input_target_detach.go) |
| V060-O06 / SF-O06 | 同步输入 | 存在差异；已有增强 | 多目标同步输入、逐目标编码、失效停止及目标快照已有。 | 编辑预览后二次校验归 I04 同一增强；不把图片/协议回复扩进广播。 | [sync_input_draft.go](../internal/desktop/sync_input_draft.go)；[terminal_features.go](../internal/desktop/terminal_features.go) |
| V060-O07 / SF-O07 | 高亮规则 | 存在差异；已有增强；界面说明差异 | 保存超过 32 条规则、仅前 32 条参与渲染、4096 UTF-16/首规则优先及已有颜色能力均在当前源码。 | 保留背景/粗体等已有渲染能力；另发现 highlights.go 界面仍写“每集最多 32 条规则”，而添加按钮不限制 32、渲染才截前 32。说明容易误导，应记录在本条，暂不修改。 | [highlights.go:113](../internal/desktop/highlights.go)；[oshell.go:85](../internal/terminal/oshell.go)；[paint.go](../internal/terminal/paint.go) |
| V060-O08 / SF-O08 | 快捷键配置 | 已实现；未定位新增功能差异 | 可配置动作、内部重复阻止、系统保留键检查和不可读提示已有；平台映射独立。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [shortcuts.go](../internal/desktop/shortcuts.go)；[shortcuts.go](../internal/core/shortcuts.go)；[shortcut_capture_other.go](../internal/desktop/shortcut_capture_other.go) |
| V060-O09 / SF-O09 | 焦点与弹窗 | 已实现；未定位新增功能差异 | 工作区快捷键、录入/模态/密码/搜索/辅助窗等输入门禁。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [shortcuts.go](../internal/desktop/shortcuts.go)；[password_input.go](../internal/desktop/password_input.go)；[file_window.go](../internal/desktop/file_window.go)；[view.go](../internal/terminal/view.go) |

### 外观与资源

| 编号 / 旧编号 | 功能与重点 | 实现状态 / 性质 | Desktop 当前实现 | 差异、缺失与边界 | 当前源码入口 |
| --- | --- | --- | --- | --- | --- |
| V060-A01 / SF-A01 | 界面主题；原34变化 | 存在差异；已授权视觉替换；新增界面未实现 | 系统/浅/深主题及当前紧凑专业风已接设置、管理器、工具栏、弹窗和列表。 | Desktop 使用 MyGo 的跨平台表面/布局，原版使用 AppKit 统一卡片，不是像素级同款；Desktop 本轮之前的视觉调整有用户授权。未有 MCP/权限/审计页面，不能称新版全部界面已覆盖。 | [workspace_style.go:7](../internal/desktop/workspace_style.go)；[settings.go](../internal/desktop/settings.go)；[workspace.go:16](../internal/desktop/workspace.go) |
| V060-A02 / SF-A02 | 终端配色 | 存在差异；已有增强；授权视觉差异 | 九预设、自定义/导入导出及 AutoCursor 已接显示。 | 保留自动对比光标；内置选区色按用户授权改为高对比，与原版预设不同。 | [colors.go](../internal/core/colors.go)；[colors-presets.json](../internal/core/colors-presets.json)；[fonts.go](../internal/desktop/fonts.go)；[paint.go](../internal/terminal/paint.go) |
| V060-A03 / SF-A03 | 字体；原34变化 | 存在差异；字体库缺失；资源策略与缩放差异 | 本机字体枚举/搜索/选择、非等宽提示、预览、缺失回退、内置 Cascadia/DejaVu 四样式及 Light 字重修复已有。 | 没有用户字体文件/目录导入、活动清单、移除/打开库目录和 20 MiB/64 文件/64 MiB 库管理。Desktop 仍内置字体，原版改为系统/用户库；Ctrl 缩放差异归 I01。ui.RegisterFont 的底层能力不等于用户字体库入口。 | [font_picker.go:35](../internal/desktop/font_picker.go)；[fonts.go:22](../internal/desktop/fonts.go)；[font_catalog.go](../internal/core/font_catalog.go)；[font_wheel.go](../internal/terminal/font_wheel.go) |
| V060-A04 / SF-A04 | 图标与 Emoji；原34变化 | 存在差异；既有资源策略替换 | 彩色 Emoji、VS15、Powerline 几何和内置 Symbols Nerd Font Mono 回退已有。 | 原版不再内置 Nerd Font，改用本机/导入候选；Desktop 仍固定追加内置回退，缺字条件和选择顺序不同。不是图标能力整体缺失，不因新版删字体而自动删除 Desktop 资源。 | [fonts.go:37](../internal/desktop/fonts.go)；[font.go](../internal/terminal/font.go)；[paint.go](../internal/terminal/paint.go)；[box.go](../internal/terminal/box.go) |
| V060-A05 / SF-A05 | 字符宽度 | 已实现；未定位新增功能差异 | 固定宽度表、创建期歧义双宽开关与宽字占位复制。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [oshell_width.zig](../third_party/ghostty/oshell_width.zig)；[terminal.go](../internal/terminal/internal/vt/terminal.go)；[terminal.go](../internal/terminal/terminal.go) |
| V060-A06 / SF-A06 | 渲染 | 存在差异；平台渲染替换 | D3D11/GL 与软件绘制、降级和重建已有。 | 恢复退避为 1–30 秒、稳定 1 分钟重置，不复制原版 Metal/CG 计时策略；尚缺实际故障画面证据。 | [window.go](../third_party/mygo/ui/window.go)；[renderer_preference.go](../third_party/mygo/ui/renderer_preference.go)；[workspace.go](../internal/desktop/workspace.go) |
| V060-A07 / SF-A07 | 历史容量 | 已实现；未定位新增功能差异 | 3000 默认、六档/500–100000 自定义、>20000 提示及运行期容量调整。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [history_settings.go](../internal/desktop/history_settings.go)；[oshell.go](../internal/terminal/oshell.go) |
| V060-A08 / SF-A08 | 本机时钟；原34变化 | 存在差异；显示粒度差异 | 九位置、字号/颜色/透明度和每秒本机时钟、可见窗口共享计时器已有。 | clockOverlay 在 nodeView 中每个 terminal.View 上调用；原版 0.6.0 每工作区窗口只挂一个时钟。共享计时器不等于每窗口只显示一次。 | [workspace.go:591](../internal/desktop/workspace.go)；[clock.go:79](../internal/desktop/clock.go) |
| V060-A09 / SF-A09 | 未读状态 | 已实现；未定位新增功能差异 | 跨块输出活动解析、BEL/正文未读及观察标签清除。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [output_activity.go](../internal/core/output_activity.go)；[group_unread.go](../internal/desktop/group_unread.go)；[app.go](../internal/desktop/app.go) |
| V060-A10 / SF-A10 | 生命周期与性能；原34变化 | 存在差异；既有平台回收差异；MCP 队列缺失 | 隐藏降频、有界输出、关闭资源释放、所有窗口无标签后延迟 Go 内存回收与诊断已有。 | 回收策略保持已有平台替换；未有 MCP 在途请求预算、审计写队列及回放队列。运行诊断助手不能替代这些新增生命周期。 | [idle_memory.go:42](../internal/desktop/idle_memory.go)；[terminal.go](../internal/terminal/terminal.go)；[lifecycle_diagnostics.go](../internal/desktop/lifecycle_diagnostics.go) |

### 外部互通与迁移

| 编号 / 旧编号 | 功能与重点 | 实现状态 / 性质 | Desktop 当前实现 | 差异、缺失与边界 | 当前源码入口 |
| --- | --- | --- | --- | --- | --- |
| V060-E01 / SF-E01 | OShell 原生归档；原34变化 | 存在差异；字体归档附件缺失 | 原生归档版本、选择范围、密码重绑定、严格校验和事务应用已有。 | 没有默认关闭的导出字体/恢复字体选项，也没有 <JSON>.assets/fonts 明文旁路目录处理；单独导入会话 JSON 不会恢复原版字体。不是共享 v2 字段缺失。 | [archive.go:14](../internal/desktop/archive.go)；[archive.go:51](../internal/desktop/archive.go)；[portable_archive.go:122](../internal/core/portable_archive.go) |
| V060-E02 / SF-E02 | 归档重绑定 | 已实现；未定位新增功能差异 | 导入新 UUID、代理/引用映射、同名副本及密码重加密。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [portable_archive.go](../internal/core/portable_archive.go)；[archive.go](../internal/desktop/archive.go)；[credential_edit.go](../internal/core/credential_edit.go) |
| V060-E03 / SF-E03 | SecureCRT 导入 | 已实现；未定位新增功能差异 | SecureCRT XML/INI/目录导入、预览以及物理空目录保留已有。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [third_party.go](../internal/core/third_party.go)；[archive.go](../internal/desktop/archive.go) |
| V060-E04 / SF-E04 | Xshell 导入 | 已实现；未定位新增功能差异 | XSH/目录/XTS/ZIP 与兼容 RC4+SHA256 密码迁移，预览后事务应用。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [third_party.go](../internal/core/third_party.go)；[archive.go](../internal/desktop/archive.go) |
| V060-E05 / SF-E05 | 补充密码 | 已实现；未定位新增功能差异 | 只补唯一匹配且无密码资料，匹配属性/目录范围，保留 ID。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [third_party.go](../internal/core/third_party.go)；[archive.go](../internal/desktop/archive.go) |
| V060-E06 / SF-E06 | 容器边界 | 已实现；未定位新增功能差异 | ZIP 4000 项/单项 2 MiB、遍历 8000 项及 64 MiB 合计，ZIP64/CRC/头一致性拒绝已有。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [third_party.go](../internal/core/third_party.go) |
| V060-E07 / SF-E07 | 启动器兼容 | 已实现；未定位新增功能差异 | ZOC 参数、FileZilla URL/单站点解析与主程序交接。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [external_launch.go](../internal/core/external_launch.go)；[filezilla_launch.go](../internal/core/filezilla_launch.go)；[launch_entrypoint.go](../internal/core/launch_entrypoint.go) |
| V060-E08 / SF-E08 | 启动交接 | 已实现；未定位新增功能差异 | 8 槽/16 KiB/300s 等待、256 请求去重、解锁后临时页及身份校验。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [launch_ipc.go](../internal/core/launch_ipc.go)；[launch_ipc_windows.go](../internal/core/launch_ipc_windows.go)；[launch_wire.go](../internal/core/launch_wire.go)；[external_launch.go](../internal/desktop/external_launch.go) |
| V060-E09 / SF-E09 | 其他客户端迁移 | 已实现；未定位新增功能差异 | 通用 OShell 归档入口可接收外部转换结果，内置来源仅 SecureCRT/Xshell。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [third_party.go](../internal/core/third_party.go)；[portable_archive.go](../internal/core/portable_archive.go) |

### 更新与分发

| 编号 / 旧编号 | 功能与重点 | 实现状态 / 性质 | Desktop 当前实现 | 差异、缺失与边界 | 当前源码入口 |
| --- | --- | --- | --- | --- | --- |
| V060-U01 / SF-U01 | 更新源设置 | 存在差异；已有增强 | 更新仓库/默认回退/自动开关与本机 token 支持已有。 | 私有仓库 token 是 Desktop 既有增强，不计原版必补项，也不允许绕过 U02/U03 签名链。 | [update_auth.go](../internal/core/update_auth.go)；[update_source.go](../internal/core/update_source.go)；[updater_other.go](../internal/desktop/updater_other.go) |
| V060-U02 / SF-U02 | 更新检查；原34变化 | 存在差异；平台更新通道差异 | 静态元数据、平台资产选择、签名/大小/版本检查、300 秒成功缓存/失败退避及合并请求已有。 | Desktop MetadataURL 为 updates/latest.json，原版 0.6.0 为 updates/macos15.json；资产/签名属于独立 Desktop 产品。不能直接改为原版 DMG 通道，不能把路径不同自动判为更新缺陷；线上部署与真实升级链仍需现行环境证据。 | [update_source.go:32](../internal/core/update_source.go)；[update_release.go](../internal/core/update_release.go)；[update_cache.go](../internal/core/update_cache.go)；[updater_other.go](../internal/desktop/updater_other.go) |
| V060-U03 / SF-U03 | 原版更新链路 | 存在差异；平台更新机制替换 | 生产更新入口已接 Ed25519 签名元数据、大小/SHA256/资产绑定，解包替换前校验。 | Windows/Linux 使用 Desktop 签名便携更新替代 Sparkle；不是旧报告中的仅校验 SHA256 状态。信任根和包命名独立，不能互用原版 DMG。 | [update_release.go](../internal/core/update_release.go)；[update_manifest.go](../internal/core/update_manifest.go)；[updater_other.go](../internal/desktop/updater_other.go) |
| V060-U04 / SF-U04 | 安装生命周期 | 存在差异；已有增强；平台替换 | 安装暂缓、活动确认、替换/重启及 Windows 失败恢复已有。 | 便携更新/回滚替代 Sparkle 生命周期，是既有增强；Linux 原生恢复仍待验，不用 Windows 回归代替。 | [updater_other.go](../internal/desktop/updater_other.go)；[update_portable.go](../internal/core/update_portable.go)；[update_process_windows.go](../internal/core/update_process_windows.go) |
| V060-U05 / SF-U05 | 平台分发；原34变化 | 存在差异；平台分发替换；MCP 辅助程序缺失 | Windows 单文件 EXE/ZIP、Linux 构建入口、终端运行库和内置字体资源已有。 | 原版主线为 macOS 15+ arm64/amd64 DMG；Desktop 的目标平台/包形态和内置字体策略不同。未有 OShell-MCP helper；Windows/Linux 不需要照搬原版 askpass/proxy 等全部辅助程序。 | [build.py](../scripts/build.py)；[library.go](../internal/terminal/library.go)；[fonts.go](../internal/desktop/fonts.go)；[go.mod](../go.mod)；[README.md](../README.md) |
| V060-U06 / SF-U06 | 配置保留 | 已实现；未定位新增功能差异 | 数据目录独立，更新准备/发布仅替换应用文件并保留配置。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [update_portable.go](../internal/core/update_portable.go)；[update_process_windows.go](../internal/core/update_process_windows.go) |
| V060-U07 / SF-U07 | 关于与许可 | 已实现；未定位新增功能差异 | 版本/构建/架构、项目链接、关于及第三方许可展示入口。 | 本轮入口核对与既有修复/验收记录未定位新的实现缺口；实现判定不等于运行验收通过。 | [about.go](../internal/desktop/about.go)；[README.md](../README.md) |

### MCP 控制与审计

| 编号 / 旧编号 | 功能与重点 | 实现状态 / 性质 | Desktop 当前实现 | 差异、缺失与边界 | 当前源码入口 |
| --- | --- | --- | --- | --- | --- |
| V060-G01 / 无（独立新增能力域） | 服务与面板；新增MCP | 缺失；新增 MCP 能力整体缺失 | 已有外部启动交接只服务 ZOC/FileZilla，不是 MCP。 | 没有 MCP 服务开关、控制中心、STDIO helper 或 Unix socket MCP 服务。 | [main.go](../main.go)；[launch_entrypoint.go](../internal/core/launch_entrypoint.go)；[unified_toolbar.go](../internal/desktop/unified_toolbar.go) |
| V060-G02 / 无（独立新增能力域） | 客户端配对与接入配置；新增MCP | 缺失；新增 MCP 能力整体缺失 | SSH Agent 只承担 SSH 认证；不是外部 AI 客户端身份。 | 没有 MCP 客户端配对、一次性凭据展示、撤销或 Codex/OpenCode 配置输出。 | [agent_windows.go](../internal/core/agent_windows.go)；[agent_unix.go](../internal/core/agent_unix.go)；[settings.go](../internal/desktop/settings.go) |
| V060-G03 / 无（独立新增能力域） | 逐终端授权与撤销；新增MCP | 缺失；新增 MCP 能力整体缺失 | 已有 pane/连接身份保护用于应用自身会话和传输。 | 没有客户端—pane UUID—连接代次授权记录及单独/全部撤销入口。 | [app.go](../internal/desktop/app.go)；[image_paste.go](../internal/desktop/image_paste.go) |
| V060-G04 / 无（独立新增能力域） | 权限与逐命令审批；新增MCP | 缺失；新增 MCP 能力整体缺失 | 密码/主机认证队列及粘贴确认不是命令权限策略。 | 没有 ask/assist/full 三档 Agent 权限和逐条命令审批。 | [authentication.go](../internal/desktop/authentication.go)；[terminal_features.go](../internal/desktop/terminal_features.go) |
| V060-G05 / 无（独立新增能力域） | 临时 Shell 集成；新增MCP | 缺失；新增 MCP 能力整体缺失 | 已有 OSC777 主机集成、OSC133 提示符语义，目的不同。 | 没有随机令牌 OSC779 临时 Bash/Zsh 集成及命令超时状态。 | [shell_integration.go](../internal/terminal/shell_integration.go)；[host_identity.go](../internal/desktop/host_identity.go) |
| V060-G06 / 无（独立新增能力域） | 授权会话查询；新增MCP | 缺失；新增 MCP 能力整体缺失 | 内部会话列表、终端屏幕和主机详情只供本机 UI。 | 没有 sessions_list/session_state MCP 工具或按授权过滤的屏幕 API。 | [session_catalog.go](../internal/desktop/session_catalog.go)；[oshell.go](../internal/terminal/oshell.go)；[tab_details.go](../internal/desktop/tab_details.go) |
| V060-G07 / 无（独立新增能力域） | 任务取得与释放；新增MCP | 缺失；新增 MCP 能力整体缺失 | 已有托管发送/文件传输占用不绑定外部客户端任务。 | 没有 task_start/task_end/task_stop 或外部 Agent 单终端任务所有权。 | [transfer.go](../internal/terminal/transfer.go)；[operators.go](../internal/desktop/operators.go)；[transport.go](../internal/core/transport.go) |
| V060-G08 / 无（独立新增能力域） | PTY 命令执行与收据；新增MCP | 缺失；新增 MCP 能力整体缺失 | 快速发送/普通键盘确实可写 PTY，但不是可重试的 Agent 命令 API。 | 没有 command_submit、审批/持久收据、幂等键及先持久化后 PTY 写入的 Agent 流程。 | [operators.go](../internal/desktop/operators.go)；[terminal.go](../internal/terminal/terminal.go) |
| V060-G09 / 无（独立新增能力域） | 执行输出增量读取；新增MCP | 缺失；新增 MCP 能力整体缺失 | 文本日志/历史搜索不具备命令输出归属、游标与敏感期权限。 | 没有 operation_read 字节游标、操作状态或授权输出读取。 | [output_log.go](../internal/terminal/output_log.go)；[search.go](../internal/terminal/search.go) |
| V060-G10 / 无（独立新增能力域） | 人工接管与自动输入仲裁；新增MCP | 缺失；新增 MCP 能力整体缺失 | 现有 Ctrl+C、同步输入停止、传输取消仍属于原交互路径。 | 没有 MCP 人工接管、双 Esc 0.6 秒及所有自动写入统一权限闸门。 | [keys.go](../internal/terminal/keys.go)；[transfer.go](../internal/terminal/transfer.go)；[sync_input_draft.go](../internal/desktop/sync_input_draft.go) |
| V060-G11 / 无（独立新增能力域） | 敏感输入；新增MCP | 缺失；新增 MCP 能力整体缺失 | 已有密码编辑保护、认证对话框和诊断字段脱敏不等于终端审计脱敏。 | 没有 Agent 敏感输入面板、审计输入省略和后续跨块同值脱敏。 | [password_input.go](../internal/desktop/password_input.go)；[authentication.go](../internal/desktop/authentication.go)；[log.go](../internal/diagnostics/log.go) |
| V060-G12 / 无（独立新增能力域） | 加密审计与记录生命周期；新增MCP | 缺失；新增 MCP 能力整体缺失 | 普通终端日志与诊断日志是不同格式；没有用它们冒充审计。 | 没有授权起点屏幕基线、加密事件链、输入/输出/display/resize 审计和封口生命周期。 | [output_log.go](../internal/terminal/output_log.go)；[log.go](../internal/diagnostics/log.go)；[lifecycle_diagnostics.go](../internal/desktop/lifecycle_diagnostics.go) |
| V060-G13 / 无（独立新增能力域） | 存档目录与内容检索；新增MCP | 缺失；新增 MCP 能力整体缺失 | 终端全文搜索、快速发送历史、传输记录不能替代审计索引。 | 没有加密审计目录、按客户端/任务过滤和记录内命令/输出搜索。 | [search.go](../internal/terminal/search.go)；[quick_history.go](../internal/desktop/quick_history.go)；[transfer_records.go](../internal/desktop/transfer_records.go) |
| V060-G14 / 无（独立新增能力域） | 只读回放；新增MCP | 缺失；新增 MCP 能力整体缺失 | 已有截图/测试快照及历史滚动不是可重建事件流的产品回放。 | 没有生产只读终端回放窗口、播放/倍速/跳空闲/时间定位。 | [oshell.go](../internal/terminal/oshell.go)；[features.go](../internal/desktop/features.go)；[terminal.go](../internal/terminal/terminal.go) |
| V060-G15 / 无（独立新增能力域） | 加密导出与手工清理；新增MCP | 缺失；新增 MCP 能力整体缺失 | 会话加密归档、诊断导出是别的数据产品；不能算审计导出。 | 没有已封口审计导出、记录派生密钥密码封装或审计删除/容量提醒。 | [archive.go](../internal/desktop/archive.go)；[portable_archive.go](../internal/core/portable_archive.go)；[sync_diagnostics.go](../internal/desktop/sync_diagnostics.go) |

## 验证与后续使用

1. 本轮验证文档的 176 个唯一编号、原 161 项映射、34+15 重点集合、源码入口存在及计数一致性；这些检查不是产品运行验收。
2. 保留旧清单中的真实窗口、Linux/macOS、网络/发行及 Swift 双端证据缺口；CSV 对每行列旧状态与有限核验步骤。旧“已通过”不覆盖后来追加的真人操作待验说明，例如 S04 双击目录。
3. “缺少实现”和“缺少环境证据”分开：MCP/rsync/字体库属于实现缺口；已存在的 GPU、IME、物理滚轮等缺运行证据不改写为整项缺失。
4. 本表不分配 P1/P2/P3，不默认实施 0.6.0 全部新增能力。下一步若做处置分类，应先区分旧范围内缺陷、已授权差异、平台替换及待决定的新版本能力，且沿用本表编号。
5. 本次不构建或更新便携包；不修改会话/凭据/共享合同。参考源码继续使用固定 0.6.0 快照，本轮不复制它、不生成临时 EXE/ZIP。
