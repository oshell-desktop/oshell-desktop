# 2026-10-26 流水线失败诊断与本地修复

针对 [Build and test #40288151755](https://github.com/oshell-desktop/oshell-desktop/actions/runs/40288151755)，提交 `9b0c87583867b80d4c403a20fdb5a4c5f3175195`。仅修复该次已出现的问题，功能归属保持原 `OShell-0.2.92-scope-1` 的 SF-I18、SF-I22；密码同步测试的运行预算属于既有验证基础设施，不新增功能。

## 失败原因与修复

| 任务 / 归属 | 实际失败证据 | 本地修复 |
| --- | --- | --- |
| package / SF-I22 | `go vet ./...` 报 `internal/testsupport/winshm/shared_windows.go:52:29: possible misuse of unsafe.Pointer` | 测试辅助代码使用 `WriteProcessMemory` 向当前进程的原生映射写入，并检查写入字节数；不再把保存的 uintptr 转成 Go 指针。不关闭 vet、不增加忽略规则。 |
| linux / 回归运行预算 | core 在整包 180 秒时被终止；当时正在 `TestSyncSelectedCategoriesPreserveBothSides/8` 的 600,000 轮 PBKDF2。未见死锁证据；此前完整 core 本机实测 261.397 秒。 | 普通完整回归统一使用有限的 8 分钟包级预算；Windows 原为 5 分钟、Linux 原为 3 分钟。竞态分片及作业总时限不变，不降低密码派生强度。Linux 是否足够仍需实际复验。 |
| linux / SF-I18 | `TestDeferredLayoutAltGrAndNoCommit`、`TestDeferredLayoutUnmappedKeysDoNotShareReleaseState` 期待 Windows Ctrl+Alt/WM_CHAR 行为，却被 Linux 选中 | 原文件名 `keyboard_windows_layout_test.go` 不满足 Go 的 `_windows_test.go` 平台后缀。两个 Windows 特有用例移到 `keyboard_deferred_windows_test.go`；通用延迟文本矩阵保留在跨平台 `keyboard_layout_test.go`。没有删除用例或修改 Linux 键盘生产语义。 |
| linux / SF-I22 | `TestKittyPOSIXSharedMemoryMedia` 的合法共享内存返回 ENOTSUP，且未执行对应清理 | 固定运行库关闭 SIMD 后未链接 libc，导致 `builtin.link_libc` 为 false，POSIX 路径被编译掉。补丁为 Linux 显式链接 libc，并为原 glibc 2.28 目标链接 librt；保留原 shm_open/unlink、范围检查和清理逻辑。Windows 链接条件不变，ABI 仍为 62。 |

Linux 四个 core-race 分片全部通过。Windows 普通/竞态任务因 package 失败被跳过；最终 core-race-audit 的 `Missing or extra shard reports` 是缺少 Windows 报告的连带失败。保持审计要求，不通过放宽报告数量让其变绿。Release 被跳过；此次没有创建标签。

## 已执行的本地验证

系统：Windows 11 amd64，Go 1.27.2，固定 Zig 0.16.0。原流水线为 Linux Go 1.27.1。没有升级固定 Ghostty 提交 `befcdfd2c3a1cb24d9ec886e93c95b2b5daa7028`。

- `go vet . ./internal/...` 通过，覆盖根程序和全部应用包，包括此前漏掉的 `internal/testsupport/winshm`。本地 `go vet ./...` 首次被不入库的 validation 历史源码快照干扰，因此显式选择全部应用包；CI 干净 checkout 继续使用 `go vet ./...`。
- Windows `TestDeferredLayout*`、`TestWindowsAltGr*`、`TestI22WindowsSharedMemory*` 定向通过，包含跨进程生产者、实际像素、偏移、边界、类型和释放；terminal 1.400 秒，VT 1.181 秒。使用上轮已验证 DLL，Windows 生产逻辑未改；新包运行库另行验证。
- 构建脚本测试 `scripts/test-ci-bundle.py` 4 项、`scripts/test-terminal-runtime.py` 1 项通过。首次沙箱执行因临时目录权限失败，原生工具链复验通过，未修改测试断言。
- 使用 `go list -json` 分别选择 Windows/Linux 测试源，确认通用延迟文本矩阵两端均在，两个 Windows AltGr 用例只被 Windows 选中。
- 修复后 Linux 运行库已在本机交叉编译成功；ELF 明确依赖 `libc.so.6`、`librt.so.1`，导入 `shm_open`、`shm_unlink`。SHA256：`e321282c8c32538aba652bc30ca10cc16e0070864539e2db710b5822d14d6f3f`。该证据证明此前被编译掉的入口已进入产物，**不代表 Linux 实际执行通过**。

原始任务日志保存于 `validation/windows37/ci-package.log`、`ci-linux.log`、`ci-audit.log`；本地验证日志为同目录 `ci-fix-*.log`，链接与测试源选择结果在 `validation/ci-fix/link-and-test-selection.json`。均为本机验证输出，不入库。

## 剩余验证与状态

本机没有可用 Linux/WSL 发行版，尚未执行修复后的 Linux 完整回归与 POSIX 共享内存实际像素/清理用例。应在现有容器流水线原配置中复验，要求 package、linux、Windows 普通/竞态及分片审计均实际通过；不能把交叉编译或此前 Linux 竞态通过当作本次整条流水线已通过。

真实窗口仍按用户先前要求待验。SF-I18、SF-I22 保持原待验状态；本次不调整 94 项通过、41 项缺环境待验、25 项缺证据待验、1 项排除的数量。收尾清单补充上述失败和修复证据，不删除历史报告。

以上为提交前的本地修复证据。用户随后已授权提交、推送并重新触发流水线；修复后的 CI 实际结果以对应运行记录为准，本文不预先宣称通过。

## 最终运行库与便携包

按既有本机授权重建 0.3.3 Windows 单文件包。新 Windows DLL SHA256 为 `80257c9c019e40d67e8985391c63693ac67e33995d65dd408e6355c5bbdcabae`；使用该最终 DLL 的 VT 整包回归通过（9.242 秒）。并行终端回归出现一次既有 `TestImplicitLinkSourcePattern` 的 100 ms 正则预算失败；保留 `validation/ci-fix/final-runtime-regression.log`，定向复验 1.193 秒、独立终端整包 17.118 秒通过，未改正则逻辑或降低断言，不声称首次整包全绿。

构建内嵌终端/存储测试通过（2.954 / 1.029 秒）；实际 ZIP 签名准备通过（1.683 秒）。ZIP CRC、仅一个 EXE、公钥内嵌与私钥排除均通过。独立空白目录验证进程存活、数据锁与正确内嵌 DLL 加载；本轮未操作真实窗口，显示、正常退出和交互验收仍待验。只清理本轮新建的无连接测试进程，没有结束用户会话。

固定 `dist` 的 EXE、ZIP 和 SHA256SUMS 已更新，11 个原配置/数据文件的集合及摘要保持不变。记录见 `validation/ci-fix/package-integrity.json`、`portable-smoke.json`、`signing-key-verification.json`、`signed-package.log`、`dist-replacement.json`。

- EXE SHA256：`4e731900df46c00c0f6e5ab3e1003835d8fa8577feff0aad41eb232527a12c84`
- ZIP SHA256：`d0530b11a09fe35db56bee9760b3fc0ecc2eee83ef8e5fcdaf96799ad25a6b03`
