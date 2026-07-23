# Issue #164 测试归档对照

本表记录原生产目录 18 个测试文件的迁移或替代位置。集中测试统一使用 `package tests`，只通过公开 API 验证行为；没有增加仅供测试使用的生产导出符号。

| 原文件 | 集中测试或替代覆盖 |
| --- | --- |
| `framework/action/schema_test.go` | `action_schema_contract_test.go`，原断言完整迁移 |
| `framework/agentslot/registry_test.go` | `agent_slot_registry_test.go`，原断言完整迁移 |
| `framework/menu/help_test.go` | `menu_help_test.go`，原断言完整迁移 |
| `framework/prompt/prompt_test.go` | `prompt_runtime_test.go`，原断言完整迁移并修正集中目录路径边界 |
| `framework/runtimecontext/types_test.go` | `runtime_context_types_test.go`，原断言完整迁移 |
| `framework/setting/change_test.go` | `setting_change_registry_test.go`，原断言完整迁移 |
| `framework/setting/conditional_required_test.go` | `setting_conditional_required_test.go`，通过 Registry 与 `ValidateValues` 公开入口重写 |
| `modules/ai/agent_diagnostics_test.go` | `ai_agent_management_test.go` 与 `ai_context_compaction_test.go` 的公开诊断/模型预算路径 |
| `modules/ai/memory_extraction_test.go` | `ai_memory_extraction_contract_test.go` 的真实会话、严格结构化输出、来源绑定和幂等提取流程 |
| `modules/ai/memory_governance_test.go` | `ai_memory_extraction_contract_test.go`、`ai_memory_recall_test.go` 的治理、投影、重复与当前事实流程 |
| `modules/ai/model_capability_test.go` | `ai_public_behavior_test.go`，通过 `Service.CreateDeployment` 公开入口重写 |
| `modules/ai/navigation_contract_test.go` | `ai_navigation_contract_test.go`，通过内置 Agent Contract Registry 完整迁移 |
| `modules/ai/navigation_embedding_register_test.go` | `navigation_semantic_search_test.go` 的批量文档与受治理 Embedding 服务流程 |
| `modules/ai/personalization_service_test.go` | `ai_public_behavior_test.go` 的公开回退契约，以及 `runtime_context_provider_integration_test.go` 的语言、时区与称呼上下文 |
| `modules/ai/prompt_test.go` | 不含断言，仅为原包级白盒测试加载 Prompt；集中测试分别在需要时加载 `../prompts` |
| `modules/ai/runtime_context_test.go` | `ai_agent_runtime_test.go` 与 `runtime_context_provider_integration_test.go` 的可信上下文持久化和公开 Provider 流程 |
| `modules/ai/tool_failure_test.go` | `ai_tool_runtime_test.go` 的公开 Load/Execute/Approve、参数校验、权限失效、确认恢复和重复加载流程 |
| `modules/system/setting_change_test.go` | `setting_service_integration_test.go`，通过 `SetChangeRegistry` 与真实提交后 Update 流程重写 |

架构门禁位于 `test_placement_architecture_test.go`，同时扫描实际目录并用临时违规样例验证错误路径输出。
