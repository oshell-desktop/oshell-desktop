# 2026-10-26 本地编译与流水线优化记录

用户要求先获得本地便携包，并处理再次失败及运行过长的问题。已先按当前 `main` 提交 `b3bdae0202a1825b5865c437be6f068d72397441` 本机编译、验证并更新固定 dist；其后修改仅涉及测试、计时数据和文档，没有改变应用生产逻辑。

## 已交付的本地包

- EXE：`dist/windows-amd64-single-file/OShell Desktop.exe`，30,764,544 字节，SHA256 `61eef77672ac337f69977a08b0fb130d20c478c6886f7a771b24c226aaaf73e2`。
- ZIP：`dist/OShell-Desktop-windows-amd64-single-file.zip`，14,246,766 字节，SHA256 `ecf8cd136accf6a49627980b0120f88174afaad3413dd8855b8b97458e9a93f6`。
- 本机构建内嵌资源及存储验证通过（3.039 / 1.093 秒）；ZIP CRC、唯一 EXE、公钥内嵌与私钥排除、实际 ZIP 签名准备（1.387 秒）通过。
- 独立空白目录验证进程存活、数据锁和内嵌 DLL 加载；真实窗口显示、正常退出和交互未重新验收，仍沿用用户要求的待验状态。
- 原固定目录 11 个配置/数据文件集合及摘要不变。运行库未改，SHA256 `80257c9c019e40d67e8985391c63693ac67e33995d65dd408e6355c5bbdcabae`。

本轮证据在本机 `validation/local-build/`：`build.log`、`package-integrity.json`、`portable-smoke.json`、`signing-key-verification.json`、`signed-package.log`、`dist-replacement.json`。产物、配置与这些验证输出不入库。

## 第二次流水线的实际结果

[运行 40292800808](https://github.com/oshell-desktop/oshell-desktop/actions/runs/40292800808) 从 08:31:06 UTC 至 08:59:08 UTC，约 28 分钟。package、linux、8 个 core-race 分片及完整性审计全部通过；windows 任务失败，release 跳过。

因此上一轮的 vet、Linux libc/librt 链接、平台测试选择和完整回归时间预算已获得这次 CI 证据。Linux VT 完整包 6.645 秒通过，包含未跳过的 POSIX 共享内存用例；这不是 Linux/macOS 真实 GPU 或物理输入验收，不自动关闭 SF-I22 等跨平台条目。

唯一显示的失败是 `TestLiveDirectorySwitchKeepsWorkingConfiguration`，原断言只输出 `live switch failed <nil>`，未区分回调、目标路径和状态。生产代码会将目标目录通过 Abs/EvalSymlinks 规范化，测试却与原始字符串直接比较。以 `目录/.` 合成输入在本机复现了完全相同的断言失败；修正测试预期为规范化路径，并分开输出目标和状态断言。Windows 短名或路径别名可触发该差异；旧 CI 日志没有记录原始目标，不能断言已经还原了该 Runner 的具体路径形态。

本地原失败/修正结果分别见 `sync-path-before.log`、`sync-path-after.log`；切换目录、后端选择和同步保存相关 3 项通过（2.264 秒）。保留回调执行、启用状态、非重启生效、本机配置字节不变、地址指向正确目标的所有检查。生产同步逻辑未改。问题仍归原 SF-Y02，不增加范围；修复是否完全解决 CI 的该次失败，以后续运行结果为准。

## 耗时来源与本地优化

旧计时表缺少 Windows 98 个、Linux 96 个新测试的实际权重。这些测试按默认 1 秒分配，造成严重不均。其中 `TestSyncSelectedCategoriesPreserveBothSides` 的 15 种同步范围组合只能整体进入一个分片，实际竞态耗时 Windows 1013.08 秒、Linux 847.02 秒。

优化采用现有 4 个分片及相同完整性审计，不增加 Runner 数量，不取消 race，不缓存测试结果，不减少 PBKDF2 轮数：

1. 将原 mask=1..15 的组合变为 15 个可独立调度的顶层用例，共用完整且未改变的原用例体；原名字对应 mask=1，新名字后缀 02..15 对应其余组合。这是测试调度颗粒度变化，不是新增 14 项功能或增加验收进度。
2. 以此次 8 个通过分片的原始 JSON 日志刷新 `scripts/core-race-timings.json`。拆分组合的权重来自对应原子测试的真实 elapsed，其他权重来自原顶层测试 elapsed；计时来源明确记录。
3. 继续动态发现全部测试并按权重分配，保持每个平台每个用例恰好执行一次的审计。没有更改手动触发规则、CI 容器编译约束或任务总时限。

| 平台 | 原 4 个分片实际测试时长（秒） | 新分配预计测试时长（秒） |
| --- | --- | --- |
| Windows | 1273.10 / 618.78 / 300.72 / 234.97 | 607.16 / 607.17 / 607.16 / 607.16 |
| Linux | 450.45 / 1062.17 / 330.09 / 281.19 | 531.28 / 531.29 / 531.28 / 531.28 |

上述是按同一轮 elapsed 重排的估算，未计入 Runner 波动、构建、缓存和下载时间。结合此次约 6 分钟 package 前置任务，预计总墙钟约 16–18 分钟；没有宣称优化后的 CI 已实测达到这个时间。原始与预计数据见 `shard-comparison.json`。

## 验证与剩余工作

- 拆分后的原 15 个同步范围组合及本轮同批样本实际通过，core 定向运行 80.673 秒；自动核对 15 个 mask 均执行且通过，原用例体未改。
- 仓库要求的四组 Go 配置兼容样本通过（4.369 秒）；缺少 macOS，真实 Swift 双端仍待验。
- 分片工具 7 项测试通过，源计时数据通过原始日志/manifest 一致性审计。原 Windows 308、Linux 297 个顶层用例全部被记录；拆分后分别为 322/311，不改变 161 个 SF 条目。
- `go vet ./internal/core ./internal/desktop` 通过。所有修改都是测试/调度计时/文档，已先交付的便携程序无需因此重建。

仍需后续 CI 验证 Windows 目录用例、全部分片执行及实际耗时。真实窗口待验项与原 161 项边界保持不变。该轮当时未提交、推送或触发新流水线；随后按用户授权，与 SF-F14 修复一并提交并手动触发。实际耗时仍待后续运行结果，不作预先通过声明。
