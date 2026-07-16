import { request } from '../request';

export function fetchJobList(data: Api.Operations.PageQuery & { job_type?: string; status?: string }) {
  return request<Api.Operations.JobListData>({ url: '/api/v1/system/jobs/list', method: 'post', data });
}

export function fetchJobDetail(id: string) {
  return request<{ job: Api.Operations.JobRun }>({ url: '/api/v1/system/jobs/get', method: 'post', data: { id } });
}

export function fetchCancelJob(id: string) {
  return request({ url: '/api/v1/system/jobs/cancel', method: 'post', data: { id } });
}

export function fetchRetryJob(id: string) {
  return request({ url: '/api/v1/system/jobs/retry', method: 'post', data: { id } });
}

export function fetchOutboxList(data: Api.Operations.PageQuery & { event_type?: string; status?: string }) {
  return request<Api.Operations.OutboxListData>({ url: '/api/v1/system/outbox/list', method: 'post', data });
}

export function fetchOutboxDetail(eventId: string) {
  return request<Api.Operations.OutboxDetail>({
    url: '/api/v1/system/outbox/deliveries',
    method: 'post',
    data: { event_id: eventId }
  });
}

export function fetchRetryOutbox(eventId: string) {
  return request({ url: '/api/v1/system/outbox/retry', method: 'post', data: { event_id: eventId } });
}
