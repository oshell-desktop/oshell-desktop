# Desktop 差异分类与修复优先级清单

当前全量验收结论见 [原 161 项最终验收汇总与收尾清单](oshell-desktop-final-acceptance.md)（[逐项 CSV](oshell-desktop-final-acceptance.csv)）。该汇总合并 P1/P2/P3、保留项验证、[C03/I22 缺口修复](oshell-desktop-gap-closure-validation.md)、[Windows 37 项补证记录](oshell-desktop-windows-evidence-validation.md)及[CI 修复记录](oshell-desktop-ci-repair-validation.md)，区分实际缺口、缺环境及尚缺验收证据；本表继续保留最初 43 项分类与分批处理历史，不用某个修复用例通过替代整项验收。

整理日期：2026-10-25。仅处理[上轮差异清单](oshell-desktop-gap-analysis.md)中的 **43 个“存在差异”SF 条目**，范围仍为 [OShell 固定功能基线](oshell-feature-inventory.md) `OShell-0.2.92-scope-1` 的 13 域、161 项，原版提交 `3f4efd9c70c82323f4e20180117c928227000580`。原版依据见[源码分析记录](oshell-source-analysis.md)，Desktop 证据列至各项生产源码入口。

按主要处置归类：**10 项功能缺口、20 项行为差异、7 项平台替换、6 项已有增强**。前两类进入修复清单，共 30 个 SF，其中 SF-M05 随 SF-P08 修复、单独联验；不是 30 个独立开发任务。其余 13 项列出可保留的机制或增强及条件，不能直接标记已验收。

本清单初始分类依据上轮静态核查并补查关键调用链形成。2026-10-25 已按原范围落实九项 P1 修复，完成本机回归并更新固定 Windows 单文件便携包；逐项结果、证据与保留限制见 [P1 修复验证记录](oshell-desktop-p1-validation.md)。Desktop 对象仍是包含未提交修改的当前工作树，HEAD `62a620541680b687cedf89f8e48adcdb77576e33` 不是完整内容快照。未提交、推送或触发 Actions。[筛选用 CSV](oshell-desktop-remediation-plan.csv)的验收状态同步更新，原分类与优先级不变。

## 分类与优先级口径

每个 SF 只按主要处置计数一次。表内“同时涉及”保留混合性质，例如 SF-P05 既有需要修正的禁存规则，也有可保留的 FTP/SFTP 保存增强。分类不能把已有明确缺口改名排除，也不能因一个增强就判定整项通过。

| 主分类 | SF 数 | 处理规则 |
| --- | ---: | --- |
| 功能缺口 | 10 | 补齐固定条目已要求但生产入口缺少的子能力；不是新增功能编号 |
| 行为差异 | 20 | 修正输入、状态、限制或失败处理与固定预期的偏差；已存在的合理保护保留 |
| 平台替换 | 7 | 可保留实现机制差异，但必须满足固定用户结果与明确保留条件 |
| 已有增强 | 6 | 记录有意差异，不为逐字复刻删除合理增强，也不把增强变成新增复刻义务 |
| 合计 | 43 | 不改变原 161 项及上轮状态统计，不计算完成率 |

| 优先级 | SF 数 | 先后依据 |
| --- | ---: | --- |
| P1 | 9 | 优先处理凭据使用/持久化、代理边界、配置复制失效、导入容器限制及更新信任链；U02 是 U03 生产链路的同批依赖 |
| P2 | 18 | 补齐主要操作路径，修正跨窗口、搜索、日志、图片、传输、快捷操作和安装生命周期；含 M05 关联验收 |
| P3 | 3 | 修正影响较局部的控件、标题和帮助入口 |
| 保留并验收 | 13 | 不安排为了相同实现机制而重写；保留条件未证实的仍待验，出现具体违反基线的结果则在原 SF 内修正 |

优先级不是风险事故结论，也不是工期承诺。以下验收输入均使用合成数据；每行是本次已知差异的有限关闭条件，与原 SF 已有验收要求共同使用。未要求穷举参数组合、所有 OpenSSH 语法或最新终端协议。

## P1 优先修复

以下主表保留最初发现及有限验收要求，不再把其“仍存在”措辞视为当前缺陷状态。当前结果如下，详细测试入口见 [验证记录](oshell-desktop-p1-validation.md)：

| 固定条目 | 本轮状态 |
| --- | --- |
| SF-I27 | OSC52 生产入口禁用已修复；本机有限用例及终端回归通过。 |
| SF-P07 | 固定提示、单问题限制及混合挑战手动规则已修复；核心/桌面回归通过。 |
| SF-P05 | 外部来源禁存与清缓存跳过保存已修复；生产连接、身份/库代次回归通过。SF-P08 仍留 P2。 |
| SF-C06 | SOCKS5 禁止降级与 HTTP 16 KiB 响应头上限已修复；本地代理用例通过。 |
| SF-C03 | aes192-cbc/aes256-cbc 已补齐；固定白名单、RSA 约束、默认拒绝/开启成功及原生 OpenSSH 8 组互通通过，见本轮缺口修复记录。 |
| SF-S07 | 配置快照绑定剪贴板代次已修复；Windows 原生用例通过，Linux/macOS 原生待验。 |
| SF-E06 | ZIP/目录数量、展开长度、头一致性及 CRC 校验已修复；固定边界样例通过。SF-E03 仍留 P2。 |
| SF-U02 | 静态签名元数据接入公开生产入口；缓存/合并/拒绝路径通过，未部署线上元数据。 |
| SF-U03 | 内置用户授权生成的 Ed25519 公钥；实际 ZIP 的签名验证/准备及篡改拒绝通过，SF-U04 安装联验仍留 P2。 |

| 固定条目与分类 | 必须处理或可保留的差异 | 限定边界 | 有限验收与依赖 | Desktop 源码入口 |
| --- | --- | --- | --- | --- |
| **SF-I27 宿主限制**；行为差异 | OSC52 写回调仍进入系统剪贴板。补齐应用层禁用闸门，写入不改剪贴板、查询不回复。 | 保留用户主动复制/粘贴；不增加远端剪贴板授权产品入口。 | 预置合成剪贴板文本，分别发送有效 OSC52 写入、查询及分块序列；剪贴板不变、终端无查询回复，手动复制仍有效。 **依赖：**独立。 | [terminal.go](../internal/terminal/terminal.go)；[view.go](../internal/terminal/view.go)；[terminal.go](../internal/terminal/internal/vt/terminal.go) |
| **SF-P07 一次性凭据**；行为差异 | 自动填入与保存判定收窄到固定原版的三个身份化密码提示，去掉通用 password/中文提示的自动识别；多问题场景不凭唯一隐藏项扩大目标密码自动使用范围。 | 未知挑战保留手动回答；代理、私钥与其他挑战维持独立路径，不建设通用挑战识别器。 | 三个固定提示及错误身份、通用提示、两问题混合挑战各测一次；只有目标提示获一次自动凭据，重复失败不再自动填，一次性值不入库。 **依赖：**与 SF-P05 联验。 | [password_prompt.go](../internal/core/password_prompt.go)；[transport.go](../internal/core/transport.go) |
| **SF-P05 成功后保存**；行为差异；同时涉及：已有增强 | 补核发现：app.go 的 allowPasswordSave 未检查 ExternalLaunch，认证弹窗直接据此提供保存选项，成功回调可进入保存。补齐外部/一次性来源的禁存闸门，并检查主密码缓存不可用时跳过保存的规则。 | FTP/SFTP 成功后保存属于已有增强，可保留普通入口中明确选择后的保存；不能放宽外部来源、身份绑定、配置/密码库代次和成功后保存约束。 | 外部 SSH 无初始密码后手动认证成功、一次性密码失败后再次询问、普通 SSH 成功/失败各一例；前两例不出现可用保存选项且库不变。普通 SFTP/FTP 显式保存各一例，失败/取消不写；身份或代次改变拒绝回写，主密码缓存缺失不嵌套询问。 **依赖：**与 SF-P07、SF-P08 联验。 | [authenticated_password.go](../internal/desktop/authenticated_password.go)；[app.go](../internal/desktop/app.go)；[authentication.go](../internal/desktop/authentication.go)；[external_launch.go](../internal/desktop/external_launch.go) |
| **SF-C06 代理类型**；行为差异 | SOCKS5 配置凭据时只宣告密码认证，禁止服务器选择无认证回退；HTTP CONNECT 响应头恢复 16 KiB 硬上限。 | 保留现有四种代理；不增加认证协议。 | 本地合成代理记录无凭据/有凭据两种握手，后者选择无认证须失败；HTTP 头在上限内成功、超过上限拒绝且关闭连接，不拨目标直连。 **依赖：**独立。 | [proxy.go](../internal/core/proxy.go) |
| **SF-C03 旧 SSH 兼容**；行为差异 | 旧 SSH 兼容开关改为固定原版逐项算法表与当前库支持能力的明确映射，不再无条件合并依赖的全部 InsecureAlgorithms。 | 缺少的固定算法明确报告并保留缺口；不把扩大不安全集合或追随依赖新增算法当兼容修复。 | 开关关/开分别核对协商集合，固定表每项有映射或明确不支持记录；以一个旧算法端点和一个非白名单旧算法端点验证仅前者在开启时可用，默认集合不被污染。 **依赖：**独立。 | [transport.go](../internal/core/transport.go)；[go.mod](../go.mod) |
| **SF-S07 配置复制**；行为差异 | 复制配置绑定系统剪贴板变化代次或等价可靠事件；令牌被覆盖再恢复也不能重用内存快照。 | 保留进程/数据目录、密码库 revision 和代理身份保护；不将快照或密码写系统剪贴板，不追加跨设备复制。 | 同进程跨窗粘贴成功且新身份可解密；复制后用其他内容覆盖再恢复旧令牌应拒绝；配置/密码库代次改变拒绝且不半写入。目标平台无法可靠追踪时明确待验。 **依赖：**独立。 | [manager_clipboard.go](../internal/desktop/manager_clipboard.go)；[manager_actions.go](../internal/desktop/manager_actions.go) |
| **SF-E06 容器边界**；行为差异；同时涉及：功能缺口 | 恢复 ZIP ≤4000 项、目录遍历 ≤8000 项、每项展开 ≤2 MiB；补齐 ZIP64 拒绝、中央/本地头一致性、已读取目录/XSH 的长度与 CRC 校验。 | 保留内存读取、64 MiB 输入/展开合计、路径与特殊文件保护；只校验固定 stored/deflate 容器，不追加 XTS 或更多压缩格式。 | 固定合成样本集：合法 stored/deflate 各一包；三项数量/大小上限各含边界及超限；ZIP64、头不一致、目录 CRC 错各一包；已有路径/加密/分卷/未知算法/特殊文件拒绝用例各保留一例。失败不落盘、不改原文件。 **依赖：**先于 SF-E03 空目录联验。 | [third_party.go](../internal/core/third_party.go) |
| **SF-U02 更新检查**；行为差异；同时涉及：平台替换 | 复刻更新入口从 GitHub API 查询改为固定静态元数据路径并接到生产调用链，恢复客户端不调用 GitHub API 的约束；保留同源成功 300 秒/失败至少 60 秒缓存及重复预取合并。 | Windows/Linux 资产映射可替换苹果架构选择；现有私有仓库能力另记 SF-U01，不可用它把复刻入口的 API 差异直接排除。只覆盖现有分发目标，不新建更新服务产品。 | 本地静态服务核对有/无更新、唯一资产/长度/架构不符、JSON >1 MiB、feed >32 KiB、重定向拒绝；模拟时钟核对两类缓存及并发合并；复刻入口无 API 请求。 **依赖：**与 SF-U03 同批设计；不触发发布。 | [update_release.go](../internal/core/update_release.go)；[update_cache.go](../internal/core/update_cache.go)；[updater_other.go](../internal/desktop/updater_other.go) |
| **SF-U03 原版更新链路**；功能缺口；同时涉及：平台替换 | Windows/Linux 生产 ReleaseUpdate 仅校验 SHA256，未接已有签名实现。接入内置可信公钥的等价 Ed25519 元数据/包验证，在解包或替换之前完成来源认证与资产绑定。 | 无需移植 Sparkle；可复用现有 signed manifest，但必须证明生产入口可达且覆盖固定验证结果。不得以同源校验和代替验签，不增加自动下载安装。 | 仅用合成本地元数据、密钥和临时包：合法签名成功；坏签名、未知公钥、缺签名、包被篡改、长度或资产绑定不符全部在解包前拒绝；未经确认不下载，暂缓后可继续。 **依赖：**依赖 SF-U02 元数据链路；SF-U04 安装联验。 | [update_release.go](../internal/core/update_release.go)；[update_manifest.go](../internal/core/update_manifest.go)；[updater_other.go](../internal/desktop/updater_other.go) |

SF-P05 最初补核的调用链仍归原条目；本轮已在 `allowPasswordSave`、认证弹窗及成功回调接入外部来源禁存，并以本地真实 SSH 连接覆盖无初始密码及失败后重试。因此该行为差异已经修复，普通 FTP/SFTP 显式保存增强继续保留；不能将两者混为整项平台豁免。

## P2 主要功能与行为修复

2026-10-25 已逐项处理本批 18 个 P2 条目，下面原差异描述作为历史输入保留。当前结果见 [P2 修复验证记录](oshell-desktop-p2-validation.md)。**SF-I22 后续已补齐 Windows 命名共享内存，仍需 Linux 四介质、原版介质/GPU 对照与实际窗口 GPU 补验；详见[缺口修复记录](oshell-desktop-gap-closure-validation.md)。** 不新增 SF，不把未验收写成通过。

| 固定条目 | 本轮结果 |
| --- | --- |
| SF-P08 | 启动锁与缓存拆分完成；已连接输入/图片、凭据与同步门禁回归通过。 |
| SF-M05 | 清缓存后实际图片路径发送及目标捕获验证通过；关闭/身份变化/启动锁仍拒绝。 |
| SF-S05 | 源名/别名及 4096 字节、32 词边界提示通过。 |
| SF-S14 | 目录会话/引用 UI 拖回根、排序、A→B→A 代次、外窗/无快照拒绝及边缘滚动通过。 |
| SF-W01 | Windows 原生两窗/零窗口存活/外部恢复/显式退出通过；Linux 待验收。 |
| SF-W02 | 同窗跨可见组循环与组内编号验证通过。 |
| SF-I05 | 按格而非 UTF-8 字节计数通过；保留 10 ms/约 1 s 检查预算差异。 |
| SF-I06 | 过滤异步日志、4 MiB 积压、慢写与失败关闭通过。 |
| SF-I10 | 8192 字节编辑/解析及生产 PTY 字面 argv 验证通过。 |
| SF-I20 | BEL 宿主回调、五态进度/钳位/15 秒清理通过；其他平台原生反馈待验收。 |
| SF-I22 | Windows 四介质实现及本机像素/清理通过，Win32 最后句柄关闭生命周期明确；Linux 四介质、原版介质/GPU 对照与实际窗口 GPU 联验待验收。 |
| SF-M01 | Windows 原生 PNG/TIFF/JPEG/DIB 与应用解码回退通过；macOS/Linux 原生待验收。 |
| SF-F04 | 32 层/10000 项、真实 SFTP 顶层链接下载/子链接跳过/本地上传拒绝通过。 |
| SF-O02 | 50 条/64 KiB/8 KiB 历史、草稿恢复、成败与回车重置通过。 |
| SF-O07 | 33 条保留/前 32 渲染、首规则优先、UTF-16 行预算通过。 |
| SF-O08 | Windows 已知冲突与不可读提示、内部冲突回归通过；Linux/macOS 系统检查待验收。 |
| SF-E03 | SecureCRT 单级/多级空目录、混合会话预览数据与最终合并通过。 |
| SF-U04 | 无活动免确认、模态延后一次恢复、取消保留、真实替换/重启/回滚回归通过。 |

| 固定条目与分类 | 必须处理或可保留的差异 | 限定边界 | 有限验收与依赖 | Desktop 源码入口 |
| --- | --- | --- | --- | --- |
| **SF-P08 安全状态保护**；行为差异 | 区分清除主密码缓存与启动/整个工作区锁定；缓存清除后已授权、已连接终端继续普通输入及图片发送，后续需要秘密的操作重新走保护流程。 | 保留启动锁、凭据读写和同步门禁；不能简单删除所有 passwordLocked 检查。确需秘密时遵守跳过保存/显式解锁规则。 | 建立终端后清缓存：普通输入可达原连接，图片按 SF-M05 发送；新连接解密、保存密码、同步保持受控。启动锁仍阻止未授权访问，解锁失败不改库。 **依赖：**主修复归本项，SF-M05 只做关联验收；联验 SF-P05。 | [configuration_unlock.go](../internal/desktop/configuration_unlock.go)；[application.go](../internal/desktop/application.go)；[security_view.go](../internal/desktop/security_view.go)；[image_paste.go](../internal/desktop/image_paste.go) |
| **SF-M05 发送规则**；行为差异 | 清缓存后的图片发送被 passwordLocked 一并阻止，随 SF-P08 的状态拆分修正；本项不另建重复修复。 | 保留 pane/term/conn、owner、宿主代次和工具上下文检查；完整工作区锁定不得被当成缓存清除绕过。 | 原目标连接保持有效时清缓存后可插入图片路径；准备期间切目标、关闭原分屏、重连或进入完整锁定各一例，均不误发到其他目标。 **依赖：**修复由 SF-P08 承担，本项 P2 联验。 | [image_paste.go](../internal/desktop/image_paste.go)；[paste_context.go](../internal/terminal/paste_context.go) |
| **SF-S05 搜索**；行为差异；同时涉及：功能缺口 | 侧栏展示搜索超限/无效错误，避免伪装成无结果；引用同时索引显示名与源会话名称，保留多词 AND 和协议/目录过滤。 | 不增加模糊搜索、拼音索引或全局搜索产品入口。 | 4096/4097 UTF-8 字节、32/33 词各核对边界提示；以不同源名和别名建立引用，两种名称均能搜到，两个词 AND 仍正确，清空错误后恢复列表。 **依赖：**独立。 | [session_search.go](../internal/core/session_search.go)；[manager.go](../internal/desktop/manager.go)；[sidebar.go](../internal/desktop/sidebar.go) |
| **SF-S14 链接栏**；功能缺口；同时涉及：行为差异 | 补齐目录/全部链接菜单中会话或引用拖回根栏的可达入口；所有拖动载荷绑定当前配置代次并拒绝过期拖动。 | 可更换菜单呈现机制来实现拖动；仍只允许本链接栏，不追加跨窗/外部拖入或文件夹嵌套拖动。 | 目录内会话与引用各拖回根一次，根排序/移入目录保存后重开一致；拖动期间配置改变后拒绝，外窗/外部载荷拒绝，边缘滚动可用。 **依赖：**独立。 | [workspace.go](../internal/desktop/workspace.go)；[link_menu.go](../internal/desktop/link_menu.go) |
| **SF-W01 多窗口**；行为差异；同时涉及：平台替换 | Windows/Linux 关闭最后窗口不应由框架默认 Quit；接好零窗口存活和已有激活/外部启动入口的新窗恢复。 | 最后窗口不退出是固定用户结果，不能仅按平台惯例排除；不新增托盘、后台服务或开机自启。 | 关闭两个窗口的最后一个后进程仍存活；从已有激活/外部连接入口恢复一窗且不重复进程；显式退出仍能结束并清理资源。 **依赖：**与 SF-A10 联验资源释放。 | [application.go](../internal/desktop/application.go)；[layout.go](../internal/desktop/layout.go)；[app.go](../third_party/mygo/app.go) |
| **SF-W02 标签导航**；行为差异 | 前后标签导航遍历当前窗口全部非隐藏标签，不局限当前组；编号跳转继续限定当前组。 | 保留最近使用切换及隐藏组规则，不增加跨窗导航。 | 同窗两组各两标签，前后导航跨组循环，隐藏标签跳过；编号跳转不跨组，另一窗口当前标签不变。 **依赖：**独立。 | [groups.go](../internal/desktop/groups.go)；[shortcuts.go](../internal/desktop/shortcuts.go) |
| **SF-I05 终端搜索**；行为差异；同时涉及：平台替换 | 正则软换行扫描上限从 65536 字节纠正为 65536 格；记录当前 10 ms 单次/约 1 s 总扫描与原版检查预算的差别。 | 不同正则引擎可保留有界执行策略，不机械追求相同墙钟耗时；若固定合法样例因此遗漏或卡住，仍归本项修正。 | 构造多字节/宽字符软换行文本在 65536 格内且超过 65536 字节，仍能匹配导航；超格停止受控；一个高耗时正则有错误/超时反馈，1000+ 后仍可导航，关闭搜索释放状态。 **依赖：**独立。 | [search.go](../internal/terminal/search.go)；[search_navigation.go](../internal/desktop/search_navigation.go) |
| **SF-I06 输出保留**；功能缺口；同时涉及：行为差异 | 原始 ANSI 同步写日志改为基线的控制序列过滤与异步写入，并补齐 4 MiB 待写队列超限停止及可见错误。 | 历史容量和结束保屏沿用既有路径；不新增原始/纯文本格式选择、日志轮转或日志平台。 | 同一合成输出混合文本、颜色和 OSC，日志只有预期可读内容；慢写端累计越限时停止记录并提示，终端输入输出不被写盘阻塞；写失败/关闭释放文件。 **依赖：**独立。 | [features.go](../internal/desktop/features.go)；[oshell.go](../internal/terminal/oshell.go)；[session_fallback.go](../internal/terminal/session_fallback.go) |
| **SF-I10 提示符编辑**；行为差异 | 编辑和执行统一 8192 UTF-8 字节上限；引号/转义解析后的独立 Shell 符号参数按字面 argv 传给允许工具，不因含符号一概拒绝。 | 始终直接执行白名单工具，不启动 Shell，不新增管道、变量展开、脚本或文件补全。 | 8192 字节可执行、追加完整多字节字符越限被拒且不破坏 UTF-8；带引号的独立管道符和含符号参数传到合成允许工具的 argv，未执行任何第二条命令；取消保编辑规则。 **依赖：**独立。 | [prompt_editor.go](../internal/terminal/prompt_editor.go)；[local_tools.go](../internal/core/local_tools.go) |
| **SF-I20 查询与状态**；功能缺口 | 把已有底层 BEL 与 OSC9;4 事件接到生产宿主：可感知响铃、五状态进度反馈及 15 秒超时清理；未读标记不能代替响铃。 | 复用当前终端 UI，不新增通知中心或任务系统。 | 经真实生产构造入口输入 BEL、OSC9;4 五状态、越界数值；检查宿主响铃、数值钳位和 15 秒消失；关闭/重连清理，其他分屏不串状态。 **依赖：**独立。 | [terminal.go](../internal/terminal/terminal.go)；[terminal.go](../internal/terminal/internal/vt/terminal.go)；[app.go](../internal/desktop/app.go) |
| **SF-I22 终端图片**；功能缺口；同时涉及：平台替换 | 明确补齐目标平台 Kitty 文件/临时文件介质的等价生产入口；Linux ABI45+ 与 Windows 禁用路径分别列证。共享内存需要平台介质适配证据，不能用直接图片可显示抵消该子能力缺口。 | 只使用固定 Kitty 介质和参数。若原 POSIX 介质在目标系统确实无等价机制，记录支持/拒绝、源码依据与影响，保留未闭合状态；不偷偷排除，也不擅建 Windows 新协议、RPC 或服务。 | Linux/Windows 分别用同一小图测试直接、文件、临时文件、共享内存；支持项必须实际绘制并正确清理临时资源，不支持项明确失败不能假回 OK。补一个不存在/越界介质拒绝用例；固定范围内路径保护继续有效。 **依赖：**平台介质待实测；无法等价的部分待范围决议，不阻断其他项。 | [terminal.go](../internal/terminal/internal/vt/terminal.go)；[graphics.go](../internal/terminal/internal/vt/graphics.go)；[kitty_local.go](../internal/terminal/internal/vt/kitty_local.go)；[graphics.go](../internal/terminal/graphics.go) |
| **SF-M01 识别与转换**；功能缺口；同时涉及：平台替换 | 核对原生剪贴板 PNG→TIFF→JPEG 的完整读取回退；应用仅读 PNG，先验证框架已做的 TIFF/DIB 转换，再只补真正缺少的表示入口。 | 文件 URL 优先、单文件和混合选择拒绝不变；不扩大现有图片格式清单，不重复实现平台已提供的转换。 | 纯 PNG、纯 TIFF、纯 JPEG、Windows DIB 四种合成表示按适用平台验收；前三者得到有效 PNG，DIB 走现有转换；含文件 URL 时遵循文件入口优先，混合文件拒绝。若框架已覆盖某项，只补证据。 **依赖：**先复现平台回退，再最小补接。 | [image_paste.go](../internal/desktop/image_paste.go)；[clipboard_image.go](../internal/core/clipboard_image.go)；[clipboard.go](../third_party/mygo/internal/darwin/clipboard.go)；[clipboard.go](../third_party/mygo/internal/windows/clipboard.go) |
| **SF-F04 文件操作**；行为差异；同时涉及：已有增强 | SFTP/FTP 递归深度恢复 32；直接 SFTP 链接下载依 STAT 跟随，递归中的已识别子链接跳过而不是停止整批。 | 保留本地上传拒绝链接、非空目录不递归删除；10000 项资源上限可作为已有增强保留并明确超限错误，不扩为任意递归跟链。 | 深度 32/33 边界；顶层远端文件链接可下载；目录含正常项与子链接时正常项完成、子链接跳过；本地链接上传拒绝；10000 项超限有明确结果且不误删既有文件。 **依赖：**独立。 | [transfer.go](../internal/core/transfer.go)；[ftp.go](../internal/core/ftp.go)；[file_actions.go](../internal/desktop/file_actions.go) |
| **SF-O02 快速发送栏**；功能缺口；同时涉及：行为差异 | 补齐上下键历史与原草稿恢复；历史恢复 50 条/合计 64 KiB/单条超过 8 KiB 不记录；成功发送后重置追加回车选项。 | 保留单行 64 KiB、多行转撰写窗、失败留草稿；历史仅窗口内存，不增加跨窗持久历史或定时发送。 | 依次形成 51 条历史以及总量越限样例，按既定淘汰规则限额；8 KiB/超限单条分别入史/不入史；上下键回原草稿；成功清稿并重置回车、失败保留，IME 回车不误发。 **依赖：**独立。 | [operators.go](../internal/desktop/operators.go)；[workspace.go](../internal/desktop/workspace.go) |
| **SF-O07 高亮规则**；行为差异；同时涉及：已有增强 | 恢复重叠首规则优先、每显示行 4096 UTF-16 及每集前 32 条参与渲染；不能把超过 32 条直接拒绝保存当等价。 | 背景、粗体和 8 位色可保留为已有增强，不覆盖首规则优先；渲染截取不应丢弃持久模型中的其余规则，保持两端字段合同。 | 两条重叠规则取首条；33 条配置可按现有模型保留但仅前 32 条生效；用非 BMP 字符验证 UTF-16 上限；256 匹配/预算样例受控，复制和日志文本不变。 **依赖：**涉及持久化时执行共享配置兼容回归。 | [highlights.go](../internal/desktop/highlights.go)；[terminal_features.go](../internal/desktop/terminal_features.go)；[oshell.go](../internal/terminal/oshell.go)；[paint.go](../internal/terminal/paint.go) |
| **SF-O08 快捷键配置**；功能缺口；同时涉及：平台替换 | 补齐变更绑定的已知系统快捷键冲突检查，以及系统状态不可读时的明确提示；内部重复仍阻止保存。 | Command→Ctrl 和物理键映射可保留；只覆盖目标系统可确定的保留/已知冲突，不承诺枚举所有第三方热键，也不新增全局热键管理器。 | 内部重复、一个目标系统已知冲突、系统信息不可读、正常物理键绑定各一例；不可读不能显示无冲突，取消不保存，成功后菜单提示更新且录入不泄漏到终端。 **依赖：**按 Windows/Linux 可用机制分别验收。 | [shortcuts.go](../internal/desktop/shortcuts.go)；[shortcuts.go](../internal/core/shortcuts.go)；[shortcut_capture_other.go](../internal/desktop/shortcut_capture_other.go) |
| **SF-E03 SecureCRT 导入**；功能缺口 | SecureCRT 目录导入遍历记录物理空目录，并在预览/应用中保留到导入目标目录下。 | 只保留输入原有目录；不跟随符号链接、不新增导入格式，计数服从 SF-E06。 | 单空目录、两级空目录、空目录与有效会话混合各一例，预览和导入后树一致；取消不改配置，目录符号链接不跟随，越限按 E06 反馈。 **依赖：**依赖 SF-E06 限额与遍历校验。 | [third_party.go](../internal/core/third_party.go)；[archive.go](../internal/desktop/archive.go) |
| **SF-U04 安装生命周期**；行为差异；同时涉及：已有增强 | 无活动终端/文件操作时去掉额外关闭确认；有模态时保存待安装回调并在可安装时继续，取消关闭确认保留后续重试机会，中止则清理。 | 保留现有失败回滚机制作为增强；不强制关连接、不自动安装。回滚代码存在不能替代真实恢复验证。 | 无活动/有活动跨窗各一例；模态关闭后只恢复一次，取消确认后会话存活且可重试；临时安装目录模拟替换失败保留可用程序，成功可重启；不替换用户正在使用的正式包。 **依赖：**依赖 SF-U03 验签先完成；桌面生命周期验收。 | [updater_other.go](../internal/desktop/updater_other.go)；[update_portable.go](../internal/core/update_portable.go)；[update_process_windows.go](../internal/core/update_process_windows.go) |

## P3 局部一致性修复

2026-10-25 已完成本批三项代码修复及 Windows 本机有限回归，仍归原 161 个 SF 条目。原差异表保留为历史输入，当前状态如下；详见 [P3 修复验证记录](oshell-desktop-p3-validation.md)。Linux/macOS 原生环境及 Swift 实机对照缺失，明确待验收。P1/P2 先前记录的能力缺口和待验项不因本批通过而关闭。

| 固定条目 | 本轮结果 |
| --- | --- |
| SF-S02 | SFTP 禁用空闲发送开关、间隔、字符串；SSH 关闭开关时也禁用两个输入。往返切换保留草稿，取消不改变配置，SFTP 保存仅关闭启用标志、保留间隔/字符串。Windows UI 交互及字段回归通过。 |
| SF-T07 | 名称模式清控制字符、限制 256 Unicode 标量；多分屏按完整显示名精确去重，再以扩展字素簇截取前 32 字符并加省略号。单分屏保留自身标题，详情不套用聚合截断。字符边界、同前缀不同名、重复名、余屏计数和焦点顺序回归通过。 |
| SF-I08 | `?` 接入现有 `help` 分派。真实控制台两种输入输出相同、返回提示符，未知命令仍拒绝；工具白名单和参数语法未改变。 |


| 固定条目与分类 | 必须处理或可保留的差异 | 限定边界 | 有限验收与依赖 | Desktop 源码入口 |
| --- | --- | --- | --- | --- |
| **SF-S02 协议相关属性**；行为差异 | SFTP 下禁用整个空闲发送控件组，包括开关、间隔和字符串，保存/协议切换遵循既有字段规则。 | 禁用不等于删除隐藏字段；不扩大协议专属设置。 | SSH 可编辑，切 SFTP 后三控件均不可编辑，切回恢复有效草稿；取消不改变已有资料。 **依赖：**独立。 | [connection_editor.go](../internal/desktop/connection_editor.go)；[editor_protocol.go](../internal/desktop/editor_protocol.go) |
| **SF-T07 分屏聚合标题**；行为差异 | 聚合前逐名按原版字符规则做 32 字符截断，名称模式先清控制字符；保持同名计数和布局顺序。 | 不以控件截宽替代数据级显示规则，不改会话原始名称或分屏身份。 | 31/32/33 字符及含控制字符名称，两个不同长名、同名分屏和第三个分屏各核对显示；切焦点不重排，详情保留各分屏信息。 **依赖：**独立。 | [session_titles.go](../internal/desktop/session_titles.go) |
| **SF-I08 本机工具入口**；功能缺口 | 在本机工具命令分派接入 ? 作为 help 的同义入口。 | 不扩工具白名单或命令语法。 | 空闲提示符分别执行 help 和 ?，显示相同帮助并返回提示符；未知命令仍拒绝。 **依赖：**独立。 | [tools_console.go](../internal/terminal/tools_console.go)；[local_tools.go](../internal/core/local_tools.go) |

## 保留项本轮验收结果

2026-10-25 已逐项核查剩余 13 项的有限保留条件，详细输入、测试入口、平台证据和待验项见[保留项验证记录](oshell-desktop-retained-validation.md)。下面两个原分类表保留为历史输入；本轮不新增 SF、不改 43 行主分类/优先级，仅同步“验收状态”。SF-C08 发现的路由绕过按原条目以 P1 严重性修复，未借平台替换排除，也不另计功能数。

| 固定条目 | 当前结果 |
| --- | --- |
| SF-P02 | 本机重开解密、缺失/损坏密钥不覆盖库、原版密钥迁移样本通过；真实 Swift 密码互通待验收。 |
| SF-C08 | 修复缺少 ssh 时不安全回退；Windows 原生 `ssh -G` 字段、多跳/ProxyCommand/显式覆盖、缺共享链及 helper 失败不直连通过。保留现有解析机制。 |
| SF-I24 | 六模式相同混排/宽字/软换行、光标命中与输入、逻辑复制和 Windows 字体软件渲染图通过；Windows 真实窗口及 Linux 显示待验收。生产默认开启，旧“默认关闭”说法不再适用。 |
| SF-F06 | 补齐 MLSD/LIST/NLST 的 4 MiB 上限及编码错误禁止降级；缺年份/时区的 LIST 时间改未知，明确 UTC 时间继续保留。协议路径及失败保留旧目录通过。 |
| SF-F07 | 禁用 SFTP 后 SCP 上传/下载、含空格/中文/符号名称、取消/失败、FTP 失败不覆盖及临时文件清理通过；保留字节进度。 |
| SF-A06 | 故障注入、重试上限和稳定重置、Windows 硬件 D3D11 像素回读通过；真实终端窗口设备丢失后的状态联验及 Linux GL 待验收。 |
| SF-A10 | 双窗关闭一终端立即释放、另一窗继续输入、全局空闲延迟回收与重开通过；图像/原生状态/定时器/日志释放通过。 |
| SF-I04 | 预览后超限/NUL 拒绝、取消和失效目标不写入、有效编辑通过。 |
| SF-O06 | 与 I04 共用用例，双目标各自遵循 bracketed paste，原目标快照不随选择变化转移，通过。 |
| SF-I14 | 固定 UTF-8、非法序列、C1、控制串分块及 OSC 预算/恢复回归通过；真实 Swift 逐字节对照待验收。 |
| SF-I17 | 隐藏段像素与空白一致、重置后显示恢复，复制和日志保留原文字，通过。 |
| SF-A02 | 自动/手动光标颜色、保存重开、共享 v2 字段往返通过；真实 Swift 互通待环境。 |
| SF-U01 | 合成 token 来源隔离、忙时保护、错误不回显秘密及私有更新缺签名拒绝通过。 |

本轮已更新固定 Windows 单文件包并核验原配置未改变，未提交、推送或触发 Actions。P1/P2/P3 的已记录能力缺口和平台待验项保持原状态；本表不是“161 项全部验收完成”的声明。

## 可保留的平台替换

“可保留”指下述实现机制，不代表表内所有运行结果已获豁免。尤其 SF-C08 的失败不绕过代理、SF-A10 的单终端资源释放必须成立；验证失败按原 SF 修正。

| 固定条目与分类 | 必须处理或可保留的差异 | 限定边界 | 有限验收与依赖 | Desktop 源码入口 |
| --- | --- | --- | --- | --- |
| **SF-P02 本机加密保存**；平台替换 | 可保留本机原始 credential-key 与独立 vault，作为原版 UUID JSON 密钥文件的本机实现替换；不强制统一磁盘文件格式。 | 共享数据仍只经配置协议 v2/共享 vault 交换，读 v1/v2、写 v2及身份重绑定不变；不能直接互换本机密钥文件。 | 本机重开可解密；合成密钥缺失/损坏时明确失败且不覆盖有效库；已支持的原版密钥迁移有样例，共享密码按协议往返。缺真实 Swift 环境仅记待验。 **依赖：**共享兼容必验。 | [local_credential_key.go](../internal/core/local_credential_key.go)；[vault.go](../internal/core/vault.go) |
| **SF-C08 路由解析**；平台替换；同时涉及：行为差异待验 | 可保留 ssh -G 取配置加 Go SSH 执行的机制；只消费固定相关字段不等于需要重做完整 OpenSSH。缺少系统 ssh 的原资料回退尚不能证明路由等价。 | 保留前提是固定别名、ProxyJump/ProxyCommand、显式共享链结果正确。若所需解析/代理不可用时静默直连，必须归本 SF 以 P1 修正，不能借平台替换排除。 | 别名含主机/用户/端口/私钥、ProxyJump、ProxyCommand、显式路由覆盖各一例；缺 ssh、helper 失败、共享链引用缺失各一例。对需要配置解析/代理者应明确失败且无直连；无需配置的字面地址另验既定行为。 **依赖：**先完成有限路由矩阵；不承诺所有 ssh_config 语法。 | [ssh_config.go](../internal/core/ssh_config.go)；[transport.go](../internal/core/transport.go) |
| **SF-I24 双向文字**；平台替换 | 可保留 Go UAX#9/字形布局替代 CoreText 探针。以生产默认启用路径为准，旧 README 的关闭描述应在本 SF 文档中校正。 | 不要求统一布局引擎或追随新 Unicode 版本；不能据 ABI 已启用就认定复杂混排通过。 | 固定六模式分别用同一组拉丁/阿拉伯或希伯来混排、宽字符和软换行样例，核对视觉顺序、光标、选择复制和输入映射；Windows/Linux 各保留实际显示证据。 **依赖：**平台视觉验收待完成。 | [bidi.go](../internal/terminal/internal/vt/bidi.go)；[terminal.go](../internal/terminal/internal/vt/terminal.go)；[layout.go](../internal/terminal/internal/bidilayout/layout.go)；[bidi.go](../internal/terminal/bidi.go) |
| **SF-F06 FTP 列表**；平台替换；同时涉及：已有增强 | 可保留 Go FTP 解析器替代 curl 输出解析；LIST 修改时间解析属于已有增强，可保留可靠解析出的时间。 | 无法可靠识别的时间保持未知，不猜时区或年份；已有列表降级、UTF-8、4 MiB 和安全名称约束仍是验收条件。 | 合成服务器覆盖 MLSD 成功、降级 LIST、再降级 NLST 三条路径；UTF-8 名称、非法名称、列表 4 MiB 边界各一例；可靠时间显示，歧义/无时间不伪造值。 **依赖：**库路径与 Desktop 列表联验。 | [ftp.go](../internal/core/ftp.go)；[ftp.go](../third_party/ftp/ftp.go)；[parse.go](../third_party/ftp/parse.go) |
| **SF-F07 传输方式**；平台替换；同时涉及：已有增强 | 可保留 Go FTP 和 SSH channel 自行编解码 SCP，使用远端 scp -rt，不要求依赖本机系统 scp；字节进度保留为增强。 | 不追加 rsync、新协议或本地工具探测产品能力；独立 SCP 必须保持无需 SFTP 的固定结果。 | 禁用远端 SFTP 时独立 SCP 仍可传输合成文件；含空格/符号文件名内容一致，取消/断连不误报完成；FTP 一次正常传输及失败清理。进度与实际完成一致。 **依赖：**与 SF-F04 的链接边界区分验收。 | [filesystem.go](../internal/core/filesystem.go)；[ftp.go](../internal/core/ftp.go)；[scp.go](../internal/core/scp.go)；[standalone_scp.go](../internal/desktop/standalone_scp.go) |
| **SF-A06 渲染**；平台替换；同时涉及：行为差异 | 可保留 D3D11/GL 与软件渲染替代 Metal/CG；1–30 秒退避、稳定 1 分钟重置可作为平台恢复策略，记录与原版重试时间的差异。 | 保留前提是失败降级可用、重建不丢终端状态且不形成持续卡顿；不要求复制苹果计时器，不新增渲染后端。 | 注入首次失败、连续失败、恢复成功各一例；输入/选择/图像和文本状态不丢，软件模式可用、重试有界；Windows D3D11 与 Linux GL 均需平台证据。 **依赖：**有限故障注入及平台实测。 | [workspace.go](../internal/desktop/workspace.go)；[renderer_preference.go](../third_party/mygo/ui/renderer_preference.go)；[window.go](../third_party/mygo/ui/window.go) |
| **SF-A10 生命周期与性能**；平台替换；同时涉及：行为差异 | 可保留 Go FreeOSMemory 的进程级回收，并以所有窗口无标签作为全局回收条件；不要求与原版单窗口延迟回收的 RSS 相同。 | 单终端关闭必须立即释放其连接、定时器与图像等资源，不得等其他窗口清空才释放；全局回收策略不能掩盖泄漏。 | 双窗保持一窗活动，关闭另一窗最后标签，确认该终端资源释放且活动窗无中断；全部空闲后延迟回收、重新开标签可用；不以固定 RSS 数字作通过标准。 **依赖：**与 SF-W01 的零窗口状态联验。 | [oshell.go](../internal/terminal/oshell.go)；[terminal.go](../internal/terminal/terminal.go)；[idle_memory.go](../internal/desktop/idle_memory.go) |

## 可保留的已有增强

以下仅记录已有实现的合理差异。混合条目中的增强已分别写在修复表里，如 SF-P05 的 FTP/SFTP 保存、SF-F04 的资源上限、SF-O07 的背景/粗体、SF-U04 的回滚，不再重复计入本表。

| 固定条目与分类 | 必须处理或可保留的差异 | 限定边界 | 有限验收与依赖 | Desktop 源码入口 |
| --- | --- | --- | --- | --- |
| **SF-I04 文本粘贴**；已有增强 | 保留预览编辑后与最终分发时的 1 MiB/NUL 再校验，修正原版校验缺口。 | 正常基线粘贴结果不变；不为复刻移除这层保护，不新增内容审查或格式转换。 | 预览前有效，编辑后超 1 MiB 或含 NUL，各应拒绝且目标无写入；有效编辑可发送，取消不发送，原目标失效不转发到新目标。 **依赖：**主验证归本项，SF-O06 联验。 | [terminal_features.go](../internal/desktop/terminal_features.go)；[terminal.go](../internal/terminal/terminal.go) |
| **SF-O06 同步输入**；已有增强 | 保留同步输入预览修改后的再次校验，关联 SF-I04；不独立建立重复修复。 | 维持逐目标 bracketed paste 和原始目标快照，不把图片/协议回复并入广播。 | 两个目标模式不同：有效文本分别按模式发送；预览改成超限/NUL 时均无写入；一目标失效按既有停止规则处理，不转向新目标。 **依赖：**由 SF-I04 共用校验，补双目标联验。 | [sync_input_draft.go](../internal/desktop/sync_input_draft.go)；[terminal_features.go](../internal/desktop/terminal_features.go) |
| **SF-I14 字符与解码**；已有增强 | OSC 8 MiB 总长预算可保留为资源保护，不为复制原版缺口去掉上限。C1/非法 UTF-8/异常控制串仍待有限对照，不能从该增强推导整个解析器等价。 | 只使用固定原版输入语义；若固定样例出现具体偏差，归回本 SF 修正，不扩为全部终端标准审计。 | 固定样本包含有效多字节的分块解码、非法 UTF-8、C1、未终止控制串及超过预算的 OSC 各一例；核对可见文本/回复/恢复到普通文本，不持续占用无界内存。 **依赖：**固定逐字节对照待验。 | [oshell.patch](../third_party/ghostty/oshell.patch)；[terminal.go](../internal/terminal/internal/vt/terminal.go)；[osc_extended_test.go](../internal/terminal/internal/vt/osc_extended_test.go) |
| **SF-I17 颜色与属性**；已有增强 | 保留实际消费 Invisible 属性的渲染行为，作为修正原版属性交接缺口的有意差异。 | 不复制原版显示缺陷；其他颜色属性仍按已有基线验收，不追加完整新 SGR 能力。 | 同一行普通文本、Invisible 开启、重置后文本，确认中段隐藏且重置恢复；选择/复制保留文本，不改变日志原文。 **依赖：**与现有 SGR 渲染用例联验。 | [paint.go](../internal/terminal/paint.go)；[palette.go](../internal/terminal/palette.go)；[oshell.patch](../third_party/ghostty/oshell.patch) |
| **SF-A02 终端配色**；已有增强 | 保留 AutoCursor 自动对比色，原版模型未接显示的行为不必复现。 | 仍沿用现有字段与用户开关；不新增共享字段、主题系统或自动主题策略。 | 同一配色切开/关，开启时光标可辨、关闭遵循手动色；保存重开稳定，共享往返字段不丢失。 **依赖：**涉及共享字段只做现有合同验证。 | [colors.go](../internal/core/colors.go)；[settings.go](../internal/desktop/settings.go)；[fonts.go](../internal/desktop/fonts.go)；[paint.go](../internal/terminal/paint.go) |
| **SF-U01 更新源设置**；已有增强 | 现有私有仓库 token 能力作为 Desktop 已有扩展保留，不计入 OShell 必补范围。 | 不增加 OAuth、账号体系或其他更新服务；与 SF-U02 复刻静态入口明确区分，不能借 token 能力跳过更新真实性要求。 | 用合成 token 验证切换来源时身份绑定、忙时禁止切换、错误路径不输出秘密；现有增强入口也须随 SF-U03 核对验签，不能默默回退无签名安装。 **依赖：**与 SF-U02/SF-U03 核对来源及信任边界。 | [updater_other.go](../internal/desktop/updater_other.go)；[update_source.go](../internal/core/update_source.go)；[update_auth.go](../internal/core/update_auth.go) |

## 执行顺序与关闭条件

1. **先处理 P1。** 凭据部分由 SF-P07 的识别规则与 SF-P05 的禁存规则联验；代理、剪贴板代次、导入边界各按本项用例关闭。SF-U02 与 SF-U03 先定生产元数据/签名调用链再接入口，避免只增加不可达的验签工具函数。
2. **再处理 P2 的共同依赖与主流程。** SF-P08 修状态后联验 SF-M05；SF-E06 限额校验后补 SF-E03 空目录；SF-U03 验签后联验 SF-U04 安装。其余 P2 按表逐项执行，不因这些依赖新增功能编号。
3. **尽早安排必要的平台验收。** SF-M01 先确定框架真实转换能力；SF-I22 分平台证明介质支持/拒绝。缺 macOS/Swift、Linux 或真实输入/渲染环境，记待验收；平台无法等价的介质保留影响和未闭合状态，若需要新协议或新服务则另记范围变更并取得授权。
4. **最后完成 P3，并核验保留条件。** 保留项的机制差异无需机械重写；明确不满足固定结果时仍在本 SF 中修复。SF-O06 与 SF-I04 共用校验的证据，不重复计算工作量。

实际修复每项至少留下：修改入口、该行有限用例结果、必要的平台证据、已知缺陷是否闭合。通过后不因继续猜测参数组合而无限追加；只有新的具体证据才重开已有 SF。本次分类不修改原基线，不把 SF-I22 的待决介质、SF-C08 的未验路由或其他缺环境项目静默改成平台排除。

后续涉及共享字段、凭据、目录/引用或持久化默认值时，按[配置协议 v2](configuration-protocol-v2.md)检查两端字段与身份绑定；执行规定的 Go 兼容样本及相关回归，真实 Swift 往返缺环境时保留待验收。每次实际功能修复后依仓库约定本机编译、测试固定 Windows 便携 EXE/ZIP、独立目录启动并校验原配置不变；这不以提交、推送或 Actions 代替。本轮 P1 已完成上述 Windows 构建验证，原 11 个数据文件哈希不变；Go 兼容样本通过，真实 Swift 互通仍待环境。

## 范围核对记录

- 本表 43 个唯一 SF 与上轮 CSV 的“存在差异”集合逐项对应，未纳入另外 117 个“已实现”或 SF-P10 平台排除项，也未新增子功能编号。
- 功能名与所属域沿用上轮清单；CSV 保存主分类、同时涉及、优先级、处置、保留边界、有限验收、依赖和源码入口，便于筛选但不作为新的功能数量。
- 输入基线 SHA256：`0FE41373FACCF99453302FA6242AB5AAA27E7E084D03462EAE3683AE810A30B1`。
- 输入差异 CSV SHA256：`B580DC8B3071F60C5D0F1BA30D42F5D8C527777CB9DE76AC904F4F4D1D345EDC`。
- 上述哈希只标识本次分类的文档输入，不代表 Desktop 工作树、发行包或运行验收的快照。

