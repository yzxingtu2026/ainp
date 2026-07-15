import { request } from '../request';

const SETTING_API = {
  list: '/api/v1/system/settings/list',
  get: '/api/v1/system/settings/get',
  update: '/api/v1/system/settings/update',
  reset: '/api/v1/system/settings/reset'
} as const;

export function fetchSettingList() {
  return request<Api.Setting.ListResult>({ url: SETTING_API.list, method: 'post', data: {} });
}

export function fetchSetting(key: string) {
  return request<Api.Setting.Definition>({ url: SETTING_API.get, method: 'post', data: { key } });
}

export function updateSetting(key: string, revision: number, values: Record<string, unknown>) {
  return request<Api.Setting.Definition>({
    url: SETTING_API.update,
    method: 'post',
    data: { key, revision, values }
  });
}

export function resetSetting(key: string, revision: number) {
  return request<Api.Setting.Definition>({
    url: SETTING_API.reset,
    method: 'post',
    data: { key, revision }
  });
}
