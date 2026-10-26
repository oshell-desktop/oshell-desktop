# SF-C03 / SF-I22 实现缺口修复与验证

日期：2026-10-25。仅处理[收尾清单](oshell-desktop-final-acceptance.md)中已确认的两项实现缺口，仍属于[固定基线](oshell-feature-inventory.md) `OShell-0.2.92-scope-1` 的原 161 项。OShell 参考版本保持 0.2.92 / `3f4efd9c70c82323f4e20180117c928227000580`，没有新增 SF、算法白名单、介质字段、服务或发行平台。

**结果：SF-C03 已通过；SF-I22 的 Windows 实现缺口已补齐，缺环境及实际 GPU 联验部分继续待验收。** 全量状态变为 82 已通过、0 已确认实现缺口、41 缺环境待验收、37 缺证据待验收、1 平台专属排除，共 161 项。0 已确认缺口不代表尚未验收的条目没有缺陷；其余 159 行逐项记录未改动。

工作树包含此前大量未提交修改，HEAD `62a620541680b687cedf89f8e48adcdb77576e33` 不是完整内容快照。本轮没有提交、推送、触发 Actions、发布更新或修改原版项目；共享配置 schema/合同未变。

## SF-C03：固定旧 SSH 算法

原缺口是 `aes192-cbc` / `aes256-cbc`。固定使用 `golang.org/x/crypto v0.57.0` 的本地副本，通过根 [go.mod](../go.mod) 的 replace 接入；[upstream.json](../third_party/xcrypto/upstream.json) 保留 374 个上游文件原摘要。371 个原文件完全未改，仅修改 `ssh/common.go`、`ssh/cipher.go`、`ssh/cipher_test.go`，新增 EtM 实现及测试；未变更上游许可证。维护说明见 [OSHELL_PATCH.md](../third_party/xcrypto/OSHELL_PATCH.md)。

两种算法复用既有 AES-CBC 原语，分别使用 24/32 字节密钥及既有 AES rekey 限额，仅加入不安全算法能力表，继续受应用 `LegacySSH` 和固定白名单约束；不进入默认或 SupportedAlgorithms 集合，原 FIPS 禁用分支不变。没有自建密码算法，未增加 RC4/DSA。

原生 OpenSSH 对照暴露了同条目的既有 CBC/EtM 分帧问题：仅用相同 Go 库作为两端会彼此匹配错误，真实对端则报损坏。新增 [cipher_cbc_etm.go](../third_party/xcrypto/ssh/cipher_cbc_etm.go) 按 [OpenSSH PROTOCOL §1.5](https://github.com/openssh/openssh-portable/blob/master/PROTOCOL) 处理明文长度、加密正文及 MAC，先验证序号/长度/密文的 MAC，再解密正文，继续限制报文大小/块对齐/填充。非 EtM 路径及其既有 oracle 缓解保留。这是现有固定算法的互通修复，仍归 SF-C03。

| 有限用例 | 结果与证据 |
| --- | --- |
| 默认关闭拒绝，显式开启后协商成功；固定白名单、RC4 拒绝及 RSA 限定 | [TestP1LegacySSHFixedAlgorithmsAndNegotiation](../internal/core/p1_connection_test.go) 通过，含 AES128/192/256-CBC 与 3DES |
| 应用 Dial 生产入口的两种新 CBC，多包中文/二进制数据双向一致 | [TestC03LegacyCBCPayloadAndDefaults](../internal/core/legacy_cbc_test.go) 通过 |
| 原生 OpenSSH ↔ 本地合成 SSH 端点；两种 CBC × SHA256/SHA512 EtM、SHA256/SHA1-96 | [TestC03NativeOpenSSHCBCPeer](../internal/core/legacy_cbc_test.go) 的 8 组通过；回环端点、临时空配置/固定合成主机公钥，不使用真实凭据 |
| CBC/MAC 组合、篡改密文/标签、错误序号/非法长度不先解密、非 EtM oracle 用例 | [EtM 测试](../third_party/xcrypto/ssh/cipher_cbc_etm_test.go)及扩展的 [cipher_test.go](../third_party/xcrypto/ssh/cipher_test.go)通过；整个 `./ssh` 包通过 |

针对性运行 `validation/gaps/c03-final.log` 通过，7.380 s；补丁库完整 `validation/gaps/xcrypto-ssh-final.log` 通过，7.147 s。运行环境为 Go 1.27.2 windows/amd64、OpenSSH_for_Windows_9.5p2 / LibreSSL 3.8.2。未将两端均用同一补丁库的用例单独当作原生互通证据，也未宣称完成独立 FIPS 环境验收。

## SF-I22：Windows 共享内存图片

在既有 Kitty `t=s` 上接入 Windows 会话内、由分页文件支持的命名映射。`/name`、`name`、`Local\name` 都指向 `Local\name`；发送方必须是本机可访问该映射的进程，不能据此访问远端 SSH/Linux 共享内存。不增加控制参数、RPC、后台服务或 Windows POSIX `shm_open` 模拟层。

生产实现位于 [Ghostty 补丁](../third_party/ghostty/oshell.patch)的 `graphics_image.zig` / `graphics_exec.zig`，并由 [kitty_local.go](../internal/terminal/internal/vt/kitty_local.go) 启用。Ghostty 固定提交仍为 `befcdfd2c3a1cb24d9ec886e93c95b2b5daa7028`，ABI 仍为 62。构建源码与补丁应用结果逐文件核对，DLL SHA256 为 `2aeefc8a283d1738ecbfecb83abb3d509810983c29591d9f4dc32d41c9629a1c`。

只读打开映射并查询实际 section 长度；拒绝 Global、路径/设备名空间、文件/映像及仅保留未提交的映射；沿用 S/O、尺寸和 400 MiB 限额。复制为接收方拥有的数据后，立即释放自身视图/句柄，再走既有解码、图片缓存和绘制流程。映射数据不作为缓存的借用指针。

**明确的平台机制差异：**Windows 没有 POSIX `shm_unlink`。发送方在收到回复后关闭自己的句柄；最后一个句柄/视图关闭时对象消失，接收方不关闭其他进程的句柄。原 POSIX 分支仍按既有规则 unlink。[Windows 命名共享内存](https://learn.microsoft.com/en-us/windows/win32/memory/creating-named-shared-memory)是同一图片介质的平台适配，不以“接收方返回 OK”代替像素及释放验收；剩余真实原版对照仍保留。

| 有限用例 | 结果与证据 |
| --- | --- |
| 真正独立子进程创建映射、父进程经原生 VT 读取、像素一致、生产者退出后对象释放 | [TestI22WindowsSharedMemoryCrossProcess](../internal/terminal/internal/vt/kitty_shared_windows_test.go) 通过；Child 是仅供父测试启动的夹具入口，普通回归中的 skip 不是跨进程测试跳过 |
| 三种名字写法 × RGB/RGBA/PNG；读取后缓存拥有数据；删除与句柄/视图释放 | 同文件 `TestI22WindowsSharedMemoryPixelsAndLifetime` 通过 |
| 正偏移 PNG、超大 S 钳制；负/极大 O/S、短数据、非法格式/尺寸/压缩、超过 400 MiB、文件/保留映射、缺失名字/非法路径 | 同文件 `EncodedOffset` / `BoundsAndTypes` 及 [非法载荷拒绝](../internal/terminal/internal/vt/kitty_local_test.go)通过；失败不破坏原图、不泄漏映射 |
| 终端绘制实际红色像素，同 ID 绿色替换、删除；生产者关闭后仍可绘制 | [TestI22WindowsSharedMemoryActuallyDraws](../internal/terminal/shared_media_windows_test.go) 通过，使用 UI.Tester 软件图，非硬件 GPU 截屏 |
| 既有直接/文件/临时文件及临时清理不回归 | `TestP2I22LocalMediaActuallyDraws`、VT 全包及终端整包通过 |
| 最终单文件实际内嵌 DLL 的共享图片像素/释放及文件/临时文件 | [TestSingleFileResourcesLoad](../internal/terminal/single_file_windows_test.go) 构建阶段通过，2.909 s；不是仅验证外置开发 DLL |

VT 专项日志 `validation/gaps/shared-memory-final.log`，1.056 s；软件绘制专项见 `focused-final.log` 中 terminal 结果（其中早期 core 失败已由 `c03-final.log` 修复后的结果取代）。合成映射工具在 `internal/testsupport/winshm`，仅由测试引用。

**待验收：**Linux 原生四介质与清理、macOS 固定原版的介质/GPU 对照缺少环境；Windows 实际终端窗口 GPU/软件交互联验尚缺证据，不能伪装成环境不存在。使用固定小图分别核对像素、尺寸/位置/滚动、同 ID 替换/删除以及不存在/越界拒绝，记录 POSIX unlink 与 Windows 最后句柄关闭的生命周期差异。Desktop 发行目标保持 Windows/Linux；不新增 macOS Desktop 包或新终端协议。

## 本机回归与构建记录

本轮使用新 DLL，并设置 `OSHELL_REQUIRE_WIDTH_EXTENSION=1` 执行核心、桌面及全部终端子包回归。日志保存在被忽略的 `validation/gaps`；这些结果不自动关闭其他 SF 的真实交互/平台要求。

| 验证 | 结果 |
| --- | --- |
| core | 通过，298.972 s |
| desktop | 通过，80.731 s |
| vt / pty / bidi / bidilayout | 通过，分别 10.755 / 4.532 / 1.956 / 1.760 s |
| terminal | 并行回归有一次既有 URL 正则预算失败；单独定向复测及完整终端包复测通过，分别 1.179 / 17.361 s |
| `go vet ./internal/core ./internal/desktop ./internal/terminal/...` | 通过 |
| 单文件构建中的资源 / 存储激活测试 | 分别 2.909 / 1.038 s 通过 |
| 实际新 ZIP 的签名验证与准备 | 1.136 s 通过 |

保留并区分两次验证问题：

- `regression.jsonl` 中 `TestImplicitLinkSourcePattern` 的长点号 URL 样例在并行运行中触发 0.20 s 预算失败；未修改该生产逻辑、测试或预算，`terminal-link-diagnostic.log` 定向及 `terminal-final.jsonl` 整包复测通过。初次全量命令退出码为 1，不能写成第一次全量无失败，也不把一次复测当作消除所有时序波动。
- `portable-build-initial.log` 中新增内嵌共享内存用例漏设 cell 像素尺寸，收到了 OK 但无绘制区域。补齐与同文件既有文件介质一致的 `Resize(12,5,8,16)` 前置条件后重新构建；原像素/资源断言保留，最终 `portable-build.log` 通过。固定 dist 只在最终测试通过后替换。

重验入口（PowerShell，需项目既有本机工具链；本地日志目录不属于交付边界）：

```powershell
go test ./internal/core -run 'Test(P1LegacySSHFixedAlgorithmsAndNegotiation|C03)' -count=1
# 在 third_party/xcrypto 目录运行：
go test ./ssh -count=1
# 回到仓库根目录，MYGO_GHOSTTY_VT 指向按当前补丁构建的原生 DLL：
$env:OSHELL_REQUIRE_WIDTH_EXTENSION='1'
go test ./internal/terminal/internal/vt -run 'Test(I22WindowsSharedMemory|KittyWindowsSharedMemory)' -count=1
go test ./internal/terminal -run 'Test(I22WindowsSharedMemoryActuallyDraws|P2I22LocalMediaActuallyDraws)' -count=1
go test ./internal/core ./internal/desktop ./internal/terminal/... -count=1 -timeout=8m
go vet ./internal/core ./internal/desktop ./internal/terminal/...
python scripts/build.py --target windows/amd64 --single-file --archive --out validation/portable-build
```

真实 Swift 双端、Linux、真实 GPU/输入场景按[收尾清单](oshell-desktop-final-acceptance.md)对应条目继续验收。测试 skip 不算通过，不通过提交/Actions 代替本机环境。

## 固定便携包与范围完整性

版本保持 0.3.3，更新固定 [EXE](../dist/windows-amd64-single-file/OShell%20Desktop.exe)、[ZIP](../dist/OShell-Desktop-windows-amd64-single-file.zip) 和 [SHA256SUMS.txt](../dist/SHA256SUMS.txt)，未新建生产输出目录。

| 产物 | 字节 | SHA256 |
| --- | ---: | --- |
| EXE | 30,763,520 | `0972f7427713eeebb8e7b95cbd30bb5948ff42367e0c2917b6842fad698cd11a` |
| ZIP | 14,246,023 | `8eb5afa5266afedae98c4bd71aa74ee8b04e2ab90f0b68a4f83667ff5a45d670` |

`package-integrity.json` 确认 ZIP CRC、仅一份 EXE、内外 EXE 摘要一致。`signing-key-verification.json` 确认既有授权密钥对匹配、EXE 内置公钥且排除私钥；未生成替代密钥、输出秘密或部署发行源。`signed-package.log` 使用本地合成签名对这份实际 ZIP 运行生产准备路径。

`portable-smoke.json` 记录独立目录启动、GUI 就绪、内嵌 DLL 摘要一致、无旁置 DLL、零窗口存活、外部入口恢复单实例、测试进程事件循环正常退出（码 0）。该启动检查不替代真实 GPU/图片交互验收。`dist-replacement.json` 确认只替换程序产物，原有 **11 个配置/数据文件集合及 SHA256 全部不变**，未结束用户会话。

`checklist-verification.json` 确认全量 CSV 仍为原 161 个 ID/名称/域/验收要求，仅 C03/I22 状态及证据更新，其余 159 行完全不变；43 项处置 CSV 仅这两行验收状态变化，原分类与 P1/P2/P3=9/18/3、保留项 13 不变。

固定基线 SHA256 仍为 `0fe41373faccf99453302fa6242ab5aaa27e7e084d03462eae3683ae810a30b1`；历史差异 CSV 仍为 `b580dc8b3071f60c5d0f1ba30d42f5d8c527777cb9de76ac904f4f4d1d345edc`。前批报告保留历史缺口与旧包摘要，并加后续结果链接；本报告和当前收尾清单为这两项的最新结论。
