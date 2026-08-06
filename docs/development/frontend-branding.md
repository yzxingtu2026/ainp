# 前端品牌资产

## AINP 产品品牌

官方产品品牌为 AINP；下游管理后台只能通过 Composition Brand 契约覆盖产品名、构建内 Logo/Favicon、登录视觉和基础主题色，不得覆盖内部组件或依赖运行时 CDN。

- 标准图形源资产固定为 `frontend/public/brand/ainp-logo.svg`。
- 应用壳、登录页、启动 Loading 与 favicon 必须引用该源资产，禁止复制 SVG path 或维护独立变体。
- Logo 外部背景透明，品牌底色为 `#646CFF`，内部标识为白色；该组合同时适用于浅色和深色界面。
- 新使用场景必须验证 16px、32px、64px 和实际容器尺寸，保持 `object-fit: contain`，禁止拉伸、裁切、加阴影或自行改色。
- 需要 PNG 等位图时从标准 SVG 自动导出；位图是派生产物，不得反向成为源资产。

## 三类信息边界

产品品牌、技术来源和许可证归属是三个不同概念：

1. 用户界面的系统标题、Logo、水印、页脚、favicon 和主题预设使用 AINP 品牌。
2. `Soybean Admin`、Elegant Router、Naive UI、UnoCSS 等真实依赖名称在开发文档、包名和技术链接中保持准确，不改写为 AINP。
3. 上游与第三方的 `LICENSE`、`NOTICE` 和版权归属必须完整保留；产品页不展示上游品牌不等于删除许可证信息。

替换品牌时必须分别检查 `frontend/src`、`frontend/public`、`frontend/index.html`、主题 preset、构建元数据和开发者文档，逐条判断命中属于哪一类，禁止用全局字符串替换混淆边界。
