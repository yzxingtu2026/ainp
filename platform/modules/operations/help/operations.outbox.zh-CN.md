---
menu_id: operations.outbox
version: 1.0.0
locale: zh-CN
use_cases:
  - 查询 Outbox 事件
  - 查看投递状态
  - 重试失败事件
permission_refs:
  - operations.outbox_list
---
# Outbox 事件

## 页面用途
查看当前租户可靠事件的发布状态及各 Handler 投递结果。

## 查询 Outbox 事件
按事件类型、状态和时间筛选，打开详情核对事件标识、尝试次数与错误摘要。

## 查看投递状态
在事件详情中查看各 Handler 的状态、尝试次数和最近错误。

## 重试失败事件
仅对失败或终止事件执行重试。确认后事件恢复为待处理，并继续使用已持久化载荷。

> 重试可能再次触发下游业务或外部投递，请先确认失败原因已消除。
