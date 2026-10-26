# 终端单元格双向布局

按 OShell `3f4efd9c` 的 `TerminalBidi.swift` 移植：六种呈现模式、段落方向、逻辑/视觉列映射、阿拉伯字形与镜像。底层 UAX #9 使用相邻的 `bidi` 包，文本缓冲保持逻辑顺序。

当前为接入中的布局层，原生段落状态、绘制、选区、光标和输入映射已在内部开关下接通并测试，生产默认仍关闭。VT 快照重放与完整交互验收尚未完成。单元测试不能替代真实 Swift/CoreText 显示对照；不可据此宣称终端已支持完整 BiDi。

Unicode 17 数据来自 SwiftTerm，保留其 MIT 与 Unicode 许可。更新源码后可重新生成：

```sh
python scripts/generate-terminal-bidi.py --native-source /path/to/OShell
```
