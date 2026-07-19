import { request } from '../request';
const base = '/api/v1/ai/memories';
export const fetchMemoryList = (data: Record<string, unknown>) =>
  request<Api.Memory.ListResult>({ url: `${base}/list`, method: 'post', data });
export const fetchMemory = (id: string) =>
  request<Api.Memory.Item>({ url: `${base}/get`, method: 'post', data: { id } });
export const fetchMemoryProfile = () =>
  request<Api.Memory.Profile>({ url: `${base}/profile`, method: 'post', data: {} });
const mutation = (url: string, data: Record<string, unknown>) =>
  request({ url, method: 'post', data, _idempotencyKey: crypto.randomUUID() });
export const confirmMemory = (id: string, revision: number) => mutation(`${base}/confirm`, { id, revision });
export const rejectMemory = (id: string, revision: number) => mutation(`${base}/reject`, { id, revision });
export const deleteMemory = (id: string, revision: number) => mutation(`${base}/delete`, { id, revision });
export const correctMemory = (data: Record<string, unknown>) => mutation(`${base}/correct`, data);
export const clearMemories = (data: Record<string, unknown>) => mutation(`${base}/clear`, data);
