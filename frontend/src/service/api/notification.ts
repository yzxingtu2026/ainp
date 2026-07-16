import { request } from '../request';

/** Fetch notification list (paginated) */
export function fetchNotifications(params: {
  unread_only?: boolean;
  exclude_archived?: boolean;
  current?: number;
  size?: number;
}) {
  return request<Api.Notification.ListData>({
    url: '/api/v1/notifications/list',
    method: 'post',
    data: params
  });
}

/** Fetch unread notification count */
export function fetchUnreadCount() {
  return request<Api.Notification.UnreadCountData>({
    url: '/api/v1/notifications/unread-count',
    method: 'post'
  });
}

/** Mark a single notification as read */
export function fetchMarkRead(id: string) {
  return request<Api.Notification.MarkReadData>({
    url: '/api/v1/notifications/mark-read',
    method: 'post',
    data: { id }
  });
}

/** Mark all notifications as read */
export function fetchMarkAllRead() {
  return request<Api.Notification.MarkAllReadData>({
    url: '/api/v1/notifications/mark-all-read',
    method: 'post'
  });
}

/** Archive a notification */
export function fetchArchiveNotification(id: string) {
  return request({
    url: '/api/v1/notifications/archive',
    method: 'post',
    data: { id }
  });
}

/** Unarchive a notification */
export function fetchUnarchiveNotification(id: string) {
  return request({
    url: '/api/v1/notifications/unarchive',
    method: 'post',
    data: { id }
  });
}

// ── Admin: Template override management ──

/** List template overrides */
export function fetchNotificationTemplates() {
  return request<Api.Notification.TemplateListData>({
    url: '/api/v1/system/notification/templates',
    method: 'post'
  });
}

/** Update or create a template override */
export function fetchUpdateNotificationTemplate(params: {
  definition_key: string;
  channel: string;
  locale: string;
  subject: string;
  body: string;
  enabled?: boolean;
}) {
  return request({
    url: '/api/v1/system/notification/templates/update',
    method: 'post',
    data: params
  });
}

/** Reset a template override to code default */
export function fetchResetNotificationTemplate(params: {
  definition_key: string;
  channel: string;
  locale: string;
}) {
  return request({
    url: '/api/v1/system/notification/templates/reset',
    method: 'post',
    data: params
  });
}

/** Preview a template with variables (no delivery) */
export function fetchPreviewNotificationTemplate(params: {
  definition_key: string;
  channel: string;
  locale: string;
  variables?: Record<string, any>;
}) {
  return request<Api.Notification.PreviewData>({
    url: '/api/v1/system/notification/templates/preview',
    method: 'post',
    data: params
  });
}

// ── Admin: Delivery management ──

/** List failed deliveries */
export function fetchDeliveryList(params: { current?: number; size?: number }) {
  return request<Api.Notification.DeliveryListData>({
    url: '/api/v1/system/notification/deliveries',
    method: 'post',
    data: params
  });
}

/** Retry a failed delivery */
export function fetchRetryDelivery(id: string) {
  return request({
    url: '/api/v1/system/notification/deliveries/retry',
    method: 'post',
    data: { id }
  });
}
