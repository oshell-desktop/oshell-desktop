# Mac 同步兼容修复交接

2026-11-27。对照 Mac OShell `b72fc2797f1d6f7636b08ef83d2b11e7a4e4a517`（`master`，Add oTerm and two-stage bastion SSH login）。本次只修改 Desktop；Mac 工作区的现有改动保留，由 Mac 端合并并验证。

## 原因及 Desktop 修复

Mac 将新的 `SessionProfile.sshAuthenticationMethods` 写入共享配置，旧 Desktop 合同不认识该字段，因而报 `configuration.profiles[].[unknown]`。使用合成 v2 配置已复现相同错误。Desktop 已将字段加入统一合同和 Profile 模型，保存、编辑、同步、归档保留顺序和原有凭据绑定；连接按已选方式认证，禁止回退到列表外的方式。

字段契约如下，Mac 当前模型已有对应实现：

| 项目 | 约定 |
| --- | --- |
| 路径 | `configuration.profiles[].sshAuthenticationMethods`；归档的 `profiles[]` 相同 |
| 类型 | 可选字符串数组，1–4 项，不允许重复，仅 SSH / SFTP |
| 值 | `password`、`publickey`、`keyboard-interactive`、`gssapi-with-mic` |
| 默认 | 缺失/null 表示使用该端现有自动策略，写入省略；禁止把空数组当默认 |
| 顺序 | 保留输入顺序，用于认证优先级；不得排序或补齐全部方式 |
| 版本 | 共享信封读 v1/v2、写 v2；不改变归档版本或密码身份绑定 |

Desktop 尚未实现 GSSAPI/Kerberos 认证。该枚举可保存、同步及导出；其他已选方式可用时仍可连接，只有 GSSAPI 可用时明确报错。此限制不允许通过丢字段、自动增加密码认证或禁用校验来规避。

## Mac 必须同步修改

1. **以 Desktop JSON 为唯一合同源重新生成 Swift 定义。** 当前 Mac 提交只加入了认证字段，缺少 Desktop 的外观扩展。不要用 Mac 的旧合同反向覆盖 Desktop，也不要单独修改生成文件里的 SHA-256。

   ```sh
   cd /path/to/oshell-desktop
   python3 scripts/sync-configuration-contract.py --native-source /path/to/OShell
   python3 scripts/sync-configuration-contract.py --native-source /path/to/OShell --check
   ```

   本轮合同 SHA-256：`47ad292255a2f94ab8acb561c589ff0aa2845994e699c6ff73b45c49e9b52163`。实际交接时以当前脚本输出为准。

2. **补齐 `Preferences.terminalAppearance` 的模型与编码。** 类型是可选 `TerminalAppearance` 对象，缺失时省略。涵盖四边距、行列间距、字重、东亚字体及字号、粗体呈现、光标形状与闪烁、字体质量、标签色和背景图位置。精确键名、类型、枚举、默认值和范围直接取 `shared_configuration_schema.json` 的 `terminalAppearance` 定义。修改 `Sources/OShellCore/Models.swift` 的 `Preferences` 属性、CodingKeys、decode/encode；补入 `TerminalAppearance.swift` 及模型验证。若此次暂不接入绘制，也必须无损保留这些已登记值，不能在重新编码时消失。背景图片文件及本机路径不进入共享配置。

3. **补齐 `TerminalColorScheme.boldForeground`。** 合同中的 `palette.boldForeground` 是可选 `#RRGGBB` 颜色字符串，无默认值；缺失时省略。修改 `Sources/OShellCore/TerminalColorScheme.swift` 的属性、CodingKeys、decode/encode 和必要的复制/比较路径，保留显式值。不要用普通 `foreground` 覆盖它。

   既有参考实现保存在 Desktop 的 `scripts/patches/oshell-v0.6.7-terminal-appearance.patch`。该补丁基于 v0.6.7，包含完整外观 UI/渲染改动；请针对最新 Mac 源码择取所需模型及编码实现，保留 `b72fc27` 的认证字段，再重新生成合同。不要整文件覆盖最新 `Models.swift`，也不要直接覆盖当前 Mac 工作区未提交的外观改动。

4. **保持认证列表和本机状态边界。** 保留 Mac 已有的认证列表校验、OpenSSH 参数及顺序；新增合同不能替代模型的重复项及会话类型校验。`runtimeSSHMode`、`runtimeProxyRoute`、`_localSSHSessionModes`、`_localTargetPasswordRevisions` 和目标服务器密码继续留在 Mac 本机，不能为修复共享协议加入共享正文。代理的 `sshAuthentication` 与会话的 `sshAuthenticationMethods` 是不同字段，不相互替换。

## 验证及交付条件

Desktop 已扩充 `scripts/interop/main.swift` 与 `internal/core/interop_test.go`：Swift 生成认证列表，Go 解码/回写，Swift 再读取；完整字段测试同时保留认证顺序、外观扩展和粗体色，覆盖非法认证值/类型/空数组/重复项拒绝及归档往返。旧合成 Swift v1 样本继续验证旧数据读取。

Mac 合并后使用更新后的 Desktop 测试脚本运行：

```sh
cd /path/to/oshell-desktop
python3 scripts/sync-configuration-contract.py --native-source /path/to/OShell --check
go test ./internal/core -run 'Test(NativeSwiftSharedFixture|SharedQuickSendScope|LegacySharedProtocol|SharedEncodingNormalization|SharedSSHAuthenticationMethods|SSHAuthenticationMethods)'
python3 scripts/test-sync-interop.py --native-source /path/to/OShell
```

有限验收需确认：

- 四种认证枚举及其顺序在 Swift → Go → Swift 与 Go → Swift → Go 中保留；未指定字段仍省略。已启用扩展外观和 `boldForeground` 的文件也能往返。
- 会话/代理密码仍可解密，UUID、代理和快捷引用不变；目录与 WebDAV 多节点同步无修改不上传，认证设置双边修改产生冲突而非覆盖。
- 未知字段/未知认证枚举、错误类型、空数组、重复项和非 SSH 会话认证列表拒绝应用；保留双方有效数据。Mac 本机模式和目标密码不进入共享正文。
- Mac 的认证偏好仍按顺序约束实际连接；Desktop 验证密码、公钥、键盘交互及跳板隔离，GSSAPI 的平台限制按上文处理。

本轮 Windows 验证和静态生成合同检查不等于真实 Swift ↔ Go 验收。Mac 完成上述命令并记录双方提交号、合同摘要、结果后，才能确认双端兼容完成。不要改主密码、删除远端文件或关闭未知字段校验来恢复同步。
