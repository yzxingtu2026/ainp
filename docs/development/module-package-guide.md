# 模块包开发指南

本文用于新建 AINP 模块，以及把下游项目的存量模块迁入严格包架构。目录与依赖原则以[后端目录与模块边界](./backend-layout.md)为准。

## 新建模块

1. 在 `platform/modules/<module>/register.go` 提供稳定 `Register` 入口，返回 `error`；根包只保留注册、Facade、contracts 与常量。
2. 把 Service、Repository、model、Action 和领域流程放入有明确 owner 的能力子包。只创建包含实际源码的目录。
3. 模块只依赖 `framework`。跨模块协作先在 `framework` 定义 Provider-neutral 最小接口，再由 `internal/boot` 或根 composition root 注入。
4. 在 `platform/tests/` 使用外部 `tests` 包覆盖公开行为；不要在生产包旁放 `_test.go`，也不要为测试导出私有实现。
5. 运行架构测试和全仓测试。不得创建 TSV 基线、通配规则或临时白名单绕过失败。

最小目录示例：

```text
platform/modules/inventory/
├── register.go
├── contracts.go
└── catalog/
    ├── model.go
    ├── repository.go
    └── service.go
```

`register.go` 只负责装配：

```go
package inventory

import (
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/modules/inventory/catalog"
)

func Register(a *app.App) error {
	return catalog.Register(a, catalog.NewService(a.DB))
}
```

## 迁移下游模块

1. 先扫描根目录实现、跨模块 imports、兜底包、空目录、父子反向依赖和散落测试，记录稳定 Action/Menu/Job/Event/Setting/resource ID。
2. 在 owner 子包建立 contracts 和实现，逐批移动并保持每批可编译；根包保留稳定 `Register`、协议常量和必要的薄 Facade。
3. 将跨模块具体类型依赖改成 framework contract，由下游 composition root 注入。禁止把旧 import 登记为架构例外。
4. 合并重复测试到下游的集中测试目录，并用公开 API 验证行为。迁移后删除空目录和临时清单。
5. 对兼容 Facade 做引用审计。只有已知下游零引用、至少经过一个发布周期且有独立 Issue/PR 时，才能删除 Service/DTO/model/repository aliases；稳定注册入口和协议 ID 不删除。

## 验证

从 `platform/` 执行：

```bash
go test ./tests -run 'TestModulePackageArchitecture|TestAICapabilityDependencyGraph|TestModuleStrictArchitectureFailures' -count=1
go test ./...
```

架构错误会包含规则名、源文件和目标 import。按提示移动实现、移除空目录、补齐注册入口或改为 framework contract；不要恢复历史基线。
