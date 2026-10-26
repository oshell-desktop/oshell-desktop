# 开发指南

开发约定见 [AGENTS.md](../AGENTS.md)。日常修改在本地测试和打包；提交、推送及触发 Actions 分别需要用户明确要求。

## 运行与构建

需要 Go 1.27.1+、Python 3、Git。构建脚本自动下载并校验 Zig 0.16.0，缓存源码和终端运行库；只构建 Windows / Linux。Linux 还需要图形桌面或虚拟显示，以及 GTK 3 / WebKitGTK。

```sh
python scripts/terminal_runtime.py --target windows/amd64 -- go run .
python scripts/build.py --target windows/amd64 --single-file --archive
python3 scripts/build.py --target linux/amd64 --archive
```

Linux 开发运行及测试将运行库目标改为 `linux/amd64`。运行库构建入口也可单独使用，输出 DLL / SO 路径；`OSHELL_ZIG` 可指定已有的 0.16.0 编译器。首次构建较慢，后续命中补丁指纹缓存。

单文件参数仅用于 Windows。省略它可生成 Windows 多文件便携包，运行时须保留 DLL、资源和 `oshell-portable` 标记。

构建结果统一放在 `dist/`。已有便携目录含用户数据时，用 `--out validation/portable-build` 暂存，检查后只更新固定目录中的程序和 ZIP，保留配置。不要运行暂存产物导致配置混入发布包；启动验证应使用另一个独立测试目录。

非便携程序可用 `OSHELL_DATA_DIR` 指定测试数据目录；单文件便携版始终使用 EXE 同级目录。

## 测试

```sh
python scripts/terminal_runtime.py --target windows/amd64 -- go test ./...
go test -race ./internal/core ./internal/terminal/internal/pty ./internal/zmodem
go vet ./...
python scripts/test-build.py
python scripts/test-terminal-runtime.py
python scripts/test-ci-bundle.py
python scripts/test-core-shards.py
python scripts/test-configuration-contract.py
```

测试使用合成配置、本机模拟服务和临时目录。部分平台测试有跳过条件；测试通过不代表其他平台或真实远端服务已验收。

- ZMODEM 实际互通：将 `rz` / `sz` 及依赖放入 PATH，设置 `OSHELL_REQUIRE_LRZSZ=1` 后运行相关测试。Windows CI 使用固定摘要的 lrzsz 工具，Linux 使用发行版 lrzsz。
- Linux 无图形会话：安装 `xvfb`、`xauth`、`dbus-x11`，用 `xvfb-run -a dbus-run-session -- go test ./...`。
- UI 截图：桌面测试设置 `OSHELL_TEST_ARTIFACTS`，终端测试设置 `MYGO_TEST_IMAGES`，均指向独立输出目录。
- 配置协议变更：按 [v2 验证要求](configuration-protocol-v2.md) 执行 Swift ↔ Go 互通；没有 Swift 环境的跳过不算通过。

每次完善后本机构建 Windows 单文件便携包，验证 ZIP 仅含 EXE、独立目录启动成功，再更新固定 `dist` 输出并校验用户配置未变。程序仍运行时请用户退出，不强制结束会话。

## 目录与脚本

| 位置 | 用途 |
| --- | --- |
| `internal/core` | 配置、凭据、连接、同步与更新 |
| `internal/desktop` | 原生 UI、窗口与交互 |
| `internal/terminal`、`internal/zmodem` | 终端、PTY、文件传输 |
| `scripts/build.py`、`single_file.py` | 打包与内嵌资源 |
| `scripts/build-on-runner.sh`、`build-container.sh` | CI 宿主入口与容器入口 |
| `scripts/ci-test-bundle.py` | Windows 测试预编译、分包和平台测试执行 |
| `scripts/core_test_shards.py`、`core-race-timings.json` | 竞态测试调度与完整性审计 |
| `scripts/sync-configuration-contract.py`、`test-sync-interop.py`、`interop/` | Swift 合同生成及真实跨端验证 |
| `scripts/test-*.py`、`test-update-package.ps1` | 工具回归与 Windows 实包升级验证 |
| `third_party/` | 固定依赖源码、本地补丁与原始许可证 |

macOS 平台适配和连接助手保留作兼容用途，不参与 Windows / Linux 发布。第三方许可证、必要协议样本和互通脚本不能作为缓存清理。

CI 的容器、缓存和分片规则见 [CI 说明](ci-performance.md)。

OShell 源码索引、功能交互对照及未完成项见 [跨端对齐记录](oshell-parity.md)。测试通过不能代替逐项行为一致性验收。
