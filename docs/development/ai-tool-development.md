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

注册 Action 时增加 `AIMeta`：

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
            Summary:             "根据采购单 ID 查询当前用户有权访问的采购单",
            HumanDescription:    "读取指定采购单",
            Risk:                "low",
            SideEffect:          "read",
            InputSchema:         action.SchemaFor[OrderGetRequest](),
        },
    },
}); err != nil {
    return err
}
```

系统自动投影为 `action:purchase.order_get`。搜索时沿用 Action permission，执行时只调用 `ActionRegistry.Invoke`，因此继续经过 realm、tenant、permission、DTO 验证、幂等和审计。

不要手写一份与 DTO 平行的输入 Schema。`action.SchemaFor[T]()` 会映射 JSON 字段、required、min/max、oneof、嵌套对象和数组，并默认拒绝额外字段。

### 写操作示例

```go
AI: &action.AIMeta{
    ExposeTool:           true,
    Summary:              "创建采购订单",
    HumanDescription:     "创建一张新的采购订单",
    Risk:                 "medium",
    SideEffect:           "write",
    RequiresConfirmation: true,
    InputSchema:          action.SchemaFor[OrderCreateRequest](),
}
```

写操作还应按业务约定配置 `IdempotencyMeta`。模型不能传 `skip_confirmation`，也不能降低风险等级。

## 新增 Provider 和 Executor

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
        Name:                "查询实时天气",
        DescriptionForLLM:   "查询指定城市当前的天气情况",
        DescriptionForHuman: "访问天气服务读取实时天气",
        Capability:          "weather",
        Tags:                []string{"天气", "温度", "降雨"},
        InputSchema:         action.SchemaFor[WeatherRequest](),
        Risk:                aitool.RiskLow,
        SideEffect:          aitool.SideEffectExternal,
        RequiresConfirmation: true,
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

Registry 在搜索、加载和模型投影前刷新，因此后注册的 Action/插件可以被发现。定义重复、Source 不匹配、非法 ID 或 Schema 会使刷新失败，不能静默跳过。

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
- `Name` 面向人类，`DescriptionForLLM` 面向检索和模型选择。
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
- [ ] Executor 不绕过 Tool Runtime，不记录敏感输入输出。
- [ ] 搜索文本和向量不含 Schema、参数、结果或 Credential。
- [ ] 测试覆盖发现、加载、执行、确认、跨租户和脱敏。
- [ ] `go test ./...`、`pnpm typecheck`、`pnpm build`、`agent-rules doctor` 和 `git diff --check` 通过。
