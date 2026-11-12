# CI 构建与发布

工作流：[build.yml](../.github/workflows/build.yml)。仅手动 `workflow_dispatch` 触发，push / PR / 标签推送均不自动运行。使用 GitHub 官方 Runner，不依赖自托管服务器。

## 执行结构

| 任务 | 执行内容 |
| --- | --- |
| package | Ubuntu Runner 在 Docker 内依次构建 Windows 单文件 ZIP 和 Linux TAR.GZ，上传两个平台的产物。版本标签构建还会上传更新清单签名工具。 |
| release | 版本标签构建完成后，核对应用版本、签名并验证更新清单、生成 SHA-256 校验文件，再发布 GitHub Release。 |

回归测试在发布前本地执行。手动选择分支时只构建产物，选择对应版本标签时同时发布。

## 构建缓存

依赖镜像按 Dockerfile、架构和月份缓存。容器复用 Go 模块、编译缓存和原生资源，结束后恢复工作区与缓存的所有权。

操作规则见 [AGENTS.md](../AGENTS.md)，发布步骤见 [更新与发布](updates-portable.md)。
