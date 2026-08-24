# Issue #296 下游认证映射

AINP #296 只提供公开认证与 Host 装配契约，不替 HOA、ERP 或其他下游迁移业务表、生产数据和领域授权。

| 下游现状 | 公开契约承接 | 下游仍需单独完成 |
| --- | --- | --- |
| HOA 后台账号和管家端 `staff_user` | `ainp.account`、`ainp.tenant-console`；管家端注册 `hoa.staff` Kind/Provider 与 `hoa.steward-client` Surface，微信解析实现 Authenticator | 把 `steward_auth.go` 移入 HOA Module；删除复制的 JWT、中间件和 system 修改 |
| HOA 社区端 `external_subject` | `hoa.community-member` Kind/Provider、`hoa.community-client` Surface、统一 Principal Session | 保留 HOA 自有主体/身份表；删除独立 `external_sessions`，制定数据转换和重新登录窗口 |
| ERP 企业 OAuth | `ainp.account` Provider + 下游 DingTalk/WeCom Authenticator；仍是后台账号时不需要新 Kind | 将 OAuth 集成和 Authenticator 放入 ERP Module；停止复制 AINP system/boot；未来非 Account 员工再注册 ERP Kind/Surface |
| 餐饮运营后台/商户/顾客/设备 | 每类主体注册独立 Kind 与 Surface；Action 显式声明允许组合 | 具体业务主体、凭据渠道、设备生命周期和业务授权 |

迁移约束：下游不得复制 TokenService、中间件、Session 表、system model 或 boot 代码；必须固定依赖正式 `platform/vX.Y.Z` Tag，通过同一 Host Module 的 `Prepare/Register` 装配。无法映射的历史分叉应拆为下游迁移 Issue，不能通过兼容默认值绕过 Kind、Surface 或 audience 校验。

## 当前分叉证据

- HOA `platform/framework/auth/principal.go` 内置产品专用 `staff_user/external_subject`，`audience.go` 硬编码 tenant/steward/community 三套 Surface，`0054_multi_client_auth.sql` 创建独立 `external_sessions`；这些分别由可注册 Kind/Surface、统一 Session 和下游迁移接管。
- HOA `platform/modules/system/community_auth.go` 与 `steward_auth.go` 同时包含渠道认证和主体解析。迁移后 Authenticator 只验证微信凭据，Principal Provider 负责加载、停用复核和授权投影。
- HOA、ERP 的 `platform/cmd/ainp/main.go` 均直接导入 `platform/internal/boot`，说明当前仍是源码复制型应用；目标入口应改为正式 Platform Tag 提供的 Host，并在同一 Module 清单中装配 API/embedded Worker/独立 Worker。
- ERP `platform/modules/system/oauth_authenticator.go` 与 `framework/authprovider` 已有清晰渠道接口，可直接迁为下游 Authenticator；其企业身份最终映射后台 Account，因此不得另签 Token 或复制 Session 链。

验证清单：

- API、embedded Worker、独立 Worker 使用同一份封存注册结果。
- 自定义主体与 Account 使用相同 ID 时，Action、Session、审计、附件、通知、MCP 不能互相读取。
- Provider 停用或 auth version 变化后，旧 Token、refresh、异步恢复和确认记录均 fail-closed。
