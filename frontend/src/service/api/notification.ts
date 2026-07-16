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
