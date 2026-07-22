---
menu_id: operations.audit_logs
version: 1.0.0
locale: zh-CN
use_cases:
  - 查询操作审计
  - 查看审计详情
permission_refs:
  - system.audit_log_list
---
# 操作审计

## 页面用途
查询当前租户用户和系统操作形成的审计记录。

## 查询操作审计
按时间、用户、Action、执行结果或资源筛选记录。

## 查看审计详情
打开记录查看请求标识、追踪标识、操作结果、资源引用和安全摘要，用于问题定位与责任追踪。

> 审计页面只展示受控摘要，不用于恢复敏感请求参数或凭证。
