import { request } from '../request';

const SETTING_API = {
  list: '/api/v1/system/settings/list',
  get: '/api/v1/system/settings/get',
  update: '/api/v1/system/settings/update',
  secretUpdate: '/api/v1/system/settings/secret/update',
  reset: '/api/v1/system/settings/reset',
  toolIndexStatus: '/api/v1/ai/tools/index/status',
  toolIndexRebuild: '/api/v1/ai/tools/index/rebuild',
  navigationIndexStatus: '/api/v1/ai/navigation/index/status',
  navigationIndexRebuild: '/api/v1/ai/navigation/index/rebuild'
} as const;

export function fetchSettingList() {
  return request<Api.Setting.ListResult>({ url: SETTING_API.list, method: 'post', data: {} });
}

export function fetchNavigationIndexStatus() {
  return request<Api.Setting.NavigationIndexStatus>({
    url: SETTING_API.navigationIndexStatus,
    method: 'post',
    data: {}
  });
}

export function fetchToolIndexStatus() {
  return request<Api.Setting.ToolIndexStatus>({ url: SETTING_API.toolIndexStatus, method: 'post', data: {} });
}

export function rebuildToolIndex() {
  return request<Api.Setting.ToolIndexStatus>({ url: SETTING_API.toolIndexRebuild, method: 'post', data: {} });
}

export function rebuildNavigationIndex() {
  return request<Api.Setting.NavigationIndexStatus>({
    url: SETTING_API.navigationIndexRebuild,
    method: 'post',
    data: {}
  });
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

export function updateSettingSecret(
  key: string,
  revision: number,
  values: Record<string, unknown>,
  secrets: Record<string, string>,
  clearSecretFields: string[]
) {
  return request<Api.Setting.Definition>({
    url: SETTING_API.secretUpdate,
    method: 'post',
    data: { key, revision, values, secrets, clear_secret_fields: clearSecretFields }
  });
}

export function resetSetting(key: string, revision: number) {
  return request<Api.Setting.Definition>({
    url: SETTING_API.reset,
    method: 'post',
    data: { key, revision }
  });
}
