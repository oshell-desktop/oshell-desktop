# OShell 细分功能源码分析记录

固定基准：OShell 0.2.92，提交 `3f4efd9c70c82323f4e20180117c928227000580`。分析对象为 `validation/oshell-reference` 中的原版代码。功能身份沿用[功能基线](oshell-feature-inventory.md)的 `OShell-0.2.92-scope-1`，不增加 SF 编号。

这里记录生产入口、执行路径和源码限定，不用文件数量或哈希匹配代替阅读。源码结论只证明静态代码行为；Desktop 是否实现、测试是否运行、平台交互是否通过，分别记录，不能互相代替。表内行号均针对固定提交。

## 核查方法与当前覆盖

每项核对入口是否可达、配置/状态读写、主要分支、取消/失败处理和与其他域的交接。测试与说明文档用于补充场景，不能替代生产实现。第三方组件中的示例、测试、其他平台实现，不因出现在依赖目录就自动成为 OShell 的产品功能。

“已核查”表示本表指定的直接生产路径已阅读并形成结论；跨域公共实现归所属域核查。“待逐项核查”表示已有宏观阅读或文件索引，但还没有足够的逐项证据。全部功能域核查完成前，不宣称完整源码分析结束。

| 功能域 | 条目 | 源码核查状态 | 后续核查入口 |
| --- | --- | --- | --- |
| 会话资料与管理 | SF-S01–SF-S14 | 直接路径已核查 | 跨域凭据、存储与连接在 P/Y/C/F 域核查 |
| 密码与本机保护 | SF-P01–SF-P10 | 直接路径已核查 | 密码库、认证 broker/helper、加密/轮换、本机密钥和 Touch ID；归档、代理编辑、共享存储调用继续归 E/C/Y |
| 连接与代理 | SF-C01–SF-C14 | 直接路径已核查 | 系统 SSH 调用、代理 UI/helper、隧道与复用、PTY 和空闲保活；文件传输协议细节继续归 F |
| 窗口与导航 | SF-W01–SF-W12 | 直接路径已核查 | 窗口/菜单、标签与组、分屏、排列、跨窗迁移、定位与专注；文件操作内部归F，终端实现归I，共享事务归Y |
| 主机信息与标题 | SF-T01–SF-T09 | 直接路径已核查 | 本机设置、TerminalPane 主机跟踪、Shell 脚本、TCP 快照和信息卡；OSC 底层分派继续归 I 域 |
| 配置与同步 | SF-Y01–SF-Y17 | 直接路径已核查 | 本机库/目录迁移、目录与 WebDAV 传输、设置/策略/状态/诊断、共同基准和冲突、加密信封及生成正文合同；外层未知键缺口显式保留，真实 Swift ↔ Go 互通仍待验收 |
| 终端 | SF-I01–SF-I27 | 直接路径已核查 | 交互/工具、解析解码、缓冲/编辑、颜色/属性、键鼠、查询/状态、刷新重置、图像协议/占位、BiDi、特殊行、提示符语义和宿主覆盖已逐项记录；已定位的解析/输入/状态/显示交接缺口显式保留，实机验收独立 |
| 图片粘贴 | SF-M01–SF-M08 | 直接路径已核查 | 已追踪到缓存写入、SFTP 上传、目标输入与提醒持久化；跨平台图像/实机传输仍待验收 |
| 文件管理与传输 | SF-F01–SF-F15 | 直接路径已核查 | 文件窗/标签、SFTP/FTP/SCP、限速/记录/续传、ZMODEM 应用握手与随包 helper 文件管理分支；helper 未暴露选项不作为产品能力，真实服务器与 macOS 交互仍待验收 |
| 快捷操作 | SF-O01–SF-O09 | 直接路径已核查 | 命令管理/发送栏/撰写窗/同步输入、规则匹配与前景渲染、快捷键录入/分派和弹窗；终端完整输入编码及共享字段合同归 I/Y |
| 外观与资源 | SF-A01–SF-A10 | 直接路径已核查 | 主题/配色/字体、字符宽度入口、Metal 恢复、时钟/未读/空闲回收；完整终端解析、字符布局和图像生命周期继续归 I |
| 外部互通与迁移 | SF-E01–SF-E09 | 直接路径已核查 | 原生归档/第三方解析及预览、密码补充、两个启动器、IPC 与应用接收；共享完整合同归 Y，文件后端归 F |
| 更新与分发 | SF-U01–SF-U07 | 直接路径已核查 | 应用更新委托、静态元数据/本机桥接、发布打包和关于；Sparkle 二进制内部与实机升级不冒充已验证 |

本次已完成逐项直接路径核查和文末的生产入口反查，包含应用启动/菜单、辅助程序和生产依赖的能力分派；结论依据是执行路径与边界证据，不是仅找到 161 个同名函数。各域保留已读生产文件、局部区段与依赖分析限制，避免把组件全部内部实现误称为已审。

## 会话资料与管理

以下 SF-S 条目的直接路径已核查。共用保存路径为 [WorkspaceController.swift](../validation/oshell-reference/Sources/OShell/WorkspaceController.swift) 的 `saveConfiguration`（377–423）：检查原数据读取失败、未处理冲突、凭据保护，执行文件会话/代理迁移、默认值校验、引用规范化，再调用存储保存；成功通过 `acceptSavedConfiguration`（425–433）和 `receiveConfiguration`（1096–1112）刷新 UI 并发布给其他窗口。存储算法本身仍须在 Y 域核查。

| SF 条目 | 生产入口与源码定位 | 源码结论与关键边界 |
| --- | --- | --- |
| SF-S01 | [SessionEditor.swift](../validation/oshell-reference/Sources/OShell/SessionEditor.swift) `init` 37–110、`run` 191–240；[WorkspaceController.swift](../validation/oshell-reference/Sources/OShell/WorkspaceController.swift) `createSession` 453–460、`editSavedSession` 463–474；[Models.swift](../validation/oshell-reference/Sources/OShellCore/Models.swift) `SessionKind` 6–11、`SessionProfile.validate` 85–103 | 保存模型四类：SSH/SFTP/FTP/local；编辑器可选前三类，本地配置在管理器只改名。保存处理名称、目录、连接、密码、代理、隧道、保活和快捷连接；连接参数下次连接生效，本机标题方式单独保存并刷新。取消不返回修改后的资料。 |
| SF-S02 | SessionEditor `protocolChanged` 130–155、`keepAliveChanged` 179–189、`run` 194–210 | FTP 隐藏代理/隧道/保活页并在保存时清除私钥、跳板、代理引用、隧道、旧 SSH 及 SSH 保活配置。SFTP 隐藏隧道页、禁用空闲字符串及标题方式；切换为 SFTP 不直接清除已有隧道数组。默认字段有条件替换，不覆盖用户已经修改的值。密码留空沿用时也经过目标身份检查/重绑，算法归 P03。 |
| SF-S03 | [SessionDefaults.swift](../validation/oshell-reference/Sources/OShellCore/SessionDefaults.swift) 7–38；[SessionDefaultsEditor.swift](../validation/oshell-reference/Sources/OShell/SessionDefaultsEditor.swift) 18–60；[WorkspaceSessionDefaults.swift](../validation/oshell-reference/Sources/OShell/WorkspaceSessionDefaults.swift) 8–10 | 默认 SSH 22、空用户/私钥、目录 `.`；FTP 21、anonymous、目录 `/`；快捷连接默认开。SFTP 继承 SSH 默认但关闭空闲字符串。默认设置不含主机、密码、代理、隧道，不批量覆盖已有/复制/导入配置。已有会话可显式“使用全局默认”保活，再保存。 |
| SF-S04 | [SessionManager.swift](../validation/oshell-reference/Sources/OShell/SessionManager.swift) 572–615、702–720、811–865；[SessionDirectoryMoves.swift](../validation/oshell-reference/Sources/OShellCore/SessionDirectoryMoves.swift) 7–75；[SessionDirectoryPicker.swift](../validation/oshell-reference/Sources/OShell/SessionDirectoryPicker.swift) 36–97、154–179；[ConnectionOptions.swift](../validation/oshell-reference/Sources/OShellCore/ConnectionOptions.swift) 129–162 | 管理器内拖动与树选择器移动；只接受既有目标目录，禁止自包含、同名目录冲突及移动 Links 根。新建/改名为单组件，最多 255 UTF-8 字节。递归迁移保留资料身份、映射子路径并保留未选空父目录。编辑器的分类选择器不可自由输入新路径；`resolvePath` 的能力不能反推成该 UI 支持。 |
| SF-S05 | [SessionSearchQuery.swift](../validation/oshell-reference/Sources/OShellCore/SessionSearchQuery.swift) 7–24；SessionManager 189–201、409–469、483–502、551–562；[SessionSidebar.swift](../validation/oshell-reference/Sources/OShell/SessionSidebar.swift) 110–175 | 4096 UTF-8 字节/32 词上限；空白分词后 AND 子串匹配，大小写/重音/字符宽度归一化，无正则。元数据含名称、主机、用户、目录、协议、端口。管理器两种范围只在有搜索词时生效；协议过滤独立。引用在管理器用引用名/路径投影，侧栏同时含源元数据。侧栏搜索展开匹配祖先，清空恢复搜索前展开状态。 |
| SF-S06 | SessionManager 63–109、177–185、208–244、287–366、478–502、623–659、731–850；SessionSidebar 44–46、177–185、215–226 | 管理器多选、双击/回车打开、上级行单击导航；批量连接仅提取会话/引用。F2 行内改名回车提交、Esc/失焦取消，输入法组字不提交。固定资料列允许宽度/排列调整，编辑列固定。文件选择模式单选且不含本地会话。侧栏复用右键动作，但重命名使用弹窗，普通双击/回车只打开当前行。 |
| SF-S07 | [SessionConfigurationClipboard.swift](../validation/oshell-reference/Sources/OShell/SessionConfigurationClipboard.swift) 8–77；SessionDefaults `SessionDuplication` 40–66；WorkspaceSessionDefaults 12–39；SessionManager 669–700 | 系统剪贴板只保存随机令牌；快照在进程内且绑定规范化数据文件 URL 和剪贴板 changeCount。批量复制引用时读取源资料；粘贴生成新身份并重绑密文、检查共享代理仍能解析、检查配置版本后一次保存。独立复制始终生成副本后缀，粘贴到目标目录无同名时保留原名。独立复制排除 local，配置剪贴板可含 local。标题选择在保存成功后复制，细节归 T02。 |
| SF-S08 | SessionManager 867–933；[SessionDirectoryDeletion.swift](../validation/oshell-reference/Sources/OShellCore/SessionDirectoryDeletion.swift) 6–40；WorkspaceController 476–479；[WorkspaceSessionLinks.swift](../validation/oshell-reference/Sources/OShell/WorkspaceSessionLinks.swift) 406–408、439–445 | 混合删除先去重祖先/子项，计算会话/目录/引用实际影响，确认后检查配置 revision 并一次保存。非空目录确认；空目录、单个引用不另弹确认。删除源资料清理其他目录引用，未选空父目录保留。运行连接继续；不触碰服务器文件。链接栏含实际会话的目录删除必须回管理器确认。 |
| SF-S09 | SessionSidebar 6–25、60–107、114–145、181–189、212–273；SessionManager 159–166、256–284；WorkspaceController `receiveConfiguration` 1096–1112 | 与管理器共用配置和右键动作，按稳定身份解析侧栏选择，拒绝过期对象。多选、搜索、全部展开/收缩、协议图标和引用标识；宽度固定 250。显隐存 UserDefaults，锁定/专注全屏时隐藏。侧栏快捷键为本机覆盖，共享快捷键冲突时禁用本机绑定。没有证据支持将管理器表格拖放直接扩为侧栏拖放。 |
| SF-S10 | [SessionPins.swift](../validation/oshell-reference/Sources/OShell/SessionPins.swift) 6–40；SessionManager 368–386、452–457、839–846；SessionSidebar 135–143；WorkspaceController 149–151、1096–1105 | 根目录会话/目录置顶，引用不能置顶；`session-pins.json` 为本机状态，变更通知刷新窗口。置顶组内保留目录/名字既有顺序。会话改名 ID 不变；目录改名保存新键；删/移入子目录时 reconcile 清理旧键。置顶文件损坏不应阻塞共享配置装载。 |
| SF-S11 | [SessionCatalogPresentation.swift](../validation/oshell-reference/Sources/OShell/SessionCatalogPresentation.swift) 5–25；WorkspaceController `rebuildQuickLinks` 484–507 | 顶部快捷连接按目录展示 quickConnect=true 的实际保存会话，不把引用重复加入。仅隐藏根/本机目录内、未被引用、其余属性等同工厂默认的 local 占位项；数据不删除。改名、改属性或被引用后展示。工厂过滤用于管理器/侧栏/选择器/快捷菜单；不能推广为删除或同步过滤。 |
| SF-S12 | WorkspaceSessionLinks 181–285；[SessionLinks.swift](../validation/oshell-reference/Sources/OShellCore/SessionLinks.swift) 13–44、86–102；SessionDirectoryMoves 17–75；SessionManager 852–865 | Links 混合实际资料、引用、目录。添加当前会话时先补存未保存的可复用资料，再在根建引用；一次性凭据留认证代理。重复同源同目录不新增。引用编辑可改名称/目录，源选择器禁用。普通资料/目录移入 Links 创建引用树并保留源；显式移动实际资料保留身份；已在 Links 内的实际资料按实际迁移。 |
| SF-S13 | SessionLinks 86–131；SessionDirectoryMoves 23–29、69–74；SessionDirectoryDeletion 16–35；WorkspaceSessionLinks 287–294；WorkspaceController 405–410 | 保存规范化清除失效源、重复引用 ID/位置并整理根顺序；连接实时查询源资料，找不到则不打开。引用和 Links 子目录只能在 Links 内移动；实际资料可移出。Links 根不可删/改/移。引用删除不删源，Links 子目录删除是否连带实际资料取决于所走管理入口。共享协议输入是否合法还须由 Y17 核查，不能把本地 normalize 当作允许静默丢弃远端字段。 |
| SF-S14 | [SessionLinkBar.swift](../validation/oshell-reference/Sources/OShell/SessionLinkBar.swift) 8–231；WorkspaceSessionLinks 8–88、303–445；SessionLinks 46–75 | 固定加号、水平滚动、全部链接菜单、目录菜单，显隐不删内容。根顺序持久化；会话/引用可拖入目录，也可从目录菜单拖回根。仅同一 bar、当前 generation、匹配剪贴板目标键的拖动有效；更新后旧菜单/拖动失效。文件夹只在根级重排，不能拖入另一文件夹；有边缘自动滚动和落点指示。 |

### 本域修订依据

2026-10-24：补齐协议显隐、搜索限制、管理器与侧栏交互差异、复制两种入口、删除确认差异、Links 引用规则与拖动限制。全部归入原 SF-S01–SF-S14，没有增加功能身份。未将平台 UI 常量、每种错误提示或新增回归场景拆成新功能。

以下属于项目约定，不能冒充“源码自动给出的结论”：不追加 Telnet/串口/RDP 模型、不建设标签分类或自定义字段列系统、不将跨设备复制纳入进程内剪贴板、有限验收用例的通过要求。原版有无某项能力，以本域生产路径为依据；排除与复刻目标有关的新产品能力，以功能基线及用户授权为依据。

### 已阅读文件范围

本域整文件已读：`SessionManager.swift`、`SessionEditor.swift`、`SessionSidebar.swift`、`SessionDirectoryPicker.swift`、`SessionLinkBar.swift`、`WorkspaceSessionLinks.swift`、`SessionCatalogPresentation.swift`、`SessionConfigurationClipboard.swift`、`SessionPins.swift`、`SessionDefaultsEditor.swift`、`WorkspaceSessionDefaults.swift`；Core 的 `SessionDefaults.swift`、`SessionSearchQuery.swift`、`SessionLinks.swift`、`SessionDirectoryMoves.swift`、`SessionDirectoryDeletion.swift`。

本域只读取相关区段的公共文件：`WorkspaceController.swift` 的保存/创建/编辑/删除/快捷菜单/配置接收（377–507、1096–1112；置顶通知另核对 149–151）、`Models.swift` 1–110、`ConnectionOptions.swift` 129–162。不能据此认定这些公共文件其余部分或相邻功能域全部已读。引用的凭据、存储、标题选择、连接和窗口发布实现继续在各自功能域展开。

## 图片粘贴

入口为 [TerminalPane.swift](../validation/oshell-reference/Sources/OShell/TerminalPane.swift) `OShellTerminal.paste`（37–43），先尝试读取图片，再尝试文字；[WorkspaceController.swift](../validation/oshell-reference/Sources/OShell/WorkspaceController.swift) 611 将图片回调接到 `pasteImage`。此路径与终端接收 Kitty/Sixel/OSC 图像并渲染的 I22 不同。

| SF 条目 | 生产入口与源码定位 | 源码结论与关键边界 |
| --- | --- | --- |
| SF-M01 | [TerminalImageClipboard.swift](../validation/oshell-reference/Sources/OShell/TerminalImageClipboard.swift) `read` 14–30、`decode` 32–49 | 文件 URL 优先，单个白名单图片为普通文件才读取；多文件含图片报错，无图片时返回 nil，不把 Finder 图标当图片。剪贴板 PNG/TIFF/JPEG 依次尝试首个存在的数据。ImageIO 按索引 0 读取，thumbnail transform 应用方向，以原最长边上限解码并写 PNG；没有动画输出，也没有显式复制全部源元数据。 |
| SF-M02 | [WorkspaceImagePaste.swift](../validation/oshell-reference/Sources/OShell/WorkspaceImagePaste.swift) 17–65 | 入口要求启动解锁、无其他模态窗、原 pane 仍属于工作区且无传输。确认显示尺寸/大小/目标说明，无图像预览控件；SSH 父目录默认 `/tmp`。有效记忆跳过确认；读取设置失败仍允许确认粘贴，但禁用“记住”。取消发生在文件保存前。 |
| SF-M03 | TerminalImageClipboard `TerminalClipboardFiles.save` 52–74、`pastedPath` 75–80；[PrivateFile.swift](../validation/oshell-reference/Sources/OShellCore/PrivateFile.swift) 7–21；WorkspaceImagePaste 20–26、68–81 | 本机成功文件进入本机状态目录下 clipboard-images，校验根目录不是符号链接、归当前用户所有、无组/其他用户权限；文件以 0600 暂存后原子 rename。允许本地 shell 和受支持的本机工具 PTY，排除 OShell 工具 ssh/telnet；纯空白 OShell 提示符无 PTY 不可粘贴。路径逐字符反斜杠转义并拒绝控制字符。 |
| SF-M04 | WorkspaceImagePaste 70–78、110–145；[RemoteFiles.swift](../validation/oshell-reference/Sources/OShell/RemoteFiles.swift) SFTPBackend 200–257、275–291、404–427；[SSHConnectionGroup.swift](../validation/oshell-reference/Sources/OShell/SSHConnectionGroup.swift) 39–56、90–103 | SFTP 基于已有组的 lease，要求目标身份匹配且私有控制 socket 可用；clone 参数用 BatchMode 和 false ProxyCommand 阻止重新认证回退。SFTP 只接受 v3。父目录 REALPATH 后创建随机子目录，请求 0700 并 STAT 检查 group/other 无权限；image.png 用排他创建，32 KiB 读取上传并报告字节进度。失败或取消不发送路径。 |
| SF-M05 | WorkspaceImagePaste `stillCurrent` 28–35、65、80–108、128–143；TerminalPane 256–264、295–298；[OperatorModels.swift](../validation/oshell-reference/Sources/OShellCore/OperatorModels.swift) `InputText.bytes` 109–115；[WorkspaceInput.swift](../validation/oshell-reference/Sources/OShell/WorkspaceInput.swift) 8 | 校验原 pane 仍在工作区、没有关闭/新传输、PTY/PID/本机工具 PID、已识别 hostname/address 和远端连接组。仅向 source 发路径，绕过广播，不追加回车，尊重 bracketed-paste；本机工具走自己的输入处理。检测的是代码已经认识的状态，不保证发现所有 shell 内嵌套身份变化。 |
| SF-M06 | [ImagePasteSettings.swift](../validation/oshell-reference/Sources/OShell/ImagePasteSettings.swift) 7–43、47–111；WorkspaceImagePaste 36–42、83–91；WorkspaceController 889–902 | 版本 1 的本机 JSON；本地全局布尔，远端键为 UUID，SHA-256 指纹含协议/host/port/user/跳板/私钥/代理 ID、内嵌代理及运行时路由字段，不含凭据、主机密钥。复制 UUID 变则重新确认，单改名不影响指纹。路径已插入后才记忆；恢复提醒先改草稿，外层应用后写入并检测并发变更。未知版本拒绝覆盖。 |
| SF-M07 | TerminalImageClipboard 8–9、23–25、33–47、61–70 | 文件读取/输出 PNG ≤20 MiB；输入 Data ≤160 MiB，每边 ≤20000，总像素 ≤40000000。不是解码峰值内存上限。缓存只清理/计数 oshell-clipboard- 前缀、png 扩展名的非符号链接普通文件；7 天以上先删除，再检查总量是否超 200 MiB，超限拒绝，不做未过期文件的 LRU 淘汰。 |
| SF-M08 | WorkspaceImagePaste 68–95、128–145；RemoteFiles 275–291、456–457；TerminalImageClipboard 61–73 | 成功本地文件保留；远端成功且插入路径后删除本地暂存，远端文件不随会话关闭删除。上传失败尝试 remove 图片和目录；取消使底层进程停止，清理请求可能无法执行，不能保证零残留。上传已成功但 target 失效时不发路径，远端文件仍可能保留。 |

### 原描述纠正

2026-10-24，SF-M01：将“颜色信息保留正确”细化为固定样例的显示颜色/透明度验收。`CGImageDestinationAddImage(..., nil)` 未显式复制全部源元数据；源码不足以支持完整 ICC/EXIF 保真承诺。跨平台转码对比仍需实际样例。

2026-10-24，SF-M05（关联 SF-P01/P08）：旧表把“锁定已保存密码”写成能阻断图片发送，混淆了两类状态。[WorkspaceController.swift](../validation/oshell-reference/Sources/OShell/WorkspaceController.swift) `lockPasswords`（915）只取消 WebDAV、清 store 主密码并调用 PasswordVault.lock；[PasswordVault.swift](../validation/oshell-reference/Sources/OShell/PasswordVault.swift) 73 只递增凭据 generation 和清缓存主密码。它不改变启动门禁 `isSecurityUnlocked`，也不关闭既有 SSH 组。图片的 `stillCurrent` 和发送路径没有凭据 generation 检查。因此已更正为“启动未解锁不允许发起；密码缓存锁定不等于终端锁定”。这属于纠正原版事实，不把既有连接停用作为原版必备功能；若 Desktop 另需该行为，应明确记录产品差异及授权，不能隐含追加。

2026-10-24，SF-M07/M08：补充单边 20000 像素、输入 Data 上限与内存占用的区别、非缓存文件不计入缓存配额、超额拒绝及失败清理仅尽力而为。SF 身份和参考版本不变。

### 已阅读文件与测试证据范围

整文件已读：`TerminalImageClipboard.swift`、`WorkspaceImagePaste.swift`、`ImagePasteSettings.swift`、`SSHConnectionGroup.swift`、Core `PrivateFile.swift`。M 域读取公共文件区段：TerminalPane 1–59、251–300；RemoteFiles 1–152、183–291、404–458；WorkspaceController 图片回调与设置应用（611、889–915）；OperatorModels 92–115；WorkspaceInput 1–12；PasswordVault 61–78。共享文件剩余部分仍归所属域，不认定全部覆盖。

已阅读 [ImagePasteTest.swift](../validation/oshell-reference/Sources/OShell/TestSupport/ImagePasteTest.swift) 和 [ImagePasteSettingsTest.swift](../validation/oshell-reference/Sources/OShell/TestSupport/ImagePasteSettingsTest.swift)：覆盖本地 PNG 转码、较大 TIFF 输入、路径转义、权限、非图片图标忽略、取消、不广播、不提交命令、记忆绑定及设置草稿。没有执行这些测试；它们也没有证明所有格式、ICC、真实 SSH 上传或所有生命周期组合通过。后续验收用有限且有代表性的样例补足这些具体缺口，不能把测试索引条数当覆盖率。

## 主机信息与标题

入口包括会话属性、设置中的标签页、视图菜单“刷新标题识别”、终端输出回调及标签悬停。主机跟踪实际位于 [TerminalPane.swift](../validation/oshell-reference/Sources/OShell/TerminalPane.swift)，不能把一个假定存在的 HostTitleTracker 文件当证据。OSC 的 SwiftTerm 底层分派在 I 域另行核查。

| SF 条目 | 生产入口与源码定位 | 源码结论与关键边界 |
| --- | --- | --- |
| SF-T01 | [SessionTitleSettings.swift](../validation/oshell-reference/Sources/OShell/SessionTitleSettings.swift) 5–12；[HostTitleMode.swift](../validation/oshell-reference/Sources/OShellCore/HostTitleMode.swift) 6–21；TerminalPane 301–344、989–995；[SessionEditor.swift](../validation/oshell-reference/Sources/OShell/SessionEditor.swift) 98–106、144–145 | 用户四种方式与共享模型三种 HostTitleMode 分离。sessionName 在展示层固定名字，hostMode 回退 shellIntegration，所以仍可接收主机信息供悬停查看；不触发主动探测。passive 拒绝 Shell 上报；shellIntegration 无脚本时被动解析；activeProbe 收到集成后停止主动探测。切换取消待发探测/超时并放回未确认回显。 |
| SF-T02 | SessionTitleSettings 15–49、53–118；WorkspaceController 453–472、565–575、889–902；SessionEditor 100–103 | 本机 tab-title-settings.json v1，默认 sessionName，按 UUID 的 overrides 优先；旧资料无 override 回退 profile.titleMode，并非无条件应用新默认。创建编辑器按 defaultMode 设置并单独保存本机选择，新建非保存 local 也使用默认。全局“同时覆盖”默认勾选，覆盖保存资料与已打开非空白 pane；取消勾选只改新建默认。应用前检查本机设置未被其他窗口改动；复制继承；失败单独报告，不假定与共享资料跨文件原子保存。 |
| SF-T03 | [oshell-integration.sh](../validation/oshell-reference/shell-integration/oshell-integration.sh) 7–73；[RemoteHostIdentity.swift](../validation/oshell-reference/Sources/OShellCore/RemoteHostIdentity.swift) 9–14、29–39；TerminalPane 470、989–995 | 用户 source Bash/Zsh 脚本；交互、stdout TTY、未禁用才安装。提示符 OSC 777 的 payload 为 hostname/serverIP/interfaces，30 秒缓存，SSH_CONNECTION 变化重算；过滤 OSC 危险字符并限制 payload。Bash 保留字符串/数组 PROMPT_COMMAND，readonly 不改；Zsh 使用 add-zsh-hook 并返回成功保持后续钩子。应用注册 777 回调、校验前缀/UTF-8/长度、拒绝备用屏幕上报。脚本不安装到远端，不改历史文件/选项。 |
| SF-T04 | [TerminalHostname.swift](../validation/oshell-reference/Sources/OShellCore/TerminalHostname.swift) 7–27；TerminalPane 872–914、936–968、997–1002、712–716；WorkspaceController 940；[main.swift](../validation/oshell-reference/Sources/OShell/main.swift) 201 | 标题提取 user@host，提示符采用固定 bracket/colon/space 三种正则，长度 ≤2048 UTF-8 字节且无控制字符；OSC 7 只认 ≤8192 字节的 file URL 且非 localhost。输出显示后延迟扫描当前光标行，不在备用屏幕自动扫描。无信息显示未获取，不做本机 DNS 推测。OSC 7 路径只有主机等于配置主机或初次识别主机且通过 RemotePath 校验时才更新 remoteDirectory，不改变连接主机。 |
| SF-T05 | TerminalPane 317、916–987；RemoteHostIdentity 17–20、59–137 | 自动扫描等待 80 ms，提示符稳定后再等 300 ms；主机代次一致、未编辑、无传输、shell ready、该代次未尝试才自动探测。手动刷新仍检查 ready/无传输/非备用屏幕/非已识别认证提示；主动方式才发送 sh -c report。随机 token、epoch 和 4 秒超时约束响应；探测可能进历史。回显过滤只吞完整匹配的自有命令，未知输出/异常控制序列/超限缓存均放回。不能将这些有限规则称为任意 TUI/认证提示自动识别。 |
| SF-T06 | [LocalHostIdentity.swift](../validation/oshell-reference/Sources/OShellCore/LocalHostIdentity.swift) 9–60；RemoteHostIdentity 22–57；TerminalPane 386–415、884–935、997–1016 | Mac 已知 runtime/configured/.local 别名归一，不截断任意远端域名；本地 IP 从活动非回环接口取，排除链路本地，优先主接口后 IPv4。远端报告优先有效 SSH_CONNECTION endpoint，其次有效接口 IPv4/IPv6。识别 ssh/telnet/rlogin/exit/logout/su 或 Ctrl-D 时重置身份代次，不声称覆盖任意命令别名/容器。原连接结束后恢复本机身份与 OShell 提示符。主机提示不用于认证或重定向连接。 |
| SF-T07 | TerminalPane 中 TerminalTab.title 1085–1102、splitTitleDetail 1104–1110；PaneLayout.panes 1034；[TabStripView.swift](../validation/oshell-reference/Sources/OShell/TabStripView.swift) 280–285 | 按布局遍历顺序精确去重，显示前两个不同名字，每名截断为 32 字符加省略号，同名追加 ×n，余项为未展示的分屏总数 +N；不是剩余独立名字数。单屏直接使用 activePane.title。详情列全体分屏序号、协议、状态和当前标记；焦点只影响状态/详情，不改变聚合顺序。 |
| SF-T08 | [TabSessionInfo.swift](../validation/oshell-reference/Sources/OShell/TabSessionInfo.swift) 5–138；TerminalPane 346–395；[SSHTransportPeer.swift](../validation/oshell-reference/Sources/OShellCore/SSHTransportPeer.swift) 15–47；TabStripView 280–285、319–334 | 非激活、不可成为 key/main 的悬停面板；七个字段、单项复制、全部复制和详情弹窗。空值用未获取/不适用，复制按钮禁用；SSH 用户为空时不伪造解析结果。连接 IP 来自已认证后对 SSH 和代理子进程的只读 TCP 快照；最多 32 进程、3 层子进程、单进程 FD 数据有上限，只有唯一 TCP 对端才显示。可能为代理/跳板而不是显示主机；不等同持续主机监控。 |
| SF-T09 | TabStripView 73–115、158–170、329–330；TabSessionInfo 96–123 | 500 ms 悬停且应用活跃、窗口 key、无按下鼠标才显示；离开 250 ms 后关，进入卡片取消关闭计时。外部 left/rightMouseDown、keyDown、scrollWheel 关闭并返回原事件；卡片内操作保留。父窗口关闭/失焦/最小化、应用失活、标签隐藏/移除、拖动/菜单及字段/detail 改变均关旧卡；常规输入法键盘事件受 keyDown 覆盖，不是独立监听全部程序化 IME 状态。 |

### 本域修订依据

2026-10-24：将标题默认/旧资料回退、仅名称模式仍接收详情信息、自动与手动探测条件、探测超时、分屏余量计算及悬停字段来源写入原 SF-T 条目。[Shell 集成说明](../validation/oshell-reference/shell-integration/README.md) 中“默认接受集成”的叙述涉及旧版本，不能覆盖当前新建会话的本机 defaultMode；既有配置的兼容回退仍保留。

“原版可识别密码/TUI状态不误注入”是有限验收要求；没有把它扩大为任意认证界面/TUI 识别器。“不同平台均显示相同真实对端”需各平台实现证据，Darwin 的 proc_pidinfo 源码不能代替 Windows/Linux 实机结果。

### 已阅读文件范围

整文件已读：`SessionTitleSettings.swift`、`TabSessionInfo.swift`，Core `HostTitleMode.swift`、`LocalHostIdentity.swift`、`TerminalHostname.swift`、`RemoteHostIdentity.swift`、`SSHTransportPeer.swift`，`shell-integration/oshell-integration.sh` 及同目录 README。脚本未包含在“标题域六个 Swift 文件”的分类中，仍作为生产交付能力证据保留。

公共区段已读：TerminalPane 301–419、450–476、594–607、701–718、869–1019、1085–1118（PaneLayout.panes 在 1034 的定义另核对）；WorkspaceController 565–614、940 及 S/M 域已列区段；TabStripView 69–136、154–181、275–291、319–339。窗口/终端其余实现仍需在所属域审查，不能据此宣称全部文件已读。

## 密码与本机保护

入口为启动解锁、工具菜单主密码操作、会话编辑器及 SSH askpass。密码库为进程共享对象；凭据转换先生成配置副本，核对 revision 后调用共用保存入口。文件持久化与多节点事务仍在 Y 域核查，不能从“生成副本”推断所有磁盘写入已通过原子性验证。

| SF 条目 | 生产入口与源码定位 | 源码结论与关键边界 |
| --- | --- | --- |
| SF-P01 | [StartupProtection.swift](../validation/oshell-reference/Sources/OShell/StartupProtection.swift) 34–141；[MasterPasswordManagement.swift](../validation/oshell-reference/Sources/OShell/MasterPasswordManagement.swift) 7–91；[MasterPasswordProtection.swift](../validation/oshell-reference/Sources/OShellCore/MasterPasswordProtection.swift) 7–74；[CredentialManagement.swift](../validation/oshell-reference/Sources/OShellCore/CredentialManagement.swift) 12–54 | 独立加密 verifier 支持无会话时主密码保护；旧无 verifier 配置通过全部主密码凭据验证。新主密码至少 8 个 Swift 字符、最多 1024 UTF-8 字节，改密必须不同且二次输入一致。启用转换 local 凭据；改密转换 master 凭据、保留 local；移除验证旧密码后转 local，不是删除保存的密码。覆盖会话、内嵌代理、旧 FTP、共享代理。当前/待生效共享保护阻止改密/移除；已建立连接和旧导出文件不改变。 |
| SF-P02 | [LocalCredentialStore.swift](../validation/oshell-reference/Sources/OShellCore/LocalCredentialStore.swift) 8–74；[PasswordVault.swift](../validation/oshell-reference/Sources/OShell/PasswordVault.swift) 20–43、65–72 | 本机文件存随机 32 字节 base64 密钥及 UUID，凭据记录 localKeyID。读取拒绝 symlink、非普通文件、错误属主/权限、超 4096 字节、坏数据/错 UUID；显式修复权限只针对已验证自有文件。创建用私有锁文件、flock、临时文件和排他 rename；原文件存在即读取，不因损坏换新。已知凭据多个 localKeyID 拒绝保存；已知密钥缺失不生成替代。此密钥与数据同目录，不是 Keychain 或主密码同强度迁移保护。 |
| SF-P03 | [SessionCipher.swift](../validation/oshell-reference/Sources/OShellCore/SessionCipher.swift) 13–110；PasswordVault 142–176；[SessionEditor.swift](../validation/oshell-reference/Sources/OShell/SessionEditor.swift) 215–236；S07 的复制路径 | AES-256-GCM/PBKDF2-HMAC-SHA256，写入 600000 次，读取允许 600000–2000000 次；16 字节 salt，凭据非空且 ≤4096 UTF-8 字节、无 NUL/CR/LF。AAD 绑定资料 UUID、配置 host/port/user 和 envelope 的解析 host/user/port；名称/目录不绑定。SSH 解密先 ssh -G 并比较解析身份，显式代理/FTP 使用直接身份。编辑留空且目标/保护方式未变直接沿用；改字段/保护方式需解密重绑；改变别名解析时不能自动使用不匹配身份。归档重绑仍归 E02。 |
| SF-P04 | [AuthBroker.swift](../validation/oshell-reference/Sources/OShell/AuthBroker.swift) 10–146、248–300；[AuthIPC.swift](../validation/oshell-reference/Sources/OShellCore/AuthIPC.swift) 7–79；[OShellAskpass/main.swift](../validation/oshell-reference/Sources/OShellAskpass/main.swift) 8–20 | 全局 FIFO 认证队列等待无模态窗口，取消按 broker owner 清 pending/active；进入队列时检查客户端仍等待。socket 位于随机 0700 临时目录，0600 端点，检查同 UID 和随机 token；长度前缀 JSON 最多 16 KiB。服务读写超时 5 秒，helper 等回复 300 秒，并非无限等候。route/ready/none 协议通知绕过弹窗队列；代理请求按 proxyID 选择配置，未知 ID 拒绝。stop 关闭当前认证窗、清策略/待保存值并删除端点，回复时再检查 stopped。 |
| SF-P05 | AuthBroker 159–168、193–245；[TerminalPane.swift](../validation/oshell-reference/Sources/OShell/TerminalPane.swift) 586–606；[WorkspaceAuthSaving.swift](../validation/oshell-reference/Sources/OShell/WorkspaceAuthSaving.swift) 8–53 | 新 SSH 连接配置 PermitLocalCommand/OShellAskpass --session-ready；ready 回调才触发 pending 保存，不以终端文字猜成功。保存选项默认 off，仅目标 SSH 三种固定密码提示可启用，一次性密码及外部组禁用。重复目标密码提示清上次 pending。保存检查原资料是否仍存在及 kind/host/port/user/原密文一致；空用户名补解析值；主密码模式无缓存时直接不保存。异步加密后核对启动状态与 revision，成功一次提交。新的临时资料可加入，要求已存在的已删除资料不复活。 |
| SF-P06 | [PasswordEntryView.swift](../validation/oshell-reference/Sources/OShell/PasswordEntryView.swift) 5–102；SessionEditor 191–240；AuthBroker 225–245 | 显隐控件只持本次输入，不读取旧密码；切换先提交编辑器，无法失焦则撤销切换，保留 UTF-16 选择区和焦点，清空另一控件。明文编辑关闭自动引号/破折号/替换/拼写处理，结束恢复共享编辑器选项。编辑器勾选保存且留空沿用旧值；取消勾选才去除密文；取消整个编辑不返回修改。clear 清文本与编辑器状态，不宣称语言运行时所有字符串副本被物理擦除。 |
| SF-P07 | SessionCipher `SavedPasswordPolicy` 114–127；AuthBroker 73–77、170–218、248–291 | 自动密码只匹配解析 user@host 的三种精确提示，排除 confirm/none，去除包围 host 的方括号；同一策略消费一次后不重试。跳板独立策略、HTTP/SOCKS 独立单次尝试；非匹配挑战手动输入，主机指纹 yes/no 单独确认。显式 oneTimePassword 禁用成功保存。源码依靠文本和来源分派，没有通用 OTP/堡垒机挑战语义识别；自定义服务使用完全相同密码提示时无法证明能识别一次性性质，不能把“不误存任意验证码”冒充源码保证。 |
| SF-P08 | PasswordVault 13–18、73–103、157–176；[CredentialTask.swift](../validation/oshell-reference/Sources/OShell/CredentialTask.swift) 8–40；[WorkspaceController.swift](../validation/oshell-reference/Sources/OShell/WorkspaceController.swift) 93–133、915；StartupProtection 81–141 | 启动 decision 一次确定，未保护且配置可读可直接开放；受保护启动取消/失败返回 false，调用方负责退出。缓存 lock 只 generation++/master=nil，工具入口另取消 WebDAV 并清 store.masterPassword，不将 isSecurityUnlocked 改 false。旧异步解密成功仍回调，generation 仅阻止重缓存。轮换 replacement 映射更新仍打开连接的凭据快照；后台转换可取消，结果交付后再核对 revision/保存。取消不提交配置，但不概括为绝无辅助密钥文件创建。 |
| SF-P09 | StartupProtection 8–32；[SessionSyncConfiguration.swift](../validation/oshell-reference/Sources/OShellCore/SessionSyncConfiguration.swift) 8–28；[Models.swift](../validation/oshell-reference/Sources/OShellCore/Models.swift) 200、236、262 | 无主密码且未确认时显示高风险条；专注全屏隐藏。点击“我已知晓，不再提醒”直接置 preferences.masterWarningAcknowledged 并保存，无第二次确认。字段在配置中持久化，但应用共享 clientSettings 时显式保留 local 值，因此不是远端控制本机风险确认。隐藏提醒不改变密码保护。 |
| SF-P10 | [SecuritySettingsView.swift](../validation/oshell-reference/Sources/OShell/SecuritySettingsView.swift) 7–53；[BiometricUnlock.swift](../validation/oshell-reference/Sources/OShell/BiometricUnlock.swift) 22–245；[BiometricLocalStore.swift](../validation/oshell-reference/Sources/OShell/BiometricLocalStore.swift) 9–92；[AutomaticBiometricPrompt.swift](../validation/oshell-reference/Sources/OShell/AutomaticBiometricPrompt.swift) 7–81 | 启用须已存在 verifier、设备可用和手动输入当前主密码；后台验证/封装后检查 revision 与数据目录 scope。Secure Enclave P256 密钥受 biometryCurrentSet/ThisDeviceOnly 约束，经 ECDH/HKDF/AES-GCM 封装主密码；并非仅检查生物识别布尔值。本机私有 TouchID 目录，记录 ≤16 KiB、拒绝硬/软链接等异常；目录排除备份，scope 绑定规范化数据路径，binding 绑定 verifier。旧 Keychain 无交互读取迁移失败则要求手动解锁重启用。自动弹出一次（150 ms），输入/组字/关闭取消并拒绝迟到结果，可手动重试；主密码/指纹变更需重新启用。停用先删启用绑定，清理失败也不再自动使用。Desktop 排除苹果机制是项目约定。 |

### 本域修订依据与阅读范围

2026-10-24：补足 SF-P01–P10 的直接路径。修正 P05 的 SSH/一次性/缓存前提，P07 的有限文本识别能力，P08 的在途回调与启动门禁区别，P09 的直接确认按钮；保留固定编号。风险提示确认值确实保留本机，依据是同步应用函数的显式保留，不能仅因字段位于共享模型就判定其随同步覆盖。

整文件已读：`PasswordVault.swift`、`AuthBroker.swift`、`WorkspaceAuthSaving.swift`、`MasterPasswordManagement.swift`、`StartupProtection.swift`、`CredentialTask.swift`、`PasswordEntryView.swift`、`SecuritySettingsView.swift`、`BiometricUnlock.swift`、`BiometricLocalStore.swift`、`AutomaticBiometricPrompt.swift`；Core `SessionCipher.swift`、`MasterPasswordProtection.swift`、`CredentialManagement.swift`、`LocalCredentialStore.swift`、`AuthIPC.swift`、`SessionSyncConfiguration.swift`；`OShellAskpass/main.swift`。`SessionSyncConfiguration` 已整读，但本节只用其本机偏好保留行为，合并调用与整体同步仍归 Y。

公共区段补读：WorkspaceController 93–136、915，TerminalPane 580–650；SessionEditor 的密码保存段 191–240 已在 S 域整读，本域重新核对。`Package.swift` 已整读，确认生产目标默认排除 TestSupport，不能用测试入口推断应用功能。加密协议兼容、Touch ID 设备交互、模态/输入法行为均未在本次静态分析中执行实机验收。

## 更新与分发

原版运行入口为应用菜单“检查更新”、设置更新页及启动解锁后的 updater.configure。窗口协调器持有一个更新器，各窗口激活时切换当前 workspace。签名下载/安装使用捆绑 Sparkle 2.9.6 二进制；本节分析调用合同和 OShell 自有源码，不宣称已阅读该二进制的全部内部实现或已成功实际升级。

| SF 条目 | 生产入口与源码定位 | 源码结论与关键边界 |
| --- | --- | --- |
| SF-U01 | [UpdateSettingsView.swift](../validation/oshell-reference/Sources/OShell/UpdateSettingsView.swift) 7–35；[UpdateSource.swift](../validation/oshell-reference/Sources/OShellCore/UpdateSource.swift) 23–44；[AppUpdater.swift](../validation/oshell-reference/Sources/OShell/AppUpdater.swift) 43–74、103–106；[Info.plist](../validation/oshell-reference/scripts/Info.plist) 29–48；WorkspaceController 885–911 | 仓库支持 owner/repo 或 GitHub HTTPS，无用户信息、端口、query、fragment，尾 .git 可去除，名字按固定规则校验。偏好空时设置页和运行时均回退包内 OShellUpdateRepository；固定包为 oshell-desktop/oshell。两者均空时不建有效源并关闭自动检查，手动入口打开设置。自动检查默认 false；勾自动且文本空拒绝保存；更新忙不允许改仓库。UI 写“每天”，应用未另设定时器/间隔，调度交 Sparkle。 |
| SF-U02 | UpdateSource 7–21、37–53；[UpdateMetadataCache.swift](../validation/oshell-reference/Sources/OShellCore/UpdateMetadataCache.swift) 5–39；[GitHubReleaseUpdate.swift](../validation/oshell-reference/Sources/OShellCore/GitHubReleaseUpdate.swift) 9–87；AppUpdater 31–40、76–100；[ReleaseUpdateBridge.swift](../validation/oshell-reference/Sources/OShell/ReleaseUpdateBridge.swift) 78–114 | 从 owner.github.io/[repo]/updates/latest.json 取元数据，不走 latestReleaseURL 的 GitHub API。成功按源内存缓存 300 秒，失败默认 60 秒，Retry-After 数字限制 60–86400 秒，过期失败不假用旧成功。请求/资源/外层等待分别 20/25/28 秒，无 cookie/凭据/cache，拒绝重定向，最多 1 MiB。只接受正式 v三段数字版本、单一 feed item/enclosure、正整数构建、匹配 minimum、唯一完成资产和字节数；禁止 XML DTD/ENTITY。Intel 均选 10.13，Apple Silicon ≥13 选 arm64，否则 11 Universal；缺平台报错，不随意回退。预取禁重复；canCheck 与 sessionInProgress 不互为反义，已显示更新窗可唤前。 |
| SF-U03 | ReleaseUpdateBridge 10–75；AppUpdater 61–74、102–113；Info.plist 29–48；[publish-updates.py](../validation/oshell-reference/scripts/publish-updates.py) 20–57；[build-update-pages.py](../validation/oshell-reference/scripts/build-update-pages.py) 13–55；[Sparkle 来源](../validation/oshell-reference/Vendor/Sparkle/ORIGIN.txt) | 已有 32 字节 Ed25519 公钥；包要求 SURequireSignedFeed、SUVerifyUpdateBeforeExtraction，签名失败过期宽限 0；关自动下载/安装、系统 profiling 和 JavaScript。OShell 只检查签名标记/长度与结构，原签名字节不改，经随机路径的 127.0.0.1 临时端口交 Sparkle 实际验签；bridge 最多 2 并发、请求 8 KiB、响应 feed 32 KiB。DMG 必须同源仓库 Releases 对应版本和 flavor。发布脚本要求当前已验证 DMG 及相符哈希/构建/平台报告，再分别签包和 feed；Pages 生成独立验证 feed Ed25519，JSON 只是运输封套。 |
| SF-U04 | AppUpdater 110–124；[WorkspaceWindows.swift](../validation/oshell-reference/Sources/OShell/WorkspaceWindows.swift) 134–145；WorkspaceController 1113–1122；main.swift 102–105 | Sparkle 请求延迟重启时保存安装回调并尝试恢复；当前模态/附属 sheet 阻止立即安装，提示稍后继续。全窗口汇总 activeProcessCount/activeFileOperationCount，有活动才确认；取消保留 pending handler 和会话。确认后取消 WebDAV、关闭文件操作和终端，再调用 installer；无活动不另确认。didAbort 清 pending。没有 OShell 自建失败回滚/会话恢复实现，替换与重启交 Sparkle；“失败保持有效程序”仍需安装失败实机验收。 |
| SF-U05 | [package-release.py](../validation/oshell-reference/scripts/package-release.py) 60–207；[release_targets.py](../validation/oshell-reference/scripts/release_targets.py)；[installer_metadata.py](../validation/oshell-reference/scripts/installer_metadata.py)；[code_signing.py](../validation/oshell-reference/scripts/code_signing.py)；[build.sh](../validation/oshell-reference/scripts/build.sh)；[publish-legacy-updates.py](../validation/oshell-reference/scripts/publish-legacy-updates.py) 32–155 | 默认发布 legacy Intel 10.13 与 arm64 13；compat 11 Universal、intel 13、modern 13 Universal 为显式目标。默认可生成 PKG+DMG，dmg 模式仅 DMG，测试 app 不可打正式包。装配 helper/文件桥/SwiftTerm bundle/字体/许可/集成脚本，legacy 拷贝 Swift runtime；检查 Mach-O 架构、嵌套签名、DMG 完整性，写包内清单与哈希（外层 DMG 另算）。固定自签名证书与 designated requirement；无私钥拒绝静默换证书，显式开发 ad-hoc 受正式签名开关约束；无 Developer ID/公证。旧仓库和旧 Intel 文件名兼容通过重签 feed/同内容别名完成，不是新编译目标或客户端自动换源。 |
| SF-U06 | [StorageLocation.swift](../validation/oshell-reference/Sources/OShellCore/StorageLocation.swift) 7–18；Models.swift 328–338；main.swift 29–77；package-release 121–170；AppUpdater 110–124 | 默认用户数据目录为 Application Support/OShell，ConfigurationStore 在传入目录建 configuration.json，主程序包为安装替换对象。PKG payload 仅 OShell.app，DMG 包含 app、Applications 链接和分发说明/清单，不打入用户配置。此分离支持保配置设计，但不证明所有失败阶段均回滚成功。Windows 固定 EXE/ZIP、只替换程序和不覆盖便携配置来自本项目要求，不能标成 Swift 源码结论。 |
| SF-U07 | [AboutSettingsView.swift](../validation/oshell-reference/Sources/OShell/AboutSettingsView.swift) 7–93；[OpenSourceLicenses.swift](../validation/oshell-reference/Sources/OShell/OpenSourceLicenses.swift) 7–25；[Dialogs.swift](../validation/oshell-reference/Sources/OShell/Dialogs.swift) 39–82；package-release 76–87；Info.plist 5–28 | 设置的关于页显示 bundle 版本/构建/最低系统、当前编译架构和 GPL-3.0-only；文字可选择。主页用有效更新仓库（含内置回退），无法解析则禁用。许可/第三方声明打开包内文本，设置页路径失败提示；应用菜单的“关于”是简短版本/构建弹窗，可再进许可弹窗，后者只在资源 URL 存在时调用系统打开，没有同样的缺文件/打开失败提示，不能混为同一页面。Universal 包显示当前运行架构，不列完整切片目录。固定版本 0.2.92、构建 98；最低系统随 flavor，不统一写 13.0。 |

### 本域修订依据与阅读范围

2026-10-24：修正 U01“留空不联网”与内置回退的矛盾，区分 U03 结构预检和 Sparkle 验签，U04/U06 源码分工与失败恢复验收；补入 U05 已存在 PKG/固定自签名及正式目标范围，未追加功能编号。Apple 公证不是固定原版已完成能力。Apple Silicon 11/12 的选择器仍指向 Universal，但默认发布列表不生成该目标；明确记选择与发布列表差异，不静默增建产物，也不据此删去已存在的显式兼容目标。

整文件已读：`AppUpdater.swift`、`ReleaseUpdateBridge.swift`、`UpdateSettingsView.swift`、`AboutSettingsView.swift`、`WorkspaceWindows.swift`（146 行）；Core `UpdateSource.swift`、`UpdateMetadataCache.swift`、`GitHubReleaseUpdate.swift`；scripts 中 `build.sh`、`package-release.py`、`release_targets.py`、`installer_metadata.py`、`embed-sparkle.py`、`sign-app.py`、`code_signing.py`、`release_compilation.py`、`release_helpers.py`、`publish-updates.py`、`build-update-pages.py`、`publish-legacy-updates.py`、`update-signing-key.py`、`Info.plist`；Sparkle ORIGIN。仅阅读签名脚本，未访问私钥/证书密码或执行发布命令。

公共区段补读：main 1–110（后续菜单另核对更新项 134），WorkspaceController 885–911、983–984、1113–1122，Dialogs 39–92，Models 328–366，StorageLocation 1–22；Sparkle SPUUpdater.h 的 canCheck/sessionInProgress 属性说明及 192–227 的调度设置说明。构建缓存算法与测试脚本不作为新增产品功能，尚未因此标记整仓所有脚本已审；Sparkle 为固定二进制依赖，内部恢复行为没有本轮源级/运行证据。运行更新检查、下载安装、最低系统启动和配置保留均待实机验收。

## 连接与代理

主路径为 WorkspaceController.makePane → 配置代理解析 → TerminalPane.start → 系统 OpenSSH/本地 zsh。显式代理链由 OShellProxy helper 递归构造；系统 SSH 配置语法和 SSH 协商交外部 OpenSSH。认证 broker 已在 P 域核查；文件窗口、SFTP/SCP 中本节只闭合连接与租约交接，不据此宣称文件操作域全部读完。

| SF 条目 | 生产入口与源码定位 | 源码结论与关键边界 |
| --- | --- | --- |
| SF-C01 | [Models.swift](../validation/oshell-reference/Sources/OShellCore/Models.swift) 85–143；[TerminalPane.swift](../validation/oshell-reference/Sources/OShell/TerminalPane.swift) 569–615、837–861、1004–1018；[WorkspaceController.swift](../validation/oshell-reference/Sources/OShell/WorkspaceController.swift) 516–522、566–596、854–863；[OpenSSHCapabilities.swift](../validation/oshell-reference/Sources/OShellCore/OpenSSHCapabilities.swift)；[OShellSSH/main.c](../validation/oshell-reference/Sources/OShellSSH/main.c) | -tt、配置 port/user/-i、ConnectTimeout=15，OpenSSH 自行使用 Agent/客户端配置。SSH 环境去 LANG/LANGUAGE/LC_*，设置 TERM（默认 xterm-256color）、COLORTERM，helper 置 PATH 前。OpenSSH <8.4 使用 C wrapper fork/setsid，保留 PTY 流且脱离控制 TTY，转发 HUP/TERM/INT/QUIT/WINCH/CONT；观察退出后禁转发再 reap 防 PID 复用。普通重连先确认活动进程，采用 old.profile 快照；可刷新匹配保存凭据与当前共享代理，未重读全部保存选项。退出保留页、退出码及本机工具提示符；不自动重连。 |
| SF-C02 | Models 110–111、136–141；WorkspaceController 370–375；AuthBroker 221–224、281–291（P04） | StrictHostKeyChecking=ask、显式 UserKnownHostsFile，路径对 %/引号/反斜线转义。无共享保护在数据目录 known_hosts；共享保护用本机 WebDAV 根下 host-trust/<数据文件路径摘要>/known_hosts，避免随共享资料交换信任。首次 yes/no 确认进入当前目标/跳板队列；实际主机密钥验证由 OpenSSH，未新增指纹管理 UI。系统其他信任源没有被概括为禁用；标题/自报 IP 不参与此校验。 |
| SF-C03 | [SessionEditor.swift](../validation/oshell-reference/Sources/OShell/SessionEditor.swift) 21、66、144、203；Models 129–132 | 显式 legacySSH 追加 HostKeyAlgorithms/PubkeyAcceptedKeyTypes 的 ssh-rsa；Kex 为 diffie-hellman-group14-sha1、diffie-hellman-group-exchange-sha1、diffie-hellman-group1-sha1；Ciphers 为 aes128-cbc/aes192-cbc/aes256-cbc/3des-cbc；MACs 为 hmac-sha1/hmac-sha1-96。是追加列表，是否可用仍取决于系统 ssh。与 OpenSSHCapabilities 的客户端 askpass 版本分支不是同一开关。 |
| SF-C04 | TerminalPane 153–167、448–462、583–611；[MacLocalTerminalView.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/Mac/MacLocalTerminalView.swift)；[Pty.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/Pty.swift) 58–136；[LocalProcess.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/LocalProcess.swift) 89–220、227–254、282–420、522–633 | 原版 local 为 /bin/zsh -l、NSHomeDirectory、LC_CTYPE=UTF-8；实际生产走 forkpty/execve，swift-subprocess 分支 #if false 不可计为能力。尺寸使用 TIOCSWINSZ，像素尺寸考虑 backingScaleFactor。持续独立写通道保序，8 KiB 读取、1 MiB/256 KiB 输出背压水位与约 1 ms UI 分片；进程退出后仍读到 EOF。terminate 关闭 I/O、发 SIGTERM、后台 waitpid，未实现全进程树强杀保证。fork helper 返回 nil 时 startProcessWithForkpty 无 else 失败通知；需把创建失败反馈列为待验证限制，不能从 exec 退出码路径推断所有启动失败均反馈。 |
| SF-C05 | [ProxyManager.swift](../validation/oshell-reference/Sources/OShell/ProxyManager.swift) 6–158；[ProxyCatalog.swift](../validation/oshell-reference/Sources/OShellCore/ProxyCatalog.swift) 5–46；[ConnectionOptions.swift](../validation/oshell-reference/Sources/OShellCore/ConnectionOptions.swift) 35–84 | 管理列表新增/双击编辑/删除；先检查直接会话和下级代理引用再允许确认删除。目录上限 2000，名称非空 ≤256 UTF-8 字节；host/user 各 ≤255、私钥路径 ≤1024，HTTP 用户不含冒号。跳板认证自动/Agent、密码、私钥；切类型仅在原默认端口未改时换 22/1080。私钥模式关闭密码保存，SOCKS4/4A 不保存密码。留空保留要求 host/port/user/kind 不变；改目标须重输或取消保存，区别于会话编辑器解密沿用规则。转保护模式可解密重绑。走共用 saveConfiguration，无独立编辑 revision 校验，不能额外宣称代理弹窗已覆盖所有并发改写情况。 |
| SF-C06 | [OShellProxy/main.swift](../validation/oshell-reference/Sources/OShellProxy/main.swift) 34–123、124–155、175–236 | SOCKS5 只提议一种方法：空用户无认证，非空用户用户名/密码，各 ≤255 字节；支持目标 IPv4/IPv6/域名。SOCKS4 本机 AF_INET 解析；4A 固定 0.0.0.1 后发送域名及 user ID，无密码。HTTP CONNECT 发送 Host 和可选 Basic，头 ≤16384 字节，HTTP/1.x 状态必须 200，无 Digest/NTLM/TLS。直连代理地址多候选 socket，各候选 15 秒 connect/握手；链内等上游认证为 300 秒。非阻塞 poll 双向中继、64 KiB 排队阈值、stdin EOF 后半关闭；不是 UDP/BIND 实现。错误走 stderr，清理本通道/上游子进程。 |
| SF-C07 | ProxyCatalog 19–76；Models 55–60、77–80、101、115–128；OShellProxy 156–164、175–229；AuthBroker 149–157（P04） | route 先拒重复 ID，再查缺失、循环、最多 8 级，倒序得到上游→目标顺序。共享代理仅用于 SSH/SFTP，不能与内嵌代理/旧 jumpHost 同设。旧内嵌代理按原 ID 迁移，若同 ID 内容不同拒绝；旧 jumpHost 保留 -J，不虚称全部自动迁入代理目录。helper 从 broker 读取去密文路由；非跳板以 socketpair 递归上级 helper，跳板以 SSH -W 递归 ProxyCommand，各 hop 通过 proxyID 取自身密码。不存在/未解析路由失败不降级直连。 |
| SF-C08 | Models 107–128；SessionCipher `SSHIdentity.resolve` 18–33；OShellProxy 188–199；ProxyCatalog 50–54；ConnectionOptions 79–84；SSHConnectionGroup 44–58 | 身份解析由 ssh -G，普通连接未覆盖的 SSH 配置也由系统解析；OShell 没有自行实现 Include/Match 等完整解析。显式 jumpHost 用 -J；共享链 helper 取代 ProxyCommand。链内 jump 禁 LocalCommand/ControlMaster/TTY，密码模式禁公钥，私钥模式 IdentitiesOnly+publickey，上级 helper 递归，否则 ProxyCommand=none。命令路径参数 quote 且预转义 %，目标使用 SSH 的 %h/%p。复用通道另用 -F /dev/null 与 false ProxyCommand 禁止自行开新路由；共享链身份检查无 helper 时设 false，仅供 -G 检查。 |
| SF-C09 | ConnectionOptions 6–33；Models 97–100、134–135；SessionEditor 242–270；[RemoteFiles.swift](../validation/oshell-reference/Sources/OShell/RemoteFiles.swift) 139–145 | 编辑三类规则、启停和说明，默认本机环回 8080 至 localhost:80。端口 1–65535，host 采用固定字符白名单；IPv6 生成括号形式。动态不验证/使用目标地址；只验证启用规则，local/dynamic 同侧监听去重，remote 分侧。启用项添加 -L/-R/-D 和 ExitOnForwardFailure=yes；远程监听仍受 GatewayPorts。文件 SSH 清空配置隧道并 ClearAllForwardings，不额外占用终端端口。 |
| SF-C10 | [SSHTunnelOwnership.swift](../validation/oshell-reference/Sources/OShell/SSHTunnelOwnership.swift) 9–66；TerminalPane 569–585；SSHConnectionGroup 28–36、62–86 | 进程单例按 profile UUID、owner、controlPath 记录 starting/established/closing；未认证也预占。其他 owner 仅在 established 且 socket 可确定不存活时回收，否则拒绝占用。重复打开清 tunnels 并 ClearAllForwardings，新连接可独立存在；不会自动迁移隧道。检查 socket 类型/属主、非阻塞 connect，ENOENT/ECONNREFUSED 才认为失效，未知错误保留。最后持有者关闭执行 ssh -O exit，若仍活着保留目录/占用，不据页面关闭直接认定端口已释放。 |
| SF-C11 | [SSHConnectionGroup.swift](../validation/oshell-reference/Sources/OShell/SSHConnectionGroup.swift) 10–105；WorkspaceController 510–545；[RemoteFileWindow.swift](../validation/oshell-reference/Sources/OShell/RemoteFileWindow.swift) 128–143；[RemoteFileSession.swift](../validation/oshell-reference/Sources/OShell/RemoteFileSession.swift) 122–154、464–472；RemoteFiles 191–220、456–457；[FTPAndSCP.swift](../validation/oshell-reference/Sources/OShell/FTPAndSCP.swift) 143–189 | 每组 mkdtemp 私有 control path，首连 ControlMaster=yes/ControlPersist=60；复用核对 profile ID/host/user/port、authenticated、未 stopped、私有自有 socket。clone 用 -F /dev/null、BatchMode=yes、false ProxyCommand、ClearAllForwardings、ConnectTimeout=3/单次尝试，失效不重新认证。复制会话开新连接；复制 SSH 通道明确传组；普通 replica 仅 external 组复用。终端、文件页、SFTP backend、SCP 各有独立 holder/lease，释放幂等；取消文件操作释放 backend 但文件页 lease 留到关闭，允许稍后重连尝试。ControlPersist/服务器仍会终止空闲传输，不能称仅持有对象就永久保活。 |
| SF-C12 | ConnectionOptions 85–110；Models 110；TerminalPane 424–445、583–595；OShellProxy 190–197、230–231；WorkspaceController 516–522、854–863 | 默认协议心跳 30 秒/最大 3 次、TCP 开；范围 interval 1–86400、maxMissed 1–100，关闭心跳传 0。新终端连接将值交 OpenSSH；TCP 开关亦传 helper，跳板自有协议心跳采用其构造 profile 默认值，不自动承诺所有跳逐项复制目标配置。协议心跳不经过 PTY 用户输入，断线决定由 SSH。普通重连沿用分屏快照，目录重新打开读取保存资料；即时属性不能改已运行 SSH/TCP 参数。 |
| SF-C13 | [LiveSessionProperties.swift](../validation/oshell-reference/Sources/OShell/LiveSessionProperties.swift) 7–90；ConnectionOptions 95–115；TerminalPane 153–167、422–445、678–685、916–934；[RemoteHostIdentity.swift](../validation/oshell-reference/Sources/OShellCore/RemoteHostIdentity.swift) 17–21 | 空闲间隔 1–86400，五种转义 n/r/t/e/反斜线，解码非空 ≤1024 UTF-8 字节且无 NUL。就绪 SSH 才计时，初次 interval、后每 min(5,interval) 秒轮询；实际输入输出重置，协议心跳不会。userEditingLine 由字节状态跟踪，3/4/10/13/21 清除，其余置 true；传输、备用屏幕、在途探测、固定认证提示正则阻止发送，非任意 TUI 检测。已有保存资料的同时保存默认 on，无保存资料隐藏；再定位 paneID/就绪/保存 ID，先只保存三个 idle 字段，再重置当前 pane 定时器；其他连接不改。 |
| SF-C14 | TerminalPane 346–395；[SSHTransportPeer.swift](../validation/oshell-reference/Sources/OShellCore/SSHTransportPeer.swift)（已在 T08 全读）；TabSessionInfo（T08） | 分开显示配置地址/端口、主机名/自报 IP、连接组实际 TCP 对端；详情注明代理/跳板/内层跳转可能不同。认证 ready 后取初始 SSH/helper PID 只读快照，回主线程检查源 pane/PID 未变后写入原 group。未知不以 DNS 推断，主机名/自报信息不改 group 身份或文件目标。这里只描述终端连接信息，文件页的地址解析展示另归 F，不能把不同字段来源混为一谈。 |

### 本域修订依据与阅读范围

2026-10-24：收紧 C01/C12 的重连快照含义、C05 代理改目标时的密码规则、C06 具体认证/DNS 支持、C10 隧道预占与连接复用区别、C11 两类复制入口、C13 有限暂停检测。全部使用原 SF 编号。C04 的 forkpty 创建失败没有显式失败回调是固定生产分支可见的限制，尚未实机复现，不据此擅改源码；归已有“创建失败反馈”验收，不新增功能。若复刻有意改善原版失败反馈，应记录差异，而非删掉正常本地终端能力。

整文件已读：Core `ConnectionOptions.swift`、`ProxyCatalog.swift`、`OpenSSHCapabilities.swift`、`SSHEnvironment.swift`；应用 `ProxyManager.swift`、`SSHTunnelOwnership.swift`、`SSHConnectionGroup.swift`、`LiveSessionProperties.swift`；`OShellProxy/main.swift`、`OShellSSH/main.c`；SwiftTerm `Mac/MacLocalTerminalView.swift`（223 行）、`Pty.swift`（136 行）、`LocalProcess.swift`（646 行，明确区分 #if false）。这些依赖只证明生产 PTY 路径，不代表 SwiftTerm 全部终端兼容已分析。

公共区段补读：Models 1–172；TerminalPane 145–179、222–301、338–476、569–615、671–711、831–863、916–935、1003–1019；WorkspaceController 368–376、510–614、850–869；RemoteFileWindow 128–145，RemoteFileSession 117–190、456–489，RemoteFiles 139–151、191–223、451–458，FTPAndSCP 143–189。SessionEditor 已在 S 域整读，认证生命周期已在 P 域闭合。没有执行真实 SSH/代理服务器、PTY 交互、租约并发或隧道端口实机测试。

## 快捷操作

这些入口共享本窗口 `inputPanes`，但发送语义不同：快速栏筛选可输入目标，撰写窗要求当前选中目标全部可输入，同步键盘在任一目标不可输入时停止整组。不能用其中一条路径的保护推断其他路径。[WorkspaceController.swift](../validation/oshell-reference/Sources/OShell/WorkspaceController.swift) 48–68、321–327、609–613 把控件和终端回调接到下列生产实现。

| SF 条目 | 生产入口与源码定位 | 源码结论与关键边界 |
| --- | --- | --- |
| SF-O01 | [InputTools.swift](../validation/oshell-reference/Sources/OShell/InputTools.swift) `QuickCommandManager`；[OperatorModels.swift](../validation/oshell-reference/Sources/OShellCore/OperatorModels.swift) `QuickCommand` 6–18；[WorkspaceInput.swift](../validation/oshell-reference/Sources/OShell/WorkspaceInput.swift) `showQuickCommands`、`chooseQuickCommand`、`rebuildCommandMenu` | UUID 身份，名称/组/文本/appendReturn；名称去空白后非空、文本非空且 ≤1 MiB/无 NUL。新建/编辑验证后保存，删除确认后按 ID 删除，取消不保存。管理器双击或按钮、主菜单都填入撰写窗；保存命令按组名排序、组内保持配置顺序，不立即发送。菜单 ID 在调用时重新查配置；管理器数据动态来自 workspace。未实现变量展开或脚本执行引擎。 |
| SF-O02 | [QuickSendBar.swift](../validation/oshell-reference/Sources/OShell/QuickSendBar.swift) 7–162；[WorkspaceQuickSend.swift](../validation/oshell-reference/Sources/OShell/WorkspaceQuickSend.swift) `sendQuickCommand`、`composeQuickCommand`、显隐/焦点 | 单行 ≤65536 UTF-8 字节且无 NUL，纯空白不发送；多行转入撰写窗，原栏不按发送成功清空。多行插入/粘贴合并当前 UTF-16 选择区及未选草稿。历史为本窗口对象内存，最多 50 条、合计 65536 字节，单条 >8192 不记；按文本和回车标志组合去重。历史/保存命令只填入，上下翻回可恢复草稿。成功发送清空输入、重置追加回车；没有对所有按钮操作另行强制焦点的代码。Esc 回当前终端，组字期间 Enter/上下不发送或翻历史；深度专注中显隐/定位无效。 |
| SF-O03 | WorkspaceInput 9–22、`sendComposed`；InputTools `CommandComposer`；OperatorModels `InputText` 109–115；WorkspaceQuickSend `composeQuickCommand` | 撰写窗初次目标为空时取当前 pane；填入已存命令保留非空旧目标；从快速栏转入则用当前可输入目标 ID 快照替换。内容 ≤1 MiB/非空/无 NUL，点击发送直接执行，不再弹确认；当前已选对象必须非空且全部可输入。收起只隐藏，正文和目标保留。发送不使用 bracketed-paste：归一化 CRLF/CR、删除 ESC、LF 变 CR，末尾原有换行则不重复追加回车。此路径独立于同步输入；不是运行时脚本解释器。 |
| SF-O04 | [QuickSendScope.swift](../validation/oshell-reference/Sources/OShellCore/QuickSendScope.swift) 6–19；WorkspaceQuickSend 15–49、`chooseQuickSendScope`；WorkspaceController 62–68、135、1096–1111 | 固定 rawValue：current=0/tab=1/all=2/visible=3/selected=4/currentGroup=5/selectedGroups=6。当前组先 activeTabGroupID、再所选标签所在组；无自定义布局时所有输入 pane 构成默认组。组范围包含隐藏组；visible 检查 view.window、隐藏祖先和非空 visibleRect。保存模式先于更新运行选择，取消/保存失败不改变选择。只持久化模式，选中 ID 集合在窗口内存中，重开选定模式时集合为空、不回退全部。receiveConfiguration 不给 quickSendScope 重新赋值，因此其他窗口收到偏好不等于本窗口运行模式立即切换。 |
| SF-O05 | WorkspaceQuickSend `refreshQuickSendBar`、`quickSendTargets`；WorkspaceInput `refreshOperatorState`；[TerminalPane.swift](../validation/oshell-reference/Sources/OShell/TerminalPane.swift) 256–264；WorkspaceController 139–143、598–604、953–955、1058–1082 | 每次发送重算范围及 acceptsManagedInput；刷新剔除不存在的 pane/组 ID，UI 提示 ready/skipped。已启动、进程运行、无传输，且本地或 SSH ready 才接受；未关闭的 ended 标签仍可接收本地提示符输入，交互式本机工具阻止管理输入。显式选择不加新 pane；选定组的后续成员按当前组动态包含。可见范围在剪裁视图滚动时异步刷新。跨窗迁移从原窗口三种显式目标集中减去 ID，重新绑定目的窗口回调；不会把旧窗口选择隐式带过去。 |
| SF-O06 | WorkspaceInput `configureSyncInput`、`refreshOperatorState`、`routeKeyboard`、`pasteText`；TerminalPane 27–29、37–42、154–167、259–264；InputTools `targets`、`previewPaste` | 显式选择至少两个可输入 pane；没有选择则停止。刷新或输入时不足两个/任一不可输入，清空整组；路由失败返回 false 让本次源输入按自身路径继续。只有 keyDown/keyUp/insertText 的 userInputDepth 内数据触发键盘同步；sendManaged 直写进程不再次广播，程序应答/图片/传输不进入此路由。文字粘贴单独使用选中 ID，≤1 MiB/无 NUL；可选多行预览并编辑，取消无字节，“下次不再预览”先保存成功才继续。对话框后重查目标可输入及 ID 仍存在，不满足则整次拒绝；各目标按自身 bracketed-paste 模式发送。交互式本机工具仅可走自己的粘贴路径。 |
| SF-O07 | [HighlightManager.swift](../validation/oshell-reference/Sources/OShell/HighlightManager.swift) 7–25、54–148；OperatorModels 19–92；TerminalPane 266–271、890、914；[MacTerminalView.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/Mac/MacTerminalView.swift) 322–325；[AppleTerminalView.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/Apple/AppleTerminalView.swift) 1103–1119、1210–1218 | 集合可新建/改名/确认删除/启用/关闭；规则可增改删，编辑字段为 pattern/regex/caseSensitive/enabled/color，新增第 33 条拒绝。默认固定 ID 的错误/警告/主机名三规则；`${hostname}` 用当前远端 hostname 或标题替换，正则场景转义主机名。匹配器先取前 32 条再过滤启用，≤4096 UTF-16 行、≤256 非空匹配、约 2 ms 进度检查，256 个文本缓存满后全清。按显示行而非整份日志匹配，重叠同一 cell 首条优先；仅构建显示属性，不修改原始缓冲/复制/日志。matcher 优先非空 foreground，否则 color；background/bold 不参与。NSColor(hex:) 只接受六位 RGB，而规则验证允许八位；八位值可能合法存储却没有着色，不能声称透明高亮已支持。 |
| SF-O08 | [KeyboardShortcuts.swift](../validation/oshell-reference/Sources/OShellCore/KeyboardShortcuts.swift) 全文件；[ShortcutSettingsView.swift](../validation/oshell-reference/Sources/OShell/ShortcutSettingsView.swift) 7–170；[ShortcutRuntime.swift](../validation/oshell-reference/Sources/OShell/ShortcutRuntime.swift) `install`、`ShortcutSystemConflicts`；[Dialogs.swift](../validation/oshell-reference/Sources/OShell/Dialogs.swift) 37–82；WorkspaceController 425–429、885–909 | 固定动作枚举，按物理 keyCode+自有四位修饰掩码；非功能键至少含 Command/Option/Control，F1–F20 可单独使用。override=nil 禁用，删除 override 恢复默认，自定义一项替换该动作所有默认别名；部分新引入默认遇已有定制会让位。录入器只拦当前窗口按键、忽略重复、Esc/失焦/外点结束，不触发菜单或终端。设定暂存，外层应用后保存；内部冲突始终验证，系统冲突在候选及变更绑定应用时检查，未改绑定不强制重新阻止应用。读取 Carbon 已启用热键、系统/应用菜单映射，加固定保留组合；不可读只提示，不能检出所有第三方。菜单显示首绑定，运行支持全部；侧栏 override 从共享偏好剥离后本机保存。 |
| SF-O09 | [WorkspaceWindow.swift](../validation/oshell-reference/Sources/OShell/WorkspaceWindow.swift) 8–47；ShortcutRuntime `dispatchShortcut`；[CommandTextView.swift](../validation/oshell-reference/Sources/OShell/CommandTextView.swift) 全文件；[PopupKeyboard.swift](../validation/oshell-reference/Sources/OShell/PopupKeyboard.swift) `PopupWindow`、`PopupAlert`、`PopupKeyboard` | 工作区只在 keyWindow、无 sheet/modal、解锁且绑定有效时分派，识别但当前不可用的动作消费掉不进入 PTY；专注全屏/侧栏重复键被消费，深度专注固定键仅无配置冲突时启用。普通 Esc 先让 IME 处理，再可关闭终端搜索，不通用关闭主窗或退出专注。命令编辑器禁富文本/智能引号/破折号/自动替换/校正等但保留原生 IME。弹窗本地监视器让位快捷键录入及正在跟踪的菜单；Esc 取消已注册 alert（无取消按钮则 abort，不点唯一肯定按钮）、取消文件面板、关闭普通辅助窗；远端文件窗无组字时最小化。文件/同步状态窗 Cmd+W 关闭自身且取消关闭提示也消费事件。Ctrl+A 仅在活动可编辑密码 field editor 且未组字时全选，不改变其他文本/终端 Ctrl+A。 |

### 本域修订与源码限制

2026-10-24：纠正 SF-O07 旧清单把 `background`/`bold` 模型字段视为已经实现的显示能力；保留这些字段的共享兼容责任于 Y17，不要求移除已有数据。八位色值与六位 UI 转换不一致也记录在 O07；若复刻中选择修正，应标记与固定原版的差异，不将其说成原版已支持。

SF-O06 补齐实际存在的文字粘贴同步及整组停止规则，SF-O03 明确点击发送没有第二层确认，SF-O02 不再泛称所有发送后强制恢复焦点。SF-O04 区分持久化模式、窗口内选定身份和配置发布后的运行模式；不是以共享字段存在推断所有窗口即时联动。以上均归原 SF 条目，没有新增功能编号。

SF-O06 的限制还需区分校验时点：`pasteText` 对进入预览前的文字检查 1 MiB/NUL，预览编辑完成后只重查内容非空及目标状态，没有再次检查编辑后大小/NUL。不能把入口校验说成覆盖整个编辑后发送路径；这是固定源码的已知限制，后续若修正归本条记录差异，不新增功能。高亮构造显示属性时，选中颜色及闪烁隐藏可覆盖规则前景色（AppleTerminalView 1219–1235），不影响原文本。

约定的有限验收覆盖：命令三种填入入口、单行成功/失败与多行转入、七范围各一组确定对象、重开选定模式空集合、加入/移出与不可输入对象、两目标同步及任一目标失效、粘贴预览取消/目标变化、重叠与超预算高亮、定制/清除/恢复/冲突快捷键、IME 与菜单/弹窗仲裁。它们是验收要求，不是本轮运行结果，不要求枚举全部键盘布局、正则或窗口组合。

### 已阅读文件范围

整文件已读：应用 `WorkspaceQuickSend.swift`、`WorkspaceInput.swift`、`QuickSendBar.swift`、`InputTools.swift`、`HighlightManager.swift`、`ShortcutSettingsView.swift`、`ShortcutRuntime.swift`、`CommandTextView.swift`、`PopupKeyboard.swift`、`WorkspaceWindow.swift`；Core `QuickSendScope.swift`、`OperatorModels.swift`、`KeyboardShortcuts.swift`。

公共区段补读：WorkspaceController 45–70、130–150、315–330、423–434、590–614、884–914、940–960、1058–1115；TerminalPane 8–110、147–171、251–280（主机名变动重建高亮的 890/914 已在 T 域读过）；Dialogs 21–91；SwiftTerm MacTerminalView 318–337、AppleTerminalView 1096–1135、1186–1238。还需 I 域核查完整字符属性/渲染链；不能据此宣称 SwiftTerm 整体已读。本轮未执行 AppKit 键盘、输入法、多窗或正则计时实测。

## 外部互通与迁移

原生归档、第三方会话导入、外部即时启动是三个独立入口。归档携带资料与依赖；第三方导入先生成报告再一次保存；启动器直接开临时终端/文件标签，不走资料库批量导入。下表不将说明文档中的转换建议或文件模型字段存在视为额外生产入口。

| SF 条目 | 生产入口与源码定位 | 源码结论与关键边界 |
| --- | --- | --- |
| SF-E01 | [SessionManager.swift](../validation/oshell-reference/Sources/OShell/SessionManager.swift) 388–407；[SessionTransfer.swift](../validation/oshell-reference/Sources/OShell/SessionTransfer.swift) `export`、`importSessions`、`importArchive`；[SessionArchive.swift](../validation/oshell-reference/Sources/OShellCore/SessionArchive.swift) 6–68；[SharedConfigurationProtocol.swift](../validation/oshell-reference/Sources/OShellCore/SharedConfigurationProtocol.swift) 7–86；[SharedConfigurationContract.swift](../validation/oshell-reference/Sources/OShellCore/SharedConfigurationContract.swift) 996–1045 | 归档读取 v1–v4，输出按代理/引用存在选择 v4/v3/否则 v2。v1 不含文件协议、v3 才有引用、v4 才有共享代理；≤16 MiB、2000 profiles、4000 directories/links，重复身份、失效引用及不规范目录拒绝。合同先处理已知旧字段再检查未知字段/类型/枚举，之后 Codable 和模型验证。右键 exportSelection 只读 selectedRow：单会话、单引用连源资料，或目录子树连被引用源；不是任意多选集合。全部导出也只提取依赖代理。原生导入单文件整体预览，可选密码保护；不自动连接，任务取消/失败或 revision 改变不保存，成功一次 saveConfiguration。 |
| SF-E02 | SessionArchive 69–152；SessionTransfer `export`、`importArchive`；[ProxyCatalog.swift](../validation/oshell-reference/Sources/OShellCore/ProxyCatalog.swift) `required`（C 域已读）；[PrivateFile.swift](../validation/oshell-reference/Sources/OShellCore/PrivateFile.swift)（M 域已读） | 共享代理、内嵌代理、会话生成新 UUID，重新映射 upstreamID/proxyID/源引用；普通目标目录加前缀，同名会话/引用加“导入副本”后缀。Links 目标创建引用，源资料单独保存；空目录和引用目录整理，保留当前 Links 显隐。三类密码按源身份解密、目标身份重新加密，保留解析身份 envelope.identity；无密码导出同时清三类密文。纯主密码导出可直接携旧密文，不强制再次解锁；含任何本机 keyID 则要求文件新密码和源解密密钥，后台重包后禁止本机 keyID 导出。目的可用本机保护或主密码，已有主密码不可选择降为本机；新启用主密码还迁移既有本机凭据。只携私钥路径、不携私钥/known_hosts/终端历史/应用偏好；文件用 PrivateFile 写入。 |
| SF-E03 | [ThirdPartySessionImporter.swift](../validation/oshell-reference/Sources/OShellCore/ThirdPartySessionImporter.swift) 8–12、32–235、238–290；[ThirdPartyImportPreview.swift](../validation/oshell-reference/Sources/OShell/ThirdPartyImportPreview.swift) 全文件；SessionTransfer `importExternal`、`importExternalReport` | SecureCRT 输入多文件 XML/INI 或目录（若含非链接 Sessions 子目录则选它），目录中只读 INI。支持 UTF-8/UTF-16 BOM、部分无 BOM UTF-16 推断和 GB18030 回退。INI 只接收固定字段，D 类型十六进制转数值；XML 要求 VanDyke/Sessions，拒 DTD/实体，禁外部实体，深度 ≤64/元素计数 ≤1000000/字段 ≤8192 字节，重复连接字段拒绝。迁移 SSH/SSH2/SFTP/FTP、地址/用户/端口、来源目录与名称、可用 POSIX 私钥路径、已知协议保活；禁空闲字符串。无主机的 default 项略过，非法协议/单项字段等记 skipped，结构或总量致命错误整次中止。预览仅报告全部行及问题，没有逐行勾选；确认后导入全部可用报告，保留旧库且不连接。 |
| SF-E04 | ThirdPartySessionImporter 135–222；[XshellPasswordImport.swift](../validation/oshell-reference/Sources/OShellCore/XshellPasswordImport.swift) 9–95；SessionTransfer `importExternalReport`、`promptXshellMaster` | XSH 按 section/key 白名单解析，接受 [Connection]，重复已识别字段跳过该文件；基础协议/字段规则同 E03，额外识别 TCP KeepAlive。密码只暂存在报告的 XshellSavedPassword，描述/调试字符串不输出密文；默认有记录时勾选迁移。解密格式是 base64 包：RC4 密文 +32 字节 SHA-256 明文摘要，RC4 key=SHA-256(master UTF-8)，摘要用累计异或比对；master ≤4096 字节，包 >32 且 ≤4128、编码 ≤8192，明文 UTF-8 非空/无 NUL CR LF。成功后按目的本机/主密码重新 AES 封装，不写厂商密文或原主密码。参与迁移记录解密失败/取消整体不保存；结束还检查启动解锁与 revision。仅保留基础信息时不解密源密码；单项解析跳过不等于整份报告失败。 |
| SF-E05 | XshellPasswordImport `importingPasswords` 54–95；SessionTransfer `importExternalReport` | fillMissingOnly 从现配置副本开始，不增加资料。仅遍历报告里有密码的项，按 name/kind/host/port/username 精确匹配，若有相同 group 或 group 后缀则优先该集合，再限制当前导入目录；无独立来源标记或源 UUID 身份证明。>1 候选整次失败；无候选累计 unmatched；同目标被两源重复匹配失败；已有密码累计 preserved 并跳过解密。只对空密码唯一目标解密/加密，其他 ID/属性保留；零补充不调用保存。因此不能泛称“本批全部密文都验证过”：已有密码和未匹配来源均可能未解密。 |
| SF-E06 | [SessionImportZip.swift](../validation/oshell-reference/Sources/OShellCore/SessionImportZip.swift) 7–81；ThirdPartySessionImporter 33–119、130–134 | ZIP 全程内存，不提取文件。EOCD/中央目录位置一致，最多 4000 项；无分卷/ZIP64/容器加密，仅 stored/deflate；每项展开 ≤2 MiB，总展开 ≤64 MiB，导入器另把压缩输入及实际展开合计限 64 MiB。检查中央/本地 flags、method、名称和数据界限；相对路径拒绝绝对/冒号/控制字符/空段/点段/上级路径，≤30 层/2048 字节，重复路径不区分大小写；拒 Unix 符号链接/特殊文件。只对 XSH/目录解压和验 CRC，其他设置条目忽略内容，不宣称它们也完成 CRC 验证。文件系统读取 O_NOFOLLOW、普通文件及大小稳定检查；遍历 ≤8000 项、跳隐藏及包后代、拒链接，报告 ≤2000 会话/4000 目录。 |
| SF-E07 | [ZOCLaunchRequest.swift](../validation/oshell-reference/Sources/OShellCore/ZOCLaunchRequest.swift) 全文件；[FileLaunchRequest.swift](../validation/oshell-reference/Sources/OShellCore/FileLaunchRequest.swift) 全文件；[OShellZOC/main.swift](../validation/oshell-reference/Sources/OShellZOC/main.swift)、[OShellFileZilla/main.swift](../validation/oshell-reference/Sources/OShellFileZilla/main.swift)；[main.swift](../validation/oshell-reference/Sources/OShell/main.swift) 227–242 | ZOC ≤32 参数/16 KiB，支持 /名:值、-名 值及 =，拒未知/重复，CONNECT 与 SSH 恰一；SSH/SSH2 设备、UTF-8、xterm/vt100/linux，支持 user:password@host、独立凭据覆盖和方括号 IPv6。TITLE 有长度/字符验证但 profile/openExternal 不使用其显示值；不是支持任意终端仿真或脚本。FileZilla ≤2 参数/16 KiB，ftp/sftp URL 禁 query/fragment，或 --site=、--site、-c 指定 0/ 用户站点路径（支持斜线/反斜线转义）。仅读本机配置中唯一站点，协议 0/1、登录 0/1/2/3/5，拒 FTPS/专有密文；可匿名、明文或 base64 密码、SFTP 私钥路径；站点 RemoteDir 不解析而从 . 开始。两 helper 解析后清 argv 内存并用不含原参数的错误；只能缩短进程参数可见时间，不是从未出现明文的保证。 |
| SF-E08 | [ExternalLaunchClient.swift](../validation/oshell-reference/Sources/OShellCore/ExternalLaunchClient.swift)、[LaunchEndpoint.swift](../validation/oshell-reference/Sources/OShellCore/LaunchEndpoint.swift)、[ExternalLaunchServer.swift](../validation/oshell-reference/Sources/OShell/ExternalLaunchServer.swift) 全文件；[AuthIPC.swift](../validation/oshell-reference/Sources/OShellCore/AuthIPC.swift) 28–61；WorkspaceController 93–112、646–654；[WorkspaceWindows.swift](../validation/oshell-reference/Sources/OShell/WorkspaceWindows.swift) 130–144；[RemoteFileSession.swift](../validation/oshell-reference/Sources/OShell/RemoteFileSession.swift) 122–188、471–472 | 通道按 UID+规范化数据目录 SHA-256 前 10 字节定位，0700 目录/0600 锁与 socket，同 UID peer 校验；launch.lock 串行客户端启动，server.lock 防同目录重复监听。既有默认应用服务未就绪先等 3 秒，不直接重启；新启动 helper 找同包应用，空标准流启动 --external-launch-service，最多等 300 秒并重查迁移后的目录。消息 ≤16 KiB、客户端接收300秒/发送15秒，服务读写10秒/最多8待处理；启动解锁回调后二次检查 stopped 和当前可用 workspace。最近256个UUID去重，只存ID，标记后调用打开，accepted 不证明 SSH/FTP 已连上；再次执行 helper 新 ID 同目标可再开。终端临时密码转 broker 且外部组禁保存；文件 launchPassword 在标签内保留供重连/SCP，shutdown 清除。不把运行期临时密码说成仅使用一次，也不保证清除所有语言运行时内存副本。 |
| SF-E09 | ThirdPartySessionFormat 8–12；SessionTransfer `importSessions`；[session-import.md](../validation/oshell-reference/docs/session-import.md) 1–15；[session-conversion.md](../validation/oshell-reference/docs/session-conversion.md) 1–38 | 生产来源选择只有 OShell JSON、SecureCRT、Xshell。其他客户端是外部编写转换脚本后交原生归档；文档提供字段/加密示例不能视为应用自带该来源解析器。FileZilla --site 的读取仅为一次即时连接，不构成 FileZilla 整库迁移；ZOC 参数同理。新增来源解析器或自动生成转换器属于另行确定范围的能力，合法归档仍走 E01/E02 验收。 |

### 本域修订与源码限制

2026-10-24：SF-E01 明确“选定导出”为当前行及目录子树，没有证据支持任意多选批量导出；SF-E03/E04 去掉预览逐项选择的暗示，真实 UI 只有展示/整批确认。SF-E05 把来源身份改为实际字段匹配规则，不承诺曾经导入的身份标记；SF-E07 记录 ZOC TITLE 接收但未应用、FileZilla 单站点读取与批量迁移不同。SF-E08 明确有限 UUID 去重及 accepted 的含义；停止服务拒绝待处理请求不等于任意旧 ID 永久失效。均归原 SF 身份，不追加功能编号。

有限验收覆盖原生 v1–v4/未知版本与字段、三类选中行及全部导出、三类凭据去除与重包、普通/Links 目标及同名、第三方三种协议/两种来源/编码/单项跳过与结构致命错误、密码正确/错误/未匹配/已有/歧义、ZIP 所列限制、两个启动器参数及同 ID/不同 ID/解锁取消。源库不变、取消和 revision 变化应分别验。源码分析不构成真实 Swift ↔ Go 往返、厂商每版本兼容或 macOS 启动实测。

### 已阅读文件范围

整文件已读：Core `SessionArchive.swift`（152 行）、`ThirdPartySessionImporter.swift`（290 行）、`SessionImportZip.swift`（81 行）、`XshellPasswordImport.swift`（95 行）、`ZOCLaunchRequest.swift`（89 行）、`FileLaunchRequest.swift`（122 行）、`ExternalLaunchClient.swift`（53 行）、`LaunchEndpoint.swift`（42 行）、`SharedConfigurationProtocol.swift`（87 行）；应用 `SessionTransfer.swift`、`ThirdPartyImportPreview.swift`（45 行）、`ExternalLaunchServer.swift`（83 行），两个 launcher 的 `main.swift`。协议规范化实现整读不代表完整 Y 域已核查；合同仅补读 archive 996–1045，其余定义须在 Y 域逐项关联。

公共区段补读：SessionManager 388–407；WorkspaceController 93–112、638–674；WorkspaceWindows 118–146；应用 main 22–106、220–255；RemoteFileWindow 54–94、112–153；RemoteFileSession 117–191，shutdown 471–472；AuthIPC 在 P 域已整读。本域只读导入教程与转换指南的上述开头区段及相关检索结果，没有把全部文档引用的外部厂商页面当作已核实，也没有访问真实站点配置或凭据。

## 外观与资源

设置通过草稿预览，应用时经 WorkspaceController 保存后传给既有 TerminalPane；设置窗的主题预览与终端配色预览各有自己的对象。以下区分 UI、存储模型、实际消费者，避免把未接线字段或底层组件的通用能力当成原版已开放功能。

| SF 条目 | 生产入口与源码定位 | 源码结论与关键边界 |
| --- | --- | --- |
| SF-A01 | [AppearanceSettings.swift](../validation/oshell-reference/Sources/OShell/AppearanceSettings.swift)、[SettingsWindow.swift](../validation/oshell-reference/Sources/OShell/SettingsWindow.swift) 全文件；[Dialogs.swift](../validation/oshell-reference/Sources/OShell/Dialogs.swift) 1–22；[TerminalPane.swift](../validation/oshell-reference/Sources/OShell/TerminalPane.swift) 510–566；WorkspaceController 885–909（O 域已读） | system/light/dark 分别映射系统、aqua、应用深色外观；正式应用更新 NSApp 和窗口/子窗。设置窗预览仅改自己及子窗；系统选项在应用强制主题时读取系统偏好，避免继承强制色。取消不提交草稿，应用后更新既有终端而不重建连接。终端搜索/传输/关闭控件按终端配色明暗设置，不能要求所有控件强制跟主窗口同色。 |
| SF-A02 | AppearanceSettings 全文件；[TerminalColorScheme.swift](../validation/oshell-reference/Sources/OShellCore/TerminalColorScheme.swift) 全文件；[Models.swift](../validation/oshell-reference/Sources/OShellCore/Models.swift) 159–244、270–300；TerminalPane 510–566 | 九预设、最多64自定义，6基础颜色+16 ANSI；ID非空且≤128 UTF-8字节，名称去首尾空白后非空/≤80字符/无控制字符，颜色为#六位RGB。改预设自动复制新UUID；删除只限自定义并回落首预设。导入≤65536字节，先试单方案JSON，再试UTF-8或带BOM UTF-16的XCS；XCS可多方案，必需背景/文字及8普通+8高亮色，忽略Names，已知键重复后值覆盖，缺项/非法整次失败。导入重新生成自定义身份；JSON导出当前草稿是独立文件写入，随后取消设置不会撤销该文件。真实 TerminalView 预览使用固定测试文字，没有PTY。autoCursor 有模型读写，未找到UI/颜色应用消费者，不宣称自动对比光标。共享字段拒绝策略另归Y，不能套用于这个独立配色JSON入口。 |
| SF-A03 | [TerminalTextSettingsView.swift](../validation/oshell-reference/Sources/OShell/TerminalTextSettingsView.swift)、[TerminalFontCatalog.swift](../validation/oshell-reference/Sources/OShell/TerminalFontCatalog.swift)、[BundledTerminalFonts.swift](../validation/oshell-reference/Sources/OShell/BundledTerminalFonts.swift) 全文件；[UICompatibility.swift](../validation/oshell-reference/Sources/OShell/UICompatibility.swift) 17–29；TerminalPane 510–566；[MacTerminalView.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/Mac/MacTerminalView.swift) 155–176、340–381 | 内置Cascadia/DejaVu各四TTF，仅进程注册一次；系统目录按代表字体基础ASCII覆盖过滤、PostScript名去重，系统/用户/内置分类；用iMW0与空格的advance差判断等宽，非等宽只警告。缺失的已存字体仍列当前配置，实际渲染回退系统等宽/Menlo等。四样式由NSFontManager转换解析，不能保证每个本机字体有四个独立字形文件。默认13，模型有限值10–26；菜单固定11/12/13/14/15/16/18/20/24/26，字体预览固定13点。仅字体名/大小改变才重设现有终端字体，重设会清选区，不改连接。 |
| SF-A04 | [TerminalSymbolFont.swift](../validation/oshell-reference/Sources/OShell/TerminalSymbolFont.swift) 全文件；MacTerminalView 400–510；[AppleTerminalView.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/Apple/AppleTerminalView.swift) 100–230、1245–1281、1315–1340；[PowerlineRenderer.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/Apple/PowerlineRenderer.swift) 全文件；[Utilities.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/Utilities.swift) 283–467 | 普通私用区先保留基础字体已有字形，仅缺字且内置Symbols Nerd Font有字时回退，按基础m宽与参考图标advance缩放。彩色Emoji开关默认开，VS15排除彩色，默认EmojiPresentation或VS16/keycap等按代码选择AppleColorEmoji；系统覆盖决定实际结果。E0B0/E0B2/E0B4/E0B6默认直接绘制三角/圆弧Powerline几何并作物理像素接缝处理，不能概括为所有PUA都走字体优先。通用fallback字体缓存1024、彩色字体缓存32满后清理；文本样式选择只作用显示，不改缓冲字符。复杂grapheme、完整布局和其余特殊块字形归I继续核查。 |
| SF-A05 | TerminalPane 448–464、510–566；Utilities 283–467；[Terminal.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/Terminal.swift) 1358–1425、7860–8004；[SelectionService.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/SelectionService.swift) 781–794；[BufferLine.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/BufferLine.swift) 504–552 | 终端创建时传ambiguousIsWide，默认false；正式偏好apply不改变既有终端宽度选项，预览可改自身并重送样例。columnWidth只有原宽1且命中歧义表时提升为2；组合/控制等仍走既有规则，East Asian wide和区域指示等按固定实现处理。AmbiguousWidthData表头标Unicode17.0.0，实际还依赖运行时Unicode属性，不承诺未来版本。选区复制经getDisplayText/行翻译跳过宽字符后占位格；真实空格仍保留，不能把所有NUL格都删掉。 |
| SF-A06 | TerminalPane 510–566、707–726；MacTerminalView 511–858；[LegacyCompatibility.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/Mac/LegacyCompatibility.swift) 全文件；[MetalRendererRecovery.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/Apple/Metal/MetalRendererRecovery.swift) 全文件；[MetalTerminalRenderer.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/Apple/Metal/MetalTerminalRenderer.swift) 221–254、345–385、416–428、463–501 | 应用默认请求Metal，组件自身默认值不同；无window时延后准备，初始化失败可软件回退，设置apply使用try?不能说每次失败必有弹窗。旧系统构建禁用GPU控件，setter为空操作。跨窗重绑先建新表面/绘制再移旧表面，失败转CoreGraphics。帧协调只允一帧在途，拒绝提交按16/33/66/125/250ms退避、1秒watchdog；仅可见、非最小化/隐藏、非零尺寸且Metal启用时重试。恢复先换renderer，30秒内再次请求则软件回退；重新启用重置策略。旧renderer有在途帧时保留至idle，不能承诺切换同步释放全部GPU资源。状态/诊断不等于双路径图像已实测一致。 |
| SF-A07 | [ScrollbackSettingsControl.swift](../validation/oshell-reference/Sources/OShell/ScrollbackSettingsControl.swift) 全文件；Models 159–244、270–300；TerminalPane 510–566；AppleTerminalView 2580–2640；Terminal 6788–6837、7226–7263；[Buffer.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/Buffer.swift) 918–1005 | 默认3000、500–100000；输入去首尾空白后只收ASCII十进制数字，六预设500/1000/3000/5000/10000/20000，超过20000资源提示加强。应用调整normalBuffer历史，备用屏无回滚；减少容量仅在当前数量超新容量时trimStart，yBase/yDisp/savedY相应扣减并下限0；增加容量不制造内容。包装器使搜索失效、更新滚动条并排队显示。它是行容量而非全应用内存字节硬上限；裁剪后的复杂选区/图片生命周期仍须I域闭合和运行验收。 |
| SF-A08 | [TerminalClockView.swift](../validation/oshell-reference/Sources/OShell/TerminalClockView.swift)、[TerminalClockPosition.swift](../validation/oshell-reference/Sources/OShellCore/TerminalClockPosition.swift) 全文件；Models 159–244、270–300；TerminalPane 510–566、829–863 | 默认关闭、右上、12点、跟随终端文字色、opacity0.5；字号8–96，opacity0–1，无效颜色回退跟随。九位置、12px边距、缩小时适配，覆盖层hitTest=nil且不接焦点/辅助访问，不占终端格。POSIX/Gregorian格式yyyy-MM-dd HH:mm:ss，每次取当前自动时区；共享计时器对齐整秒、1秒间隔/tolerance0.08，common与modalPanel模式。弱订阅条件为启用/非零透明/有window/未dispose，隐藏本身未取消订阅；可见集合空则停计时，无订阅时移观察者。可见判断不查occlusionState，不能声称被完全遮挡必停。显示只更新变化文字区域，不写PTY/复制/日志。 |
| SF-A09 | [OutputActivity.swift](../validation/oshell-reference/Sources/OShellCore/OutputActivity.swift) 全文件；TerminalPane 1004–1022、1108–1127；WorkspaceController 153–178、690–734、784–808；[TabStripView.swift](../validation/oshell-reference/Sources/OShell/TabStripView.swift) 180–202及标题/菜单构造 | 跨块字节状态机忽略已识别ESC/CSI/控制字符串；正文BEL或>32且非DEL字节触发。不是屏幕变化比较：ASCII空格/换行不触发，非ASCII空白仍可能触发。应用激活且key/可见/未最小化时，只有选中标签中实际观察的pane直接视为已读；其他可见未选标签也会未读。选中刷新/窗口回到观察状态清除，放大分屏只清该pane，其余保留。标题橙色/半粗、菜单标记；状态点优先连接状态，不保证未读总覆盖其颜色。无计数、闪烁或系统通知入口。 |
| SF-A10 | AppleTerminalView 2918–2970；TerminalPane 829–863、1004–1022、1108–1127；[IdleMemoryReclaimer.swift](../validation/oshell-reference/Sources/OShell/IdleMemoryReclaimer.swift) 全文件；WorkspaceController 的reclaimer调用 | 隐藏/脱窗跳过常规display排队，输出仍解析并进入日志/搜索；常规排队按16670000ns合并，约60Hz。shutdown幂等，清焦点/回调/任务/计时器、一次性凭据与传输，停日志/broker/本机工具/PTY，解绑连接组并关闭Metal；对象或在途引用仍存时不保证缓冲立即释放。进程自然结束可保留标签历史，不等同关闭。每Workspace独有回收调度，关闭最后标签后2秒只检查该窗口仍空；新建/关闭取消待调度，但已进入后台回收不可据此撤销。主线程清共享字体/颜色/行缓存，后台malloc_zone_pressure_relief，记录返回字节/耗时；不检查全应用所有窗口空闲，也不承诺固定RSS下降。 |

### 本域修订与源码限制

2026-10-24：补齐原版已存在的XCS配色导入、四Powerline专用几何和Metal恢复路径，均归原SF-A条目。纠正autoCursor模型存在即代表功能启用、所有PUA都先用基础字体、所有隐藏时钟都退订、未读等同可见屏幕变化、单窗口空闲等同全应用空闲等过度概括。宽度表、颜色/字体常量和资源预算是既有能力的边界，不各建新的功能身份。

有限验收覆盖主题预览/取消/应用，配色导入/导出/上限，字体缺失/样式/固定字号，四Powerline与PUA/Emoji样例，宽度新旧终端差异，Metal初始化/跨窗/单次及重复恢复，历史增减，时钟九位置/隐藏/销毁，分块活动检测/放大pane已读范围，多窗口持续输出及空闲回收。这里是约定的验收集合，不是本轮实测结果；不要求穷举系统字体、全部Unicode组合或显卡型号。

### 已阅读文件范围

整文件已读：应用AppearanceSettings、TerminalTextSettingsView、ScrollbackSettingsControl、TerminalFontCatalog、BundledTerminalFonts、TerminalSymbolFont、SettingsWindow、TerminalClockView、IdleMemoryReclaimer；Core TerminalColorScheme、OutputActivity、TerminalClockPosition；SwiftTerm MetalRendererRecovery、Mac/LegacyCompatibility、Apple/PowerlineRenderer。上述名称均为.swift文件。

公共区段补读：Dialogs 1–22；Models 159–244、270–300；UICompatibility 17–29；TerminalPane 448–464、510–566、617–676、707–726、829–863、1004–1022、1108–1127；WorkspaceController 153–178、690–734、784–808及回收调用检索；TabStripView 180–202及标题/菜单构造检索。SwiftTerm MacTerminalView 155–176、340–381、400–858；AppleTerminalView 100–230、1245–1281、1315–1340、2580–2640、2918–2970；MetalTerminalRenderer 221–254、345–385、416–428、463–501；Utilities 283–467；Terminal 1358–1425、6788–6837、7226–7263、7860–8004；Buffer 918–1005；BufferLine 504–552；SelectionService 781–794。宽度数据表只查表头/使用点，没有逐项复核全部码位；SwiftTerm完整渲染/协议/图片与布局继续归I，不能把本域区段阅读当成整依赖核查完毕。

## 窗口与导航

原版同时存在三个不同对象：WorkspaceController拥有窗口内运行标签与发送目标；TerminalTab拥有一棵PaneLayout和当前活动叶子；TabGroupNode只存标签UUID、组名、显隐和布局。新分屏走replica创建pane，组内移动/跨窗迁移复用现有对象；两类操作不能用相同的“不重连”描述。

| SF 条目 | 生产入口与源码定位 | 源码结论与关键边界 |
| --- | --- | --- |
| SF-W01 | [WorkspaceWindows.swift](../validation/oshell-reference/Sources/OShell/WorkspaceWindows.swift) 7–90、130–146；[WorkspaceController.swift](../validation/oshell-reference/Sources/OShell/WorkspaceController.swift) 7–178；[main.swift](../validation/oshell-reference/Sources/OShell/main.swift) 98–120、121–220；[WorkspaceToolbar.swift](../validation/oshell-reference/Sources/OShell/WorkspaceToolbar.swift) 7–16 | 协调器共用store/updater/WebDAV和配置快照，新窗传快照，不重载旧运行会话；PasswordVault为shared。窗口初始1180×760、最小760×460，禁AppKit自动tabbing，首窗记OShell.main frame；新增窗清autosave名并相对活跃窗偏移28。激活转向菜单target、更新服务和快捷/命令菜单；移除活跃窗激活最后剩余窗，最后关闭清菜单target但应用不退出，Dock/reopen可再建窗。工具栏小于1180部分按钮改仅图标，不是额外工作区模式。配置发布向其他窗receive，完整存储原子性归Y。 |
| SF-W02 | WorkspaceController 83–90、656–701、751–769、916–952；[WorkspaceWindow.swift](../validation/oshell-reference/Sources/OShell/WorkspaceWindow.swift) 全文件；[TabStripView.swift](../validation/oshell-reference/Sources/OShell/TabStripView.swift) 280–349、433–444 | 编号由每组自身tabs顺序从1开始；numberedTabs取当前可见组，其次选中标签所属可见组，再首可见组。编号1–9及输入任意有效编号走同一选择函数；输入去首尾空白、只收ASCII数字、越界弹窗继续。前后循环却按窗口tabs顺序过滤隐藏组，并非只当前组；MRU按selectedTab变化记录UUID，跳过自己及隐藏组。关闭/迁移清历史，排序不改身份；选中后显露标签并滚到活动pane。WorkspaceWindow拦有效快捷键，IME/搜索Esc另处理；底层输入编码归I。 |
| SF-W03 | WorkspaceController 509–579、618–679、783–808、854–863；[WorkspaceSessionLinks.swift](../validation/oshell-reference/Sources/OShell/WorkspaceSessionLinks.swift) 90–175；[TerminalPane.swift](../validation/oshell-reference/Sources/OShell/TerminalPane.swift) 278–298；TabStripView 44–153、280–349、351–445 | 加号建空白OShell提示符；本地菜单另建真实shell。双击duplicate要求非空白/未关闭/未结束/PTY运行，SSH还须sessionReady，外部组须available；只复制活动叶子。右键copyTabSession只要求标签存在，空白复制空白，其余按原配置建独立连接，允许结束页；copySSHChannel要求已就绪可用组并复用连接。replica对外部组不可用报错，其余新建。新开终端可替换当前非关闭/无活动进程/无传输叶子，保留tab/group；批量最多首个终端复用。重连先确认、创建替代pane再shutdown旧pane，只替换活动叶子；不复制历史。关闭最后普通标签显示欢迎页，窗口保留。拖动5px阈值，按目标标签中点插入，边缘24px按360px/s滚动；拖动提交再改序。 |
| SF-W04 | WorkspaceController 711–769、813–863；[WorkspaceNavigation.swift](../validation/oshell-reference/Sources/OShell/WorkspaceNavigation.swift) 7–23；TerminalPane 1021–1142；[TerminalChrome.swift](../validation/oshell-reference/Sources/OShell/TerminalChrome.swift) 29–53；WorkspaceToolbar 61–98 | PaneLayout递归二叉树，新分屏复制活动叶子的配置，左右/上下，初始位置约一半；外部SSH组按replica规则复用，无组时不冒充同一连接。叶子建议最小220×140、原生1px分隔条。循环按panes深度优先展平顺序；放大记录所有split对象比例并暂时拆视图，恢复重新挂载后按比例设置，切换放大目标不另存一套比例。关闭叶子移除节点并提升兄弟，活动叶子取原位置邻项；保留先前终端焦点或快捷发送编辑焦点。结构增删调用adjustSubviews，不能保证所有祖先比例一概不动。无选中tab时split函数走newLocal，但主要UI在无tab时禁用。 |
| SF-W05 | [TabArrangement.swift](../validation/oshell-reference/Sources/OShell/TabArrangement.swift) 全文件；[WorkspaceTabGroups.swift](../validation/oshell-reference/Sources/OShell/WorkspaceTabGroups.swift) 21–80、109–166；[TabGroups.swift](../validation/oshell-reference/Sources/OShell/TabGroups.swift) 54–77、135–175；WorkspaceController 725–782；TerminalPane 45–61 | 四模式：单组、上下、左右、瓷砖。网格按ceil(sqrt(n))列估算行，再均分每行；旧无命名结构重新排列，含命名组时按组重组并保留其成员/显隐；合为单组选项卡清custom树，原隐藏成员恢复可达。组布局有最小尺寸及比例，溢出进入双轴滚动；重排本身不建PTY。普通同窗内容拖动按归一化最近边距离<0.27分左右/上下，否则中心合组；同组中心/单成员自分割拒绝，空可见组可接收。Option或横向手势可滚排列，普通纵向先尊重终端历史/鼠标报告/备用屏，相关边缘才让出；只有横向溢出时普通纵轮可转横向。 |
| SF-W06 | [WorkspaceNamedGroups.swift](../validation/oshell-reference/Sources/OShell/WorkspaceNamedGroups.swift) 全文件；TabGroups 7–52；WorkspaceTabGroups 42–80；WorkspaceController 656–701、747–778 | 新建空命名组默认放现树右侧并可激活，名称trim后非空/≤80字符/≤1024 UTF-8字节、无控制字符、caseInsensitiveCompare去重。改名只刷新标题菜单，不拆终端；移入隐藏组保留隐藏并选其他可见组，显式激活才展开。支持隐藏、仅显示、显示全部，可全部隐藏而连接继续。解散选首个其他可见组（否则任意其他组）接收成员，最后组解散转单组；没有关闭成员连接。空命名组保留，空匿名组删除折叠。组内排序走W03，没有单独拖动整组排序入口；运行树不写共享配置，不与会话目录映射。隐藏组是否发送继续按O域显式范围规则。 |
| SF-W07 | TabStripView 114–153、381–432；TabGroups 79–133；WorkspaceTabGroups 82–108；WorkspaceController 268–307、1017–1069 | 拖动数据为UUID，但同时校验真实TabStripView.Item来源/dragEnabled/source.id/move权限和落点；仅同一应用。内容外层host覆盖侧栏/工具栏/欢迎页；外窗命中可见组，否则选活跃/首可见组，全部隐藏无接收目标；外窗内容固定center合组，不按边缘创建split。标签栏用before指定插入。两窗同协调器、已解锁、无loadFailed/sheet/modal且目标有效，拒收不改所有权。拖出只在operation为空、未取消、源可见未最小化、源frame外扩12后仍在外且落有效屏幕时建新窗；落在其他可见OShell窗口内即使拒收也不另建第三窗。Esc/释放时仍按左键均识别取消；新窗尺寸限屏幕可见区，失败关闭新空窗。 |
| SF-W08 | WorkspaceController 580–615、1057–1094；[RemoteFileWindow.swift](../validation/oshell-reference/Sources/OShell/RemoteFileWindow.swift) 47–55；[RemoteFileSession.swift](../validation/oshell-reference/Sources/OShell/RemoteFileSession.swift) 63–68；TerminalPane的prepareForDisplay 556–567及A06 Metal重绑路径 | move先校验全部目标身份/插入项，移除源syncTargets/composerTargets/quickSendSelected的pane IDs，移出现有TerminalTab并在目标插入，逐pane重绑焦点/输出/输入/密码保存/文件操作回调。没有replica/shutdown，连接/历史/layout/zoom属于原对象；prepareForDisplay仅对未启动未结束pane启动，不能把尚未启动对象说成已有连接。目标显式选择集合不自动追加；按全部/当前等动态范围是否包含由O计算。源最后标签移出才把所有fileWindows及其sessions重指workspace、重设onClosed，并先清源列表再关源窗，避免shutdown取消独立文件传输；非最后移出文件窗仍归源。实际后端并发取消与图像上传中途身份校验继续归F/M。 |
| SF-W09 | WorkspaceNavigation 24–61；[QuickSessionPicker.swift](../validation/oshell-reference/Sources/OShell/QuickSessionPicker.swift) 全文件；[SessionSearchQuery.swift](../validation/oshell-reference/Sources/OShellCore/SessionSearchQuery.swift)（S域已读）；WorkspaceController 618–644、751–769 | 快照entries先当前窗每个终端pane（含结束页/隐藏组），后按名称排序的可见保存资料，不纳入其他窗/独立文件管理标签。搜索文本含pane标题、组名/编号/状态及profile元数据；保存资料含名称/协议/主机/用户/端口/目录，没有独立解析TCP对端字段。沿用4096 UTF-8字节/32词、归一化AND子串；三范围，先过滤再截前200，状态显示总匹配数。显示剔控制字符并截500字符，搜索仍用原索引。选择按paneID+profileID保持；回车/上下让位IME，取消恢复原firstResponder。已开项按paneID重新查找并显露隐藏组/对应放大pane，保存项按profileID重新读配置再open；不是一定新增tab。 |
| SF-W10 | [WorkspaceFullscreen.swift](../validation/oshell-reference/Sources/OShell/WorkspaceFullscreen.swift) 6–10、32–104；WorkspaceWindow 全文件；WorkspaceController 186–267、965–975；SessionSidebar 259–266（S域已读）；StartupProtection `refreshMasterWarning`（P域已读） | 维护requested/当前chrome/transition三状态，进入先隐藏工具栏、分隔线、链接/侧栏、主密码提醒和标题/三按钮，工作区顶约束改到content.top；保存标题可见/fullSizeContent/按钮隐藏值，退出还原；侧栏链接等按当前偏好显示。原生toggleFullScreen过程有回调，动画内相反请求延至主队列；进入失败清请求并恢复chrome，退出失败保留请求为true。Dock/菜单栏改自动隐藏。Esc不经cancelOperation退出专注；IME/搜索/弹窗各自处理后其余输入仍走终端，不消费为全屏退出。目标平台等价全屏是项目约定，不等同要求复造AppKit Spaces。 |
| SF-W11 | WorkspaceFullscreen 12–30、49–59、93–99；WorkspaceWindow 24–30；WorkspaceController 711–721、965–975、994、1106–1110 | 本机固定deepFocus快捷键只有未被配置动作占用时触发并忽略重复；不是共享schema新动作。开启自动请求专注全屏，隐藏所有pane标题和quickSendBar并令其高度0；不移除控件/草稿，不写quickSendBarVisible。关闭深度模式只恢复普通专注，正常单pane标题仍按原规则隐藏；退出整个专注或进入失败会清isDeepFocus。标签栏/撰写窗没有在该分支隐藏，不能扩成“所有非终端UI全部消失”。配置接收时仍尊重isDeepFocus。 |
| SF-W12 | WorkspaceController 783–863、1009–1015、1113–1122；WorkspaceWindows 134–144；[SessionTitleSettings.swift](../validation/oshell-reference/Sources/OShell/SessionTitleSettings.swift) 15–46、66–95；TerminalPane 278–298；RemoteFileWindow 20、RemoteFileSession 59 | confirmTabClose默认true、旧字段缺失true、读取损坏/未知版本也回退true；在本机tab-title-settings.json，各窗每次关闭读取，设置应用检查旧快照防并发覆盖。它同时管标签和单pane，仅hasActiveProcess（!ended或localTool存在）时确认，单pane标签委托整标签关闭；空白/结束无工具可直接关。重连独立看hasActiveProcess；窗口独立统计本窗，应用退出协调器汇总全部，确认后才shutdown/取消同步。文件operation计数为含busy session的文件窗口数，不是所有任务总数，普通空闲文件连接不触发该忙碌确认。取消保持原对象；无selectedTab的closePane转window.performClose，仍走窗口确认。 |

### 本域修订与源码限制

2026-10-24：SF-W02纠正“全部导航都限焦点标签栏”的含混描述，编号与前后/MRU分开；W03区分双击活跃复制、右键独立复制和SSH渠道复制，结束页禁双击不等于禁配置复制。W04不再把放大恢复比例泛化为所有结构增删都保持比例。W06把“关闭组”改为真实入口“解散并保留会话”，去掉没有独立生产入口的整组排序暗示；组内标签排序仍在W03/W06。W07明确外窗内容合组与同窗边缘分组不同；W09限定当前窗终端和200项；W12纠正本机确认选项同时覆盖分屏。以上为固定源码支持的原条目修订，不新增SF或产品能力。

有限验收覆盖三窗建/关/重新打开及菜单归属，两个组的编号/前后/MRU，运行/结束/空白三类复制，普通与外部SSH渠道，空闲叶子替换和批量开连接；嵌套分屏/放大目标切换/恢复/关闭，四排列及窄窗滚轮，隐藏全部/移入隐藏组/解散保留会话，跨窗各接收区/插入/拒收/Esc/最后移出文件窗交接，快速搜索200项与IME，以及全屏连续请求/失败恢复和各种关闭取消。这里没有执行真实鼠标、AppKit动画、macOS窗口焦点或服务器连接；列举的是有限验收约定，不能当作已经通过。

### 已阅读文件范围

整文件已读：WorkspaceWindows（146行，本域重核UI交接）、WorkspaceNavigation（61）、WorkspaceTabGroups（166）、WorkspaceNamedGroups（238）、WorkspaceFullscreen（104）、TabGroups（175）、TabStripView（445）、TabArrangement、QuickSessionPicker（115）、TerminalChrome、WorkspaceToolbar（110）、WorkspaceWindow（47）。TabStrip信息卡构造调用同时核对，卡内主机信息提取仍用已完成T域证据。

公共区段补读：WorkspaceController 1–377、509–864、916–1132；TerminalPane 37–64、278–299、549–567、1021–1142；WorkspaceSessionLinks 90–181；应用main 96–220（启动部分已在P/E/U记录）；RemoteFileWindow 40–55及busy/关闭入口检索，RemoteFileSession 53–69；SessionTitleSettings 1–46、66–100。这里列的是本域阅读范围，不以公共文件的相邻功能推定Y/F/I完成。WorkspaceController的保存/设置等其余区段已由S/O/P记录，存储实现与完整文件后端仍须按各自SF核查。

## 文件管理与传输

入口由 [RemoteFileWindow.swift](../validation/oshell-reference/Sources/OShell/RemoteFileWindow.swift) 的 `showFiles/openFiles`（124–153）、终端拖入文件（[TerminalPane.swift](../validation/oshell-reference/Sources/OShell/TerminalPane.swift) 128–133）及保存资料/外部启动交接。文件标签的 [RemoteFileSession.swift](../validation/oshell-reference/Sources/OShell/RemoteFileSession.swift) 负责连接、浏览、批次与 UI；[RemoteFiles.swift](../validation/oshell-reference/Sources/OShell/RemoteFiles.swift) 和 [FTPAndSCP.swift](../validation/oshell-reference/Sources/OShell/FTPAndSCP.swift) 实现后端。ZMODEM 走终端 PTY 与随包 helper，不经过文件标签后端。

| SF 条目 | 生产入口与源码定位 | 源码结论与关键边界 |
| --- | --- | --- |
| SF-F01 | RemoteFileWindow 21–120；RemoteFileSession 53–69、122–187；[PopupKeyboard.swift](../validation/oshell-reference/Sources/OShell/PopupKeyboard.swift) 165–173、194–205；[FileSessionAddress.swift](../validation/oshell-reference/Sources/OShellCore/FileSessionAddress.swift) 7–30 | 窗口初始空白文件页；打开资料复用当前无资料页，否则新增。双击标签复制资料/目录/运行时密码或连接组，不复制记录；同窗拖动重排，未接外部拖入/拖出新窗回调。关最后页补空白；关页或窗口在 busy 时确认，包含连接/浏览，不限传输。Esc 在无组字等拦截时最小化；Cmd+W 关闭整个文件窗口。标签地址取数字 IP 或 DNS 唯一结果，歧义显示待识别，不是 TCP 实际对端证明；异步结果有身份令牌校验。 |
| SF-F02 | RemoteFileWindow 124–153；RemoteFileSession 122–219；RemoteFiles 140–152、200–222；[SSHConnectionGroup.swift](../validation/oshell-reference/Sources/OShell/SSHConnectionGroup.swift) 39–56、90–103 | 活动 SSH 终端入口要求已解锁、未结束和可用连接组，沿用当前远端目录；终端拖放仅 SSH 资料可达。保存 SSH/SFTP/FTP 资料可独立打开。组复用先取得 lease，clone 以 BatchMode 和 false ProxyCommand 阻止回退认证，失败不尝试新密码；独立连接正常使用认证 broker。FTP 优先显式密码，其次解密保存密码，否则弹框；输入后清框，不由文件入口落库。取消/切换令牌使晚到认证回调失效。 |
| SF-F03 | RemoteFileSession 71–94、282–328；RemoteFiles 248–274；FTPAndSCP 50–100 | 路径和列表仅在 canonical/list 都成功后更新，目录优先再按名称本地化排序。失败保留旧目录/列表，但手输路径框没有自动还原；筛选重载也没有按条目身份恢复选择。过滤是单一大小写不敏感子串，不是会话搜索的多词 AND。双击目录/链接/未知类型先尝试进入，普通文件选择下载目录；右键按所点行调整选择。拖入目标只按已知 directory 标志，不对链接/未知项先探测。SFTP 单次名称响应最多 20000 项、累计 100000 项。 |
| SF-F04 | RemoteFileSession 329–423；RemoteFiles 293–295、404–455；FTPAndSCP 102–139 | 创建/改名/删除为独立操作；删除确认后按目录标志用 RMDIR/RMD 或 REMOVE/DELE，非空目录不会递归清空。SFTP/FTP 递归深度最多 32，上传拒绝资源属性已识别的本地符号链接，包含隐藏子项；下载递归跳过子链接，但 SFTP 直接下载入口的 STAT 会跟随指定根链接。SCP 使用系统递归行为，不能套用两后端的全部限制。失败保留已完成项目/已建目录；普通文件下载失败清当前新文件，续传留片段，上传不保证回滚。 |
| SF-F05 | RemoteFileSession 79–88、289–305；RemoteFiles 8–23、171–188；[RemoteFileMetadata.swift](../validation/oshell-reference/Sources/OShell/RemoteFileMetadata.swift) 1–73；[RemoteFilePresentation.swift](../validation/oshell-reference/Sources/OShell/RemoteFilePresentation.swift) 1–124 | 七列展示名称/类型/大小/权限/用户/组/修改时间，可横纵滚动和调列宽。图标按链接/未知/目录优先，再按名称分类；无内容下载或缩略图。权限包含 rwx、特殊位和八进制。SFTP 可选 UID/GID 默认转数字，longName 的 Unix 提示仅在类型/大小不矛盾时辅助显示名称。缺 mode 为未知类型；可选权限/时间缺失显示占位。但 size 的非可选零默认可把缺失 size 显示 0；MLSD 未知或缺 type 未统一标 unknown，不能宣称全部属性缺失都正确表示未知。 |
| SF-F06 | FTPAndSCP 8–104；RemoteFileMetadata 7–73；RemoteFiles `FileProcess.outputToEnd` 100–126 | 每次调用系统 curl，列表先 MLSD，失败或解析失败后 LIST，再 NLST。MLSD 小写 facts、忽略 cdir/pdir、识别 dir/slink，时间读 UTC 秒；LIST 解析所实现的 Unix/DOS 行，坏行使整个来源降级（total 行除外），未生成修改时间。NLST 仅安全名称、零 size、unknownType，双击后探测目录。输出最多 4 MiB、严格 UTF-8；无猜编码和 FTPS。curl 仅允许 ftp，忽略用户默认配置及环境代理；凭据从 stdin 配置传入，不放密码命令行。 |
| SF-F07 | RemoteFiles 42–152、200–274；FTPAndSCP 105–189；RemoteFileSession 149–150、334–405、475–515；[FTPStreamingUpload.swift](../validation/oshell-reference/Sources/OShell/FTPStreamingUpload.swift) 1–73 | SFTP 只协商 v3，序列请求 ID 校验、包/字段上限 4 MiB；初始读取 300 秒，普通读取 60 秒、写入 30 秒。FTP 使用 curl，上传经私有 FIFO，下载流写本机文件。常规 SCP 模式仍需文件后端连接供浏览/避名；独立 SCP 只需 SSH 资料且非 busy，可绕过 SFTP 子系统不可用。scp 递归参数、旧协议标志按 OpenSSH 能力检测生成，复用组或独立认证；没有解析字节进度。取消停止子进程，未将系统工具所有其他选项视为产品功能。 |
| SF-F08 | RemoteFileSession 334–418、475–505；RemoteFiles 313–403、407–454；FTPAndSCP 105–139；[OperatorModels.swift](../validation/oshell-reference/Sources/OShellCore/OperatorModels.swift) `RemotePath` 125–141 | 常规批次先列目标，冲突名加 .1/.2 等并更新记录实际目标。SFTP 文件写入用 EXCL；续传上传检查目标不存在后用标准 SFTP RENAME，不用覆盖式 POSIX 扩展；本机普通下载排他创建，续传用排他 rename。SCP 下载到新随机目录，再把预期名称移动到最终避名目标，最后清暂存。FTP/SCP 上传只有预检查，不能保证并发目标不被覆盖；独立 SCP 上传明确提示可能覆盖。RemotePath 拒绝 NUL/CR/LF、超过 8192 UTF-8 字节的路径；safeName 排除空、点/双点、斜杠等，但不是 Windows 保留名/全部非法字符规则。 |
| SF-F09 | RemoteFileSession 95–100、221–281；[FileTransferRateLimiter.swift](../validation/oshell-reference/Sources/OShell/FileTransferRateLimiter.swift) 1–52；FTPStreamingUpload 16–73；RemoteFiles 100–126、323–403；FTPAndSCP 153–186 | 默认不限；预设 128/512 KiB、1/5/10 MiB 每秒，自定义 Double 有限且非负，0 或 1 KiB–1 GiB/s，换算后取整。每标签一个限速设置，SFTP/FTP 共享该任务的可调整 limiter，等待时可唤醒取消/调速；桶容量限 1–32 KiB，按单调时钟补充，不累计无限空闲额度。控制应用负载而非 SSH/协议开销，续传前缀读取也限速。FTP 上传 FIFO 私有目录/0600，凭据另走 stdin；SCP 把构造时速率转为 -l，正在运行的批次对象不随 UI 调整。 |
| SF-F10 | RemoteFileSession 189–219、262–281、334–405、437–469；[FileTransferHistory.swift](../validation/oshell-reference/Sources/OShell/FileTransferHistory.swift) 44–104 | 每标签 worker 串行，operationID 阻止旧操作回调覆盖 UI；批次预排队后逐项运行。失败保留成功项，运行项失败、后续未执行；准备阶段即失败则首个等待项记失败。取消把运行与等待项记取消，停后端并置 connected=false，后续须重连。暂停当前项、后续记未执行。状态回调约 100 ms 限频，目录顶部进度沿用每个子文件并会重新计数，记录不累计目录总量；SCP 没有字节回调。不把进度百分比/connected 标志当实际连接健康或最终成功证明。 |
| SF-F11 | FileTransferHistory 7–107、118–187；RemoteFileSession 63–67、122–132、424–428、471–472 | 记录状态为等待/运行/暂停/成功/失败/取消/未执行，后三种结束结果连同成功受 500 条上限约束，等待/运行/暂停不裁剪。倒序展示、按 ID 保留记录选择、双击只读详情含路径/时间/片段路径/最多 2000 字错误。clearCompleted 仅清 finished，不清暂停；观察者同步移除已消失记录的 resumeJobs。资料 UUID、host、port、user、kind 任一变化即清记录（在新连接成功前）；其他路由/凭据变化不在此判断中。关闭页清全部，不持久化。 |
| SF-F12 | [ResumableFileTransfer.swift](../validation/oshell-reference/Sources/OShell/ResumableFileTransfer.swift) 7–55；RemoteFiles 296–403 | 检查点路径固定，随机 .oshell-part-UUID 与目标同父目录。上传本地源用 NOFOLLOW/CLOEXEC，fstat 普通文件及设备/inode/size/纳秒 mtime，始末同时核对句柄和路径；新远端片段 EXCL，已有片段不新建/截断，LSTAT/FSTAT 和 size/mtime 校验后逐字节比较完整已传前缀，再续写，最终再次查源/片段大小/目标不存在后 RENAME。下载远端要求可识别普通文件和 mtime，LSTAT/FSTAT 始末匹配；本地片段 NOFOLLOW、单硬链接、设备/inode 和前缀校验，fsync 后重开核对身份/长度，再 RENAME_EXCL；失败留片段。远端没有 inode 证明，未传部分在元数据不变下替换不一定检测，亦无完整端到端哈希保证。 |
| SF-F13 | RemoteFileSession 334–405、424–472；FileTransferHistory 87–105 | 只有批次顶层 SFTP 普通文件注册 job；目录子项走普通递归，不注册续传。暂停要求运行中、有 job、总量>0 且尚未完成；失败/取消/暂停且 job 存在可继续，先重连，再以原记录 ID 新批次校验/续传。FTP/SCP、目录、零长度/已完成项无暂停按钮。关闭或基本连接身份变化清 job；清空已结束仅删失败/取消等结束项及其 job，暂停仍保留。没有扫描磁盘重建检查点、跨重启/标签恢复、遗留片段自动 GC；暂停后未开始项需用户重新选择。 |
| SF-F14 | TerminalPane 141–174、678–829；[Streams.swift](../validation/oshell-reference/Sources/OShellCore/Streams.swift) 33–116；[Transfer.swift](../validation/oshell-reference/Sources/OShell/Transfer.swift) 1–187；[ZmodemFinish.swift](../validation/oshell-reference/Sources/OShellCore/ZmodemFinish.swift) 1–87；[ZmodemProgress.swift](../validation/oshell-reference/Sources/OShellCore/ZmodemProgress.swift) 1–60；[ZmodemProgressView.swift](../validation/oshell-reference/Sources/OShell/ZmodemProgressView.swift) 1–107 | 默认自动检测开；跨块收集 CRC 有效的十六进制 ZRQINIT/ZRINIT，非握手继续显示。上传多文件不选目录，下载选目录；等待选择暂存上限 64 KiB，helper 写入队列上限 2 MiB，溢出取消。传输字节绕过正常显示/日志；右上角卡显示当前文件/字节/速度及取消，100% 不是退出成功。helper stdout 排空且退出后才处理结束，stderr 不阻塞结束；下载等待 OO 或匹配已知主机的提示符，超时失败。完成恢复不发送额外回车；取消先发 abort，必要时 Ctrl+C，若 2 秒仍未排出取消响应可终止终端连接。helper 的文件名/避重名和能力边界详见下段。 |
| SF-F15 | SSHConnectionGroup 7–104；RemoteFiles 200–222、456；FTPAndSCP 153–187；RemoteFileSession 122–153、437–472；RemoteFileWindow 48–55、112–146；WorkspaceWindows 的窗口交接见 W08 | 组由 UUID 持有者计数；lease 校验连接身份、控制路径与 socket 所有权，文件页和后端分别持有。暂停/取消释放后端，但文件页保留组以供重连；原终端关闭不主动结束仍有持有者的组。最后引用关闭发控制退出并清关联隧道/目录；迁移将文件窗 workspace 和关闭回调转交，保留现有队列。配置中的 ControlPersist 为 60，保留引用不保证控制连接永不失效；复用失败不可隐式开启新认证，也不是跨进程/全局主机连接池。 |

### ZMODEM helper 的真实边界

[Transfer.swift](../validation/oshell-reference/Sources/OShell/Transfer.swift) 47–60 上传调用 `lsz -b -e -vv -w 16384 -- 文件…`，下载调用 `lrz -b -e -vv -E --junk-path`，固定 `LC_ALL=C`。helper 位置来自应用包，支持 OSHELL_HELPERS 开发覆盖；[build.sh](../validation/oshell-reference/scripts/build.sh) 29–55 和 [release_helpers.py](../validation/oshell-reference/scripts/release_helpers.py) 核对随包构建/发布入口。未因第三方程序有其他命令行选项而扩大产品范围。

静态读取 [lrzsz-0.12.20.tar.gz](../validation/oshell-reference/Vendor/lrzsz-0.12.20.tar.gz) 内的 C 源码，未解包执行 helper：`src/lrz.c` 1092–1428 的文件处理使用 junk_path 取最后斜杠后的名称，`-E` 的普通冲突改名从 .0 起有限尝试；检查与最终 fopen 分离，非排他创建。对端 ZCRESUM 分支可绕过普通改名进入已有文件续接，不能把 `-E` 描述成任意对端下绝不修改旧文件。`checkpath`（1593–1638）按 Restricted 等级做路径/隐藏文件检查；Restricted 默认 1，应用未传 `-U`，不是无约束接收，但也不是 SFTP 续传的片段身份/排他发布保证。远端命令默认关闭，应用未传开启参数，ZCOMMAND 分支拒绝执行；X/YMODEM 和远端命令不构成应用功能入口。

`src/lsz.c` 996–1116、1187–1261 上传时打开源文件并在文件头发送名称/size/mtime/mode，目录或块设备跳过；应用选择框不选目录，不能据 helper 内部选项推导递归上传 UI。helper 处理直接文件的链接/模式与 SFTP 不同。下载取消后的片段是否清除取决于 helper 分支和退出方式，应用没有 SFTP 式检查点恢复入口。

本次确认的是应用所用参数、可达传输分派、文件管理和结束交接；没有声称逐行审查 lrzsz 的全部 CRC、协议重试算法和未暴露选项。该压缩包 SHA-256 为 `c28b36b14bddb014d9e9c97c52459852f97bd405f89113f30bee45ed92728ff1`，以固定包而非网络最新版为证据。

### 原描述纠正与有限验收

2026-10-24：修正 F03“失败保原选择”的过度概括；F04 区分递归跳过链接与直接跟随链接；F05 补出 size 零值及 MLSD 类型限制；F08 区分各后端并发防覆盖和独立 SCP 明示覆盖；F10 取消断开后端、目录非聚合进度；F11/F13 暂停不属于结束记录；F12 远端身份/内容检查的可观测范围；F14 取消无响应会结束连接。源码限制与项目安全要求分别记录，不能据原版存在薄弱分支降低 Desktop 已有防损坏要求，也不能把更强保证伪称原版已有。全部保持原 15 个 SF 身份。

有限验收使用合成资料/文件：独立与复用连接各一次成功/拒绝、取消认证晚到回调；三个文件标签复制/排序/关末页及忙时取消关闭；正常/失败导航和子串筛选；普通文件/空目录/含子目录/直接链接/递归子链接；MLSD/LIST/NLST 分别成功、坏行/非 UTF-8；每后端一个普通重名及并发目标样例；限速上下界/运行调整/取消；两文件批次首项成功后失败、准备失败和取消；记录裁剪与暂停保留；SFTP 上传/下载各正常续传、已传前缀变更、可检测源变、片段替换/硬链接、目标已存在/排他发布失败；ZMODEM 分块握手/结束、选择取消、取消响应/无响应和同名接收；关闭源终端/最后持有者、转交窗口及控制 socket 失效。这里只界定有限场景，没有执行真实服务器传输、macOS 选择框/窗口事件或 helper 互通，不能写为已验收。

### 已阅读文件范围

本域整文件已读：RemoteFileWindow（153）、RemoteFileSession（516）、RemoteFiles（458）、FTPAndSCP（189）、FTPStreamingUpload（73）、FileTransferRateLimiter（52）、FileTransferHistory（187）、RemoteFileMetadata（73）、RemoteFilePresentation（124）、ResumableFileTransfer（55）、Transfer（187）、TransferDiagnostics（25）、ZmodemProgressView（107）、SSHConnectionGroup（104）；Core 的 Streams（159，日志公共实现供 I 域继续核对调用）、ZmodemFinish（87）、ZmodemProgress（60）、FileSessionAddress（30）。

公共区段：TerminalPane 120–174、678–829；PopupKeyboard 160–207；OperatorModels 的 FTPProfile/RemotePath 117–141；autoZmodem 默认值/设置入口在 Models/Dialogs（O 域已读）交叉核对；build.sh 29–55、release_helpers.py 全文。压缩包只读上述 lrz/lsz 文件处理区段，以及 lrz 的选项/默认值/Restricted/ZCOMMAND 关联分支；未把依赖整包标成逐行已审。TransferDiagnostics 只记录传输生命周期/字节队列计数等本机诊断并裁剪旧文件，未读取任何用户真实日志、密码或文件。

## 配置与同步

全应用同步任务由 [WorkspaceWindows.swift](../validation/oshell-reference/Sources/OShell/WorkspaceWindows.swift) 13、87–128 持有和分派。设置入口是 [StorageSettingsView.swift](../validation/oshell-reference/Sources/OShell/StorageSettingsView.swift)；[WebDAVSync.swift](../validation/oshell-reference/Sources/OShell/WebDAVSync.swift) 同时驱动 WebDAV 和目录两种传输。不能因类名含 WebDAV 就漏掉目录同步，也不能把本机 ConfigurationStore 的条件保存、本机冲突草稿和远端共同基准视为同一层事务。

| SF 条目 | 生产入口与源码定位 | 源码结论与关键边界 |
| --- | --- | --- |
| SF-Y01 | [StorageLocation.swift](../validation/oshell-reference/Sources/OShellCore/StorageLocation.swift) `activeDirectory/activateReplica/activatePending`；[main.swift](../validation/oshell-reference/Sources/OShell/main.swift) 29–86；[Models.swift](../validation/oshell-reference/Sources/OShellCore/Models.swift) ConfigurationStore 328–417 | activeDirectory 始终为本机 base；current 保存同步目标，正常默认 Application Support/OShell，OSHELL_DATA_DIR 有开发覆盖入口。旧直接共享目录仅启动升级，先取得本机/旧目录锁，验证主密码/密钥条件，备份既有本机配置，写加密本机副本和旧目录基准，保留共享源文件。新目录设置只改变目标、清该目标基准，不移动工作库；停用不删资料。读取已观察过的文件后发现删除，不能当空库创建。 |
| SF-Y02 | StorageLocation `validate/schedule/activateReplica`、SharedDataFile；[DirectorySyncClient.swift](../validation/oshell-reference/Sources/OShellCore/DirectorySyncClient.swift) 1–65；StorageSettingsView 167–174、233–241 | 目录必须已存在，与本机库解析符号链接后既不相同也互不包含；不能含 local-credential-key.json。新目标除 .DS_Store 外为空，已有目标要求加密 configuration.json。iCloud 未下载资源或 .icloud 占位会请求下载并报错；未解决文件版本冲突阻止传输。NSFileCoordinator 协调读写，SHA-256 作为本机条件版本；初次发布排他 rename，既有文件比较后原子替换。此协调不覆盖其他设备云端写入窗口，成功仅本机目录对齐。 |
| SF-Y03 | [WebDAV.swift](../validation/oshell-reference/Sources/OShellCore/WebDAV.swift) 6–42、86–205；[WebDAVHTTP.swift](../validation/oshell-reference/Sources/OShellCore/WebDAVHTTP.swift) 1–116；StorageSettingsView 176–180、206–231 | 地址限 HTTP/HTTPS，无 URL 内用户信息/query/fragment，有效端口，账号/密码最多 4096 UTF-8 字节并禁 NUL/换行，用户名禁冒号。HTTP 由系统 curl，HTTPS 由无缓存/无 cookie 的临时 URLSession，均 Basic 认证且拒重定向。非显式 oshell 路径先 GET 旧位置文件，存在且有强 ETag 则保留；否则 Depth:0 PROPFIND 校验唯一匹配 DAV collection，必要时 MKCOL 后复查，405 不单独算成功。probe 可能创建目录但不 PUT，也不解密共享内容；测试结果晚到时检查设置仍匹配。 |
| SF-Y04 | [SharedVault.swift](../validation/oshell-reference/Sources/OShellCore/SharedVault.swift) SharingProtection、encode/decode；StorageSettingsView 116–147、243–282；[MasterPasswordProtection.swift](../validation/oshell-reference/Sources/OShellCore/MasterPasswordProtection.swift) 8–46（凭据轮换全路径见 P） | 同步要求 verifier 存在、保存凭据无 localKeyID、主密码已缓存；包含共享代理凭据。设置可引导设置/解锁/转换本机凭据，转换逐项解密后重加密并保留身份绑定。启用传输前先保存本机加密配置。密码操作独立保存，取消同步草稿不会撤销先前已完成的密码设置；连接设置回退不等于回退本机加密升级。共享 decode 先解封/正文验证，再迁移和校验主密码、会话/代理/引用/默认值/命令/高亮。 |
| SF-Y05 | [SyncContentSelection.swift](../validation/oshell-reference/Sources/OShellCore/SyncContentSelection.swift) 1–15；[SessionSyncConfiguration.swift](../validation/oshell-reference/Sources/OShellCore/SessionSyncConfiguration.swift) 7–60；[SyncPolicySettingsView.swift](../validation/oshell-reference/Sources/OShell/SyncPolicySettingsView.swift) 10–13、47–63 | 四类中只有 sessions 默认 true，UI 允许取消，非空类别集合才有效。sessions 包含 profiles/proxies/directories 和 links 的 entries/folders/rootOrder，旧 ftpProfiles 迁入 profiles 后清空。links.visible 归 clientSettings。应用远端选择类别时从本机配置出发替换所选部分，未选本机部分保持；线上的未选类别以远端为底保留。不是只编码被选字段后丢弃其余远端正文。 |
| SF-Y06 | SessionSyncConfiguration 7–60；SyncPolicySettingsView 25–41、55–69；StorageSettingsView 255–316；WebDAVSync 225–241、305–320 | clientSettings 同步 preferences/highlightSets/links.visible，但应用时保留接收方 updateRepository、automaticUpdateChecks、masterWarningAcknowledged；quickCommands/sessionDefaults 各独立。比较用选中类别投影，完整远端先 SharedVault.decode，所以未选类别的正文错误也拒绝。类别变化清共同基准，下一轮内容有差异才需要首次选择；仅策略修改不连服务器或索要凭据。设置应用以原始三个设置文件快照拒绝跨窗旧草稿，失败尝试恢复设置与基准；恢复也失败则明确提示，不能写成跨文件完美原子事务。 |
| SF-Y07 | [SyncPolicy.swift](../validation/oshell-reference/Sources/OShell/SyncPolicy.swift) 1–37；WebDAVSync 20–114、173–205、367–375；StorageSettingsView 255–316 | 策略 v1 本机文件；默认 automatic/onStartup/afterChanges=true，check/retry=60 秒、changeDelay=2。检查/重试 0 或 15–86400、修改等待 1–300。手动入口与后台入口分开 guard；修改防抖可合并，模态窗/busy 自动延后。失败停常规检查，仅允许配置的重试或手动；诊断 summary 恢复失败暂停状态，retry=0 不在重启后自动绕过。取消更新 token、停任务/计时器/客户端，cancelCurrent 再按策略安排合法检查；不是永久关闭自动同步。 |
| SF-Y08 | StorageLocation `baselineURL/activateReplica`；WebDAVSync 149–172、228–289、305–356；SessionSyncConfiguration 61–94 | 基准按目录规范路径哈希或固定 WebDAV 本机文件保存。WebDAV 地址/用户名改变清基准，同地址账号只换密码/证书保留；目录切换清新目标基准。没有基准且投影不相同，才弹采用共享/合并/取消；相同投影可直接对齐。新空目标只有 baseline=nil 且 allowCreate 才创建；既有数据采用前备份本机原文件。初次合并先合并不同 ID 的代理/引用依赖及目录/引用顺序，再走统一冲突算法。取消首次选择保留资料并阻止后台重复推进，手动继续。 |
| SF-Y09 | [ConfigurationMerge.swift](../validation/oshell-reference/Sources/OShellCore/ConfigurationMerge.swift) 6–113；SessionSyncConfiguration 61–94；WebDAVSync 254–303；[SharedConflictResolution.swift](../validation/oshell-reference/Sources/OShell/SharedConflictResolution.swift) 34–88 | 会话按 UUID 整项三方比较，远端/本机/基准次序合成稳定 ID 顺序；独立会话修改可合并，双方修改不同字段但都不同于基准仍冲突。其他顶层字段按整值比较，代理/目录/命令/高亮/引用内部不同项目不保证自动合并；非会话字段不能套会话逐 ID 语义。主密码保护状态改变不能直接合并；远端同步先验证同一密码再对齐随机 verifier。冲突详情脱敏，需逐项选本机或共享全部就绪后才能确认；解析后校验代理依赖并规范引用。 |
| SF-Y10 | WebDAV.swift 186–204；DirectorySyncClient 35–64；WebDAVSync 237–289、305–365；Models 377–415 | GET 必须强 ETag（引号包围、受限内容、最长 1024 字节）；已有 PUT If-Match，初建 If-None-Match:*，412 冲突。只下载应用也重新 fetch 确认版本。网络操作后检查 token、主密码和本机 revision；PUT 成功但有新本机编辑/模态草稿时，持久化上传前本机快照作为续接基准，不把尚未应用的合并结果假作共同基准。本机保存仍过文件指纹条件检查。上传失败不写成功基准；取消不能撤销服务器已提交 PUT，多个本机文件写入也不是单个数据库事务。 |
| SF-Y11 | [SharedConfigurationProtocol.swift](../validation/oshell-reference/Sources/OShellCore/SharedConfigurationProtocol.swift) 1–87；SessionSyncConfiguration 29–60；WebDAVSync 43–71、183–198、254–285；Models 207–268、300–326 | 正文合同补缺省值并迁移旧 quickSendScope 字符串、activeHostProbe、FTP 初始目录、缺省 none proxy；UUID 经类型解码/编码归一化，缺省数组由模型一致化。比较的是所选配置编码，远端/基准 verifier 在认证后对齐本机；不同随机共享信封不是变化依据。本机/远端/基准投影相同直接 unchanged，不 PUT。仅本机/远端相同而基准不同时可重写本机基准；不承诺日志/摘要/本机文件零写入，也不证明任意重加密的单项密码语义等价。 |
| SF-Y12 | SharedVault 65–101；SharedConfigurationProtocol 13–64；WebDAVSync 200–208、228–290；Models 347–415；SharedConflictResolution 89–134 | 外层密码/认证失败、正文未知字段/类型/枚举/验证失败使远端 decode 在应用前失败；本机冲突草稿存在阻断远端同步，未决选择不覆盖。已接入目标突然消失不自动重建，云端占位与冲突文件不当作空文件。并发快照过期由 ETag/指纹保护，但没有受信时间戳或独立单调版本，不能据此保证发现服务器回放有效旧包。外层 Envelope 未拒未知键的原版缺口单独列下；项目严格拒未知数据要求不因此取消。 |
| SF-Y13 | [WebDAVCertificate.swift](../validation/oshell-reference/Sources/OShellCore/WebDAVCertificate.swift) 1–98；WebDAV.swift DAVRequest 43–85；[WebDAVCertificateDialog.swift](../validation/oshell-reference/Sources/OShell/WebDAVCertificateDialog.swift) 1–20；StorageSettingsView 176–231 | 系统 TLS 校验优先；例外绑定标准化主机、端口、完整 leaf DER，非共享 CA。显示清理控制字符后的主体/颁发者、有效期、SHA-256。换证即使系统信任也需重新确认；过期/不可恢复验证不允许信任。IP 直连可显式接受当前证书的用途/名称例外，仍查有效期；域名路径不套 IP 例外。确认后先进入设置草稿并测试，点击应用才保存本机加密 WebDAV 设置；忘记信任亦为草稿。测试结果检查窗口仍存在/当前方式/地址账号密码未变，不能自动信任旧请求结果。 |
| SF-Y14 | [SyncStatusWindow.swift](../validation/oshell-reference/Sources/OShell/SyncStatusWindow.swift) 1–42；[SyncStatusView.swift](../validation/oshell-reference/Sources/OShell/SyncStatusView.swift) 1–135；[SyncStatusPresentation.swift](../validation/oshell-reference/Sources/OShell/SyncStatusPresentation.swift) 1–122；[SyncStatusButton.swift](../validation/oshell-reference/Sources/OShell/SyncStatusButton.swift)；WorkspaceWindows 13–23 | 一个同步实例向多窗口通知，工具栏图标/卡片共用 presentation。独立窗非模态，关闭不取消任务；设置中排队等待模态窗结束后按已保存配置执行。阶段条工作时不定、完成才满，明确不是字节/iCloud 百分比；显示最近尝试/成功、下次检查/重试、待同步、本机诊断写入问题及错误建议。图标工作环考虑不可见/最小化/减少动态效果，完成短暂提示。锁定时引导解锁，启停按钮受 busy/queued 状态约束。 |
| SF-Y15 | [SyncDiagnostics.swift](../validation/oshell-reference/Sources/OShellCore/SyncDiagnostics.swift) 1–163；SyncStatusView 103–135；WebDAVSync 43–73、209–213、368–374 | 仅枚举事件、后端、时间、run UUID、手动标志、耗时和安全错误种类/数值代码入库；不持久化原始错误/账号/地址/路径/配置。保留 7 天、最多 2000 条且编码文件不超过 1 MiB，过大进一步裁旧；坏旧日志启动新内存文档。写盘失败保留内存并提示，不改变同步结果。摘要按目标及范围指纹区分，只有最近 scope 摘要；清空事件不删 summary。导出只含安全事件和应用/系统版本，不含 scope/localFingerprint；不会上传遥测。 |
| SF-Y16 | Models ConfigurationStore 328–417；WorkspaceWindows 87–128；[WorkspaceController.swift](../validation/oshell-reference/Sources/OShell/WorkspaceController.swift) 377–433、914；SharedConflictResolution 7–134；main 89–100 | 激活/手动触发刷新，不是持续任意文件监控。未启同步时读文件指纹变化，验证主密码状态、更新任务约束、本机密钥身份后发布各窗，不重开连接；启同步时刷新转交 WebDAVSync。本机保存比较 observed digest，外部改变则抛含基准/远端指纹的冲突；先把 base/local 草稿写私有本机文件，再读取新快照、逐项选择并 saveResolved(expected:fingerprint)。期间再变化拒写且留草稿。未决本机草稿阻断普通保存和远端同步；放弃只清草稿再刷新。 |
| SF-Y17 | SharedVault 22–101；SharedConfigurationProtocol 1–87；[SharedConfigurationContract.swift](../validation/oshell-reference/Sources/OShellCore/SharedConfigurationContract.swift) 全部生成 JSON 定义；[shared_configuration_schema.json](../internal/core/shared_configuration_schema.json)；[sync-configuration-contract.py](../scripts/sync-configuration-contract.py) | 共享配置 encode 明确 version=2，open 读 1/2，AAD 绑定版本；通用 seal 默认 v1 用于本机 WebDAV 设置，不能把它误判为共享仍写 v1。信封上限 48 MiB、明文 32 MiB、PBKDF2-SHA256 600000–2000000 轮读取/600000 轮写入、AES-GCM；正文先合同规范化再 Codable，拒未知键、类型、非法枚举/范围，nullable/default 按规则处理，归档根版本另为 1–4。固定 Swift 合同与当前字段源按生成脚本换行口径完全一致，21 个定义已结构化展开核对；这不是 Swift ↔ Go 运行互通通过。外层 Envelope 普通 Codable 的未知键校验缺失见下。 |

### 需要区分的源码限制与项目要求

2026-10-24：纠正 Y05/Y06 的“默认会话资料不可取消”暗示，四类都可选，至少一类；补充客户端设置中三个本机保留字段和 Links 显隐归属。纠正 Y09“所有对象独立合并”的过度概括：只有 profiles 按 UUID 整项合并，其他顶层字段可能整类冲突。纠正 Y08“每次首次接入都弹选择”：无基准但所选内容相同可以直接对齐。Y03 的测试连接会创建缺失子目录，但不验证 PUT 或包解密；Y14 的阶段条不是真实传输百分比。这些细节归原 SF，不增加产品入口。

Y04 的主密码操作与同步设置提交分离：设置页面内先完成的密码设置/凭据转换可能在取消后续页面时仍保留；启用同步先加密本机库，随后连接设置失败回退不意味着本机加密被撤销。Y10 的取消是停止后续工作，不是撤回已经成功的远端 PUT；上传后的新本机编辑另存续接基准，后续重新比较。不能把不同文件/服务的操作描述成一个可随时回滚的全局事务。

Y12/Y17 已定位的原版限制：SharedVault.open 通过 `JSONDecoder().decode(Envelope.self, ...)` 读取外层 format/version/iterations/salt/ciphertext，没有对外层 JSON 键集合执行 SharedConfigurationProtocol 那样的检查；普通 Codable 会忽略其他键。已解密的配置正文则执行严格合同检查。ConfigurationStore 读取非加密本机 JSON 也直接 Codable 解码并做模型校验，不能据共享正文检查推导本机任何格式都拒绝未知键。**项目 AGENTS.md/配置协议对共享数据“未知字段拒绝应用和覆盖”的要求继续有效**；这里登记原版与目标安全约定的差异，不能据原版缺口允许 Desktop 静默丢字段。本轮不改生产代码，不声称该差异已通过双端测试闭合。

正文校验、模型迁移和引用规范化本身也有不同职责：合同负责支持的结构、类型和枚举，模型负责具体业务约束，部分模型会补默认/规范路径。不能把“通过合同”表述为所有输入语义都未经改变地保存，也不能把 ETag 并发保护表述为具有历史反回滚证明。后续发现固定正文模型实际丢失合法资料的可复现缺陷，应归 Y12/Y17 修复，不另造功能编号。

### 有限验收与已阅读范围

有限场景：离线本机读写/旧目录迁移/停用；目录空、已有、未下载占位、冲突副本、含本机密钥；WebDAV 旧路径/子目录/非目录/405 后复查/重定向/弱 ETag；四类别开关和至少一类、未选类别非法正文；手动与自动防抖、禁重试重启恢复、模态延后；首次采用/合并/取消/相同内容；独立会话修改、同会话双边改动、非会话整类冲突、删除对编辑；412/远端删除、PUT 后本机编辑/模态草稿/取消、本机保存指纹再变；无变化 UUID/缺省/空数组；密码错/包损坏/未知正文与外层字段/未来版本；证书正常/例外/换证/过期/IP 限定；状态窗关闭/多窗同步、日志裁剪/写盘失败/导出；本机冲突草稿解决/放弃/重启。只列有限验收输入，不声称已运行。

本域整文件已读：Core 的 StorageLocation（含 SharedDataFile）、DirectorySyncClient（65）、WebDAV（205）、WebDAVHTTP（116）、WebDAVCertificate（98）、SyncContentSelection（15）、SessionSyncConfiguration（94）、SharedConfigurationProtocol（87）、SharedVault（101）、ConfigurationMerge（113）、SyncDiagnostics（163）；App 的 WebDAVSync（376）、StorageSettingsView（321）、SyncPolicy（37）、SyncPolicySettingsView（70）、WebDAVCertificateDialog（20）、SharedConflictResolution（134）、SyncStatusWindow（42）、SyncStatusView（135）、SyncStatusPresentation（122）、SyncStatusButton。生成 SharedConfigurationContract（1048 行）按完整 JSON 解析后展开所有 21 个定义及每字段类型/default/nullable/required/enum/range/ref 等规则，与字段源完整比对；没有用文件名或哈希代替字段阅读。

公共区段补核：Models 159–417（Preferences、Configuration、ConfigurationStore）；WorkspaceController 377–433/914、WorkspaceWindows 全文（本域重点 13–23、87–128）、main 17–101；PrivateFile 全文、MasterPasswordProtection 8–46（凭据和身份绑定完整证据见 P）。同步字段的具体会话/代理/默认值/命令/高亮/快捷键模型已在 S/C/O/A/P 的所属路径核查，本域追踪选择、编码、迁移、合并与应用。未运行 macOS 原版、iCloud、WebDAV 服务或真实 Swift ↔ Go 往返，不能用静态合同一致代替双端验收。

字段源按生成脚本的文本换行口径 SHA-256 为 `9a82a335e40d315f9a54455c874f87bcaea0fe3551a69770d67955f439261657`，与固定 Swift 文件头及生成内容一致；Windows 原始 CRLF 文件字节哈希不同不构成字段差异。本轮未修改字段源、生成合同、协议生产实现或其他项目。

## 终端：用户交互与本机工具

分析的是应用实际链接的本地 SwiftTerm。根 Package.swift 引用本地 Vendor，默认排除 OShell/TestSupport；Vendor 的 [Package.swift](../validation/oshell-reference/Vendor/SwiftTerm/Package.swift) 明确排除 iOS、文档及 Mac README，默认 macOS 13、legacy 10.13。不能把 iOS 的选择手柄/系统服务、测试快照或上游其他产品入口纳入 OShell。以下 I01–I13 记录已读的直接路径；字符/缓冲/协议/布局等公共层继续在 I14–I27 闭合。

主要文件简称：[TerminalPane](../validation/oshell-reference/Sources/OShell/TerminalPane.swift) 含 OShellTerminal 的应用覆盖；[MacTerminalView](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/Mac/MacTerminalView.swift)、[AppleTerminalView](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/Apple/AppleTerminalView.swift) 和 [Terminal](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/Terminal.swift) 为随包生产组件。行号沿用固定提交。

| SF 条目 | 生产入口与源码定位 | 源码结论与关键边界 |
| --- | --- | --- |
| SF-I01 | TerminalPane 27–29、153–173、256–263、837–861；MacTerminalView 1326–1368、1598–1670、1902–2082、2455–2529 | 用户按键/提交用 userInputDepth 区分于终端协议回复；同步路由只处理前者。marked text 显示独立覆盖层，提交才发送，unmark 清覆盖层；Kitty 下组字及 Option 组合字符另有分支，完整编码归 I18。第一响应者更新光标/焦点，关闭清 owner/回调和响应者。失焦/隐藏未显式调用 unmarkText；不能以有 NSTextInputClient 接口即宣称所有 IME 生命周期已正确，见下方证据限定。 |
| SF-I02 | TerminalPane 11–25、63–120；[TerminalSelectionSettingsView.swift](../validation/oshell-reference/Sources/OShell/TerminalSelectionSettingsView.swift) 8–24、68–77；[SelectionService.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/SelectionService.swift) 1–794；MacTerminalView 2790–2882、2907–2943、2955–2968、3021–3062 | 自定义分隔符仅本机，最多 256 Character、无控制字符；空白始终分词。双击设置非 nil 分隔符，组件中 nil 时的括号表达式选取不能当应用默认行为。连续第二次双击需无修饰键、同选区/设置/缓冲/视口，时间不超 max(1 秒, 系统双击间隔×3)、位移各轴≤4；种子文本≤65536 UTF-8 字节。三击选行，词拖选保原词锚；宽字续格归前格，跨软换行扩词。局部滚动只有完整位于滚动区的选区平移，重叠等情况清除；不是持续输出时无条件保留。越界拖选 50 ms 定时滚动，松开停止；失焦/隐藏取消缺口见下。 |
| SF-I03 | TerminalPane 30–36、111–121、251–254；[OperatorModels.swift](../validation/oshell-reference/Sources/OShellCore/OperatorModels.swift) 92–105；Terminal 7226–7229、7860–7999 | 手动 copy 无选区守卫，直接清剪贴板再写提取值；自动只在鼠标松开/全选且 active、原始提取值非空时触发，不对程序化搜索选区自动复制。底层按逻辑顺序取字形、跳过宽字续格、NUL→空格，逐行 trimRight、软换行拼接、忽略末尾空白行；应用随后仅裁整段首/尾 Unicode 空白。内部缩进保留不等于原始 PTY 字节或尾空白保真。 |
| SF-I04 | TerminalPane 37–42、123–126、256–263；[WorkspaceInput.swift](../validation/oshell-reference/Sources/OShell/WorkspaceInput.swift) 46–72；[InputTools.swift](../validation/oshell-reference/Sources/OShell/InputTools.swift) 62–75；OperatorModels 107–115 | 先读图片，非图片再读字符串。初始字符串非空、≤1 MiB、无 NUL；多行预览允许编辑，取消不发，关闭后续预览须先保存设置。预览后复查内容非空及目标存活/可输入，但未重验字节数/NUL。一般目标取当前同步集合或单源，交互工具走单源特殊路径；没有图片发送同等的进程身份快照。CRLF/CR→LF、去全部 ESC；bracketed 保 LF 并加 200/201 framing，否则 LF→CR；其他控制字符不一概删除。 |
| SF-I05 | [TerminalSearchPanel.swift](../validation/oshell-reference/Sources/OShell/TerminalSearchPanel.swift) 1–180；[SearchService.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/SearchService.swift)、[SearchEngine.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/SearchEngine.swift)、[SearchLineCache.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/SearchLineCache.swift)、[TerminalViewSearch.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/TerminalViewSearch.swift) 全文；AppleTerminalView 2827–2945 | 每分屏独立非模态栏，默认不区分大小写/非正则/非整词；开启时可取单行≤4096 字节选区，正则时转义。编辑延迟160 ms、输出统计刷新500 ms，IME 组字期阻止编辑搜索/导航/Enter/Esc。Enter 前进、Shift+Enter 后退；关闭清搜索/选区锚但保查询框/选项。逻辑行拼接软换行、宽格映射及字素边界；整词为字母数字/组合标记/下划线，与选择分隔符无关。findAll 非重叠计数，1001 截断显示1000+，导航另找不受该数量限制。4 KiB 查询、正则软换行65536格及0.25秒检查预算；不是硬实时保证。解析/搜索错误禁导航与自动刷新，统计受限仍可导航。feed/尺寸/容量变化失效缓存，锚用缓冲身份/列数/裁剪计数恢复。 |
| SF-I06 | TerminalPane 455–457、533、712–717、830–836、1004–1015；[WorkspaceController.swift](../validation/oshell-reference/Sources/OShell/WorkspaceController.swift) 871–881、961–962；[Streams.swift](../validation/oshell-reference/Sources/OShellCore/Streams.swift) 7–31、118–159；AppleTerminalView 2921–2945 | 回滚数来自共享偏好，运行期 changeScrollback；底层裁剪/reflow 归 I15。日志用保存面板选择路径、0600 创建文件，display 的输出串行经 PlainTextFilter 写入；只记录开启后的输出，不重建终端屏幕，不逐项录用户按键。过滤器跨块跟踪 ESC/CSI/控制串，保 CR/LF/Tab 和普通字节；不是完整终端协议解释器或脱敏器。待写累计超4 MiB停止并报错，写失败也停止，stop 在队列末关闭。连接结束先停日志再显示结束提示；本机提示符直接 feed 的编辑回显不经 display，工具 PTY 输出经 display。 |
| SF-I07 | Terminal 2624–2673、7231–7858；AppleTerminalView 1402–1508；MacTerminalView 1217–1230、1488–1567、2796–2821、2955–2968、3110–3191；TerminalPane 175–177 | OSC8 存 payload，显式优先于隐式；隐式正则能识别多种 scheme 和路径、拼接软换行，并以接缝两侧各96格等启发式识别编辑器折行。识别不等于能打开；宿主仅 URL scheme http/https/mailto。默认 hoverWithModifier：Command 悬停并在松开点再次命中才打开，拖选抑制；没有单独要求松开点与按下 URI 相同。命中借 BiDi 反映射，渲染实现归 I24。OSC8 字节以 ASCII 解码；显式点击 urlAndParamsFrom 按全部分号拆分只取第二段，URI 后续分号被丢，不能宣称任意 URI 保真。 |
| SF-I08 | [LocalToolCommand.swift](../validation/oshell-reference/Sources/OShellCore/LocalToolCommand.swift) 7–67；[LocalToolProcess.swift](../validation/oshell-reference/Sources/OShell/LocalToolProcess.swift) 1–70；TerminalPane 616–677 | help/? 展分类，tools 逐项解析路径/未安装。启动独立 LocalProcess PTY、HOME 工作目录，移除 OShell/SSH askpass 认证变量，设 TERM/COLORTERM/LC_CTYPE；ssh 另用既有 SSHEnvironment。程序按名单 basename 匹配，绝对路径允许、相对带斜杠拒绝；固定系统/Homebrew目录先于绝对PATH项，检查符号链接目标为普通可执行文件，无签名/来源验证。resize 跟随窗格；退出每20 ms等输出排空后回提示符，无显式排空期限；stop 只向专用PTY前景组发SIGHUP再终止。工具输出不走SSH的ZMODEM检测。 |
| SF-I09 | LocalToolCommand 11–20、22–35；LocalToolProcess 22–23；TerminalPane `blocksManagedToolInput` 296 与 `acceptsManagedInput` 256–258 | 固定八组完整名单已列基线 SF-I09，不把各工具全部协议/参数当产品功能。允许工具数量和本机已安装数量不同；没有自动安装或提权。permitsManagedInput 排除 nc/telnet/ssh/nslookup/curl/openssl/scutil/iperf3，防止托管/同步输入进入这些交互工具；手动键盘及本分屏粘贴仍走其 PTY。 |
| SF-I10 | [LocalPromptInput.swift](../validation/oshell-reference/Sources/OShellCore/LocalPromptInput.swift) 65–173、175–295；[EndedSessionInput.swift](../validation/oshell-reference/Sources/OShellCore/EndedSessionInput.swift) 7–23；LocalToolCommand 36–57；TerminalPane 630–638 | Swift Character 行编辑、分块 UTF-8、8192字节上限；非法/不完整字节及C1输入替换字符。词以空白划分，Ctrl+A/E/B/F、Ctrl/Alt箭头及Alt+b/f，Backspace/Delete、Ctrl+U/K/W/Y、Alt+d/退格、Ctrl+T；仅一份最近kill文本，无undo栈。Ctrl+L清可见屏保输入，Ctrl+C清草稿，Ctrl+D空行关当前分屏。Tab只补名单和help/tools/exit/quit命令名，大小写敏感前缀；解析命令名则不区分大小写。引号/反斜杠拆argv，未闭合报错；只拒独立Shell操作符参数，嵌入符号仍是字面参数，无eval。OShell总是allowsLocalTools=true，不把EndedSessionInput的128字节旧close-only分支算生产功能。 |
| SF-I11 | LocalPromptInput 9–63、157–173、219–244、272–287 | 每提示符对象的内存历史100条，非空且未超限、非exit/quit才存，存原输入全串并仅去相邻完全重复；先入历史后parse，故含无效命令/help/tools。上下浏览保存草稿；Ctrl+R发起/向旧、搜索中的Ctrl+S向新，大小写敏感子串、无循环；Ctrl+G恢复原草稿与光标，Esc接受为可编辑输入，Enter仅在成功匹配时执行。工具启动后的输入直接到PTY不入这份历史；命令参数如含秘密并无自动删除/脱敏。 |
| SF-I12 | TerminalPane 616–677、837–861、1004–1018；LocalToolProcess 41–70；LocalPromptInput 255–257、272–287 | 连接结束关闭broker/保活/探测、取消传输、排剩余文本并停日志，保原显示进入本机模式；不自动重连。提示符只重置SGR和显光标，工具退出额外CAN、退出备用屏和bracketed paste，不是RIS。exit/quit/Ctrl+D空行回onCloseRequested关当前pane。一次输入在启动工具后的剩余部分留到工具结束，排空期间用户字节可进≤1MiB队列，协议回复不排入；Ctrl+C字节清队列，shutdown清队列/工具/回调。排空分支超限不追加且无提示，另一个异步合并分支超限会清队列并提示，不能笼统称所有溢出有通知。 |
| SF-I13 | TerminalPane 154–167、256–263、678–829；WorkspaceInput 41–72；MacTerminalView 3222–3299 | isTransferring 先于普通同步路由，吞掉发送字节；其中含0x03则取消。托管入口禁传输目标，粘贴在目标复查失败时不发。本地选择/复制/主屏历史滚动不需进PTY；鼠标上报与备用屏滚轮生成的远端字节仍被拦截。I18已确认：disambiguate模式Ctrl+C产生ESC[99;5u，不含0x03，因此不会触发此取消分支；不能宣称所有物理Ctrl+C都可取消。传输检测/初始字节/取消排空的完整证据已归F14，不另造传输功能。 |

### 源码事实与验收要求的差异

2026-10-24，I01/I02：基线中的“失焦/隐藏取消拖动”和输入法生命周期是验收要求。固定 MacTerminalView 的 resignFirstResponder 只改焦点/光标，viewDidMoveToWindow/viewDidUnhide 处理追踪/渲染；拖选定时器只在 mouseUp 或后续拖动不再越界时停止，未找到失焦/隐藏/析构主动停止该定时器的调用。marked text 只在提交/设置/显式 unmarkText 清理；系统 AppKit 是否补发取消须实机验证。静态缺口归已有条目，不能写为“已支持所有切换”，也不能删掉验收要求。selectedRange 2465–2466 对 active 选区以 displayBuffer.rows 计算线性位置，而插入点2477使用cols；列/行不等时与行优先网格索引不一致，保留为 I01 输入法范围映射的待复现差异，不在本轮改生产代码。

2026-10-24，I03/I04：将“保留原字符内容”收窄为固定选区提取结果，明确底层行尾/末空行裁剪；应用首尾清理不修改内部缩进。粘贴预览允许改内容却仅在预览前执行1MiB/NUL检查，确认后也无进程代际快照；原版证据不支持“全程已重新校验”。相关保护仍是复刻验收关注项，不因原版缺口放宽安全要求。

2026-10-24，I07：显式 OSC8 的 ASCII 解码和 URI 分号截断是生产路径事实。例如 `;https://example.invalid/a;b` 在模型链接提取时保留 URI，而 AppleTerminalView.urlAndParamsFrom 在点击时只取 `https://example.invalid/a`。这是由读取代码直接推导的差异，未执行点击测试；不能把底层广泛URL/文件路径识别，误写成OShell允许打开这些方案。

2026-10-24，I08/I10/I11/I12：补齐完整工具名单、程序解析/路径、有限编辑、历史、工具排空及排队边界。允许basename不是可执行来源沙箱；“工具中的密码不入提示符历史”不是“所有敏感命令均脱敏”。提示符关闭的是当前分屏，原help里的“关闭标签”不能覆盖实际回调。退出仅部分模式恢复；完整键盘/鼠标/字符模式生命周期继续核I18–I21。

### 已阅读范围与有限核验场景

整文件已读：应用 TerminalSelectionSettingsView、TerminalTextSettingsView、TerminalSearchPanel、LocalToolProcess；Core LocalPromptInput、LocalToolCommand、EndedSessionInput；SwiftTerm SelectionService、SearchEngine、SearchService、SearchLineCache、SearchOptions、SearchState、TerminalViewSearch。Package.swift 与 Vendor/SwiftTerm/Package.swift 已读以确认生产目标。公共文件新增/复核范围：TerminalPane 1–183、190–320、550–677、704–718、830–871、1004–1021；WorkspaceInput 1–82、InputTools 62–75、OperatorModels 92–115、Streams 7–31/118–159、WorkspaceController 871–881；MacTerminalView 807–869、1217–1230、1326–1370、1488–1567、1598–1685、1902–2083、2455–2531、2790–2884、2907–3064、3110–3195、3222–3299；AppleTerminalView 1402–1440/1460–1512/2827–2955；Terminal 2624–2673、7121–8072。公共文件不是整文件已读；其余生产分派与渲染保持待核查。

已有验收要求可用以下有限场景落地，不取所有组合的笛卡尔积：I01中文组字提交/取消及切窗关闭；I02自定义分隔符、同位置连续双击、宽字软换行反向词拖选、越界后失焦；I03内部缩进/尾空白/末空行与手动空选区；I04多行预览取消/编辑后超限、同步目标关闭、bracketed与普通换行；I05大小写/整词/正则字素、软换行、1000+后导航、裁剪及IME；I06日志跨块控制串/写失败/积压停止；I07显式含分号/非ASCII URI、隐式软换行、受限scheme、拖动不打开；I08/I09安装缺失/绝对路径/交互工具不广播；I10/I11分块UTF-8、8192上限、引号/字面符号、补全与历史草稿；I12工具结束排空/排队/关闭；I13普通与增强编码中断。以上是从已读源码导出的待执行场景，未声称测试已运行。

## 终端：解析、解码与后续分派

| SF 条目 | 生产入口与源码定位 | 源码结论与关键边界 |
| --- | --- | --- |
| SF-I14 | [EscapeSequenceParser.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/EscapeSequenceParser.swift) 1–959；Terminal 1144–1638、1858–1874、3449–3487、6180–6209、6686–6707、6747–6757；[CharSets.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/CharSets.swift) 全文；[Utilities.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/Utilities.swift) 1–64、270–467；[Buffer.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/Buffer.swift) 1580–1708 | 状态机区分 ground/ESC/CSI/OSC/APC/DCS/SOS/PM，跨调用保留控制串/参数/打印未完UTF-8；实际字节快路径优先于转换表，不能单看表即承诺完整C1兼容。CSI/DCS参数最多24项，超项丢弃分派，数值饱和65535；CSI冒号与分号分隔信息单独保存。OSC编号必须全十进制且不溢出。解码非法首字节按自身Unicode scalar处理，畸形多字节消耗预期字节数再按首字节处理；不足则保留待下一块，不是一律U+FFFD。当前行光标前的有效宽字首格作为组合目标，结合符/ZWJ/肤色/VS/区域指示符仅在Swift Character判断能成一个字素时合并，零宽孤立字丢弃；VS15/16及窄RI有具体宽度调整，OShell默认RI为wide。固定G0–G3表、SI/SO和ESC锁定移位；未知字符集不自动转系统编码。插入写入链接/OSC133语义并为宽字写续格；完整滚动/重排/编辑归I15/I16。 |

### 固定分派入口与已发现的源码限定

此表是后续核查的封闭入口索引，不代表被调用处理器已经全部核完。具体参数/回复/状态仍须阅读所属 SF 的实现，不能只根据函数名认定通过。

| 分派家族 | 固定生产入口 | 归属与限定 |
| --- | --- | --- |
| Execute | BEL、BS、HT、LF、VT、FF、CR、SO、SI；转换表中的IND/NEL/HTS | I01/I14/I15/I17。其他execute默认不处理；ground的高字节打印快路径会绕过C1转换，见下。 |
| ESC | 6/9横向索引，7/8保存恢复，D/E/H/M索引/下一行/制表/反向索引，等号/大于号键盘模式，c重置，n/o及竖线/右大括号/波浪号锁定移位；百分号@/G默认字符集；#3/4/5/6/8行显示；空格F/G回复控制编码；括号等前缀指定字符集 | I14/I15/I18/I20/I21/I25。未分派的单次移位等不按完整ISO2022追加。 |
| CSI | @–M的编辑/定位分支，P/S/T/X/Z、反引号/a–i中的已注册分支及k–n、p–v、x/y/z/左右大括号/波浪号 | 以403–488的精确switch为准，字母范围中的空缺不是支持；I15–I21/I24。S空格前缀为方向，q大于号前缀为版本，r/s问号前缀为私有模式保存恢复，s还有移位Escape与左右边界分支；各自参数校验留所属项。 |
| OSC | 0/1/2标题，4色表，6文档，7目录，8链接，9进度，10/11/12颜色，52剪贴板，104/112颜色重置，133提示符，777通知，1337图片/内容 | 自定义处理器先于内置。OShell注册777作主机身份，因此不能把内置777通知作为应用能力；I07/I17/I20/I22/I26/I27及T域。 |
| APC / DCS | APC G→Kitty；DCS无前缀q→Sixel、美元号q→DECRQSS；其余DCS返回nil | I20/I22。Sixel能分派与对外宣告Sixel是两件事；宿主选项enableSixelReported=false。 |

2026-10-24，I14 静态证据差异（未运行Swift用例）：

- ground 快路径711–716将所有大于0x1f的字节交给print，原始0x9b等C1不会在ground按CSI启动；Utilities对0x7f–0x9f宽度为-1，而纯ASCII插入快路径又直接写格。因此“7/8位回复可切换”不等于“任意位置8位控制输入都按标准解释”。有限回归应区分ESC+[与裸0x9b、独立DEL与混合非ASCII片段。
- OSC/APC的oscPut批量循环867仅遇BEL/CAN/ESC停止；SUB在该循环中被跳过，0x9c被追加为正文，而两者若位于新调用起始则由转换表取消/终止。例如以ASCII正文后接SUB与在SUB前拆块，会走不同分支。保留为控制串分块不一致，不能仅因有SUB/ST转换项判定正确。
- DCS的dcsUnhook只有当前调用dcs起点不为-1时才调用unhook并清handler（833–840）。正文先前调用已put、下一调用才收到ESC+反斜杠时，不满足该条件；与正文和终止符同块不同。CAN/SUB通用跳转也不是同一套unhook清理。后续I20/I22分别核对DECRQSS与Sixel后果；不把这一静态发现写成已运行失败或新功能。
- 参数项数有界不等于所有解析状态有界：collect及OSC/APC缓冲在本层未设同等长度上限；图形处理器自身的资源校验须另核。超长或未终止控制串属于既有I14/I22边界，不扩成新的协议功能。

字符集表的固定键为0/2（DEC图形）、A（英）、B（美）、4（荷）、5/C（芬）、R（法）、Q（法加）、K（德）、Y（意）、6/E（挪丹）、Z（西）、7/H（瑞典）、等号（瑞士）。指定G0–G3仅作用于ASCII映射；ESC百分号@/G都恢复默认，不代表支持任意传统字符编码。字宽使用固定 [UnicodeWidthData.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/UnicodeWidthData.swift)/[AmbiguousWidthData.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/AmbiguousWidthData.swift) 数组与Swift标量属性/Character结合；后者仍受构建运行环境影响。已核入口、消费逻辑及生成表声明，不把逐个码点遍历或追随Unicode最新版作为分析完成条件。

### 颜色、属性与光标：I17

| SF 条目 | 生产入口与源码定位 | 源码结论与关键边界 |
| --- | --- | --- |
| SF-I17 | Terminal 592–653、807–825、2563–2589、2865–2983、4125–4164、4208–4214、4795–5115、5250–5258/5490–5499、8080–8097；EscapeSequenceParser 574、582–587；[Colors.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/Colors.swift) 12–50、156–490；AppleTerminalView 290–316、383–427、540–994、1194–1254、1586–1717；MacTerminalView 941–978、1009、1036–1052、3377–3382；[MacCaretView.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/Mac/MacCaretView.swift)、[Apple/CaretView.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/Apple/CaretView.swift)；AppearanceSettings 20–33、TerminalPane 516–542 | 固定SGR属性、分号/冒号扩展色、五种下划线、动态色表/默认色查询、主题派生256色、反显、文字闪烁和六种光标样式有生产路径。状态存储、查询与画面不是同一证据：SGR8隐藏、DEC12光标闪烁和OSC12/112光标颜色存在下述显示交接缺口；OSC104负索引及短色解析也有具体限制。主题及Metal恢复归A，普通字形/装饰几何关联A03–A06和I17，特殊行归I25、BiDi归I24、图像/占位归I22/I23，不据本条提前验收。 |

| 子路径（均归I17） | 固定细分行为及边界 |
| --- | --- |
| SGR基本属性 | 无collect的m进入cmdCharAttributes；0恢复当前默认Attribute（含下划线样式/颜色）。1粗体、2暗淡、3斜体、4单下划线、5闪烁、7反显、8隐藏标志、9删除线、21双下划线；22同时去粗体/暗淡，23/24/25/27/28/29分别关闭。未匹配如6快闪烁只打印未知属性，不据注释追加；CSI大于号m的XTMODKEYS分支为空，不是另一种增强键盘支持。 |
| 基础/索引/真彩色 | 30–37/40–47为前/背景0–7，90–97/100–107为8–15；39/49仅将当前前/背景属性设默认引用，不重建调色板。38/48/58分别设前景/背景/下划线色；分号5;index或2;r;g;b，冒号5:index或2:r:g:b以及2:colorspace:r:g:b，通道/index上限255。冒号扩展链整体消费，多余字段忽略，colorspace槽位不校验；不是任意颜色空间转换。 |
| 扩展参数失败 | 不完整或未知色种类不改对应颜色，但分号路径只消费已识别长度，剩余参数仍按独立SGR处理。如38;2;1;4因RGB不足不设颜色，后续1和4可开启粗体/下划线。冒号4:0–5关闭/单/双/波浪/点/虚线，其他子样式回单线；此分支只消费两个参数，不保证更长冒号链被作为整体忽略。SGR59清下划线色，SGR24清样式但不清颜色；SGR0同时清。参数数量/饱和限制归I14。 |
| 默认256色来源 | OShell未覆盖TerminalOptions的base16Lab默认：前16色由应用主题安装，16–231基于前8色、默认前背景替代0/7锚点作LAB三线性插值，232–255为24级LAB渐变；浅主题交换暗/亮锚点。组件另有xterm固定色立方及base16LabHarmonious策略，但OShell没有在此暴露选择入口，不能自动列成三个用户设置。LAB转换按固定sRGB/D65常数，输出量化到8位再扩16位。 |
| 调色板覆盖与重建 | OSC4逐对处理索引和色值；?回复当前ansiColors的rgb:rrrr/gggg/bbbb，设置只替换运行时索引并通知缓存。索引解析失败或≥256停止余下列表，非法颜色值跳过该对。installPalette要求16色并重建default/active两表；默认前景/背景在非xterm策略下变化也重建全部表，因此会丢先前OSC4运行时覆盖。OSC104空正文恢复default表；指定列表逐个恢复，不等于恢复所有动态默认色。 |
| 动态默认色 | OSC10/11/12走同一oscSetColors，按起点0/1/2依次设默认前景/背景/光标，空字段split后被省略会移位，超过第三个目标忽略。首字符?即查询，回复OSC10/11/12加16位RGB和ST；前/背景取delegate.getColors，Mac未另实现，默认读取Terminal当前值；光标nil时报告前景，不一定等于应用主题实际光标颜色。前/背景setter与native属性有递归保护，颜色变化清缓存/标全屏脏。未注册OSC110/111，不追加其重置支持。 |
| 色值文本 | OSC内部parseColor接受#后3/6/9/12位和rgb:分量；不支持名称/透明度。与公开严格Color.parse不同，OSC未先校验完整十六进制：内部遇非法字符停止，允许部分值/未消费尾部。rgb各通道统一按最长分量位数缩放，非逐通道缩放。4位通道乘0x1010，所以#fff为f0f0/f0f0/f0f0，而#ffffff为ffff/ffff/ffff；不能宣称二者等价保真。UI原生色转核心sRGB、钳位并去alpha。 |
| 显示属性映射 | bold/italic选择normal/bold/italic/boldItalic字体；dim把前景RGB与背景各取50%，代码保留前景alpha（注释“完全不透明”不作结论）。默认useBrightColors=true，bold仅把ansi索引0–6加8，7不变；组件关闭该属性时所有>7索引减8且可用粗体，但OShell未暴露此开关。默认色的反显交换前背景，不对RGB求反；SGR7与DEC5全屏反显分别生效。truecolor和属性/链接字典各有4096清空重建上限。 |
| 下划线/删除线 | 属性携带独立underlineStyle/underlineColor；共享映射保留自定义样式键，CoreGraphics显式画单/双/正弦波/点/虚线，Metal读取同键生成片段，双线单独追加第二行。删除线按字体x-height定位。链接高亮可覆盖为虚线，选区可覆盖文字/装饰颜色；不能把装饰存在解释成任意字体/双宽行几何已通过，普通装饰几何归I17，特殊行交接归I25。 |
| 文字闪烁 | SGR5按可见行扫描，在有window、应用活动、未请求减少动态效果且有闪烁行时以0.7秒定时器切换；停用时恢复可见。消隐帧清文字颜色并去下划线/删除线，保留底色。条件未显式检查祖先隐藏；排显示另有隐藏闸门，不能据此声称隐藏tab完全不运行定时器。与光标闪烁是不同生命周期。 |
| 光标样式/显示 | CSI空格q的0/1闪块、2稳块、3闪下划线、4稳下划线、5闪竖线、6稳竖线，其他忽略；setCursorStyle改options并通知Mac视图，25模式控制显隐。CoreGraphics按style和焦点启动0.7秒渐隐动画，失焦画空心框；Metal按options.style/显隐/焦点用0.7秒定时切换。不是DEC12字段直接驱动两者；下述已知差异须单独验收。 |
| 初始化与用户改色 | setupOptions仅creatingTerminal时写默认前/背景；现有Terminal进入该方法不重复写这两个默认值。OShell TerminalPane.apply每次主动应用偏好仍会重装主题、光标和16色，可能覆盖远端运行时颜色，这是应用设置入口，不能与“首帧无条件覆盖”混为一谈。完整重置/软重置保留差异见I21；SGR查询序列化缺口见I20。 |

已定位的原版差异（静态证据，未运行Swift或显示测试）：

- SGR8/28设置/清除invisible位，toSgr也会报告；共享getAttributes、文字构造和已读渲染消费路径未据该位隐藏字符，生产Swift源码中该枚举的其他引用仅是定义和序列化。不能宣称“隐藏文字已显示为空白”，也不能因此从固定SGR范围删掉该条。A06/I17共享绘制路径的最终反向核查继续交叉确认。
- DECSET/DECRST12只写Terminal.cursorBlink；两个光标渲染器以options.cursorStyle决定闪烁，未读该字段。DECRQM12报告前者，DECRQSS空格q又固定回2，三者可能不同。相关问题仍归I17，查询关联I20，不新增光标协议。
- TerminalDelegate要求setCursorColor(source:color:)；AppleTerminalView实现的是另一个带textColor参数的方法，没有默认参数。实际协议调用落默认实现，只将source.cursorColor回写；递归保护避免循环，但没有调用更新caretColor的方法。OSC12/112可改变查询状态，不能证明实际主题光标颜色跟随改变/复原。OShell主题直接设caretColor，Metal/CoreGraphics都读取effectiveCaretColor，不能以核心cursorColor取代画面证据。
- OSC104把每段转Int，失败用0，但resetColor只拒绝>255，负数仍用于数组下标；例如正文-1的路径没有下界检查，属于既有颜色重置输入缺口。未在用户应用上发送该序列。OSC色值缩放和无效值处理也按上表保留，不把“宽松解析”美化成严格校验。

有限待执行场景：基本开/关属性与组合SGR0，4:0–5和SGR21/24/58/59，分号/冒号RGB与索引0/7/8/15/16/255/超限，短色#fff对比#ffffff、混合位宽rgb、非法色值，OSC4多对设置/查询及默认色改变后的覆盖，OSC104空/单项/无效/负索引，OSC10–12空字段/查询及112，DEC5与SGR7组合，CSI空格q与DEC12/DECRQM的差异，SGR8显隐，文字与光标闪烁在焦点/减少动态效果下的区别，初建/首次显示/主动主题应用。状态和回复可用合成数据；实际文字、装饰、光标颜色及闪烁仍需有限截图/实机证据，未执行不得记已通过。

新增已读：Terminal 592–653、807–825、2563–2589、2865–2983、4125–4164、4795–5115、8080–8097，DEC5/12分支复核；Colors 1–52、156–490（预设色常量53–155不在本次新增阅读范围）；AppleTerminalView 280–449、540–678、708–994、1180–1254、1586–1717；MacTerminalView 941–978、1009、1031–1066、1158–1164、3370–3385；MacExtensions 11–83、ColorBridge全文、MacCaretView与Apple/CaretView全文；MetalTerminalRenderer 1405–1458、2782–2860、2932–2969；AppearanceSettings 1–42、TerminalPane 516–542。这里核对属性传递和直接消费，未把完整Metal或CoreGraphics渲染条目提前标为已核查。

### 键盘、鼠标与焦点：I18–I19

生产链为 OShellTerminal → Mac TerminalView → KittyKeyboardEncoder/Terminal → TerminalViewDelegate.send → OShellTerminal.send；协议回复与鼠标消息也经过最后的应用发送闸门。不能把编码器枚举、通用delegate接口或iOS事件路径当作macOS实际可产生的输入。

| SF 条目 | 生产入口与源码定位 | 源码结论与关键边界 |
| --- | --- | --- |
| SF-I18 | [KittyKeyboardProtocol.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/KittyKeyboardProtocol.swift) 全文；[KittyKeyboardEncoder.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/KittyKeyboardEncoder.swift) 全文；Terminal 404–425、940–1079、5238–5239/5266–5269、5472–5473/5502–5505、6725–6739；MacTerminalView 1514–1555、1576–2452；AppleTerminalView 2650–2678、2949–3041；TerminalPane 27–29、154–167；WorkspaceWindow 8–47 | 传统键与增强键共用编码器但有AppKit分派差异；应用光标/数字键盘、导航/功能键、Meta前缀、控制字符、Kitty五标志和主/备用屏独立16层栈。报告文本需reportAllKeys且reportText；释放需reportEvents，非all模式Enter/Tab/Backspace不发释放。IME走已提交文字，Option非Meta可直接发组合文本。布局备用字符是固定ANSI硬件表，不是动态读取所有物理布局。快捷键keyup、纯修饰键广播和增强Ctrl+C存在下述交接缺口；固定编码事实不等于所有物理组合验收通过。 |
| SF-I19 | Terminal 568–589、655–735、5273–5306、5506–5556、6898–7017；MacTerminalView 67–153、807–867、1204–1214、1326–1485、2782–2884、2907–3064、3164–3299；TerminalPane 44–127、154–173；AppleTerminalView 2949–3041 | 跟踪模式9/1000/1002/1003与坐标编码1005/1006/1015/1016分别控制；Shift本地选择与XTSHIFTESCAPE、主屏历史/备用屏滚轮、1004焦点回复及macOS26窗口移动监视器均在生产路径。宿主坐标由视图点/格和BiDi反映射产生，不能泛称物理像素。X10按下/释放、任意移动绕过Shift判定、窗口焦点通知及右/中键缺少等价显式路由的限制见下。关闭时应用发送闸门拦截，但隐藏/跨窗事件正确性仍需实机证据。 |

I18 的细分功能与固定限制：

| 子路径（均归I18，不新增编号） | 固定源码行为 |
| --- | --- |
| 传统文本/控制键 | 普通文字经interpretKeyEvents/insertText发UTF-8；旧Control路径仅单字节字母及反斜杠、下划线、方括号、脱字符/6、空格映射，其他输入返回空数组。不能拿编码器更丰富的legacyControlMapping覆盖宿主这条分支。Option-as-Meta默认true，Cmd+Option+O切换当前view；未见应用持久化设置入口。Backspace默认DEL，组件另有Control-H属性但OShell未设置。 |
| 导航/功能键/数字键盘 | 默认导航CSI，应用光标下无修饰方向/Home/End可用SS3；F1–F4传统SS3，其余固定tilde/修饰映射；增强F3用13~。编码器具F1–F35和传统扩展修饰能力映射，Mac映射相应AppKit功能标量；不保证每种键盘能产生这些标量。硬件ANSI数字键盘和numericPad导航有单独映射，ESC等号/大于号及模式66切换应用键盘。无修饰PageUp/Down在传统模式且非应用光标时滚本地历史。 |
| Kitty协商/栈 | 已知位掩码31：消歧义1、事件2、备用字符4、全部键8、关联文本16；未知位丢弃。CSI?u查询当前屏，CSI=flags;modeu按1替换/2并入/3移除，其他mode不改；CSI>flagsu推入旧值，超过16先丢最旧；CSI<countu至少弹1，超过现有深度清空。主/备用屏独立，setup清两者；完整RIS/DECSTR保留矩阵归I21。普通CSIu仍恢复光标。 |
| 编码与事件 | 全部键隐含消歧义；修饰数值为bitmask+1，repeat/release为2/3且只在请求事件时附加；单独请求文本位不生效。关联文本过滤C0/C1，Alt/Ctrl/Super/Hyper/Meta修饰阻止携带文本，释放无文本。编码器CSI始终ESC+[，不跟随核心cc.send8bit。普通文本在非all模式可继续发UTF-8而无独立释放。 |
| 布局/修饰键 | Mac只生成Shift、Ctrl、按includeOption决定的Alt、Command→Super及CapsLock；Hyper/Meta/NumLock位或媒体/音量等编码器枚举不证明宿主产生。charactersIgnoringModifiers首标量小写化为主键，Shift字符来自characters首标量；备用基础键来自静态ANSI键位表。左右Shift/Ctrl/Alt/Super和CapsLock有flagsChanged映射，按聚合modifierFlags判断按下状态，非独立物理按键状态机。 |
| IME/死键/AltGr | pending事件只用于单标量提交；多标量提交构造无物理键、无修饰的文本事件。组字时宿主不发flagsChanged增强事件，Enter携带非控制提交文本可直接UTF-8、Backspace编辑组字不发。Option-as-Meta关闭且无Ctrl/Command时打印文本直发，避免把Option产生的字符变成Meta；不据此承诺所有输入法布局的物理keyup一致性。粘贴沿用独立bracketed路径，见I04。 |

I18 与其他域交接的静态差异（未执行macOS事件测试）：

- WorkspaceWindow.navigate只消费keyDown，sendEvent/performKeyEquivalent没有维护被消费键集合；MacTerminalView.keyUp只检查当前增强标志/修饰/功能键，没有核对该键按下是否送入终端。由此不能宣称O09消费的快捷键在reportEvents/reportAllKeys下绝无keyup泄漏。例如关闭终端搜索的Esc按下被消费，随后送达终端的Esc释放仍可被编码。具体AppKit投递顺序保留为实机待验收，不把静态路径写成已复现结果。
- OShellTerminal只在keyDown/keyUp/insertText增加userInputDepth；纯修饰键flagsChanged直接经sendKittyEvent发送，深度为0，因此不走onUserInput广播，而普通增强键事件可以广播。左右同类修饰键同时按住时，flagsChanged用聚合标志判按下/释放，不能保证区分松开其中一侧。均归I18，广播关联O06；WorkspaceInput.routeKeyboard把源编码字节直接发给各目标，不按目标Kitty模式重新编码。
- disambiguate模式Ctrl+C经主键99和Ctrl数值5编码为ESC[99;5u，不含0x03。TerminalPane传输闸门只搜索字节3，因此此物理组合被吞掉而不取消；这是I13/I18已有交接问题，不追加一种“增强传输”功能。

I19 的细分功能与固定限制：

| 子路径（均归I19，不新增编号） | 固定源码行为 |
| --- | --- |
| 跟踪与编码独立 | DECSET9→x10、1000→vt200、1002→按键拖动、1003→任意移动；1001高亮与locator无生产实现。对应reset关闭跟踪。1005 UTF-8、1006 SGR、1015 urxvt、1016 SGR像素；任一编码reset回x10字节编码，不关闭跟踪，不维护多编码优先级栈。 |
| 坐标/按钮字节 | 普通x10字节编码将位置+33钳至255；UTF-8编码存在ch==2047单独发NUL、超限钳2047的分支。SGR/urxvt格坐标+1；SGR像素直接用视图内截断的点坐标，未乘屏幕backingScale，也未在发送处+1。释放先统一为3，SGR清最低两位并用小写m，故不能从此回复恢复原释放按钮。按钮编码显式接受0/1/2和轮4/5，其他按0。 |
| 命中/本地优先级 | 格列钳至屏宽、行按displayBuffer.yDisp计算并在上报时转回视口行；BiDi视觉列转逻辑列。Shift且未请求capture时点击/拖动/滚轮走本地路径，CSI>1s开启捕获、>0s关闭。mouseup先尝试打开允许的链接，再处理远端释放，最后延迟语义点击；不是远端跟踪绝对优先。选择自动滚动归I02、链接归I07、语义点击归I26。 |
| 滚轮/排列交接 | 精确滚轮累计点数除格高保余数；非精确刻度至少一行。跟踪发轮4/5，否则主屏滚历史；备用屏1007开则用传统Up/Down，关闭则不发并清累计值。OShell在外层先允许Option、水平手势或主屏历史到边时交给TabArrangementView；不能宣称所有TUI滚轮都发远端。备用滚轮生成的方向键未经过Kitty编码器。 |
| 焦点/窗口 | 1004开启立即按reportedFocusState发CSI I/O，初始状态true；setTerminalFocus不去重。Mac只在成功become/resignFirstResponder调用；窗口didBecomeKey/didResignKey观察者只刷新光标/Metal，不发焦点报告。视图仍为firstResponder时单纯窗口失焦不能由此路径保证得到CSI O。 |
| 移动事件生命周期 | macOS26共用每窗口弱引用view集合和单一mouseMoved监视器，首响应者不重复合成；检查event.window、跟踪需求与点落bounds，迁移解绑旧窗口，最后退出移除监视器并有条件恢复acceptsMouseMovedEvents。监视器在应用不活动时不收到事件；过滤未显式检查isHidden/祖先隐藏，因此不据此宣称隐藏目标永无上报。关闭pane另由OShell发送闸门拦截。 |

I19 原版缺口：MouseMode.sendButtonPress不含x10，而sendButtonRelease含所有非off模式，和“X10仅按下”的注释不符；实际Mac按下落本地选择分支、释放可上报。mouseMoved的anyEvent分支没有allowMouseReporting和Shift绕过判断，和按下/拖动/滚轮不同。编码器接受右/中键并不等于AppKit已接入：MacTerminalView没有right/otherMouseDown、Up或Dragged的显式转发，OShell只覆盖右键按下以按设置粘贴或交给父类；不能以通用按钮枚举宣称完整多按钮远端上报。上述是有限、具体的源码差异，复刻时要记录修正或保留的预期，不能偷偷缩小“正确仲裁”的验收要求。

有限待执行场景：I18分别验证主/备用屏协商及第17次push/超深pop、传统/应用方向与数字键盘、F3及一个修饰F键、全部键+文本+重复释放、中文多字提交/Option组合字符、关闭搜索Esc的keyup、纯修饰键广播、双侧Shift释放、增强Ctrl+C取消。I19分别验证9/1000/1002/1003入口、编码切换不关跟踪、五种坐标编码的边界值、Shift与capture、链接优先、普通/精确轮与1007开关、排列接管、1004初次报告及窗口切换、右/中键、隐藏/跨窗移动/关闭后输入。协议字节可用合成事件验证；物理键、AppKit焦点及事件投递必须另有实机证据。不对所有模式、布局、平台取笛卡尔积，也不以每个回归新增SF。

已读范围：KittyKeyboardProtocol、KittyKeyboardEncoder、EscapeSequences（含CC）、WorkspaceWindow全文；MacTerminalView 67–153、807–869、1204–1230、1326–1567、1576–2454、2782–3064、3110–3299；AppleTerminalView 2650–2678、2949–3055；Terminal 404–425、568–736、940–1143、5230–5325、5460–5575、6725–6739、6890–7017；TerminalPane 1–183及455–462。与先前已读区段有交叉，不能把重读计作新增功能或把公共文件剩余部分认作已覆盖。完整查询、重置及BiDi渲染分别继续归I20/I21/I24。

### 查询、状态与宿主回复：I20

| SF 条目 | 生产入口与源码定位 | 源码结论与关键边界 |
| --- | --- | --- |
| SF-I20 | Terminal 1087–1140、1184–1192、2732–2775、3562–3596、3719–3741、3819–4059、4167–4205、4363–4589、4657–4725、5719–5778、5855–5861、6129–6154、7027–7041；[CharData.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/CharData.swift) 133–179；[SwiftTermBuildInfo.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/SwiftTermBuildInfo.swift) 全文；MacTerminalView 414–435、870–932、3385–3401、3437–3553、3626–3628；TerminalPane 455–457、872–882 | DA/DSR/DECRQM/DECRQSS、矩形校验和、XTVERSION、窗口尺寸及标题状态均有固定分派，但实际状态与固定报告混合。宿主只实现部分尺寸回复，忽略窗口操控；标题查询空回、图标标题无UI效果，标题模式位未参与解码。DECRQSS序列化和标题单栈出入存在具体差异。BEL默认声音、OSC9;4终端进度条按下文处理；不推出打印机、宏引擎、任意窗口远控或完整DEC设备仿真。 |

| 子路径（均归I20） | 实际回复/行为及边界 |
| --- | --- |
| DA1/DA2设备属性 | 第一参数>0不回复。OShell未覆盖TerminalOptions.termName默认xterm-256color，且enableSixelReported=false，故DA1为CSI ?65;1;2;6;21;22;17;28c；含打印等固定宣告不是对应产品入口证据。DA2优先固定CSI >65;20;1c；函数后半的按TERM分支对同一大于号前缀已被前面return挡住，不应列成OShell多种实际回复。外部启动的terminalType仅设置进程TERM，不改此options。 |
| 普通/DEC光标状态 | CSI5n回CSI0n；6n回行/列，原点模式扣上下/左右边界并至少1。私有?6n回?行;列;1R，列使用usingMargins判断、行未同样max(1)，与普通CPR公式不同。主要读取当前活动buffer；不能用已有显示截图替代参数验证。 |
| 固定DSR | 私有15→?10n，25→?21n，26→?27;1;0;0n，55→?53n，56→?57;1n，62→0*{，63→DCS id!~0000 ST（缺省id0），75→?70n，85→?83n；53无回复。分别是固定打印/UDK/键盘/locator/宏/完整性/会话状态字符串，未查询硬件或构造对应服务，不增打印、宏录制或locator能力。 |
| DECRQM | CSI模式$ p或私有?模式$ p只处理首个参数，回对应mode;状态$y；未知0，动态set/reset为1/2，若干不支持项固定1/2/4。内部空参数数组分支回CSI ;0$y，但线协议省略参数由解析器形成0，应按mode0查询验证。动态DEC集合为1、3–7、9、12、25、40、45、47、66、69、1000、1002–1007、1015、1016、1243、2004、2026、2500、2501；ANSI动态4/8/20。固定DEC2/8/18/19/95回1，13/14/30/41/1001回2，10/42/44/46/67/80/1034回4；ANSI2/3回2、12回1、1/5/7/10/11/13–19回4。1047/1048/1049虽有切屏/光标操作却未列查询，返回未知；查询名单不等于可设置名单。 |
| DECRQSS | DCS美元号q解析ASCII正文；r/s返回当前上下/左右边界，m调用toSgr；双引号p固定65;1双引号p，空格q固定2空格q，双引号q只回同后缀。成功DCS1$r结果ST，未知DCS0$rST且不回显未知正文。固定光标/一致性报告未跟随真实设置；输入终止分块还受I14的DCS unhook缺口影响，不能把已有handler认作所有分块均回复。 |
| SGR状态序列化 | toSgr从0起，报告bold/underline/blink/inverse/invisible和前背景色；没有dim/italic/crossedOut、下划线细分样式和颜色。ansi索引判断为c>16才用38/48;5，索引16误走基础色拼接为98/108；低索引还附尾分号。此处是查询回复缺口，显示属性支持范围继续归I17，不能用序列化删掉可显示属性。 |
| 矩形校验和 | CSI y分派未限制collect必须星号，忽略页参数；rid缺省1。宿主isProcessTrusted恒true，参数足够且矩形有效时累加当前屏单元的Unicode标量，空code按空格，格式为最少四位小写十六进制；不是固定16位截断，也不含属性。矩形由getRectangleFromRequest处理默认/原点偏移/钳位；不足参数或无有效矩形回0000。没有额外授权弹窗。 |
| 版本查询 | CSI >0q使用构建元数据，固定源码身份为SwiftTerm 1.20.0+v1.20.0-oshell:；不是OShell应用版本。tag去首v，tag/branch/version仅取可打印ASCII，总长≤256；回复ESC P、>和竖线、身份、ESC反斜杠。此路径硬编码7位，忽略cc.send8bit。 |
| 窗口尺寸/动作 | CSI14t由Mac以格尺寸×backingScale×行列并round回4;高;宽t，16t回6;格高;格宽t且Int截断。14;2和15因宿主nil走核心格尺寸推算，18/19分别回8/9;行;列t；不是查询真实外框/显示器尺寸。无宿主格尺寸时核心用10×16后备。11/13等未在cmdWindowOptions匹配，无响应；移动/最大化/全屏/升降/调整大小等宿主分支nil，不实际操控。分派中4;...误发moveWindowTo、8;...按cols/rows顺序转发，但宿主忽略；不因此追加窗口控制实现。 |
| 标题/图标/栈 | OSC0/2 UTF-8解码只setTitle，OSC1 setIconTitle；非法UTF-8为空，OSC0不同时设icon。setTitle经MacLocalTerminalView到TerminalPane身份/主机名解析，不是把任意远端文字直接用作窗口标题；图标回调为空。CSI20/21t只回OSC L/l ST空正文。22;0（可另带0）双入栈，22;1仅icon，22;2仅title；23;0双弹，但23;1弹title、23;2弹icon，与单独push不对应。栈无显式容量上限，空栈不动作。 |
| 标题编码/回复控制 | CSI>各参数t/T只改四个title set/query UTF/hex布尔值；生产OSC标题始终UTF-8，查询始终空，没有消费这些布尔的解码路径。ESC空格F/G切7/8位回复，DECSCL61强制7位，62–65选conformance，非61时第二参0/2为8位、其他7位。未改变DA固定身份及DECRQSS固定conformance；cc生成CSI/DCS/OSC/ST等，字符串仍UTF-8。输入裸C1问题归I14、Kitty键与XTVERSION固定7位分别归I18/本项。 |

以上把“代码实际回复什么”和“功能应如何验收”分开。原版固定宣告不证明完整设备仿真；复刻若纠正状态误报、栈对应或属性序列化，应记录同一SF内的差异和有限回归，不静默当作新增能力，也不能把已知错误写成正确标准行为。

有限待执行场景：默认DA1/DA2及非零参数，普通/私有CPR在原点和边界下的区别，动态/固定/未知模式及多参数DECRQM，六种DECRQSS后缀/未知/分块终止，SGR索引15/16/17和遗漏属性，矩形空格/Unicode与页忽略，XTVERSION与7/8位切换，Retina尺寸和nil回退，标题双栈/单栈/空栈、无效UTF-8和编码开关；BEL及进度条按下文。尚未执行Swift/macOS用例。

已读新增/复核：Terminal上述区段、EscapeSequenceParser 403–619中的相关分派、CharData 1–218、EscapeSequences全文、SwiftTermBuildInfo全文；MacTerminalView 3313–3315、3437–3553，MacLocalTerminalView标题转发132–134、TerminalPane 455–457/594/872–882。颜色OSC查询详细处理继续归I17，图形状态回复归I22；这里不提前认定相邻条目完成。

I20状态展示及I27宿主限制交接：

OSC9;4在Terminal 2732–2775解析移除/确定/错误/不确定/暂停，百分比钳位0–100，确定态缺省0；MacTerminalView 414–429创建终端内2点高进度条，870–929接收/保留最近百分比并在15秒无更新后清除。[TerminalProgressBarView.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/Apple/TerminalProgressBarView.swift) 全文显示确定宽度、错误红/暂停橙、不确定往返动画且不截鼠标。这是固定生产入口，现映射既有I20状态展示；基线原概括未显式列出，补充证据而不新增SF或扩为系统任务栏服务。

BEL由EscapeSequenceParser 387分派到MacTerminalView 3385–3401，默认bellStyle=sound，再经TerminalViewDelegate默认bell 3626–3628调用NSSound.beep；OShell未设置其他bellStyle。因此固定能力包含默认响铃，不把组件的视觉闪屏/静音等枚举值自动当成OShell已开放的设置。该遗漏也映射I20状态反馈。

Terminal 2684–2712的OSC52读请求遇nil直接不回复，写请求解base64后转delegate。MacTerminalView 3560–3565转至LocalProcessTerminalView，后者原可读写系统剪贴板，但OShellTerminal 179–181覆盖为空/nil，因此应用禁用且查询不返回空内容回复。MacTerminalView 3481–3553的图标标题回调为空，多数窗口动作不执行；单元格/文本区域像素查询有宿主回复，其他尺寸的nil回退已在上方I20逐项核实。默认打开任意URL/路径的openDefaultLink被OShell的scheme白名单覆盖。OSC777的自定义身份处理先于内置通知；组件notify默认空，未知OSC1337内容回调默认空。不能因解析器/父类有接口而追加远端剪贴板、任意URI、系统通知或窗口操控权限；其余宿主覆盖与测试入口限制见下文I27。

本段记录先前已读：EscapeSequenceParser、CharSets、TerminalOptions、MacLocalTerminalView、Apple/TerminalViewDelegate、TerminalProgressBarView全文；Terminal 1144–1638、1858–1874、2676–2785、3449–3487、6180–6253、6686–6707、6747–6757、8103–8121；Buffer 1580–1722；Utilities 1–64/270–467（65–269为未作为本结论依据的旧静态表）；MacTerminalView 414–435、870–932、3359–3640。尚未完成的编辑/属性处理器、图片、BiDi、行显示及OSC133不能因已找到分派入口改成已核查；重置证据见下方I21。

### 刷新、同步输出与重置：I21

| SF 条目 | 生产入口与源码定位 | 源码结论与关键边界 |
| --- | --- | --- |
| SF-I21 | Terminal 458–474、760–798、881–1002、1144–1167、1275–1286、1980–1984、2091–2107、2563–2567、4227–4280、4320–4360、4595–4631、5332–5333、5584–5585、6222–6302、6404–6425、6714–6718、6759–6775、6817–6875；Buffer 232–237、421–423、613–652、708–768；EscapeSequenceParser 640–650；AppleTerminalView 818–825、2376–2625、2841–2893；MacTerminalView 1166–1170、1243–1284、3471；KittyGraphics 132–178、1467–1479 | 同步输出2026是活缓冲上的刷新闸门，非快照；重复开启续期，默认1秒安全超时，关闭/实际尺寸变化/完整重置解除。DECSTR与RIS分别清理有限状态，详细保留表如下；完整重置不是整个Terminal对象重建。原版存在软重置插入模式镜像不一致、备用屏内容和图形pending保留等具体限制，不能概括为所有模式恢复初始值。 |

| 刷新子路径 | 固定行为与限定 |
| --- | --- |
| 脏范围与常规显示 | feed继续解析并累积refreshStart/End及滚动不变坐标；updateDisplay在读范围前检查2026，返回时不清脏范围。正常更新清范围、扩展BiDi依赖行，主屏历史视口离底部时整视口无效化；Metal接收脏行/游标变化，CoreGraphics请求对应区域重画，然后通知无障碍变化。精确绘制关联A06/I17/I22–I25，不能仅因有脏范围证明渲染正确。 |
| 合并与输入响应 | queuePendingDisplay在macOS无窗口或祖先隐藏时不排显示；同一pending标志合并更新，实际延迟16,670,000ns，源码旧注释“1/600秒”不准确。用户输入后150ms内feedFinish可直接更新/主队列合并，仍受2026检查；持续解析不等于后台持续绘制。 |
| 同步输出生命周期 | DECSET2026置active并安排主队列1秒超时；再次开启取消旧任务并续期，不是嵌套计数。DECRST2026、resetToInitialState、尺寸确实改变的resize结束；尺寸相同的resize提前return不结束。end只在active时生效，取消超时、标全屏脏、通知宿主更新滚条/排显示/通知滚动。主队列忙时不能保证墙钟精确1秒。DECSTR不立即结束，仍靠关闭/超时等解除。 |
| 冻结边界 | displayBuffer就是活动buffer，没有输出快照；解析、状态查询和其他宿主回调继续。updateDisplay/排显示/即时输入刷新/ensureCaretIsVisible有闸门，但Mac draw→drawTerminalContents和Metal draw→buildDrawData读活buffer，未统一检查2026。系统/其他UI触发直接重画时不能仅凭常规刷新闸门保证全帧静止。此为I21与A06绘制交接限制；最终反向核查仍须覆盖直接重画入口，不能把同步输出扩为事务或离屏录制服务。 |

下面是**固定源码实际状态**，不是建议的兼容标准。每个字段组只列本次可证明的直接重置行为；后续I15–I17/I22–I26的细节归所属条目，不把整个公共文件算已读。

| 状态组 | DECSTR / CSI ! p | RIS / ESC c（parser.reset后resetToInitialState） |
| --- | --- | --- |
| 同步刷新/解析器 | 不关闭2026；不调用parser.reset | 结束2026；清解析器OSC/APC/参数/collect/activeDcsHandler并清ReadingBuffer的putback/idx。直接调用公开resetToInitialState不含parser.reset，不能与ESC c混称 |
| 屏幕/历史/活动屏 | 保留当前屏、主/备用内容与历史 | 新建normalBuffer并填空视口，主屏历史消失，切主屏；activateNormalBuffer(clearAlt:false)保留旧备用文本，fillViewportRows在非空时不重填。新主屏x/y被该函数从altBuffer复制，不能承诺必回0,0 |
| 当前/保存光标 | 当前x/y保留；当前buffer.softReset把保存x/y设0、保存属性/字符集/模式改默认 | 新主屏保存光标为初始值；备用保存字段未统一清空。完整重置主动保存并恢复cursorHidden，软重置则显光标；两者均未恢复options.cursorStyle到初始形状 |
| 边界/制表 | 当前上下/左右边界恢复整屏；marginMode未关闭；Tab保留 | 主屏边界/Tab随新Buffer恢复，左右边界模式向两buffer设false；旧备用边界坐标/Tab仍随旧buffer保留 |
| 插入/自动换行 | Terminal.insertMode直接false，但未调用setInsertMode；Buffer镜像未清。setWraparound(true)更新两buffer，reverseWraparound=false | setInsertMode(false)、setMarginMode(false)、setWraparound(true)更新两buffer；reverseWraparound没有赋默认值，保留旧值 |
| 应用键盘/Kitty | 应用数字键盘、应用光标false；Kitty双屏标志/栈保留 | 应用数字键盘/光标false，Kitty双屏标志/栈清空 |
| 鼠标/焦点/粘贴/滚轮 | 跟踪、坐标协议、Shift capture、1004和reportedFocusState、2004、1007均未清 | 跟踪off、capture false、2004 false、1007 true；坐标协议、1004和reportedFocusState未清 |
| 属性/调色板 | curAttr默认；ansiColors恢复defaultAnsiColors并通知；动态前/背景及光标颜色未恢复 | curAttr默认、反显false；不调用resetAllColors，ANSI动态色保留，动态前/背景及光标颜色也未恢复 |
| 闪烁/滚动模式 | cursorBlink、smoothScroll保留 | cursorBlink=false，smoothScroll保留；cursorBlink字段与实际cursorStyle不是同一状态，查询/显示对应继续归I17 |
| 字符集/控制编码 | charset先nil，再setgLevel(0)重新读取现有G0；并未清G0–G3表。conformance=vt500，cc.send8bit保留 | G0默认、G1–G3 nil，gLevel/gcharset0；conformance=vt500，cc.send8bit=false |
| 换行/列切换授权 | lineFeedMode=options.convertEol；allow80To132保留 | lineFeedMode同左；allow80To132=false；行列保持重置前大小，不强制80×25 |
| 标题/图标/目录 | 标题正文/两栈/标题编码位/hostCurrentDirectory保留 | 标题正文和两栈保留，四个标题编码位false，hostCurrentDirectory=nil |
| 链接/载荷 | activeHyperlink=nil，已有单元payload与缓冲内容保留 | activeHyperlink=nil；主屏旧单元丢弃、备用文本仍可能持有payload。TinyAtom归终端管理，非在此立即全释放；不能承诺重置等于资源归零 |
| 图像/分块 | 不调用图像清理 | 主/备用行images置nil，Kitty imagesById/imageNumbers/placementsByKey清空，字节计数0、访问tick1；pending分块和nextImageId/nextPlacementId未清。图像格式、传输及缓存细节归I22 |
| BiDi/语义提示符 | 当前BiDi及两屏默认BiDi恢复options，箭头交换恢复options，私有模式保存表清；当前段仅段首可应用。两屏语义输入idle/content none/点击none/特殊键false，清当前屏行标记及输入扫描器；历史标记不在该循环 | 相同入口；新主屏内容已清，旧备用屏当前行标记清。此处不宣称旧备用所有段落BiDi都被重写；语义标记/分组完整行为继续归I26 |

明确差异与验收约定：

- 软重置的insertMode仅改Terminal字段，而Buffer.insertAsciiRun以自己的insertMode决定快路径，insertCharacter也用该镜像插入；因此SM4→DECSTR后，DECRQM可报告已reset而文本仍按旧插入状态执行。证据为Terminal 4595–4613与Buffer 1580–1722；不能仅看查询结果验收软重置。
- RIS保留备用文本及复制其光标、保留鼠标编码/焦点标志、保留标题栈和Kitty pending，是读取完整调用链得到的行为，不是根据“完整重置”名称推断。KittyGraphics.handleKittyGraphics在后续非more块可继续消费旧pending，故“图像缓存清空”不等于“旧传输不能续接”。保留为已有I21/I22生命周期差异；不会新增协议或扩大验收到无限输入组合。
- 缓冲切换/完整重置通过Mac.bufferActivated清选区并更新滚条；syncScrollArea本身为空。不要把它当成窗口几何已经同步的证据。纯颜色/选区/窗口重画与2026的交互必须用固定可见场景验证，不能以核心active布尔值代替画面验收。
- 私有模式保存/恢复CSI?模式s/r仅处理1243/2500/2501三个BiDi模式；其他参数被忽略，不能概括成保存任意DEC模式。普通光标保存/恢复与Kitty栈另归I15/I18，避免重复建功能。

有限待执行场景：2026开始/结束、重复开始续期、超时、同尺寸与变尺寸resize、同步期查询/直接重画、隐藏后恢复；主/备用分别DECSTR与RIS，并在此前放置屏幕/历史文本、非零备用光标、保存光标、Tab/边界、SM4、reverseWrap、Kitty栈、焦点/坐标编码/粘贴/1007、ANSI与动态色、标题栈和未结束Kitty分块。按上表选择具体种子分组验证，不要求这些状态取笛卡尔积；任何有意修正写明原行为、目标行为和同一SF回归。尚未执行Swift、GPU或macOS时序验收。

新增已读区段：Terminal 458–474、760–798、881–1002、1275–1286、1980–1984、2091–2107、2563–2589、4227–4284、4320–4360、4595–4631、5216–5234/5429–5458、5325–5340/5575–5595、6220–6322、6404–6426、6714–6718、6758–6815、6817–6895；Buffer 226–243、415–433、613–768（插入消费1580–1722此前已读）；EscapeSequenceParser 640–650；AppleTerminalView 805–837、1841–1878、2376–2627、2835–2895；MacTerminalView 227、290–330、1166–1203、1243–1313、3313–3315、3437–3480；KittyGraphics 126–180、1467–1523；MetalTerminalRenderer 359–440、634–700，MetalRendererRecovery 246–269。后两者仅检查同步刷新闸门与活buffer读取，不表示Metal的图形占位I23或全部绘制交接已完成。

### 特殊行显示：I25

本条核对固定 ESC 分派、行状态生命周期及 macOS 的 CoreGraphics/Metal 消费路径。它与字符自身的宽度（A05/I14）、图像生命周期（I22/I23）、BiDi 排列（I24）分别归属；双宽行不是宽字符，也不是将两个逻辑行合并为一个对象。

| SF 条目 | 生产入口与源码定位 | 源码结论与关键边界 |
| --- | --- | --- |
| SF-I25 | EscapeSequenceParser 528–535；Terminal 3145–3177、3250–3365、3489–3537、6116–6124、6512–6579、6580–6597、7044–7110；[BufferLine.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/BufferLine.swift) 13–109、157–172、275–391、411–496；[CircularList.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/CircularList.swift) 294–315、332–346；Buffer 670–681、771–798、875–902、1110–1546；AppleTerminalView 1813–1837、1889–2303、2538–2580；MacTerminalView 2524–2530、2790–2830；MacCaretView 55–75、Apple/CaretView 11–97；MetalTerminalRenderer 977–1458、2341–2525、2666–2767、2860–2882 | 四种逐行显示模式和校准入口均可达。普通字符、背景/选区、装饰及图片有两倍变换和上下半行裁剪路径；逻辑缓冲仍按完整列数工作。鼠标/输入法坐标未反向缩放，块光标内的普通字形未复用行变换；行回收和重排也不保证模式随内容一致移动。以下分别记录实际行为与有限验收要求，不宣称已有像素或实机通过证据。 |

| 内部子路径 | 固定源码行为与范围 |
| --- | --- |
| 指令与对象 | ESC #3设doubledTop、#4设doubledDown、#5设single、#6设doubleWidth，仅修改当前buffer的yBase+y那一行并标脏；renderMode.didSet递增generation。没有上下行自动配对、复制文本、重设光标或右半行擦除。两条双高半行需各自有内容及标记。 |
| 逻辑列与换行 | setLineRenderMode不改cols、边界或x。restrictCursor按cols钳位；Buffer.insertAsciiRun/insertCharacter（1580–1722）按cols或左右边界决定写入/换行，未读取renderMode。故两倍显示不意味着逻辑可写列减半；右半逻辑内容仍可写入，切回#5可见。不能用完整DEC硬件语义补写源码未实现的截断。 |
| CoreGraphics画面 | 行模式在文字、背景/选区、装饰、自绘字符及图片绘制外围设置CTM；双宽x×2，双高x/y×2。上半行以行顶为pivot、下半行以行底为pivot，并裁到该物理行高度。完成该行后恢复CTM。双高裁剪矩形宽度取dirtyRect.width、x固定0，并非脏矩形自身x；局部横向重画须列入验证。图片沿所在行一起变换，不是单独的全局等比布局服务。 |
| Metal画面 | buildRow以renderMode求pivot及双高clipRect，普通/双宽没有额外行高裁剪。字形、背景/选区、自绘块/框/Powerline经transformRect，装饰经transformUnderlineRect，图片经transformImageRect。图片与字形裁剪同时调整UV，避免只裁顶点而压缩整张纹理。Powerline接缝在行变换后补最终设备像素。此处只确认调用/几何规则，不把GPU和CoreGraphics认定为像素一致。 |
| 光标几何与字形 | 两路径均将光标横坐标和块/下划线宽度乘2，再乘字符自身占格数；高度仍一物理行，竖条仍用原固定厚度。CoreGraphics的CaretView.setText只记录字符宽度，drawCursor普通字形未获行模式；Metal普通光标字形纹理尺寸也未乘行倍率/上下半高pivot。Powerline光标另按光标矩形绘制，不能代表普通字形正常。因而光标框放大不证明其内部文字保持同一双宽/双高外观。 |
| 命中与输入法 | Mac.calculateMouseHit先用point.x/cellWidth算列，之后可做BiDi映射，但没有按renderMode除2；sharedMouseEvent、选择/拖动等调用共用此路径。像素报告保持视图点。NSTextInputClient.characterIndex同样按普通网格，候选框则取caretView.frame。这些是不同坐标来源，不能笼统承诺“显示、选择、报告、输入法已一致”；语义点击更深入的有效性校验归I26。 |
| 新行、复制与保留 | 新BufferLine默认single；克隆构造init(from:)复制renderMode及images。resize/fill/replaceCells不改renderMode；普通EL保留模式，ED0/1当前行保留模式、其他重置行恢复single，ED2所有可见行恢复single。整行copyFrom(line:)与区间copyFrom均不复制renderMode；前者在生产源码检索中未发现调用，不能以它冒称正在运行的整行移动有缺陷。实际引用移动/克隆保留对象标记。 |
| 历史与滚动 | 普通全宽滚动按行对象移动/保留历史；新空行由默认single模板克隆。历史已满、复用最旧对象走CircularBufferLineList.recycle→clear：清单元、图片、语义和isWrapped，但没有清renderMode；旧特殊行被回收后可能将其模式带到新的空白末行。窄左右边界滚动/反向索引明确把所有受影响整行renderMode清成single，列边界外的文字虽未搬动，其显示模式也一起改变。 |
| 缩放与reflow | 行数组resize保留原对象模式；reflow按isWrapped/文本宽度收集段落，跳过含当前光标的段，并按完整newCols搬单元，不按renderMode隔离特殊行或分配半宽列数。缩窄增加的行来自getBlankLine，默认single；区间复制不转移模式，所以原行与新续行可能用不同显示方式。扩宽合并保留目标对象的模式，而非每段文字原来的模式。公共重排算法其余语义继续归I15，不据本条认定完整缓冲已核查。 |
| ESC #8校准 | 用curAttr.justColor生成宽度1的E填行，清isWrapped，前后调用setCursor(0,0)；fill不会清renderMode、images或语义标记，因此不能写成“恢复所有行到普通模式并清空图像”。setCursor仍受originMode/scrollTop影响；启用非零原点时，循环从buffer.y+yBase填rows行，而非固定从视口第0行开始。CircularBufferLineList下标按容量取模、可惰性创建槽，未按count校验，须保留影响视口外槽/环绕内容的风险；没有实际执行越界场景，不宣称已复现崩溃。 |

已定位差异归原SF-I25，并关联I15/I16/I19/I21/I22/I24/I26：命中缺少逆变换；普通块光标字形未用行倍率；满容量行回收保留旧模式；reflow新增行丢失模式；校准保留行模式/图片且受原点影响。原版实际行为与复刻目标应分别记录；后续决定有意修正时保留原证据，不靠删去功能或追加新编号处理。

有限待执行场景（本轮未运行）：

1. 在相邻两行写相同ASCII和一个宽字符，分别#3/#4；独立#6/#5切换，检查文字、五种下划线中的单线/波浪代表、选区、单张图片和行间裁剪。两种renderer各执行一次固定样例，加入非零x的局部脏区重画。
2. 以80列为固定种子，在#6行写入超过40个ASCII后切#5，检查逻辑列、光标及右半内容；在可见第2个放大字符中心点击，分别记录选择、鼠标报告与输入法候选框，不把三者当同一验收。
3. 将块光标置于特殊行ASCII和宽字符上，对比未被光标覆盖时的字形；检查#3/#4上下半形状与普通行恢复。Powerline只作为单独代表，不能替代普通字形。
4. 放置带不同模式的行，分别执行EL2、ED0/1/2、全宽滚动、窄边界正反滚动；把一条特殊行推到固定历史容量的回收点，检查被复用空行。容量与循环次数在用例中固定，不穷举所有上限。
5. 使非光标所在的特殊逻辑段参与一次80→40→80重排，核对原行/新增续行模式及内容；另取当前光标段验证跳过规则。
6. 普通原点与非零scrollTop+originMode各执行一次#8，事先放置特殊行及一张图，检查E覆盖范围、光标、图片和历史。任何修正记录明确目标，不把“校准图案”名称当隐含清理全部状态的依据。

新增已读区段：EscapeSequenceParser 523–538；Terminal 3145–3185、3240–3365、3480–3555、6112–6128、6510–6612、7040–7110；BufferLine 1–112、153–174、275–395、411–504；CircularList 106–121、294–351；Buffer 650–695、770–810、864–905、1102–1552；AppleTerminalView 706–717、1790–1837、1882–2303、2525–2592；MacTerminalView 2515–2555、2790–2835；MacCaretView 1–149及Apple/CaretView 1–98；MetalTerminalRenderer 960–1458、2340–2525、2640–2782、2860–2890。局部路径阅读不等于整个渲染器、图像/占位协议或缓冲都已审完；I17闪烁停止与下划线交接的少量区段同时补核。

### 宿主能力与禁用边界：I27

本条沿实际对象与代理链核查：Terminal以Mac TerminalView为核心delegate；LocalProcessTerminalView.setup把视图delegate设为self，OShellTerminal继承并覆盖相关方法；TerminalPane再接收processDelegate事件，且直接注册OSC777。不能只看协议声明、默认extension或底层函数名判断应用能力。

| SF 条目 | 生产入口与源码定位 | 源码结论与关键边界 |
| --- | --- | --- |
| SF-I27 | Terminal 18–115、138–264、1175–1182、2592–2621、2684–2729、2784–2861、8004–8121；EscapeSequenceParser 558–594、621–638；[TerminalViewDelegate.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/Apple/TerminalViewDelegate.swift)；MacTerminalView 295–303、3437–3465、3481–3640；[MacLocalTerminalView.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/Mac/MacLocalTerminalView.swift) 90–155；TerminalPane 153–181、448–470、871–890、989–1003；RemoteHostIdentity 9–14、29–39；根Package.swift 6–18、Vendor/SwiftTerm/Package.swift；main.swift 26–28、94–96、243–252 | OShell明确覆盖OSC52及链接打开，并以自定义777替换组件通知。目录、标题、像素查询等有条件转发；文档上报、未知iTerm内容及多数窗口控制没有产品动作。正式构建排除TestSupport；诊断截图与Desktop额外快照不构成原版持久终端恢复功能。解析器拒绝/存值/回调/操作系统动作是不同结果，下表固定边界。 |

| 入口/子路径 | 原版实际结果与归属 |
| --- | --- |
| OSC52写 | 核心找selection/payload分隔符，base64合法后转核心delegate→视图delegate。父类MacLocalTerminalView原本可清系统剪贴板并写UTF-8，但OShellTerminal.override为空，因此不会落到父类写入。没有“允许一次”的UI，也没有按会话开启选项；用户主动复制/粘贴归I03–I05/M，不受远端OSC禁用推论影响。 |
| OSC52查询 | payload恰为问号才走读；OShell覆盖返回nil，核心立即return，既不读系统剪贴板，也不发送空base64答复。selection空值映射c、其他值不实质校验，但在OShell均不会开放另一个剪贴板；缺分隔符或非法base64无回调。不能以父类实现或查询解析成功声称开放远端读取。 |
| 链接打开 | Mac默认openDefaultLink允许有scheme的URL或存在的本机路径（含行列后缀剥离）；OShell覆盖只允许URL解析成功且scheme小写属于http/https/mailto，再交NSWorkspace。file/ssh/自定义scheme及无scheme路径不打开；不使用OSC8 params作额外授权。鼠标激活/显式与隐式链接识别归I07，本条不追加远端自动打开URL能力。 |
| OSC777及通知 | dispatchOsc先查自定义handler并return，不会因handler拒绝而回退内置。TerminalPane为777注册receiveShellIdentity；只接受OShellHost=1前缀、限定长度和有效身份，在被动标题模式、非观察状态或备用屏幕时拒绝。普通777;notify即便语法符合底层通知协议，也不会进入内置oscNotification；组件notify默认空是另一层事实。OShell主机显示归T03，不能扩成系统通知中心。 |
| OSC6/7与信任字段 | Mac.isProcessTrusted恒true，OShell没有覆盖；这是允许核心记录目录/文档及做矩形校验的布尔值，并非SSH认证、用户授权或远端主机可信校验。OSC6存hostCurrentDocument后调用默认空回调，没有打开编辑器；OSC7经视图/进程delegate到TerminalPane，主机显示与remoteDirectory更新还检查观察状态、file scheme、匹配host、非空且合法路径。只存上报，不会因OSC7在本机chdir或直接发起文件访问。T域及I20保留各自具体分支。 |
| OSC1337与文件动作 | 只有File且inline=1的可解码base64走createImage；尺寸解析与图像资源归I22。非内联File（包含inline=0/缺省）及其他可解析key进入iTermContent；Mac转视图delegate后默认空，OShell无覆盖，故不会自动保存远端下载文件或执行其他iTerm扩展。无等号/不合法头部可能更早return，不能把所有未知载荷统称已发回调。此限制不等于关闭I22中其他图形协议的介质。 |
| 窗口、标题与报告 | Mac图标标题回调为空；移动、前后置、最小化、最大化、全屏及远端窗口尺寸请求多数返回nil/忽略，resizeWindowTo仅打印请求。像素报告有实现，nil后核心的固定尺寸/空标题回复见I20。标题可到TerminalPane的主机识别，但不等于直接控制系统窗口标题。80/132列的核心模式/resize链归I15，不能把CSI t忽略推广为一切终端尺寸变化都禁止。 |
| 解析扩展与默认回退 | registerOscHandler只写字典，无独立设置入口或插件装载器；生产应用检索到的自定义注册为777。Terminal.init实际调用configureParser，覆盖默认CSI/ESC/OSC/APC等回退为log，DCS仍默认空；这些未知指令没有协议或产品动作，但可能输出诊断，不能只读Parser的默认闭包就称完全无日志。silentLog在DEBUG默认false、其他构建true，OShell源码未覆盖；没有自动按标准补齐处理器。BEL→NSSound和OSC9;4终端内进度确有消费，不能因777通知禁用将其误排除，具体归I20。 |
| 测试、截图与额外快照 | 根包仅OSHELL_TEST_HARNESS=1才纳入TestSupport并定义测试编译标志；main的prepare/run也受该标志控制。正式包对识别的测试请求报错exit64；测试包另要求独立OSHELL_DATA_DIR。Mac.drawMetalFrameNow仅立即draw，当前调用位于三处TestSupport绘制测试，没有状态序列化或恢复。Vendor在macOS排除iOS目录。历史清单I86所述VT Snapshot是Desktop的额外接口，当前internal/terminal/terminal.go 656–667导出t.term.VT；非测试终端代码只见此定义，其他同名共享配置/诊断快照不是终端恢复。不可把历史Desktop问题反推成OShell源码功能。 |

有限待执行场景：以合成字符串作为剪贴板种子发送OSC52写/读/非法base64，观察系统内容不变且读无回复，同时主动复制/粘贴仍有效；分别点击https/http/mailto/file/ssh/无scheme路径并记录宿主调用；同一终端发送合法OShellHost上报、777 notify和畸形上报，确认仅符合条件的身份更新；OSC6/7匹配/非匹配host不触发本机文件打开；OSC1337 File inline=0无下载、inline=1交I22图像用例；抽取一个忽略的窗口动作与一个有回复的尺寸查询；在正式/测试构建分别验证测试入口拒绝/独立目录要求。所有用例只在明确测试环境执行，本轮未触碰真实剪贴板、发起URL、运行应用或创建测试配置。

本条是宿主边界的静态闭合，不代替I22图像介质、I24布局或I26点击路由，也不是对应用所有安全性的保证。后续具体缺陷仍回归已有SF，新增授权UI、可配置远端剪贴板、任意URI或系统通知需要按范围变更处理。

新增已读区段：Terminal 18–115、138–265、759–801、1144–1184、2591–2624、2670–2745、2784–2863、6151–6183、8000–8121；EscapeSequenceParser 554–595、620–639；TerminalViewDelegate整文件；MacLocalTerminalView 81–174；MacTerminalView 290–318、3438–3650；TerminalPane 153–183、448–477、571–617、871–892、989–1003；RemoteHostIdentity 9–39；AppleTerminalView 383–427、789–800；根与Vendor包清单；main 1–35、88–107、219–252；SyncDebug整文件（enabled是常量false，无运行时开关）；Desktop terminal.go 651–675只用于澄清历史额外接口归属。测试代码调用检索不计为测试运行，未逐行阅读截图测试正文。

## 终端屏幕、历史和编辑（I15–I16）

本节核查固定 SwiftTerm 的生产实现，沿用 I14 的字符输入、I21 的刷新/重置以及 I25 的行对象与重排阅读。核心文件：[Terminal.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/Terminal.swift)、[Buffer.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/Buffer.swift)、[BufferLine.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/BufferLine.swift)、[CircularList.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/CircularList.swift)、[EscapeSequenceParser.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/EscapeSequenceParser.swift)。下述实现限制是静态结论；没有运行 Swift/macOS 验收。

| SF 条目 | 生产入口与源码定位 | 源码结论与关键边界 |
| --- | --- | --- |
| SF-I15 | Terminal 458–473、881–938、1712–1865、3539–3554、4061–4124、5216–5342、5475–5588、6431–6680、6759–6817、6860–6875；Buffer 613–943、1006–1078、1110–1546、1580–1710；AppleTerminalView 2691–2884；MacTerminalView 1077–1115、1166–1208 | 两个缓冲有各自文本、坐标、Tab、保存光标和边界；终端 wrap/margin/insert 模式通过 setter 同时传给两屏，不是两套完全隔离的终端状态。47 返回主屏保留备用文本；1047/1049 返回时清备用屏，1049 另保存/恢复主屏光标状态。主屏有历史，备用屏无历史重排；缩放跳过含当前光标的逻辑段。滚动历史、手动视口和容量淘汰分别处理；切屏、重排、保存坐标及左右边界存在下述具体限制。 |
| SF-I16 | Parser 403–489；Terminal 2991–3248、3250–3431、3557–3813、3860–3884、4143–4166、5605–5710、5784–6124、6431–6512、6738–6757；BufferLine 130–504；CharData 265–410；AppleTerminalView 2376–2525、2827–2884 | 分派支持固定字符/行/列插删、光标移动、滚动、重复前格和矩形操作；参数、边界、擦除属性并不统一。矩形属性修改与字符保护设置未实现，DECSERA 对每格改为空格并保留属性/附加数据，不是保护字符筛选。原始单元格搬移没有完整宽字修复；部分编辑缺少 dirty range，矩形复制与受限 IL/DL 有可定位的索引缺口。不可用协议名称替代函数实际行为。 |

### I15：屏幕状态与可达操作

| 子范围 | 固定源码行为 | 限定及交接 |
| --- | --- | --- |
| 47 / 1047 / 1049 | 入备用屏先复制主屏 x/y，只在备用行集合为空时填充；入屏清 Kitty 图像。出屏先复制备用坐标到主屏；47 不清备用文本，1047/1049 清备用缓冲及 Kitty 图像；1049 再恢复主屏保存状态 | 不将源码注释“备用屏总是清空”当事实。重复激活的 helper 提前返回，但模式分派的保存、显示光标、刷新及激活通知仍须按实际路径区分。宿主切屏清选择、更新滚动条；键盘栈归 I18，语义切屏归 I26 |
| 保存/恢复 | 保存 x/y、当前属性、当前字符集映射以及 origin/margin/wrap/reverse-wrap；恢复时先把坐标夹到物理屏幕，再恢复模式 | 不保存滚动区坐标、完整 G 字符集表或全局终端模式；恢复 margin/wrap 会同步两屏。resizeBuffers 额外用主屏旧 savedY-y 差值重算 savedY，不能只读 Buffer.resize 的 savedY 调整得出最终结果 |
| 原点与边界 | 原点开关只改标志。纵向滚动区只接受 top<bottom，设置命令仍会 home 光标；69 开启左右边界，普通 CSI s 此时设置边界，否则保存光标 | 左右边界允许单列，省略 right 与显式 0 不同；设置左右边界不 home。关闭 69 保留坐标。CUP、HVP、CHA、HPA/VPA、CNL/CPL 对原点/边界的用法不同，不能抽象成统一坐标换算 |
| 换行与回绕 | 普通输入保存行末待回绕位置；wrap 开时后续字符进入下一行/滚动，关时单宽覆盖边缘、放不下的双宽不写入。LF、CR、BS、IND 各有独立边界判断；LNM 控制 LF 是否归列 | LF 在滚动底部还检查水平范围；IND 的超过底部判断不同。反向回绕受设置入口和现有 wrap 状态影响，关闭 wrap 不清 reverse-wrap 标志。宽字占位格、IRM 插入空格使用 Buffer 自身默认属性，不能假定均继承 Terminal 当前属性 |
| Tab | 默认每 8 列；HTS 设置当前屏停止点，TBC 清当前或全部，前后 Tab 扫描该屏停止点。公开 resize 为两屏重建新增列的停止点 | 前向停止受右边界限制，后向扫描到 0、不受左边界对称限制。Buffer.resize 的临时数组长度须连同 Terminal.setupTabStops 看，不能把中间状态误报为公开 resize 的最终结果 |
| 历史与滚动 | 主屏容量为可见行加配置历史，备用屏无 scrollback。全宽且 scrollTop=0 的输出滚动可追加历史，满容量复用最老行；局部纵区或窄左右区在屏内搬移 | CSI SU 与输出换行滚动不同，SU 不增加历史。手动上翻时新输出保留视口，满容量淘汰会相应移动 yDisp；未手动滚动时跟随底部。窄区滚动的行模式、图像清理差异归 I25/I22 |
| 清历史与改容量 | clearScrollback API 只作用主屏，剪掉 yBase 之前的行，重置 yBase/yDisp 并调整 savedY；降容量只剪超额最旧行并修正坐标，升容量不预填所有槽位 | ED3 是另一入口：按 lines.count-rows 裁历史并重置 linesTop，其 savedY 处理不同。环形数组裁剪/弹出不保证立即清空所有旧对象引用；它不是敏感内存擦除功能 |
| 视口导航 | AppleTerminalView 按有效行数限制滚动位置，更新视图与终端的 userScrolling、yDisp、刷新及滚动条；主屏 PageUp/Down 按屏移动 | 备用屏不能浏览历史，翻页入口转发按键；setViewYDisp 本身只赋值，不能脱离调用方夹取和刷新推断其保护。Mac 激活/滚动回调负责 UI 更新 |
| 尺寸变化 | 公开 Terminal.resize 最小 2 列、1 行；实际变更会结束同步输出、调整两屏、更新 options/Tab/刷新。缩高可能移除光标下面的行；增高可回收历史进入可见屏 | 缩高删除下面行不检查其是否空白。纵区重置到新屏，横向边界只在超出时缩小，变宽不自动扩到新的最右列。不要把实现注释中的“空行”理解为数据保护条件 |
| 主屏重排 | 以 isWrapped 组合逻辑段；变宽合并、变窄拆分并避让双宽分界，调整视口和历史；含当前光标的逻辑段跳过 | 变窄后仍裁各行右尾，因此跳过当前段不等于该段内容无损。变宽 getLinesToRemove 的初始 destCol 使用整个 lines 的第 0 行，随后却复制当前 wrappedLines；当两者宽字尾部结构不同，起始复制列可能相差一格，作为静态索引缺口保留 |
| 备用屏缩放 | isReflowEnabled 为 false；缩窄只改逻辑 cols，不走主屏的逐行裁尾，右侧原数据可保留在行对象中，随后扩宽可能再出现 | 不能把主屏的 reflow/裁剪规则套到备用屏，也不能将隐藏尾部当作跨重启恢复。图像坐标及完整 BiDi/语义重排分别继续归 I22/I24/I26 |

I15 的有限核查场景：①写入不同文字后分别执行 47、1047、1049 的进出，比较文本、坐标、属性和保存状态；②设置纵横边界后分别开关原点/69，比较 CUP、HVP、CR、Tab 和 LF；③主屏历史已满时上翻，再连续换行，比较视口与最旧行；④非空光标下方行缩高，再增高；⑤主屏历史中的软换行段与当前光标段同时缩窄/扩宽；⑥备用屏写满右尾后缩窄/扩宽；⑦让缓冲第 0 行与待合并段首行的宽字尾部结构不同，检查变宽复制起点；⑧分别清历史、ED3、降低容量，再恢复保存光标。每项验证上述指定差异，不枚举所有行列组合。原版数据丢失或索引缺口须记录修复预期，不能据此降低复刻已有的内容完整性要求。

### I16：编辑命令、属性与数据边界

| 子范围 | 固定源码行为 | 限定及已定位缺口 |
| --- | --- | --- |
| 光标移动 | CUU/CUD 根据当前位置与纵区限位；CUF/CUB 结合 marginMode；CUP 走 setCursor/restrictCursor；HVP、CHA、HPA/VPA、HPR/VPR 各自计算 | HVP 加原点偏移后按屏幕而非 scrollBottom 限位；CNL/CPL 无条件使用已存 marginLeft；CHA 加左边界偏移后没有相同的最终夹取。保留这些差异，不能声称全部遵循同一标准算法 |
| ICH/DCH/ECH | ICH/DCH 在行内移动原始 CharData，按当前边界补擦除格；DCH 显式拒绝 x==cols；ECH 替换从 x 开始的一段格，按行存储长度截断 | ICH 没有同样的待回绕保护，底层 insertCells 对位置取余；ECH 不按左右边界截断且没有 updateRange。原始格操作不自动修复被切开的宽字两半 |
| EL/ED | 普通擦除使用默认前景、当前背景、默认样式的 eraseAttr；EL 擦格但不统一清行元数据；ED 根据 0/1/2/3 分支处理可见格、行标记和历史 | EL/ED 不依据 private collect 建立保护字符筛选；ED2 清行图像并清 Kitty 集合，局部擦除不同；不能把所有擦除等同全行对象重置。图像生命周期仍须由 I22 核查 |
| IL/DL | 非 marginMode 时在纵区内增删整行，调整选择及软换行接缝；marginMode 时只搬左右边界间单元格。IL 入口检查光标在纵区，DL 的 margin 分支没有同等独立检查 | 两个 margin 分支都以 scrollBottom-scrollTop 作为搬移行数，但从当前光标行起算；光标不在 scrollTop 时会读写纵区以下。IL 计数上限为两倍缓冲容量，DL 为 rows+1，不能概括成统一的剩余行数截断 |
| SU/SD | 屏内滚动；全宽分支移动行对象，受限水平分支复制局部格，填充使用默认属性。SU 最多 rows×2，SD 最多 rows | SU 不产生历史；与 LF/IND 输出滚动的当前背景、行元数据处理不同。选择与 wrap 接缝处理是执行路径的一部分，不能只比较字符串 |
| 列编辑和横向索引 | DECIC 在 apostrophe 分派下遍历纵区插格；DECDC 遍历纵区删格；DECBI/DECFI 在左右边缘横向滚动，否则移动光标 | DECIC 不要求当前 y 在纵区内、没有 updateRange；DECDC 有 y 检查但 handler 不验证 collect。横向索引内部使用存储边界，不能只看 marginMode 假定全屏。各分支 wrap 标记处理不同 |
| REP | 最多 cols×rows×2 次；取前一格（x=0 时默认格），经 insertCharacter 重复写入 | 不是读取完整前一可见字形；前一格为宽字占位时仍取该格。新写入会按当前活跃超链接与语义状态处理，不能承诺所有前格附加语义原样复制 |
| 矩形坐标 | 共用 helper 以 1 起算，省略 bottom/right 才补全屏；显式 0 不等同省略。原点模式加 scrollTop/marginLeft，然后夹物理屏幕 | 该 helper 的水平偏移不另检查 marginMode；复制目标位置另算且不加同样原点。必须检查实际参数数组长度，不能用注释推断默认值 |
| DECCRA | 仅 page 1 内复制；先快照源 CharData 再写目标，所以重叠不会边读边覆盖；目标超出底部时停止 | 各参数读取条件多要求一个元素，正常第八参数不会被读取，省略源 bottom/right 默认 rows-1/cols-1。cright 上限及半开写循环使源触及最右列时少复制最后一列；不搬行图像/行模式，不标记刷新 |
| DECFRA / DECERA | 填充使用当前完整属性，生成新的格；DECFRA 从 UnicodeScalar 构造，固定 width=1，无效标量回退空格；DECERA 填空格 | 与 eraseAttr 的背景擦除不同；不按字符真实显示宽度建双宽格，原格链接/语义数据被新格清除。两者无 updateRange |
| DECSERA | 对矩形内所有格调用 updateCharData，仅改字符和宽度，因此保留属性、payload、semantic | 没有“受保护字符不擦”判断，空格仍可保留原链接/语义；同样无 updateRange。不能把函数名中的 Selective 当作完整字符保护能力 |
| 未实现的属性协议 | CSI $r 路由到拒绝非空 collect 的设置滚动区；$t 不匹配窗口/标题分支；带双引号的 q 不匹配光标样式；*x 只记录未实现 | DECCARA、DECRARA、DECSCA、DECSACE 没有实现；XTPUSHSGR/XTPOPSGR 也只有未实现路径。原清单“矩形属性操作”据此纠正；不新增编号，不擅自删除 Desktop 已有额外能力 |
| 单元格与刷新 | BufferLine 搬移原始格，可保留 payload；局部 copy 还处理行标记，完整行对象状态与格数据不同。CharData.setValue 仅改 code/width。feed 解析不会自动将所有编辑标记为脏 | AppleTerminalView.updateDisplay 无 dirty range 时通常只处理光标变化；矩形、ECH、DECIC 的缓冲变化缺少自己的范围标记。光标不动且没有别的刷新触发时存在显示滞后的静态路径，须独立实机确认，不能把内存修改当已显示 |

I16 的有限核查场景：①待回绕位置分别执行 ICH/DCH，再以宽字两半为插删边界；②带背景色、链接与语义的格分别执行 EL/ECH、DECERA、DECSERA，对比字符、属性及附加数据；③在纵区中间行启用左右边界后执行 IL/DL，检查纵区以下哨兵内容；④比较 LF、SU/SD、DECIC/DECDC 的内容、历史和空白属性；⑤对重叠矩形、最右列矩形执行 DECCRA，分别给省略、显式零、八参数与额外参数；⑥原点/左右模式组合下比较矩形源和目标坐标；⑦在已完成绘制、固定光标的屏幕只发送一次 ECH/DECIC/矩形操作，检查缓冲与两种渲染器是否同步；⑧发送上述未实现属性命令，确认不能改变字符保护或矩形属性。宽字不破坏、有效区域不越界等是复刻质量要求；原版缺口须显式列出修正差异，不作为无限兼容性扩展。

本次修订只纠正已有 I15/I16 的源码事实。若要求增加原版没有的完整矩形属性/字符保护协议，应按范围变更处理；这不改变原范围内已授权缺陷的修复责任。图像和语义分别由后面的 I22/I26 核查，BiDi 完整算法见后面的 I24，占位绘制由 I23 单独核查，不能由本节的交接阅读替代。

新增及复用阅读范围：Terminal 440–475、881–940、1600–1640、1712–1865、2991–3248、3250–3431、3539–3813、3860–3884、4061–4124、4143–4166、5216–5342、5475–5588、5605–5710、5784–6124、6198–6222、6301–6373、6431–6680、6738–6817、6860–6875；Buffer 1–274、613–943、1006–1078、1110–1546、1580–1710（重排完整区段沿用 I25 阅读并复查本节入口）；BufferLine 130–504；CircularList 211–446；CharData 265–410；Parser 403–489、668–710、907–944；AppleTerminalView 2376–2525、2691–2884；MacTerminalView 1077–1115、1166–1208；TerminalOptions 默认 tabStopWidth=8。Buffer 未列出的语义区段不冒称已读。

## Unicode 图形占位（I23）

入口是 Parser 的 APC `G` 分派到 KittyGraphics，创建已存图像的虚拟位置；随后普通文本输入保存占位字符和属性，由 AppleTerminalView 组行时解码，再分别交给 CoreGraphics 与 Metal 绘制。它不是普通文字渲染的总括，也不是全部 Kitty 传输/删除协议的代称。

| SF 条目 | 生产入口与源码定位 | 源码结论与关键边界 |
| --- | --- | --- |
| SF-I23 | [KittyPlaceholder.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/KittyPlaceholder.swift) 1–500；[KittyGraphics.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/KittyGraphics.swift) 153–369、434–455、1011–1055、1180–1213、1242–1285、1333–1431、1467–1523、1685–1693、1742–1755、1834–1854；[AppleTerminalView.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/Apple/AppleTerminalView.swift) 1020–1049、1130–1369、1870–1877、2225–2267；[MetalTerminalRenderer.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/Apple/Metal/MetalTerminalRenderer.swift) 128–168、529–549、680–845、1516–1583、2231–2239、2575–2640、2971–2992；Parser 597–607 | U+10EEEE 与固定 296 项附加字符表解码图片 ID、位置 ID 和图内格坐标；每行按可视顺序继承前一相邻格的省略信息。U=1 只登记虚拟位置，不插入占位文字、不推进光标；两种渲染器按当前屏记录选图、等比居中并裁到单格。记录缺失、ID 歧义、z/裁剪未消费以及缓存刷新限制见下表；完整传输介质/删除算法仍归 I22。 |

| 子范围 | 已核实行为及边界 |
| --- | --- |
| 编码和未知字符 | 首标量必须是 U+10EEEE；后续只收集固定表中字符的索引，不认识的标量忽略。前三个已识别附加字符分别是行、列、图片 ID 高字节，更多识别字符不产生额外字段。高字节使用 min(msb,255)，所以表索引超过 255 不形成新的高位范围。表内 296 个值唯一，不自动追随新版 Unicode/Kitty 扩展 |
| 颜色身份 | 前景 ANSI 索引直接作为低位数值，trueColor 组合 RGB 24 位；default/defaultInverted 都为 0，不取实际主题 RGB。下划线颜色按同一方式得到 placementId，未设为 0。解码使用原始 CharData 属性，选中高亮或规则渲染颜色不会换图片身份 |
| 省略继承 | 零附加字符时，同前景、同下划线颜色且当前行相邻的前格可继承 row/msb 并 col+1，否则从 0/0 开始；一个附加字符另要求行相同才继承；两个附加字符还要求显式列恰为前列+1 才继承 msb；三个及以上直接采用前三项。其他样式/背景变化不阻断身份继承 |
| 行内状态 | previousPlaceholder 只在本次行生成内存在，非占位分支或无法取得属性时清空；不会跨行或沿软换行自动延续。传入的 col 是 visualCol，BiDi 后以可视相邻判断；完整重排算法由 I24 核查。占位解码分支本身不以 blinkHidden 为门禁，不能套用普通文字的闪烁隐藏结论 |
| 虚拟位置建立 | U=1 拒绝任一父图片/父位置参数；需可解析 imageId 才登记，未提供 ID 的传输即使返回内部成功也没有可供绘制的虚拟记录。c/r 非正数记 0，绘制时拒绝；缺省 placementId 自动生成非零值。像素偏移负数归零，获得格像素尺寸时夹到格内。登记后直接返回，不执行普通位置的裁剪、图像行附着及光标移动 |
| 位置匹配 | 绘制先从全局字典筛当前主/备用屏的虚拟记录，按 imageId 分组。placementId 非零时精确匹配，0 时取首个尺寸足够的记录；候选没有排序，不承诺多位置时选择固定一个。占位行/列必须小于记录 rows/cols，图片必须仍在 imagesById。相同 imageId+placementId 的登记可替换跨屏同键记录，记录键本身不含屏身份 |
| 绘图几何 | 根据占位当前列减图内列推算整图原点，结合图内行、记录 rows/cols 与像素偏移；整幅图片等比居中到该范围，然后只绘当前格交集。CG 通过 clip 绘图，Metal 计算交集 UV 再提交，后者继续应用 I25 的行变换。虚拟记录中的登记 row/col 不作为占位绘图锚点，字面占位格移动才决定显示位置 |
| 文本与层次 | 成功解码后先在文本布局中加入空格；后续找不到图片、位置或有效范围也不回退为占位符字体。占位图在文字/装饰之后、普通 over-text 图片之前画；不读取记录 zIndex 来排序或决定 under-text 层。不采用普通位置的源 crop 字段，不能列为所有图形放置参数都支持 |
| 删除和资源交接 | 虚拟记录与图片共用 I22 状态。清屏/切屏清图会删除对应记录；按图像 ID 的删除也检查记录，不只查行上图片。资源使用集合包含 placementsByKey，因而虚拟记录可保留图片引用；字符被擦除不等于记录或缓存自动释放。完整删除模式、缓存淘汰和介质读取见后面的 I22 记录 |
| 刷新与替换缺口 | 虚拟登记/存储本身不调用 updateRange，添加/替换后若没有字符或其他刷新不保证立即显示。Metal 行缓存的 Kitty stamp 只有图像/位置数量和两个自动编号计数，同键替换可不改变 stamp；纹理签名只含类别、尺寸、总字节数及前 64 字节哈希。同 ID、相同尺寸/长度/头部而尾部像素变化时，即使重建行也可能复用旧纹理。该静态路径需用合成图片在实际 GPU 上验证，不冒充已运行结果 |
| 未用辅助函数 | KittyPlaceholderRenderPlacement.compute 定义了另一套等比几何与整数舍入，但全生产源码检索只有定义及自身构造，没有调用；生产渲染使用上面的独立路径。不能拿该函数存在或它的孤立测试证明实际单格显示正确 |

有限验收场景：①使用固定表的 0/1/2/3 个附加字符，并插入一个不在表内的标量，核对坐标与高字节；②逐项改变前景、下划线、背景、其他样式，并插入普通格/换行，核对继承；③同图片创建两个虚拟位置，比较 placementId=0 与明确 ID、缺失图/位置及 0 尺寸；④用非正方形合成图片比较两种渲染器的等比留白、像素偏移和单格边缘；⑤移动/擦除占位文字、切换主备用屏与删除记录，区分文字和图片生命周期；⑥更换相同 ID、相同尺寸/字节数/前 64 字节、仅尾部像素不同的 RGBA 图片，分别在无其他输入和强制刷新后检查旧图；⑦组合闪烁与选择高亮，确认身份取原属性；⑧在已有的 RTL 和特殊行样例中检查可视邻接与单格裁剪，关联 I24/I25，不增加功能身份。当前没有执行这些实机用例。

本节整文件阅读 KittyPlaceholder.swift（包括固定字符表）；其余文件只读本表所列区段，另补读 KittyGraphics 的键/记录定义 66–87、AppleTerminalView 等比函数结尾 1050–1051。普通图片介质与通用删除/缓存内部未据此宣称完成。生产调用检索确认了 decoder、虚拟记录和两个渲染器的可达链。原版树中未找到独立 KittyPlaceholder 命名测试文件；这一检索不证明没有覆盖，也不计作任何测试通过。

## 终端图像协议与宿主绘制（I22）

本域与主动图片粘贴 M 域不同：这里是进程向终端输出图像协议。主生产入口的 [TerminalPane.swift](../validation/oshell-reference/Sources/OShell/TerminalPane.swift) 455–459 明确把 `enableSixelReported` 设为 false、`kittyImageCacheLimitBytes` 设为 **4 MiB**；不能把组件默认 320 MiB 当作 OShell 的配置，也不能把不宣告 Sixel 解释为关闭其解析。OShell 未覆盖图像创建代理，沿用 AppleTerminalView 的实现；非内联 OSC1337 回调由 I27 已确认不保存文件。

| SF 条目 | 生产入口与源码定位 | 源码结论与关键边界 |
| --- | --- | --- |
| SF-I22 | [KittyGraphics.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/KittyGraphics.swift) 1–1940；[SixelDcsHandler.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/SixelDcsHandler.swift) 1–408；[Terminal.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/Terminal.swift) 2784–2862、4435–4436、6644–6659、6759–6791；[EscapeSequenceParser.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/EscapeSequenceParser.swift) 597–618、825–906；[AppleTerminalView.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/Apple/AppleTerminalView.swift) 1949–1980、2079–2091、2270–2303、3043–3268；[MacTerminalView.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/Mac/MacTerminalView.swift) 3313–3357；[MetalTerminalRenderer.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/Apple/Metal/MetalTerminalRenderer.swift) 1233–1322 及 I23 所列缓存/绘图段；Buffer 216–260 与 I15/I25 的行回收/重排路径 | 三个入口分别解析，但都通过宿主生成按行附着的图片条带；Kitty 另外维护原始图像、编号和位置记录。支持固定格式、介质、裁剪、相对位置、两层 z 及多种删除条件；不等于完整 Kitty/Sixel 标准。原始缓存、条带、纹理及输入累计是不同资源；存在分片终止、尺寸/压缩、删除跨屏和绘制成功反馈缺口，逐项保留如下。 |

### Kitty 控制、传输和资源

| 子范围 | 固定源码行为 | 限定与缺口 |
| --- | --- | --- |
| 动作与字段 | APC G；a=q 查询载荷、t 传输、T 传输并显示、p 显示已存图像、d/D 删除。识别 a/q/f/t/s/v/x/y/w/h/i/I/p/P/Q/H/V/X/Y/U/z/m/o/c/r/C/d/S/O；默认 a=t、f=32、t=d，其余按函数默认 | ASCII 键值字典中后值覆盖前值；未知键不报错，坏整数回默认，ID 只接受正 UInt32。无动画动作、完整协议协商或任意新字段。q 查询直接解码给定 base64，不按所请求传输介质读取文件 |
| 分片 | 仅直接传输保留 m；m=1 把 base64 字节加入唯一 pending，首块保存控制字段，最终块按首块字段统一处理；d/D 先清 pending | 不按图片 ID 分开并行累计，后续块修改字段不会重新定义首块。累计前没有总体字节上限；400 MiB 是解码/解压完成后的检查，不能当作输入队列硬上限。Parser 的 OSC/APC 收集也不受 CSI 的 24 参数限制保护 |
| 回复 | q、带可回复 ID 的传输/显示可返回 APC G…;OK；错误返回 EINVAL/ENOENT/ENOTSUP/ENOPARENT 等固定文本 | q 字段只要非零就连成功和错误都抑制，不区分 1/2 的不同错误反馈等级。无 ID 的传输不存原始图片且通常不回复；删除没有成功回复。宿主 createImage 返回 Void，无法把解码/条带失败反馈成协议失败，因此 OK 只说明核心路径接受 |
| 编号与存储 | 显式 i 使用该 ID；I 分配新 ID 并更新 imageNumbers 映射；传输拒绝 i 与 I 同时存在。显示优先按 I 查询，再回退 i；同 ID 存储替换 payload | 自动 ID 递增没有寻找空闲 ID，可与显式 ID 碰撞；旧编号映射不因一次替换自动全部清除。普通放置只有提供 p 才登记可引用的键，未提供 p 的条带仍可显示，但不能假定存在自动生成的普通位置 ID；虚拟 U=1 的自动 p 行为见 I23 |
| 24 / 32 / 100 | RGB 24 与 RGBA 32 要求正尺寸、每边不超过 10000、字节数严格等于 w×h×3/4；RGB 转 RGBA 并补 alpha=255。100 保存编码数据，由 ImageIO 读第一图元数据并检查尺寸 | 单体输入/解码结果限 400 MiB。100 未检查 PNG 签名或强制图像类型；实际可接受类型受固定宿主 ImageIO/NSImage 解码路径约束，不能把系统今后新增格式自动加入基线。原始 RGBA 按宿主 premultipliedLast 使用，没有显式逐像素预乘转换 |
| 压缩 | o=z 调用系统 Compression 的 COMPRESSION_ZLIB，64 KiB 分块产生输出；其他压缩值拒绝 | 解压循环内部没有 400 MiB 累计提前终止，结束后才检查；没有 Compression 的构建返回 nil。单体上限不保证压缩峰值内存。不能仅因名称 z 就宣称跨平台压缩互通已验证 |
| 文件介质 f | base64 解 UTF-8 路径、拒绝 NUL、realpath，再拒绝 /proc/、/sys/ 与除 /dev/shm/ 外的 /dev/；只读普通文件，整文件不超过 400 MiB，O 偏移及 S 大小非负 | 读取本机文件，不是远程下载；stat 后再 open，未核对打开对象与先前 stat 的身份，也没有仅限工作目录/用户所有权的额外条件。S=0 表示读剩余至上限；不能把“普通文件检查”描述成完整文件访问授权 |
| 临时文件 t | 复用文件路径检查，另要求路径在临时前缀下且含 tty-graphics-protocol；open 成功后 defer unlink | 前缀判断是 hasPrefix，没有统一目录边界检验；临时文件在后续 seek/read/解码失败时也可能已经删除。这里记录固定行为，不执行真实文件测试，也不把不严密的前缀规则作为复刻必须照搬的保护策略 |
| 共享内存 s | 解 UTF-8 名称，shm_open 只读、fstat 大小、mmap 后按 O/S 截取；raw 格式按期望未压缩尺寸确定截取上界，100 使用对象大小；结束关闭并 shm_unlink | 打开成功后，大小/映射检查失败也会 unlink；并非只在成功显示后删除。raw+压缩时“对象至少为未压缩尺寸”的前置条件仍存在，不能承诺所有压缩组合可用。f/t/s 在组件 Windows 条件分支直接 ENOTSUP；这不是自动删除 Desktop 跨平台复刻要求的授权 |
| 原始图像缓存 | OShell 配置 4 MiB；组件将设置夹到 0…4 GiB。按 payload 字节计数；先淘汰未被条带/位置引用的旧项，再淘汰仍被引用的旧项；显示查询更新访问时序 | 淘汰只移除 imagesById 与编号映射，不删除已绘制条带或位置记录。大于 4 MiB 的新图可在存储后被淘汰，而本次 T 仍持有 payload 可显示；之后 p 可能找不到。条带、解压队列、图像解码和 GPU 纹理不计此预算；无“终端图像总内存 4 MiB”保证 |

### 放置、绘制、滚动与删除

| 子范围 | 固定源码行为 | 限定与缺口 |
| --- | --- | --- |
| 普通尺寸与裁剪 | c/r 正值解释为格数，否则 auto；任一值恰为 0 时保留比例。x/y/w/h 触发源裁剪，起点夹到原图内、非正宽高取剩余，空裁剪拒绝；编码图先转 RGBA。X/Y 负值归 0，有格像素信息时夹到格内 | 没有把任意负数/极大 c/r 统一限制到安全产品尺寸；格式数据限制不等于目标绘制尺寸限制。U=1 提前走独立分支，不消费普通裁剪规则。NSImage 固定宽高的 scale 函数采用其现有比例/居中计算，不能据注释宣称全部等比组合正确 |
| 普通锚点和光标 | 无父位置时从当前光标开始；插入条带每行 LF 并恢复 x，完成后核心按计算格数确定光标。C=1 或相对放置在外层恢复先前 x/y | 恢复坐标不会回滚条带插入时发生的滚动。c/r、偏移、宿主显示比例会进入两处格数计算，需实机比较；其他 C 值没有完整标准枚举校验。普通图像有实际条带，虚拟图只有记录/文字，占用与移动方式不同 |
| 相对位置 | 同时给 P/Q 才查父键；要求父记录存在于当前屏，递归求位置并以 visiting 集合防循环；用 H/V 得到目标。输出滚动且 buffer.hasAnyImages 时，从实际条带位置更新记录并搬移子条带 | 只给一个父字段会按非相对位置处理，却仍可能保存该字段。目标 y 校验使用 lines.count 而非可见 rows；不保证一切目标均在可见屏。唯一更新入口是输出 scroll，不能推断 resize、IL/DL 或所有编辑会同步修复父子位置。移出缓冲范围的条带被丢弃 |
| 宿主条带 | NSImage/CGImage 创建后按当前 cell 高度切片，保存为 AppleImage 附到 BufferLine；每个成功切片触发 updateRange/LF，滚动时加全屏刷新。CG 与 Metal 都消费同一组条带 | 每行图像对象可独立保留像素数据；改变字体/尺寸并不重读原始图像重新切片。某个切片创建失败会跳过，协议侧仍可能成功。OSC1337/Sixel 生成非 Kitty 条带，不进入 Kitty ID/删除/缓存规则 |
| 透明度与层级 | bitmap 用 DeviceRGB、premultipliedLast；CG 图像绘制、Metal 纹理绘制保留 alpha。Kitty z<0 在文字下方，z>=0 在文字上方；组内按 z 再 imageId 排序；非 Kitty 图片最后画 | 负 z 图片仍在单元格背景之后，不存在第二个更深负 z 背景层分支。相同 z/ID 的多个位置没有额外稳定排序规则。完整颜色管理、动画和任意 ICC/EXIF 保留不是这里已有能力 |
| 行生命周期 | 图像附着于行对象；整行移动/滚动可随行保留，行回收和指定清图操作清除。局部格复制不带图像；重排只是搬格/行对象，不重新按文本位置切片 | I15/I25 的窄区滚动、ED2、备用屏清理与部分擦除规则继续适用。Buffer 的图像行计数只在专门 attach/clear、环形回调及部分重排时更新，Kitty 直接改 line.images 不同；不能仅依 hasAnyImages 推断资源精确释放 |
| 删除选择 | d 默认 a：可见输出屏（以 yBase 为基准）；i/n 按图像 ID/编号且可限定 p；c 光标、p 坐标、q 坐标加 z；x/y 列/行、z 层、r ID 范围。模式字母大写后额外清理所有未引用原始图像 | 不以当前上翻 yDisp 作为 a 的视口。ID/编号/范围/z 扫两屏；几何条带扫描当前屏，但对应 record 谓词没有相同的 isAlternateBuffer 过滤，因此可能删到另一屏同坐标记录。r 接收 Int 后直接转换 UInt32，缺少该处显式上界校验 |
| 删除粒度与刷新 | 条带移除后按绝对行减 yBase 更新可见 dirty range；记录另按几何/身份移除，extraKeys 再扫两屏条带；大写清理检查条带与记录引用 | 无完整“位置事务”：a 可只删当前屏内条带，屏外条带可能继续存在；几何命中使用存储的 kittyRow/Cols 等，并非每次重算像素交集。记录单独变化没有相同刷新，尤其虚拟位置继承 I23 的刷新缺口。清图/删除不保证回收全部 GPU 对象的即时性 |

### OSC 1337 与 Sixel

| 子范围 | 固定源码行为与限定 |
| --- | --- |
| OSC1337 入口 | 仅 File 且 inline=1 进入图像解码；base64 使用严格 Data 解码。width/height 接受 auto、1…100%、1…4095px、1…199 格，非法值回 auto；preserveAspectRatio 默认 1。name/size 等字段未用于落盘或认证，非内联内容交宿主空处理。无请求失败回复；宿主解码失败直接返回 |
| OSC1337 尺寸与资源 | 百分比按视图 frame 宽高计算，auto 涉及 backingScale；宿主 NSImage 决定可解码格式。固定显式尺寸上限不限制输入 base64 或 auto 图像尺寸；也不套用 Kitty 的 400 MiB/10000 像素校验。多帧播放、下载管理或自动保存没有生产实现 |
| Sixel 可达性 | 无 collect 的 DCS q 创建 SixelDcsHandler；hook 丢弃 DCS 参数，只清收集数据；unhook 完整解析后调用 bitmap 代理。enableSixelReported=false 只影响 DA，DECRQM 80 回固定 reset，没有 Sixel 独立滚动模式实现 |
| Sixel 数据与颜色 | 先跳到第一个 #，再做尺寸扫描与绘像两遍；# 选择/定义调色板，RGB 百分比或 HLS 值夹限，未定义索引用默认 ANSI 颜色，超表索引透明。! 重复、$ 回带起点、- 前进 6 像素带、?…~ 点阵、CR/LF 忽略；未绘制像素初始透明 |
| Sixel 范围限制 | 不解析完整光栅尺寸/像素比例参数；# 之前的数据被跳过，没有 # 的点阵不会按普通 Sixel 默认色完整显示；没有跨图像持久调色板。尺寸按实际置位像素及扫描 x 计算。收集数组、十进制重复数、两遍循环及 maxX×maxY×4 分配没有统一产品上限，不能套用 Kitty 资源保护 |
| Sixel 分块与显示 | 解析器仅在当前调用存在 dcs 起点时执行 unhook；正文与终止符分属不同调用的路径见 I14，不能仅凭 handler.put 支持多次调用就宣称结束行为正确。bitmap 走通用宿主条带，过宽时按视图 100% 缩放，每行 LF；不具有 Kitty 的身份、相对放置或专用删除 |

本域有限验收集：①合成 RGB/RGBA 与编码图片，取有效、尺寸/长度不匹配各一例，并比较半透明像素；②同图单块/两块、首块字段与尾块字段不同、删除打断 pending、正文与终止符分块；③普通文件 O/S 截取、合成临时文件成功/坏数据后的删除、合成共享内存成功/失败后的解绑，只在隔离目录与名称运行；④小于及大于宿主 4 MiB 缓存的图像，比较 T 本次显示、后续 p、条带仍在及原始缓存淘汰；⑤带 c/r 的裁剪、像素偏移、C=1 和屏底插入，比较像素与滚动/光标；⑥父子位置滚动、单独 P/Q、超可见区目标和父项删除；⑦两屏同坐标位置与跨历史图片，分别验证 a、i、p、z 和大写清理的作用范围；⑧负/非负 z、同层图、非 Kitty 图与文字重叠；⑨OSC1337 的 auto、合法上界、非法回退、inline=0 及坏 base64；⑩Sixel 的 RGB/HLS、重复/换带、无 #、光栅参数、末带及分块终止。资源失败用受控合成小样和静态边界检查，不要求耗尽真实内存。已知缺口修正应保留原版差异说明；没有执行这些 macOS/介质/GPU 用例。

本节完成 KittyGraphics.swift 1–1940 和 SixelDcsHandler.swift 1–408 的整文件阅读；Kitty 的控制/虚拟位置/缓存交接部分沿用并复核 I23 阅读。新增宿主/解析/渲染阅读范围如上表，另核对 TerminalOptions 默认值与 TerminalPane 的生产覆盖、BufferLine 的 images/attach 及 I15/I25 已读的回收/重排路径。字段名来自固定分派，不引用新标准追加范围。两屏共享缓存不等于共享配置 Y 域内容，图像缓存不随配置同步/导出。

## OSC 133 提示符语义与点击（I26）

该能力由固定 SwiftTerm 的终端解析、缓冲元数据和 Mac 鼠标路由共同提供；OShell 沿用默认 `semanticPromptClickBehavior=.enabled`，即无修饰键点击。检索 OShell 生产 Sources 与 shell-integration 未发现发送 OSC133 的内置脚本或配置入口，故需要 shell/进程自己输出该协议；它与 OShell 使用 OSC777 上报主机身份的 T 域能力不同。元数据 API 的存在也不代表 OShell 提供提示符侧栏、跳转菜单或命令历史 UI。

| SF 条目 | 生产入口与源码定位 | 源码结论与关键边界 |
| --- | --- | --- |
| SF-I26 | [SemanticPrompt.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/SemanticPrompt.swift) 整文件；[Terminal.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/Terminal.swift) 378–385、1712–1793、1879–2560、940–950、4595–4614；[Buffer.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/Buffer.swift) 275–607、708–731、946–998、1598–1698 及 I15 重排路径；[BufferLine.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/BufferLine.swift) 24–55、85–109、157–172、193–269、275–390、411–495；[CharData.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/CharData.swift) 265–404；[MacTerminalView.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/Mac/MacTerminalView.swift) 2790–2821、2880–3060；[AppleTerminalView.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/Apple/AppleTerminalView.swift) 2950–3003；OShell TerminalPane 63–114、154–167、256–264，WorkspaceInput 42–81，WorkspaceQuickSend 76–84；Parser 588 | 存储 shell 声明的格分类、行标记与续行组 ID，按需推导当前组与点击几何；主屏、armed、有效点击模式及手势条件满足时，发送单次坐标报告或一组方向键。实际宿主普通输入经过提交扫描，管理发送直接写 PTY；延迟点击只捕获行身份/回收代数/列及手势，不捕获完整组或内容版本。没有退出码持久化、命令执行记录或自动注入 shell 钩子的功能。 |

### 协议字段与状态转换

| 子范围 | 固定源码行为 |
| --- | --- |
| 解析门禁 | OSC133 只在主屏消费；须有效 UTF-8、非空且首 action 字段为一个字符。未知 action 忽略；选项没有 = 时跳过，未知选项名跳过，已知选项值非法则整条忽略，发生任何光标/状态改变之前完成校验。重复已知字段以后值覆盖 |
| 选项 | k=i/r/c/s 为初始/右侧/续行/次级提示符；cl=line/m/v/w 为同逻辑行、跨行、保守垂直、智能垂直；click_events=0/1/2 关闭事件覆盖/绝对/相对报告；special_key=0/1 控制左右移动编码。click_events=1/2 优先于 cl；0 仅移除事件模式，仍可使用同条的 cl。未提供选项按该条默认值解析，不是全局继承所有字段 |
| A / N | 非右侧先 fresh-line：不在左边界时 CR+LF，已经在左边界则不动。k=r 仅加右侧标记，不换行、不改分类/armed；k=c 在 fresh-line 后置 prompt/continuation，不存标记、不另分配组。其余情况下 N 新分组；A;k=s 加入已开组，没组才创建；A;k=i 按同一行对象及状态复用或新建 |
| 初始/次级组配置 | 初始提示符或本次新分组重设点击模式与 special_key；A;k=s 加入已有组时保留组配置。新组会清新起点行继承的旧 hard-continuation epoch，防止旧组 LF 留下的连接把新提示符吸入旧内容 |
| P | 不移动光标；k=i 按同一规则分组并重设点击配置，其他 k 加入当前组。存非 continuation 的标记，并把后续格分类设为 prompt(kind)；不把 semanticInput 自动改为 prompt。因此 P;k=r 与 A/N;k=r 的分类副作用不同，不能合并表述 |
| B / I | 设后续写入为 input、交互状态为 armed。没有组或点击模式时仅 armed 不足以成功点击；不会凭空创建提示符起点 |
| C / D | 设后续格为 output、交互状态 submitted，清光标所在行的 hard-continuation epoch，防止回显 Enter 落点继续作为输入行。D 后的裸退出码被选项解析跳过，没有退出码字段、成功/失败标记或命令记录 |
| L | 只执行同一 fresh-line，保留内容分类与交互状态；其 LF 仍按当前 prompt/armed 状态决定续行 epoch |
| 输入提交扫描 | Terminal.sendUserInput / AppleTerminalView.send 在发送前扫描：非括号粘贴内 CR/LF、SS3 keypad Enter、Kitty 13u 非 release 使 armed→submitted；识别跨调用 ESC/CSI/SS3 与 200~/201~ 粘贴边界，CSI 临时参数只保留前 32 字节。扫描只能取消 armed，不能推测重新 armed |
| 分类与交互分离 | 提交扫描只改 semanticInput，不立即把后续格分类改为 output；直至 C/D 或切屏等事件，格仍可能按旧 input 分类写入。SS3/Kitty Enter 分支没有与普通 CR/LF 相同的粘贴门禁；不能把“粘贴中的所有提交编码均忽略”当作已实现事实 |
| 切屏与重置 | 真正切屏时两屏 input 状态均变 submitted，旧 prompt/input 分类变 output；备用屏忽略 OSC133，返回后需新 B/I 才重新 armed。RIS 重建主屏；DECSTR 将两屏状态设 idle、清当前组/点击配置/输入扫描，并清各屏可见行的标记和 epoch；不由此清所有历史格的语义字节 |

### 元数据、重排和派生行

| 子范围 | 固定源码行为及边界 |
| --- | --- |
| 格语义 | CharData 用一个字节表示 none、四种 prompt、input、output；未知字节读作 none。普通字符与 ASCII 快路均在 Buffer 写入入口盖当前分类，双宽尾格继承同类；新空格默认 none，单独 setValue 不改原语义，因此 I16 的矩形保属性擦除仍可能保留空格的 input 标签 |
| 行标记 | 每行同 kind 最多一个，重写替换并按列排序；零宽标记位置夹到行内。continuation 只作为派生种类，不实际写标记。initial/right/secondary 可以共存，组 ID 随标记保存；不把每个标记当独立功能 |
| 活动组身份 | 每屏单调组计数器避开 0；起点保存 BufferLine 对象和行号缓存。缓存命中后验证存在 opening kind；行移动后按身份向两侧寻找，再按光标附近扫描当前组 opening mark 重绑；找不到则起点失效，拒绝点击。复用需 prompt/armed、同起点行对象及当前组 opening mark |
| 身份校验的具体边界 | 快速行校验只检查 initial/secondary kind，重绑谓词才检查 active group；rawSemanticOrigin 优先 initial 再 secondary，不额外按组过滤。若不同组、不同 kind 标记同处一行，不能仅据“行身份有效”宣称一定选中当前组正确起点；需保留同一行新建 secondary 组的指定核查场景 |
| 续行 epoch | 只有实际 LF/IND 推进行后且状态 prompt/armed 时写入当前组 ID；先让滚动决定落到哪个行对象。若落点残留别组 epoch，先清该行旧 prompt/input 格标签；submitted 输出不新增 epoch。新组分配与 C/D 的指定清理是另两个显式边界 |
| 派生行分类 | initial 标记行，或活动起点的 secondary 标记行，派生为 initial；secondary/right 的非零组标记可派生 continuation。否则向前沿 isWrapped 或同组 hard epoch 寻找起点/加入标记；硬链跨组则中止。历史组也可分类，不等于都允许点击 |
| 插删/擦除 | ICH/DCH 把受影响标记列随格平移并夹在边界，删除标记对应格不删除标记。EL/ED/replaceCells/fill 操作格，不统一清标记或 epoch；因此擦空行仍可能保留结构标记。缩窄夹标记列；行回收通过 clear 增加 recycleGeneration 并显式 destroySemanticState |
| 范围搬移与 reflow | BufferLine.copyFrom 的范围路径把范围内标记从源移到目标，目标覆盖区域的旧标记移除；同 kind 冲突优先保留 live origin，否则保留原目标标记。跨行移动起点会改指向新行对象；完整宽度搬移才带 epoch，窄范围不带。重排沿用 I15 的真实格复制/裁剪，不能承诺所有标记与被裁掉字符无损 |
| 校验辅助函数 | semanticPromptInvariantsHold 检查重复 kind、列界、组号/owner 及派生分类等；生产调用检索只有定义，没有自动调用。它不是每次输入后的运行保护，也不能以函数存在代替执行测试 |

### 点击几何、编码与宿主仲裁

| 子范围 | 固定源码行为及限定 |
| --- | --- |
| 可点击条件 | 主屏、armed、策略允许修饰键、点击在有效行列、可解析活动组且组内至少有一个非零宽 input 格；鼠标快照还要求按下时未被应用鼠标报告消费、单击、无拖动、按下前无选择。默认策略仅无修饰键，组件另有 disabled/精确修饰键 API，但 OShell 未提供产品配置入口 |
| 实际目标限制 | 检查的是“组里有 input”，没有逐格强制目标本身为 input；同组提示符/空白格也可进入路线。方向键模式通过目标前 input 格计数得到位置；坐标事件模式直接报告格坐标。因此不能用注释“target resolves onto input cells”扩写为严格的目标格门禁 |
| 逻辑几何 | 从活动起点依次跟随软换行或当前组硬续行/加入标记；只计算 input 且 width!=0 的格，一个双宽字计一步，硬行边界计一步，软换行不加步。光标待回绕 x==cols 保留整行计数；点中 input 宽字尾格时向前归主格 |
| 坐标报告 | click_events=1 输出单次 ESC[<0;x;yM，x 是逻辑列+1，y 按 row-yBase+1 夹到可见屏；=2 的 y 为当前组主起点或此前最近 secondary 的行差，可为 0，x 仍是列+1，不减提示符起点列。不输出 release 配对或像素报告 |
| 方向键报告 | cl=line 禁止跨硬逻辑行，但允许跨软换行；cl=m 按全组偏移差发左右键。cl=v 与 cl=w 在固定实现中采用同一分支：先退至当前逻辑行首、按硬行数上下、再到目标列；没有独立 smart 算法。special_key=1 用 CSI 1u / CSI 1;1u 表示向前/后退，跨行也按左右步骤；普通键按 applicationCursor 选 CSI/SS3。整次点击合成一个发送数组，零偏移不发送 |
| 鼠标优先级 | 按下先处理应用鼠标报告，再处理清选择/双击/三击；抬起先打开符合规则的链接，再处理应用 release，最后才安排语义单击；拖动用于选择。OShell 的四击转双击及选择复制继续经过这些入口，不把语义定位扩为所有鼠标操作的最高优先级 |
| 延迟与取消 | 抬起在快门禁通过后等待 NSEvent.doubleClickInterval；本视图下一次 mouseDown 取消待发工作。保存目标行对象、recycleGeneration、列与修饰键/手势；发送前重新查行位置与组/armed 条件。行淘汰/回收或切到备用屏可使其拒绝 |
| 延迟期边界 | 不捕获普通 generation、原始内容、原活动组 ID、窗口焦点或 resize 后字符位置；同一行对象未被回收但内容/组变了，或 reflow 把原点中字符搬到别行，保存列不随字符追踪。源码没有独立失焦取消路径。不能将现有回收检查描述为“所有过期点击均阻断”；该缺口归本项有限用例 |
| 坐标与 BiDi/特殊行 | Mac calculateMouseHit 先按格尺寸/yDisp 得到行，再用 BiDi visualToLogicalCol 映射列，语义路由使用逻辑坐标。特殊双宽行的鼠标反向比例限制见 I25；完整 BiDi 一致性仍由 I24 核查 |
| OShell 发送交接 | 普通 TerminalView.send 先 registerUserInput，再送代理；核心生成的语义坐标/方向键直接走代理，通常在 userInputDepth=0，不进入同步键盘广播。OShell send 仍检查关闭/结束/传输状态，实际网络/进程归 C 域 |
| 管理发送缺口 | TerminalPane.sendManaged 在检查可输入后直接 terminal.process.send，不调用 registerUserInput；WorkspaceInput 的广播、文本粘贴、撰写发送以及 WorkspaceQuickSend 都可达。主动输入端可能已扫描，但其他目标或程序发送的 Enter 不保证同步撤销 armed；需等 shell C/D 或其他状态事件。跨域 O/I 的同一缺陷，不能算新增同步功能 |

有限验收集：①A/N/P 的 i/r/c/s 及 B/I/C/D/L，比较移动、组号、分类、armed 与点击配置；加入未知名、坏已知值和 D 裸退出码；②同一行重绘复用、新 secondary 组与旧 initial 并存、旧组续行落点，比较真实起点和派生分类；③带 prompt/input 的宽字、组合字及软/硬换行，核对 line/m/v/w 与 special_key 输出字节；④点击组内 prompt/空白/input，比较绝对/相对事件及 secondary 相对行；⑤普通、括号粘贴、SS3 Enter、Kitty Enter press/release 的分块输入，比较取消 armed 与格分类；⑥EL/ED、ICH/DCH、窄区复制、resize/reflow、历史裁剪与循环行回收，检查标记/epoch/列；⑦切屏、RIS/DECSTR 后点击以及历史标签保留；⑧已有选择、链接、应用报告、拖动、双/多击与修饰键仲裁；⑨待发点击期间分别滚动、回收、同一行重写新组、缩放搬字、失焦，记录发送或拒绝；⑩广播目标/快捷发送带 Enter 后、shell 尚未 C/D 的窗口内点击。各场景使用合成协议，不执行真实远端命令；本轮只完成静态核查，没有运行实际 AppKit/PTY 点击测试。

阅读覆盖：SemanticPrompt.swift 整文件；Terminal 378–385、1712–1793、1879–2560、940–950、4595–4614；Buffer 275–607、650–768、946–998 及 I15 已读重排/写入段；BufferLine/CharData 如证据表；CircularList 330–351 的回收接缝；MacTerminalView 2790–2821、2880–3060；AppleTerminalView 2950–3003；OShell TerminalPane 53–118、153–183、250–280、WorkspaceInput 22–84、WorkspaceQuickSend 65–86。语义 API/配置/发射脚本的生产调用检索不作为实机证明；iOS 与 Headless 的独立事件宿主不是本次 macOS OShell 范围入口。

## 终端：双向文字（SF-I24）

本项分析的是随固定 OShell 生产构建使用的 SwiftTerm 实现。OShell 的 TerminalPane 455–459 构造 TerminalOptions 时没有覆盖 BiDi 参数；对应用 Sources 的 Swift 生产代码检索也没有 BiDi 设置入口。MacTerminalView 的默认 hostPolicy 为 respectTerminal，因此底层功能实际可达；组件的 legacyLeftToRight API 不自动成为新的产品设置。

| SF 条目 | 生产入口与源码定位 | 源码结论与关键边界 |
| --- | --- | --- |
| SF-I24 | [Bidi.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/Bidi.swift) 全文；TerminalOptions 106–153；EscapeSequenceParser 424–475；Terminal 4216–4360、4470–4495、4597–4599、5226–5228、5288–5293、5455–5457、5541–5546、5949–5983、6406–6408；[TerminalBidi.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/Apple/TerminalBidi.swift) 全文；[ArabicShapingData.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/Apple/ArabicShapingData.swift) 固定表；[BidiMirroringData.swift](../validation/oshell-reference/Vendor/SwiftTerm/Sources/SwiftTerm/Apple/BidiMirroringData.swift) 固定表；AppleTerminalView 1110–1365、1511–1583、2413–2475、2554–2579、3016–3041；MacTerminalView 1011–1018、1785–1856、1981–2095、2436–2451、2462–2529、2790–2830；MetalTerminalRenderer 768–803、982–983；SelectionService 781–788、SearchLineCache 61–104、TerminalViewSearch 79–88、Terminal 7226–7229、7860–7940；MacCaretView 55–72；Buffer/BufferLine 的行状态交接沿用 I15 已读路径 | 六种模式、段落状态、CoreText 探针排序、固定阿拉伯字形及镜像、双向列映射和生产渲染均有直接路径。缓冲文字保持逻辑顺序；段落上限、近似字符分类、显式模式、光标/IME及键盘旁路必须分别限定，不能宣称所有 Unicode/交互已经一致或通过实机验收。 |

### 模式、段落与数据生命周期

| 子范围 | 固定源码行为及限定 |
| --- | --- |
| 默认及六模式 | 默认 implicit、autodetect=true、fallback=LTR、boxMirroring=false、arrowKeySwap=false；最大段落行数 500，构造时至少 1。隐式固定/自动 LTR/RTL 共四种，显式 LTR/RTL 两种；显式模式忽略自动方向开关的效果，但保留其存储值 |
| 协议入口 | 普通 SM/RM 8 切换 implicit/explicit；DECSET/DECRST 2500 控制框线镜像、2501 控制自动方向、1243 控制左右键互换。CSI Ps SP k 的 SCP 接受 0=配置默认方向、1=LTR、2=RTL；CSI Ps SP S 的 SPD 别名接受 0=LTR、3=RTL，其余忽略。二者只取首参数，不等同全部字符路径标准 |
| 查询/保存/重置 | DECRQM 查询普通 8 及私有 1243/2500/2501 的当前设置；CSI ? Ps s/r 仅保存/恢复后三者，不保存 SCP 方向或普通 8。RIS/DECSTR 恢复 options 初值和箭头开关、清私有保存字典；历史格与屏幕整体重置效果沿用 I21，不由本项声称所有历史段落都改为新模式 |
| 模式修改生效点 | 更新 currentBidiState 和两屏 defaultBidiState；只有光标在当前段落第一行的逻辑 x=0 时，才把本次改动的属性应用到已有整段所有软换行行。段中/续行的修改影响后续新段默认，不能把当前全部历史内容重新解释；只改指定属性可保留段内其他旧设置 |
| 行状态流转 | BufferLine 保存 bidiState，修改会 bump generation；新行取默认，软换行继承原段状态，硬换行进入非 wrapped 行时写当前状态。整行复制/移动带状态；主屏 reflow 以原段首行状态赋新行。滚动区域及 IL/DL 的新接缝显式断开 isWrapped，避免不相关行被合成一段；精确历史、裁剪和备用屏缩放限制见 I15 |
| 段落范围与上限 | 从目标行沿 isWrapped 向前/后寻找完整段落；超过 max(1, maximumBidiParagraphRows) 返回 nil，不截取前 500 行继续排，也没有另一个逐行隐式重排实现。渲染退回逻辑格序并按 RTL 触发情况施加 LTR 覆盖；resolvedBaseDirection 此时返回该行 fallback，因此箭头方向与可见逻辑排列不保证一致 |
| 段首状态 | 隐式计算先检查所请求行 supportMode，再以软换行段首的 bidiState 解析整段；不同模式不会额外作为段落切分条件。正常模式/重排行为维护段状态，不能从渲染器推断它能修复任意混合状态数据 |

### 排序、连写、镜像与缓存

| 子范围 | 固定源码行为及限定 |
| --- | --- |
| 快速触发 | 扫描 Hebrew/Arabic 及固定历史 RTL 码段和 RLM/RLE/RLO/RLI；按格宽跳过宽字尾格，多标量格逐标量检查。无触发且 fallback=LTR 的整段直接返回恒等路径，不能把这组启发式范围说成所有 Unicode 双向类表 |
| 自动基方向 | 沿逻辑格扫描标量，RLM/LRM 分别通过 200F/200E 判 RTL/LTR；其余以第一个 alphabetic 标量及是否落在 RTL 触发范围判方向，无结果用 fallback。这里不是独立实现完整 UAX #9 的首强字符规则；字母/类别属性来自运行时 UnicodeScalar.properties |
| 排序探针 | 提取每行到 trimmedLength，空格保留为占位、每个主格一个探针 UTF-16 单元，宽字尾格不另占单元。独立组合标记及多数非 BMP 字符替换为中性点，固定连写范围含 ZWJ 替换为不连写的 hamza，非 BMP RTL 触发字符替换 RLM；其余 BMP 保留。这样维持格索引，但不是逐字符原始 Bidi_Class 的精确等价 |
| CoreText 分派 | 用整段探针、指定基方向、关闭普通连字创建 CTTypesetter，再对各物理行的探针范围创建 CTLine。按 glyph x 及索引排序、去重得到格序，并保存 run RTL 标志；没有 glyph 的格式控制按最近已解析邻居推断方向，再插到相邻索引旁。若顺序结果无效该行没有布局，退回逻辑路径；不跨物理行重新分配字符格 |
| 阿拉伯字形 | 以整段逻辑格求前后连接，不因 SGR 属性改变断开；邻接寻找跳过 transparent，ZWNJ 明确断开，ZWJ/causing 参与连接。只对固定 forms 表内字符选择 isolated/final/initial/medial，保留附加标量；不修改原 CharData。固定 joining 表包含其他文字的分类，不等于给这些文字实现了全部显示字形 |
| lam-alef | 四种 alef 与紧接的 lam 格构成 ligature；显示字形放 lam 格，alef 格显示空格，两格原始文本及列仍保留，附加标量合入显示字形。匹配的是相邻提取格，可跨软换行；不会跳过独立透明格去寻找 alef。连写与两行显示的组合须按有限样例验证 |
| 镜像 | 隐式模式只在 CoreText 判 RTL 且没有连写 display override 的格应用固定 Unicode 镜像；要求原文字是单标量，带组合标记的括号不走单标量镜像。未命中时可按 2500 使用固定框线映射；不是任意图形水平翻转 |
| 显式模式 | 显式 LTR 无 BiDi 布局；显式 RTL 反转整行主格顺序，将尾部未用列从左填充，使逻辑列 0 靠右；宽字格内部宽度保持，不反转其标量。仍对单标量应用 Unicode 镜像及可选框线镜像，但不调用 shapeArabic 或 CoreText 排序，不能泛称“显式模式完全不改显示字符” |
| 双向列映射 | RTL 基方向把尾部未用列倒序放左侧，LTR 放右侧；logicalToVisual 为每个宽字列保留视觉偏移，visualToLogical 将宽字覆盖的各视觉列都映到主格。映射不是尾格位置的一一逆函数；布局只产出显示 override，不改复制/搜索的数据 |
| 固定表与平台依赖 | 两张表标注 Unicode 17.0 来源；本次本地结构检查：428 个镜像键，无重复且反向对称；157 个 joining 范围有序、无重叠或反向区间。此检查只证明随包表结构，不证明与外部标准逐项一致，也不证明 CoreText/系统字体在所有系统版本输出一致 |
| 布局缓存 | 身份缓存键含 buffer 身份、段行范围、所有行身份/generation/isWrapped 的组合 revision、列数、字体对象、段状态。内容缓存忽略绝对行号，hash 命中还逐格比较相对行、逻辑列、宽度和原文字，再把布局重定位；属性不参与内容键，显示属性仍从当前行原格读取。两个静态缓存各到 256 项整体清空，不是按字节限制或 LRU |
| 刷新依赖 | Mac 更新把有效变动范围向段首/尾扩展；显式或超过上限的段不扩展。Metal 行缓存额外比较 paragraphRevision，变化时重建；滚动离底部时常规视口整区重画沿用 I21。单格改动可能改变同段其他行，不能只按当前行 generation 验收 |

### 绘制与交互交接

| 交接 | 固定源码行为与已定位限制 |
| --- | --- |
| 两条绘制路径 | CoreGraphics/Metal 共用 buildAttributedString：按 visualCells 取逻辑原格属性、链接、选择和规则高亮，按 visualCol 放置；display override 仅用于字形。对已重排及检测到 RTL 的逻辑路径加 LTR writing-direction override，防止 CoreText 再排一次；BiDi 下阿拉伯/多标量格单独锚定，避免连字或附加 glyph 推移后续格 |
| 选择与复制 | 鼠标命中先视觉列→逻辑列，选择存逻辑端点；高亮按逻辑范围逐个可视格判断，因此混合方向中可能显示为分离的视觉区间。复制走 getDisplayText→原缓冲文字，不复制镜像字形/连写替代，也不按画面从左到右拼接。不能扩大为独立的视觉矩形选择编辑器 |
| 搜索与规则高亮 | SearchLineCache 沿软换行拼逻辑文字；结果变逻辑选择范围，再由统一绘制映到视觉格。规则前景高亮亦先在逻辑文字算范围再查原列；是否正确处理全部宽字/UTF-16 细节继续使用 I02/I05/O 域既有规则，不由本项承诺额外搜索标准 |
| 鼠标与链接 | calculateMouseHit 按 yDisp 找行、钳位列后反映射，格报告、链接命中、选择和语义点击共用逻辑列；pixels 仍为原视图点坐标，不做 BiDi 反映射。特殊双宽行缺少比例逆变换见 I25；SGR 像素报告与格报告不是相同坐标空间 |
| 光标 | 光标 x 在映射数组范围内时转视觉列，随后应用特殊行倍率；宽度来自原 CharData。待回绕 x==cols 不映射，宽字尾格不自动归主格。setText 仍传原 CharData，MacCaretView 从原字生成 CTLine，没有使用布局的镜像/连写 display override，故块光标覆盖后的字形不能宣称与底下图文一致 |
| 输入法 | firstRect 返回已映射 caretView.frame，markedText 浮层从 caret x 避让左侧并按普通 NSTextView 排版，后续折行从左开始；未设置与当前 BiDi 段相同的方向。characterIndex(for:) 直接按普通网格 row*cols+col，不用 visualToLogical；selectedRange 对既有选择用 row*buffer.rows+col，而无选择用 cursorY*cols+x。候选框锚点接上不等于点位、范围和组字排版完全一致 |
| 左右键互换 | sendHorizontalKey 在箭头开关开且当前光标段 resolvedBaseDirection=RTL 时互换普通 CSI/SS3 左右键；OShell 默认关闭。Mac doCommand 在此之前先尝试 Kitty 编码，编码成功直接发送，sendKittyFunctionalKey/sendKittyEvent 无 BiDi 互换；协议键、带修饰键及其他直接发送路线不因存在该开关自动一致，关联 I18 |
| 宿主策略 | respectTerminal 为生产默认；组件 legacyLeftToRight 会让 layout=nil、方向查询返回 LTR，但不删除已存模式。OShell 未暴露此选择，不追加用户可配置语言/方向设置；实际 AppKit、CoreText、Metal 和 IME 结果仍需 macOS 证据 |

有限验收集：①六模式与 SCP/SPD 有效/无效参数，比较段首 x=0、段中、续行及下一硬行的状态；②普通 8 与私有 1243/2500/2501 查询、保存/恢复及 RIS/DECSTR；③拉丁/数字/希伯来/阿拉伯混合、RLM/LRM、非 BMP、独立组合标记与纯 LTR 快路；④跨软行、属性变化、ZWJ/ZWNJ、四种 lam-alef 及独立透明格，检查逻辑数据与显示；⑤单标量/带组合标记括号及框线镜像、显式 RTL 宽字/尾部空格；⑥499/500/501 行同段以及单格修改、滚动、IL/DL 接缝、resize 和字体变化后的两渲染路径；⑦鼠标选中宽字两半、混合方向选区、复制、搜索及链接命中，比较逻辑结果；⑧块/条光标、尾格/待回绕、候选框、组字折行及输入法点位/范围；⑨开关 1243 下普通/应用光标/Kitty 编码与直接发送，核对实际字节；⑩与已有特殊行、图形占位和 OSC133 样例组合，沿各自主条目记录差异。以上是固定场景，不是全 Unicode 穷举；本轮没有运行 Swift/CoreText/Metal/IME 实机测试。

已读覆盖：Bidi.swift、TerminalBidi.swift、ArabicShapingData.swift 全文；BidiMirroringData 的固定键值结构全量本地检查，未做外部标准逐项复核。上述核心/视图区段与 I15/I17/I18/I19/I21/I25/I26 已读交接共同构成本项证据。已定位的退化、IME、块光标和键盘旁路归 SF-I24 及关联条目，不新增功能编号；复刻若有意修正，保留原版行为与目标差异说明。

## 最终生产入口反查

反查范围固定为本提交的应用启动与全部顶层菜单、设置页装配、生产模型、窗口/终端/文件服务、辅助可执行文件、打包依赖和终端协议分派。目的是从入口回到 SF，发现遗漏；不把测试工具、未被宿主使用的依赖 API 或未来标准新增内容引入清单。以下入口结合各域已有逐项执行路径核查，不以“文件名在文档中出现”代替分析。

| 反向入口 | 已核对的实际分支与归属 | 反查结果 |
| --- | --- | --- |
| Package.swift、SwiftTerm/Package.swift | OShell 与 OShellCore、系统 SSH 包装器、Proxy/Askpass、ZOC/FileZilla 两启动器；SwiftTerm、Sparkle、legacy CryptoSwift；生产默认排除 TestSupport，SwiftTerm 排除 iOS/文档 | 全部生产目标归 C/P/E/I/U；测试 harness 和非目标平台不能变成用户功能。Sparkle 为二进制，系统 SSH/FTP 和 CoreText/字体按调用合同分析，不声称其全部内部源码已读 |
| main.swift 22–120、223–258 | 数据目录激活/回退、加密库启动解锁、IPC 服务及重复实例、启动同步/更新、冲突草稿提醒、激活刷新、退出清理、关闭末窗后重开/Dock 新窗；命令行转两启动器与拒绝正式包测试参数 | Y01–Y04/Y08–Y17、P08–P10、E05–E09、U01/U04/U06、W01/W12、I27 均有归属；无独立后台终端恢复或未记录的协议入口 |
| 应用菜单 main 131–142 | 关于/许可、检查更新/更新设置、通用/外观设置、锁定/修改/清除主密码、隐藏/退出 | U01–U07、A01–A02、P08–P10、W01/W12；补清菜单“关于”与设置“关于”两种入口及许可打开失败处理差别，保留 U07 身份 |
| 会话菜单 main 143–165 | 新窗、代理/会话管理、快速切换、同步/冲突/放弃草稿/刷新、导入导出、默认/当前属性、新建/连接/编辑/删除、空白/本地标签、重连、关闭 | S01–S14、C01–C14、W01/W03/W04/W09/W12、Y08–Y13、E01–E04、I09–I10；每个 selector 由既有 Workspace/编辑器/存储分析承接 |
| 编辑与视图菜单 main 166–202 | 复制/粘贴/全选响应链，搜索前后，专注/全心专注、命名组、allCases 排列、分屏/放大/切换、日志、标签前后/最近/编号、刷新主机识别 | I01–I05、W02/W04–W06/W10–W11、T01–T09；排列与编号循环生成的项归原项，不按菜单数量追加功能 |
| 工具/动态/窗口菜单 main 203–219 | 侧栏/链接栏、快速命令/发送栏/定位、撰写、同步输入开停、文件管理、高亮集、动态快捷连接/命令、最小化 | S09/S11–S14、O01–O09、F01–F15、W01；动态内容由 WorkspaceWindows/WorkspaceController/ShortcutRuntime 及既有各管理器分析闭合 |
| Dialogs.preferences 13–82、SettingsWindow | 常规字体/字号/历史/GPU/自动 rzsz/右键/多行及图片粘贴；外观、更新、快捷键、同步、选择、文字、时钟、安全、标签、关于各页及应用/取消/校验 | A01–A08、F13–F15、I01–I03、M01–M08、U/O/Y/P/T 对应条目；本机选择/图片/标题/安全参数及共享偏好分别归已有持久化路径，不把每个控件追加为 SF |
| Models、ConnectionOptions、OperatorModels、共享合同 | 四类会话；代理/跳板与三类隧道；偏好、目录/Links、主密码标记、命令/高亮、旧 FTP 迁移、同步选择和 v1/v2 合同 | S/C/P/O/A/Y/E 已有逐字段与保存路径证据；旧 FTP 模型不是新独立协议产品，模型可存字段与 UI 真正可编辑能力保持区分 |
| WorkspaceController、WorkspaceWindows 与局部菜单 | 标签栏/组/侧栏/Links/工具栏/文件窗的右键、双击、拖放、快捷键、关闭、跨窗重绑及共享服务发布 | W/S/F/O 既有整文件与调用区段覆盖；运行对象迁移和配置复制分属 W08/S07，不遗漏后台运行，也不推成跨重启会话恢复 |
| TerminalPane 与 OShellTerminal 宿主 | options、PTY/SSH、凭据/代理、主机探测、传输接管、图片上传、同步发送、选择/搜索、OSC 覆盖、GPU/关闭/空闲清理 | C/P/T/F/M/O/I/A 的执行路径已闭合；终端默认 BiDi 生效、OSC52 禁用、OSC777 被宿主覆盖、OSC1337 非内联回调空实现等按 I24/I27 限定 |
| EscapeSequenceParser 403–619 与 Terminal 分派 | CSI/ESC、OSC0/1/2/4/6/7/8/9/10/11/12/52/104/112/133/777/1337、APC G、DCS $q 与 q；用户注册回调优先 | I14–I27 及 T 已逐处理器归属；未实现矩形属性/保护协议、不消费的通知/剪贴板/文件回调不能因标准存在被纳入。普通颜色、键鼠、缓冲、图形、BiDi 和语义路径均有独立证据 |
| 五个辅助目标及随包 lrz/lsz、Shell 脚本 | SSH 包装/父进程与 PTY，代理握手/路由，Askpass IPC，两启动器参数/凭据/接收，ZMODEM 应用发起参数与 helper，Bash/Zsh 主机上报 | C/P/E/F/T 对应域已整读自有入口及相关调用；lrzsz 只纳入 OShell 实际暴露路线，不把随包全部选项变为产品要求 |
| 分发脚本/资源、更新与兼容工具 | helper/启动器/字体/许可/Shell 集成随包，目标与签名/元数据/更新桥；本地 IO 和摘要适配 | U/A/T/C/E 与 P/Y/F 底层调用闭合；构建缓存、测试脚本和签名密钥管理操作不新增产品功能，未执行发布或访问真实凭据 |

本次补读此前未单列的四个生产辅助文件：

- [OpenSourceLicenses.swift](../validation/oshell-reference/Sources/OShell/OpenSourceLicenses.swift) 全文：上述 U07 两入口差异；资源缺失时菜单路径无额外提示，不能沿用设置页的失败提示结论。
- [SessionListIcon.swift](../validation/oshell-reference/Sources/OShell/SessionListIcon.swift) 全文：父级/目录/Links 与四协议图标、快捷引用角标、明暗外观刷新、可访问名称及有限缓存；归 S06/S09/S14、A01，不追加图标编辑器或协议模型。
- [PlatformDigest.swift](../validation/oshell-reference/Sources/OShellCore/PlatformDigest.swift) 全文：CommonCrypto SHA-256 包装。调用点用于本机配置指纹、目录条件比较、路径/身份作用域和证书指纹等；归 P/Y/E/M/W 的现有完整性与身份约束，不是独立用户哈希工具。
- [FileHandleCompatibility.swift](../validation/oshell-reference/Sources/OShellCore/FileHandleCompatibility.swift) 全文：现代 FileHandle API 与旧系统 Darwin read/write/fsync 适配、EINTR 重试、短写循环、EOF/零读/错误返回及关闭。已回查 PrivateFile、Streams、RemoteFiles、归档/导入、凭据及 launcher 等调用，归对应 P/Y/I/F/E；不额外创建文件系统产品域。

反查未发现无法归入固定 SF 的独立产品能力；以上补证和文字纠正不改变 13 域/161 项身份，也不把原版已定位缺陷静默排除。源码索引用于锁定证据文件，不将其中测试、资源表、辅助构建文件的数量解释为功能数量或阅读完成率。

## 分析完成与验收区分

固定提交的 13 个功能域、161 个细分条目已完成直接生产路径静态分析，并完成上述从生产入口到条目的反向遗漏核查。功能基线已按源码修正包含范围、实现限制、关联缺口和有限验收要求；当前没有未归属的已发现原版产品能力。这里的完成范围是本次功能梳理，不是每个第三方包逐行审计、全部标准符合性认证或 Desktop 产品完成声明。

本轮只做源码与文档核查，没有执行 Swift 原版应用、真实双端共享配置互通、Linux 桌面或 Windows 实机交互验收。上述运行证据缺口不阻止静态源码分析，但不允许记录为产品验收通过。

以下按时间保留过程记录；其中的“待核查/尚未完成”描述对应当时状态，最终状态以上方覆盖表、反查结果及末尾最终一致性检查为准。保留它们用于追踪证据演变，不据旧记录扩大功能边界。

2026-10-24 文档一致性检查（补齐 F/Y 后）：功能基线仍为 161 个唯一的五列 SF 条目，必填列非空；本记录含 S/P/C/W/T/Y/M/F/O/A/E/U 十二域的 134 条唯一逐项证据，终端域明确待核查；两份文档的本地文件链接均存在。源码索引 607 个文件重新计算 SHA-256，零不匹配；参考仓库 HEAD 仍为固定提交，Sources、Package.swift、shell-integration 和 scripts 无工作区改动。哈希检查只验证固定证据来源，不改变上面的阅读覆盖状态。

2026-10-24 文档一致性检查（补齐 I01–I14 后）：基线仍为161个唯一五列SF条目，逐项证据148个唯一条目；缺项恰为I15–I27，无多余/重复编号、空必填列或失效本地文件链接。固定HEAD不变，参考Sources、Vendor及Package.swift相对HEAD无差异。本轮新增的源码事实、缺口和待执行场景只更新上述两份文档；未修改生产程序、运行Swift实机验收、提交或推送。此记录不替代尚未完成的13项与最终反向遗漏核查。

2026-10-24 文档一致性检查（补齐 I18–I19 后）：固定五列SF目录仍为161项，三列逐项证据为150项；缺项恰为I15–I17及I20–I27，无多余/重复编号、空必填列或失效本地文件链接。参考Sources、Vendor和Package.swift相对固定HEAD无差异。本轮仅修订功能基线与本记录；没有运行Swift/macOS事件验收，也未修改生产代码、构建、提交或推送。下一步继续查询/状态及其余终端处理器，不能用已完成的键鼠路径代替尚未核查的重置、缓冲或渲染。

2026-10-24 文档一致性检查（补齐 I20–I21 后）：固定目录161项，逐项直接路径证据152项；待核查恰为I15–I17、I22–I27，无多余/重复编号、空必填列或失效本地文件链接。参考HEAD仍为3f4efd9c70c82323f4e20180117c928227000580，Sources、Vendor和Package.swift相对HEAD无差异。此次只更新上述两份文档，未运行Swift/macOS/GPU验收，未改生产代码、构建、提交或推送。静态查询回复表和重置状态矩阵已建立，剩余九项及最终反向遗漏核查仍未完成；不以文档条数换算产品完成率。

2026-10-24 文档一致性检查（I17及本次I25/I27）：固定目录161项，逐项直接路径证据155项，缺项恰为I15、I16、I22、I23、I24、I26；无多余/重复编号、空必填列或失效本地文件链接。固定HEAD未变，参考Sources、Vendor和Package.swift无工作区改动。补齐特殊行的解析/存储/绘制/命中及回收重排差异，宿主代理链、OSC777覆盖、非内联文件/测试入口边界；纠正把完整普通渲染误归I23的交叉引用，I23始终只指Unicode图形占位，不扩大功能身份。只更新两份分析文档，未修改程序、运行Swift/macOS/GPU或互通验收、构建、提交或推送。六项与最终反向遗漏核查仍待完成，155是证据目录数，不是产品完成率。

2026-10-24 文档一致性检查（补齐 I15/I16/I23）：按五列固定目录统计仍为 161 个唯一 SF 条目，三列逐项证据为 158 个唯一条目，缺项恰为 I22、I24、I26；内部子范围表不重复计数。必填列无空值，本地文件链接均存在，原版 HEAD 不变且 Sources、Vendor、Package.swift 无工作区差异。补齐屏幕/历史/编辑及占位绘制证据，纠正未实现的矩形属性和字符保护表述，保留已定位索引、刷新与缓存缺口及有限验收场景。只更新两份文档；未修改程序、运行 Swift/macOS/GPU 或互通验收、构建、提交、推送。三个条目及最终反向遗漏核查仍待完成，不用证据条数换算产品完成率。

2026-10-24 文档一致性检查（补齐 I22/I26）：五列固定目录 161 个唯一条目，三列直接路径证据 160 个唯一条目，仅缺 I24；无重复编号、空必填列或失效本地文件链接。参考 HEAD 仍为固定提交，Sources、Vendor、Package.swift 无工作区差异。补齐 Kitty/OSC1337/Sixel、宿主 4 MiB 原始缓存与条带资源差异、介质/删除/刷新边界，以及 OSC133 状态、组与行身份、点击几何/仲裁、管理发送和延迟内容变化限制；全部归已有条目，没有增加功能身份。仅修改功能基线及本分析记录，未执行真实文件/共享内存介质、Swift/macOS/GPU/PTY 验收，未改程序、构建、提交或推送。I24 和最终反向遗漏核查未完成，不能认定全目标完成。

2026-10-24 最终一致性检查：补齐 I24，并完成上述生产入口反查及四个辅助文件补证。五列固定目录和三列逐项证据均为 161 个唯一 SF，无缺项/多项/重复、空必填单元格或失效本地链接；域数及分布为 S14/P10/C14/W12/T9/Y17/I27/M8/F15/O9/A10/E9/U7。607 个索引文件重新计算 SHA-256，无缺失/不匹配；参考 HEAD 为 `3f4efd9c70c82323f4e20180117c928227000580`，Sources、Vendor、Package.swift、scripts、shell-integration 相对 HEAD 无工作区改动。哈希与目录校验仅证明证据来源及文档一致性，分析完成依据仍是逐项执行路径、边界及生产入口反查。本次目标“分析固定 OShell 源码并完善细分功能清单”已完成；无待补的已发现功能条目，不将有限场景列出误称为已执行。未修改生产程序、编译便携包、提交、推送或触发 Actions，实际产品及跨端验收继续按各条要求单独进行。
