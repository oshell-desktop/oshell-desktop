# P1 九项修复与本机验证记录

> 后续更新：本文保留该批次的历史结果及包摘要。SF-C03 的两种 CBC 与 SF-I22 的 Windows 命名共享内存实现缺口已在[后续修复](oshell-desktop-gap-closure-validation.md)补齐；当前状态/待验环境见[收尾清单](oshell-desktop-final-acceptance.md)，最新便携包见后续修复报告。

日期：2026-10-25。范围仍为 [固定功能基线](oshell-feature-inventory.md) `OShell-0.2.92-scope-1`，原版 OShell 0.2.92 提交 `3f4efd9c70c82323f4e20180117c928227000580`。本轮仅落实 [修复清单](oshell-desktop-remediation-plan.md) 的九个 P1 条目，不增加 SF 编号，不把用例数量算作功能进度，不提前实施 P2/P3。

九项代码修复和本机回归已完成。**这不等于九项在所有平台完全等价：SF-C03 保留两种 CBC 算法的库能力缺口，SF-S07 的 Linux/macOS 原生验收及真实 Swift 双端互通仍待环境。** 更新验证使用本地模拟响应、合成签名及实际 Windows ZIP，没有发布元数据、运行 Actions 或安装远端更新。工作树包含本轮开始前的未提交改动；没有提交、推送或修改原版项目。

## 逐项结果

| 原条目 | 本轮修复 | 有限验证与证据 | 当前结论 |
| --- | --- | --- | --- |
| SF-I27 宿主限制 | 生产终端的 OSC52 剪贴板回调直接拒绝；保留用户复制/粘贴路径。 | [生产终端测试](../internal/terminal/p1_host_clipboard_test.go) 使用实际终端库，覆盖写入、查询、分块及不同选择目标；剪贴板不变，主动粘贴的写入屏障证明之前无查询回复。终端整包回归通过。 | 本机有限用例通过。 |
| SF-P07 一次性凭据 | [提示识别](../internal/core/password_prompt.go) 仅接受原版三个身份化提示；非单一隐藏问题的挑战全由用户回答，不自动填入或保存。 | 提示匹配测试覆盖正确/错误身份、通用提示和混合问题；[连接测试](../internal/core/p1_connection_test.go) 及 Desktop 认证测试覆盖已存密码、混合 MFA、失败重试、手动回答及保存规则。 | 本机有限用例通过。 |
| SF-P05 成功后保存 | [保存规则](../internal/desktop/authenticated_password.go)、连接入口和弹窗共同禁止外部来源保存；主密码缓存不可用时跳过保存，不嵌套询问解锁。普通 FTP/SFTP 显式保存保留。 | [新增生产入口测试](../internal/desktop/p1_credentials_clipboard_test.go) 验证外部 SSH 无密码、密码失败再询问及普通入口对照；即使残留保存标志也不能写入。既有认证、FTP/SFTP、身份及库代次测试随 Desktop 全量回归通过。 | 本次禁存缺口通过；SF-P08 的缓存/工作区状态拆分仍留在 P2。 |
| SF-C06 代理类型 | SOCKS5 有凭据时只宣告用户名/密码方法，拒绝无认证降级；[HTTP 头读取](../internal/core/proxy_header.go) 限制为 16 KiB，保留已缓冲的隧道字节。 | [连接测试](../internal/core/p1_connection_test.go) 覆盖无凭据/有凭据握手、拒绝降级、HTTP 精确边界/超限及隧道数据；既有四种代理本地服务器回归通过。 | 本机有限用例通过。 |
| SF-C03 旧 SSH 兼容 | [固定白名单映射](../internal/core/legacy_ssh.go) 替代合并全部不安全算法；RSA SHA1 认证只在兼容开关打开时允许。 | 核对固定算法与库支持集合；合成 SSH 端点验证 AES128-CBC 默认拒绝/开启后成功，RC4 始终拒绝，RSA 签名受开关约束。 | 白名单行为修正通过；`aes192-cbc`、`aes256-cbc` 不受当前 `x/crypto v0.57.0` 支持，明确报错并保留缺口，未宣称完全等价。 |
| SF-S07 配置复制 | [配置快照](../internal/desktop/manager_clipboard.go) 同时绑定系统剪贴板代次与既有身份/密码库代次；读取前后再次核对，无法取得可靠代次时拒绝使用。 | Desktop 测试覆盖覆盖后恢复旧令牌（期间不读取）、重新复制，以及既有跨窗/重绑定/代次用例；[原生 Windows 测试](../third_party/mygo/internal/e2e/clipboard_generation_test.go) 实测覆盖再恢复会改变代次。 | Windows 原生验证通过；Linux/macOS 适配已接入但原生环境待验。 |
| SF-E06 容器边界 | [ZIP 校验](../internal/core/vendor_zip.go) 恢复 4000 项/每项展开 2 MiB，拒绝 ZIP64，校验中央与本地头、长度及 CRC；目录遍历恢复 8000 项，包含目录。 | [固定容器样例](../internal/core/p1_vendor_test.go) 覆盖 stored/deflate、4000/4001、2 MiB/超限、8000/8001、ZIP64、头不一致、目录/文件 CRC、加密、分卷及特殊文件；既有导入保护回归通过。 | 本机有限用例通过；SF-E03 空目录保留仍属 P2。 |
| SF-U02 更新检查 | [生产更新入口](../internal/core/update_portable_signed.go) 接到静态 `/updates/latest.json`；公开入口不调用 GitHub API，沿用缓存与合并。私有 token 增强仍用 API，但必须取得同格式签名附件。 | [生产入口测试](../internal/core/p1_update_test.go) 验证有/无更新、8 次并发合并为 1 次请求、静态 URL、平台绑定、超限、重定向及私有附件；既有模拟时钟缓存测试通过。 | 本机链路通过；真实静态站点未部署、未作线上发行验收。 |
| SF-U03 原版更新链路 | 内置固定 Ed25519 公钥；元数据验签、产品/版本/平台/资产绑定及包验签接入实际 `ReleaseUpdate`；未验签不能进入解包准备。 | 合成密钥覆盖错误公钥、缺签名、元数据错误和资产不匹配；[实际便携 ZIP 测试](../internal/core/p1_update_package_test.go) 走 Check→Prepare，检查不下载，合法包解包成功，篡改包在解包前拒绝。既有下载失败/取消清理及 UI 回归通过。 | 本机签名链路通过；SF-U04 安装生命周期联验仍属 P2。 |

SF-C03 的保留缺口会影响只能接受 AES192-CBC/AES256-CBC 的旧设备；不能用其他可连接算法替代其支持证明。按原 P1 验收要求已逐项映射并明确不支持项，本轮没有扩大算法集合或引入新的密码库实现。

## 回归与环境

本机 Windows amd64，Go 1.27.2；所有服务器、挑战、导入及同步样本使用合成数据或临时目录。共享配置 schema 没有新增字段。

| 验证 | 结果 | 本机日志 |
| --- | --- | --- |
| `go test ./internal/core -count=1 -timeout=8m` | 通过，291.083 秒 | `validation/p1/core-full.log` |
| `go test ./internal/desktop -count=1 -timeout=8m` | 最终通过，71.195 秒 | `validation/p1/desktop-full-final.log` |
| `go test ./internal/terminal -count=1 -timeout=8m` | 通过，17.153 秒；在合并调用中执行 | `validation/p1/desktop-terminal-full.log` 中 terminal 结果 |
| `go vet ./internal/core ./internal/desktop ./internal/terminal` | 通过 | 本机命令退出码 0 |
| AGENTS.md 指定四组 Go 共享兼容样本 | 全部通过，4.477 秒 | `validation/p1/shared-compatibility.log` |
| `MYGO_E2E=1` 原生 Windows 剪贴板代次测试 | 通过，未跳过 | `validation/p1/native-clipboard.log` |
| 内置正式公钥后的更新定向回归 | 通过 | `validation/p1/pinned-key-update.log` |
| 最终实际 ZIP 的签名准备测试 | 通过，1.337 秒 | `validation/p1/signed-package-final.log` |
| 本机单文件构建、内嵌资源与存储激活测试 | 通过 | `validation/p1/build-pinned-key.log` |

最初合并 Desktop/terminal 回归中，两处旧测试仍期待通用混合挑战自动使用密码或旧更新 UI 文案；按本轮固定行为调整后 Desktop 全量重新通过。该旧日志中的 Desktop 失败不能当作最终结果，terminal 通过结果独立保留。

未运行 `scripts/test-sync-interop.py --native-source ...`：此项需要 macOS 和真实 Swift 运行环境。Go 兼容样本通过不等于真实 Swift ↔ Go 互通通过。Linux/macOS 原生剪贴板和 Linux 便携运行也未在本机执行，不以源码存在代替平台通过。

## 更新签名密钥

经用户明确要求，本机生成 Ed25519 更新签名密钥对，不是 TLS 证书。程序通过 `go:embed` 固定内置 [Base64 公钥](../internal/core/update_public_key.txt)，更新服务器不能选择信任公钥。公钥原始字节的 SHA256 指纹：

`06d75df60e575c8e7f1cc01bf52e5c781b8a3a3444bebdd5fd2829b590fb749a`

私钥保存在本机 `.release-private/update-signing/ed25519-private.pem`（PKCS8 PEM）；该目录已被 Git 忽略，ACL 仅允许当前用户访问。私钥没有写入日志、测试夹具、文档或便携包。内存中的随机挑战签名验证了密钥配对；最终 EXE 检查确认公钥存在，私钥 PEM、DER 与种子不在程序中。对应非秘密检查结果：`validation/p1/signing-key-verification.json`。

后续发布更新需使用这把私钥签署元数据及包摘要，并妥善备份私钥。本轮没有上传私钥、发布包或触发工作流；在线端尚未提供匹配的签名元数据时，客户端会拒绝未经验证的更新。

## 固定便携包

本节是 P1 当时的产物记录；固定 dist 后续已由 P2 更新，当前包及摘要见 [P2 验证记录](oshell-desktop-p2-validation.md)。

版本仍为 **0.3.3**，产物包含当前工作树与本轮修复；没有用 HEAD 冒充完整构建快照。

| 文件 | 字节数 | SHA256 |
| --- | ---: | --- |
| [OShell Desktop.exe](../dist/windows-amd64-single-file/OShell%20Desktop.exe) | 30639104 | `46d5d1d7ac4c019f431768f24238338381d92c9b46a52b2c6ee013e141010c0a` |
| [Windows 单文件 ZIP](../dist/OShell-Desktop-windows-amd64-single-file.zip) | 14212316 | `fc3de6e7d7aebd8fe8c01bac23f3c974e5859309c295f12c219760c72f40e187` |

ZIP 完整性/CRC 通过，只包含单个 EXE，解包内容与 EXE 摘要一致。独立验证目录仅复制 EXE，启动 GUI 成功，加载内嵌终端库，正常关闭退出码 0；没有依赖旁置 DLL 或外部数据目录。固定 EXE 替换时未被占用，未强制结束任何用户会话。替换只涉及程序、ZIP 与摘要清单，原有 **11 个配置及数据文件逐个 SHA256 均未改变**。

非秘密构建与启动结果保存在本机忽略目录 `validation/p1/`；无需将这些本机日志当作新增功能条目。原 161 项基线及输入差异 CSV 未改写，43 项分类、9/18/3 项优先级及 13 项保留项数量不变；当前状态以修复清单 CSV 的“验收状态”和本记录为准。
