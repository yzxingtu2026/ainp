import { request } from '../request';

export function fetchJobList(data: Api.Operations.PageQuery & { job_type?: string; status?: string }) {
  return request<Api.Operations.JobListData>({ url: '/api/v1/operations/jobs/list', method: 'post', data });
}

export function fetchJobDetail(id: string) {
  return request<{ job: Api.Operations.JobRun }>({ url: '/api/v1/operations/jobs/get', method: 'post', data: { id } });
}

export function fetchCancelJob(id: string) {
  return request({ url: '/api/v1/operations/jobs/cancel', method: 'post', data: { id } });
}

export function fetchRetryJob(id: string) {
  return request({ url: '/api/v1/operations/jobs/retry', method: 'post', data: { id } });
}

export function fetchOutboxList(data: Api.Operations.PageQuery & { event_type?: string; status?: string }) {
  return request<Api.Operations.OutboxListData>({ url: '/api/v1/operations/outbox/list', method: 'post', data });
}

export function fetchOutboxDetail(eventId: string) {
  return request<Api.Operations.OutboxDetail>({
    url: '/api/v1/operations/outbox/deliveries',
    method: 'post',
    data: { event_id: eventId }
  });
}

export function fetchRetryOutbox(eventId: string) {
  return request({ url: '/api/v1/operations/outbox/retry', method: 'post', data: { event_id: eventId } });
}

export function fetchDeliveryList(params: { current?: number; size?: number; status?: string; channel?: string }) {
  return request<Api.Operations.DeliveryListData>({
    url: '/api/v1/operations/notification-deliveries/list',
    method: 'post',
    data: params
  });
}

export function fetchDeliveryDetail(id: string) {
  return request<Api.Operations.DeliveryDetail>({
    url: '/api/v1/operations/notification-deliveries/get',
    method: 'post',
    data: { id }
  });
}

export function fetchRetryDelivery(id: string) {
  return request({
    url: '/api/v1/operations/notification-deliveries/retry',
    method: 'post',
    data: { id }
  });
}
