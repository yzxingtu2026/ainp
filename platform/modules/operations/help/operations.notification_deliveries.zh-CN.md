---
menu_id: operations.notification_deliveries
version: 1.0.0
locale: zh-CN
use_cases:
  - 查询通知投递
  - 查看投递错误
  - 重试通知投递
permission_refs:
  - operations.notification_delivery_list
---
# 通知投递

## 页面用途
查看当前租户通知通过站内、邮件等渠道的投递状态。

## 查询通知投递
按渠道、状态和时间筛选记录，打开详情查看通知定义、收件目标和错误摘要。

## 查看投递错误
检查失败阶段、尝试次数和最近错误，先处理渠道配置或目标地址问题。

## 重试通知投递
对允许重试的失败记录发起重试。系统复用原通知事实，不允许在此修改正文或收件人。

> 重试可能再次触达外部收件人，提交前必须确认。
