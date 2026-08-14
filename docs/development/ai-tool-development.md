# AI 工具开发指南

本文说明如何向 AINP Tool Runtime 新增工具。运行时架构、安全边界和渐进式披露协议见 [统一 Tool Runtime](./tool-runtime.md)。

## 选择接入方式

优先复用 Action Tool。只有能力不属于现有业务 Action，例如内置搜索、插件、MCP 或外部服务协议时，才新增 Provider/Executor。

| 场景 | Source | 接入方式 |
| --- | --- | --- |
| 已有业务能力 | `action` | 在 Action Metadata 中声明 `AIMeta` |
| 框架内置能力 | `builtin` | 实现 Provider + Executor |
| 插件能力 | `plugin` | 插件实现 Provider + Executor 并在启用时注册 |
| 声明式 HTTP | `http` | 受控 HTTP Provider + 沙箱化 Executor |
| MCP Server | `mcp` | MCP Provider + Executor |

禁止业务模块自行把完整工具 Schema 放入模型请求，禁止 Executor 绕过 Tool Runtime 直接向模型暴露或执行。

需要把资源 ID、变更集合等复杂参数解释为普通用户可读授权信息时，在 Tool 所属模块通过 `ToolRegistry.RegisterConfirmationPresenter` 注册领域 Presenter。Presenter 必须使用当前请求上下文下的 TenantDB/Guarded Repository 做只读解析，返回 `ConfirmationPresentation`，并允许任何查询失败由 Runtime 安全降级。禁止在 AI Runtime、SSE Handler 或前端按 Tool ID 编写展示特例；简单工具直接使用 Definition Human 文案和默认生成器。

## 将 Action 暴露为工具

请求 DTO 是 Action 验证和 AI Tool Schema 的唯一事实源：

```go
type OrderGetRequest struct {
    OrderID string `json:"order_id" validate:"required,max=64"`
}

func (s *Service) GetOrder(ctx context.Context, req OrderGetRequest) (any, error) {
    // tenant 和 actor 必须来自 AuthContext，不能来自请求字段。
    return s.getOrder(ctx, req.OrderID)
}
```

Action 是否暴露以其注册处的 `Metadata.AI` 为唯一事实源。适合 Agent 使用的 Action 填写完整 `AIMeta`；不暴露的 Action 保持 `AI=nil`，由 `ActionProvider` 默认忽略。禁止新增 `action_ai_inventory.go`、Action ID switch 或其他与 Registry 平行的手工暴露清单。

注册可暴露 Action 时增加 `AIMeta`：

```go
if err := a.RegisterAction(action.Action{
    ID:      "purchase.order_get",
    Handler: action.JSON(service.GetOrder),
    Meta: action.Metadata{
        Capability:      "purchase",
        Name:            "查询采购单",
        PermissionGroup: "purchase.order",
        AI: &action.AIMeta{
            ExposeTool:          true,
            ToolTitle:           "查询采购单",
            Summary:             "根据采购单 ID 查询当前用户有权访问的采购单",
            HumanDescription:    "根据采购单 ID 读取当前用户有权访问的采购单",
            Tags:                []string{"采购单", "采购", "查询"},
            Risk:                "low",
            SideEffect:          "read",
            ResultSummary:       "已返回当前用户有权查看的采购单",
            InputSchema:         action.InputSchemaFor[OrderGetRequest](),
            OutputSchema:        action.SchemaFor[OrderView](),
        },
    },
}); err != nil {
    return err
}
```

系统自动投影为 `action:purchase.order_get`。搜索时沿用 Action permission，执行时只调用 `ActionRegistry.Invoke`，因此继续经过 realm、tenant、permission、DTO 验证、幂等和审计。

不要手写一份与 DTO 平行的输入或输出 Schema。`action.SchemaFor[T]()` 会映射 JSON 字段、required、min/max、oneof、嵌套对象和数组，并默认拒绝额外字段。`ResultSummary` 只能描述结果类别，不得拼入参数、业务正文或敏感字段。

未声明 `AIMeta` 的 Action 不会进入 Tool Registry，也不会被 `tool.search`、`tool.load` 或模型调用。需要新增暴露能力时，直接修改该 Action 的注册定义并补齐契约测试，不同步维护第二份 ID 清单。

### Capability 白名单门禁

`ExposeTool=true` 只表示 Action 可以投影为工具，不代表 Agent 可以发现它。新增或首次暴露一个 Capability 时，必须在 `platform/modules/ai/register.go` 审查 builtin Agent Contract，并显式更新目标 Contract 的 `AllowedToolCapabilities`。框架 boot 会汇总真实 Action Registry 与所有 builtin Contract：任何暴露 Capability 未被至少一个 Contract 覆盖时直接启动失败，错误会指向该注册文件；禁止通过自动聚合绕过安全审查。

确有安全原因需要让某个暴露 Capability 暂时不属于任何 builtin Contract 时，可在统一 boot 校验调用中加入显式排除。排除清单会双向校验：Capability 不再暴露或已被 Contract 覆盖后，旧排除同样导致失败，必须及时删除。下游新增模块的测试还应从目标 Contract 创建真实 ToolScope，验证授权用户可发现、无权限用户不可见。

### 写操作示例

```go
AI: &action.AIMeta{
    ExposeTool:           true,
    ToolTitle:            "创建采购订单",
    Summary:              "创建采购订单",
    HumanDescription:     "在当前租户创建一张新的采购订单",
    ResultSummary:        "采购订单已创建",
    Risk:                 "medium",
    SideEffect:           "write",
    RequiresConfirmation: true,
    InputSchema:          action.InputSchemaFor[OrderCreateRequest](),
    OutputSchema:         action.SchemaFor[OrderView](),
}
```

`ToolTitle` 是工具过程卡片使用的简短动作标题，必须由 Action 注册处显式声明为“动作 + 对象”，例如“查询采购单”“准备下载附件”“预览通知模板”。`Summary` 面向模型描述能力，`HumanDescription` 面向用户补充范围与约束，`ResultSummary` 描述结果类别。禁止根据 `SideEffect`、Action ID 或模型工具名推导标题。

写操作还应按业务约定配置 `IdempotencyMeta`。模型不能传 `skip_confirmation`，也不能降低风险等级。

### 可重复创建默认使用批量契约

新建面向 AI、可重复创建同类资源的能力时，优先保留现有单条 HTTP/管理端接口，并为 AI 注册一个显式批量 Action；`items` 长度为 1 即覆盖单条场景。禁止让模型生成多个独立单条写 Tool Call，也禁止由 Runtime 合并或一次批准任意调用。

```go
type OrderBatchCreateRequest struct {
    Items []OrderCreateRequest `json:"items" validate:"required,min=1,max=50,dive"`
}

type OrderBatchCreateResponse struct {
    CreatedCount int         `json:"created_count"`
    Items        []OrderView `json:"items"`
}

AI: &action.AIMeta{
    ExposeTool:           true,
    Operation:            action.AIOperationCreate,
    Batch:                &action.AIBatchMeta{Mode: action.AIBatchAtomic, ItemsPath: "items"},
    ToolTitle:            "批量创建采购订单",
    Summary:              "在一个事务内创建一至多张采购订单",
    HumanDescription:     "整批校验并按请求顺序返回结果，任一项失败时全部回滚",
    ResultSummary:        "采购订单已批量创建",
    Risk:                 "high",
    SideEffect:           "write",
    RequiresConfirmation: true,
    InputSchema:          action.InputSchemaFor[OrderBatchCreateRequest](),
    OutputSchema:         action.SchemaFor[OrderBatchCreateResponse](),
},
Idempotency: &action.IdempotencyMeta{Mode: "required", TTL: 24 * time.Hour},
```

Registry 会校验顶层 object、required `ItemsPath`、array 上下限、closed object item、确认、必需幂等以及 `created_count + items` 输出。Tool Runtime 使用完整 JSON Schema 在确认前校验嵌套字段与 `minItems/maxItems`；领域 Service 仍必须执行同一 DTO 验证、租户校验、事务与数据库不变量，不能把 Schema 当作授权或事务保证。

不能同步原子完成时只能显式选择 `preview_commit`、`async_job` 或 `single_only`。`single_only` 仅接受框架原因常量；外部副作用、付费调用、文件导入和长任务不得伪装为 `atomic`。完整模式与结果协议见 [Action 元数据](./action-metadata.md#ai-创建与批量契约)。

## 新增 Provider 和 Executor

框架内置、需要动态可用性或专用预算/观测的核心工具应在能力 owner 包实现 `aitool.RuntimeSlot`，并只通过根装配中的 `tooling.RegisterBuiltins` 显式聚合。禁止恢复横切 `platform/modules/ai/tools` 包。Slot 必须声明：

- `Definition` 与 `direct/progressive` 发现模式；
- 基于可信 `RuntimeScope` 的动态投影；
- Executor；
- 通用 Step/Invocation 分类、预算类别/硬上限；
- 允许持久化的安全观测字段白名单。
- 允许暴露的稳定失败子分类白名单；未声明分类统一收敛为 `execution_failed`。

禁止为新核心工具修改 `ToolService` 或 `AgentRuntimeService` 的 Tool ID 分支。模型参数不能提供 tenant、user、run 或 scope；这些标识只能来自 `RuntimeScope`。动态投影必须用于 project/search/load/execute 全路径，且只能收紧静态 Registry、Scope 和权限结果。

普通动态来源使用下述 Provider/Executor 接口；不要为了一个核心工具注册第二个同 Source Provider。

Provider 只提供定义，不执行工具：

```go
type WeatherProvider struct{}

func (*WeatherProvider) Source() aitool.Source { return aitool.SourceBuiltin }

func (*WeatherProvider) List(context.Context) ([]aitool.Definition, error) {
    return []aitool.Definition{weatherDefinition()}, nil
}

func (*WeatherProvider) Get(_ context.Context, id string) (*aitool.Definition, error) {
    if id != "builtin:weather.current" {
        return nil, errs.New(errs.CodeNotFound, "tool not found")
    }
    definition := weatherDefinition()
    return &definition, nil
}
```

Definition 必须完整声明安全元数据：

```go
func weatherDefinition() aitool.Definition {
    return aitool.Definition{
        ID:                  "builtin:weather.current",
        Source:              aitool.SourceBuiltin,
        Name:                "weather.current",
        TitleForHuman:       "实时天气查询",
        DescriptionForLLM:   "查询指定城市当前的天气情况",
        DescriptionForHuman: "访问天气服务读取实时天气",
        Capability:          "weather",
        Tags:                []string{"天气", "温度", "降雨"},
        InputSchema:         action.InputSchemaFor[WeatherRequest](),
        Risk:                aitool.RiskLow,
        SideEffect:          aitool.SideEffectExternal,
        RequiresConfirmation: true,
        BindingMode:         aitool.BindingOptional,
        LoadingMode:         aitool.LoadingDirect,
        RuntimeRef:          "weather.current",
        SchemaVersion:       1,
        Status:              "enabled",
    }
}
```

Executor 只处理已经过 Scope、Handle、权限、Schema 和确认校验的调用：

```go
type WeatherExecutor struct {
    client WeatherClient
}

func (e *WeatherExecutor) Execute(
    ctx context.Context,
    loaded aitool.LoadedTool,
    arguments json.RawMessage,
) (*aitool.Result, error) {
    var req WeatherRequest
    if err := json.Unmarshal(arguments, &req); err != nil {
        return nil, err
    }
    result, err := e.client.Current(ctx, req.City)
    if err != nil {
        return nil, mapWeatherError(err)
    }
    return &aitool.Result{
        Status:  "completed",
        Summary: "天气查询完成",
        Data:    result,
    }, nil
}
```

不要在 Executor 中记录完整 arguments、Authorization、Credential 或第三方原始响应。Tool Runtime 会再次限制结果大小、深度并递归脱敏，但 Executor 仍应只返回业务需要的受控 DTO。

## 注册 Provider

AI 模块初始化完成后注册：

```go
if a.ToolRegistry == nil {
    return fmt.Errorf("tool registry is not initialized")
}

if err := a.ToolRegistry.RegisterProvider(
    &WeatherProvider{},
    &WeatherExecutor{client: weatherClient},
); err != nil {
    return err
}
```

每种 Source 当前只能注册一个 Provider/Executor 组合。若一个 Source 下有多个插件或 MCP Server，应由该 Source Provider 聚合它们的定义，不能重复注册同一个 Source。

核心 Slot 不按 Source 共享 Executor。Registry 根据 Tool ID 定位 Slot Executor，因此多个 `builtin` Slot 可以独立注册、投影和执行，且不会互相覆盖。

Registry 在搜索、加载和模型投影前刷新，因此后注册的 Action/插件可以被发现。定义重复、Source 不匹配、非法 ID 或 Schema 会使刷新失败，不能静默跳过。新增或变更生产 Tool 后，应用启动和新租户初始化会自动 Reconcile Catalog；运行中动态注册来源必须显式触发同一 Reconcile，禁止依赖首次搜索写库。当前租户可通过 `ai.tool_catalog_rebuild` 做幂等修复。

## 命名规范

工具 ID 格式为 `{source}:{namespaced-id}`：

```text
action:purchase.order_create
builtin:weather.current
plugin:amap.route_plan
http:erp.stock_query
mcp:github.create_issue
```

- ID 使用小写字母、数字、下划线、连字符和点。
- ID 一旦发布应保持稳定；改名按删除旧工具、新增工具处理。
- `Name` 是稳定工具名，`DescriptionForLLM` 面向检索和模型选择；`TitleForHuman` / `DescriptionForHuman` 是插件自带的人类展示契约。框架不要求插件注册平台 i18n key，管理界面不得直接展示 LLM 描述。
- Human 展示文案不参与 Schema Hash、Handle 或 Embedding 文档；修改展示文案不得改变工具执行契约。
- 描述应说明何时使用、必要输入和主要结果，不写 Credential、Endpoint 或内部实现。
- `SchemaVersion` 从 1 开始；契约变化时递增。

安全相关定义变化会改变 schema hash，使旧 Handle 自动失效。

## 风险和副作用

| Risk | 使用建议 |
| --- | --- |
| `low` | 可撤销或只读的低风险操作 |
| `medium` | 常规业务写入或有限外部影响 |
| `high` | 重要写入、敏感数据或明显外部影响，必须确认 |
| `critical` | 默认禁止 Agent 执行 |

| Side effect | 默认策略 |
| --- | --- |
| `none` | 无外部读取或修改 |
| `read` | 读取数据 |
| `write` | 修改业务数据，默认确认 |
| `external` | 调用外部系统，默认确认 |
| `paid` | 产生费用，默认确认 |
| `destructive` | 删除或不可逆操作，默认确认 |

最终策略取 Tool Definition、Action、Agent Scope 和租户策略中的最严格值。

## 搜索文档设计

检索字段只使用安全元数据：ID、名称、LLM 摘要、capability、tags 和输入字段提示。不要把完整 Schema、用户参数、执行结果、Header、Credential、内部 Endpoint 写入搜索文本或向量。

为提高检索准确率，Tags 应包含领域词、同义词、缩写和常见用户意图，例如：

```go
Tags: []string{"采购单", "采购订单", "供应商下单", "PO"}
```

Embedding 不可用时精确匹配和 PostgreSQL FTS 必须继续工作。

## 测试模板

测试统一放在 `platform/tests`，至少覆盖：

```go
func TestWeatherToolDefinition(t *testing.T) {
    definition := weatherDefinition()
    if err := definition.Validate(); err != nil {
        t.Fatal(err)
    }
}

func TestWeatherToolExecutor(t *testing.T) {
    executor := &WeatherExecutor{client: newFakeWeatherClient()}
    result, err := executor.Execute(
        context.Background(),
        aitool.LoadedTool{Definition: weatherDefinition()},
        json.RawMessage(`{"city":"杭州"}`),
    )
    if err != nil || result.Status != "completed" {
        t.Fatalf("unexpected result: %#v %v", result, err)
    }
}
```

还应验证：

- 未授权用户搜索不到工具；
- load 数量、Scope 总量和 Schema 字节预算；
- Handle 跨租户、跨用户、过期和 schema hash 变化；
- 写操作在批准前不执行，批准只能消费一次；
- 参数、结果、日志和 Outbox 不泄露 Secret；
- 外部依赖使用 Fake，不要求真实额度或 Credential。

## 交付检查清单

- [ ] 工具 ID、Source、Capability 和 RuntimeRef 稳定。
- [ ] DTO 与 Schema 同源，没有手写平行契约。
- [ ] 风险、副作用和确认策略按最严格原则配置。
- [ ] permission、tenant 和 realm 沿用事实源，不复制权限逻辑。
- [ ] 新增或首次暴露 Capability 已审查并同步 builtin Contract 白名单，架构门禁无遗漏或 stale 排除。
- [ ] Executor 不绕过 Tool Runtime，不记录敏感输入输出。
- [ ] 核心 Slot 只修改独立工具目录和显式聚合入口，公共主循环没有业务 Tool ID 分支。
- [ ] project/search/load/execute 共用动态可用性判定，投影后撤权会立即拒绝执行。
- [ ] Slot 预算、分类和观测字段由通用元数据声明，事实摘要只保留白名单字段。
- [ ] 搜索文本和向量不含 Schema、参数、结果或 Credential。
- [ ] 测试覆盖发现、加载、执行、确认、跨租户和脱敏。
- [ ] `go test ./...`、`pnpm typecheck`、`pnpm build`、`agent-rules doctor` 和 `git diff --check` 通过。
