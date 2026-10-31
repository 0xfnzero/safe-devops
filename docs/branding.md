# safe-devops 图标

标志将金色盾牌与终端提示符 `>_` 结合：外层代表独立授权边界，内部终端代表开发与运维操作。沿用控制台的黑金配色，应用图标采用深灰圆角底板；矢量原稿不含文字，以便在小尺寸下辨认。

这是项目内原创 SVG 设计，没有调用 AI 图片生成 API。原稿及导出资源随项目采用 MIT 许可。

## 编辑与导出

- 原稿：`assets/brand/logo.svg`。
- 应用图标：`assets/brand/app-icon.svg` 与 1024px `app-icon.png`。
- 编辑原稿后运行 `make icons`；底板、留白与导出规格在 `scripts/build-icons.mjs` 中维护。
- 导出使用 Web 开发依赖 Sharp。首次运行先执行 `make setup`，普通桌面打包直接使用已提交的资源。

## 使用位置

- macOS：`desktop/assets/icon.icns`，包含标准与 Retina 尺寸；开发模式也会设置 Dock 图标。
- Windows：`desktop/assets/icon.ico`，含 16–256px 多尺寸图像。
- Linux：`desktop/assets/icons/` 下的 16–512px PNG；窗口使用 `desktop/assets/icon.png`。
- Electron 启动页与 Web 登录页、侧边栏使用同一套图形。
- 浏览器 favicon、SVG icon 和 Apple touch icon 由 Next.js 文件约定自动加载。

图标资源会随桌面应用和源码包分发。已运行的 Electron 进程需重启后加载新图标；已安装的旧发布包需要安装包含新资源的版本。
