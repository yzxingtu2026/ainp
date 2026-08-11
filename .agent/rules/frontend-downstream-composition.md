# 管理后台下游装配

- 本规则只适用于 AINP Vue 管理后台，不覆盖 Taro、React Native 等客户端。
- 下游只通过 `src/composition` 稳定入口注册品牌、页面、路由、i18n、initializer、service 和模块；禁止修改 Elegant Router 生成文件或平台 Store。
- 下游 service 必须复用 AINP 请求错误边界：默认请求层全局提示，接管展示时显式声明 `errorMode`，禁止页面展示 Axios 原始 `error.message`。
- 后端 Metadata 决定授权，前端 registry 决定产物内容；未知/重复 key、非法 path、远程代码和非法模块组合必须 fail-closed。
- 下游生产默认不装配 Demo；模块显式启用并满足依赖。品牌资源必须构建内离线可用。
- 固定不可变 Tag/Commit，使用 `admin-baseline.json` 校验漂移；同步不得覆盖既有目标，通用修复优先回提 AINP。

详规：`docs/development/frontend-downstream-composition.md`
