# OShell 统一配置协议 v2

状态：两端源码已实现；旧版本兼容入口保留。同步范围更新于 2026-10-23，对齐 OShell 0.2.92 `3f4efd9`。

本文件是当前协议入口。字段的唯一可执行定义是 [shared_configuration_schema.json](../internal/core/shared_configuration_schema.json)，根对象 `configuration`；会话导入导出根对象为 `archive`。它是项目自有的字段合同，包含类型、必需性、默认值、枚举、范围和子对象引用，不是通用 JSON Schema 引擎。

Go 通过 embed 直接使用该文件；Swift 的 `SharedConfigurationContract.swift` 由同一文件生成。必须通过 `--check` 验证完全相同，禁止手工维护第二套定义。

## 1. 范围与版本

两端的共享配置和会话导入导出使用相同字段协议。本机缓存、凭据库的内部布局仍独立；不得同时写同一本机数据库。功能在不同系统上的渲染或执行能力可以不同，但不支持的本机能力不能成为丢弃已登记配置字段的理由。

**同步范围与完整字段合同不同。** 默认比较、合并和应用会话、目录、快捷链接（不含显隐）、代理及加密密码；本机策略可另选客户端偏好与高亮、快速命令和默认会话属性。未选类别仍严格校验、保留本次读取的远端值，不应用到本机；新目标写兼容默认值。同步范围不新增共享字段，改变范围后重新确认。更新来源、安全确认、平台专属状态始终留在本机；导入导出、备份及完整文档编解码能力保持不变。

WebDAV 目录发现对齐 OShell `e29a03c`：优先沿用原地址中的共享文件，否则使用或创建 `oshell` 子目录；显式 `oshell` 地址不再嵌套。PROPFIND 须确认目标 href 的成功 DAV collection 属性，MKCOL 返回 201 / 405 后仍需复查。地址及凭据身份不重写，共享文件名、加密格式和条件写入规则不变。

| 项目 | 当前规则 |
| --- | --- |
| 共享文件 | WebDAV `oshell-vault.json`；目录同步 `configuration.json` |
| 外层 format | `OShell.shared-vault` |
| 新配置写入 | `version=2` |
| 兼容读取 | `version=1`、`version=2`；其他版本拒绝 |
| v2 AAD | UTF-8 `OShell.shared-vault.v2` |
| v1 AAD | 保留 UTF-8 `OShell.shared-vault.v1` |
| 密钥派生及密文 | PBKDF2-HMAC-SHA256 / AES-256-GCM，参数与 v1 相同 |
| 内层会话密码 | 继续使用 EncryptedPassword v1 和原有身份绑定 AAD |
| 会话导入导出 | `OShell.sessions` 版本 1–4，使用同一会话/代理/引用字段定义，不是共享信封版本 |

加密大小限制、身份绑定公式、WebDAV ETag 条件写入及整会话三方合并规则沿用 [v1 历史规范](configuration-protocol-v1.md) 第 1、2（除版本和 AAD）、6、7 节。v2 的字段、默认值和未知字段行为以当前可执行合同及本文为准。

`SealShared` / `SharedVault.seal` 也用于应用内部加密容器，这些内部容器可以继续采用 v1。**对外共享 Configuration 的 encode 入口必须写 v2**，不能用内部通用加密函数替代。

v1 客户端会拒绝 v2，防止旧代码读取新字段后又静默丢弃并覆盖。请先升级全部参与同一目标的客户端，再启用新版同步；旧节点不会自动获得新能力。迁移仍使用原主密码和原文件地址，不重建用户身份。

## 2. 完整字段与默认值

字段合同覆盖：

- Configuration、SessionProfile、历史 FTPProfile、ProxyProfile、ProxySettings、TunnelRule、KeepAliveSettings。
- SessionLinks、SessionLink、SessionDefaults、QuickCommand、HighlightSet、HighlightRule。
- Preferences、自定义配色、快捷键与覆盖项、EncryptedPassword 和已解析连接身份。
- SessionArchive 的 format/version、会话、目录、链接与代理。

两端在解码为语言模型前执行相同的字段验证和默认值补全，写入也通过同一合同。数组为空写 `[]`；缺失的可选字段按合同补全；没有默认值的可选 null 字段移除。`highlightSetID=null` 和快捷键禁用项的 null 保留对应语义。

兼容迁移规则在两端一致：快速发送范围旧字符串转 0–6 整数；历史 `activeHostProbe` 转 `titleMode`；缺失 FTP 初始路径用 `/`；缺失且禁用的代理使用由会话 ID 派生的稳定 ID。该 ID 使用 SHA-256(`OShell absent proxy v1\n` + 大写会话 UUID) 的前 16 字节，设置版本位为 5、变体位为 RFC 4122，不使用随机数。

双方均保存以下完整字段，不得只保留其中一部分：

| 对象 | 补齐字段 / 行为 |
| --- | --- |
| 自定义配色 | `autoCursor: boolean`，缺失默认 false |
| HighlightRule | `name: string`、`bold: boolean`、`foreground: string`、`background: string`；默认分别为空、false、空、空 |
| 字号 | 两端均用双精度存储 `fontSize`；仅绘制接口允许转换为单精度，不能回写降精度值 |
| 高亮颜色 | 保留原 `color` 和明确设置的 `foreground`；有效前景优先 foreground，不能在同步时擅自抹掉二者之一 |
| 规则颜色 | color 支持 `#RRGGBB` / `#RRGGBBAA`，foreground/background 另允许空值 |
| 默认高亮规则 | 三条标准规则使用固定 UUID，不能因新建默认对象而随机改变 |

已登记的远端旧设置保留在兼容载荷中回写，不作为本机设置应用，也不参与同步冲突。完整文档导入仍遵循平台校验：例如 Command/Control 在 Windows 上归一化后发生冲突时，Desktop 禁止执行歧义绑定，设置编辑器提示冲突。

## 3. 未知或不合法数据

两端均采用**拒绝写入不理解的数据**的策略，而非假设可以无损保留任意未知字段：

1. 未登记顶层或嵌套字段、未知枚举、错误类型或合同范围外数值，在转换成语言模型前报错。
2. 不修改本机配置、凭据库或远端对象；保留原文件供升级或人工处理。
3. 错误只报告协议路径，不输出字段值、密文、密码或未经处理的未知字段名。
4. 快捷键动作名使用合同中的共同登记表，包含两个 desktop 终端编辑动作；平台尚不能执行的已登记动作仍保留。未登记动作拒绝读取，其值也严格遵守相同结构。
5. 未来增加字段时必须更新合同、两端模型及测试。未升级的 v2 客户端会拒绝未知字段，不会重写成少字段的文件。

未知字段保留、能力协商和自动跨版本降级并未实现，也不是默认行为。不得为了打开未来文件而绕过校验后保存。

## 4. 迁移和同步一致性

新版读取 v1 后规范化为统一模型，后续共享保存写 v2。Desktop 会对已有 v1 目标执行一次带 ETag 保护的升级；macOS 共享保存写 v2。外层版本属于认证 AAD，修改 version 不能让 v2 以 v1 身份通过解密。

多节点同步继续保留首次接入确认、备份、共同基准、整会话冲突、远端删除保护和条件写入。编码补全不应变成业务冲突。不同会话的独立修改可合并，同一会话冲突必须确认，未解决时不能上传。

同一逻辑配置的新加密信封不必逐字节相同，因为盐和 nonce 随机；“协议一致”指字段语义、默认值、验证规则及加解密身份约束一致。无逻辑变更不应重复上传。

## 5. 必须通过的验证

在 oshell-desktop 目录执行：

```sh
# 校验生成的 Swift 合同未漂移
python3 scripts/sync-configuration-contract.py --native-source /path/to/OShell --check

# 真实模型、双向加解密、传输、字段及拒绝行为
python3 scripts/test-sync-interop.py --native-source /path/to/OShell

# 所有平台常规回归
go test ./...
```

修改合同后先生成 Swift 定义，再测试：

```sh
python3 scripts/sync-configuration-contract.py --native-source /path/to/OShell
```

验收覆盖：v1 读取、v2 写入、版本篡改失败、两端字段合同一致、精度和扩展字段连续往返三轮、未知字段拒绝、旧 scope 迁移、密码与代理引用、WebDAV / 共享目录多节点合并、冲突和无变化不重写。缺少真实 Swift helper 时的跳过不能视为通过。

发布必须同时提供两端对应版本，并记录两个 Git 提交号、合同 SHA-256 和测试结果。未提交或未打包的源码修改不等于用户已安装版本具备 v2 能力。
