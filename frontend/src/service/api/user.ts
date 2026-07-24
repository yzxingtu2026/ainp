import { request } from '../request';

const USER_API = {
  list: '/api/v1/system/users/list',
  get: '/api/v1/system/users/get',
  create: '/api/v1/system/users/create',
  update: '/api/v1/system/users/update',
  statusUpdate: '/api/v1/system/users/status/update',
  rolesUpdate: '/api/v1/system/users/roles/update',
  passwordReset: '/api/v1/system/users/password/reset'
} as const;

export function fetchSelfProfile() {
  return request<Api.User.SelfProfile>({ url: '/api/v1/system/profile' });
}

export function updateSelfProfile(payload: { display_name: string; avatar_attachment_id: string }) {
  return request<Api.User.SelfProfile>({
    url: '/api/v1/system/profile/update',
    method: 'post',
    data: payload
  });
}

export function fetchUserPreferences() {
  return request<Api.Setting.Definition>({ url: '/api/v1/system/profile/preferences' });
}

export function fetchMCPConnectionInfo() {
  return request<Api.User.MCPConnectionInfo>({ url: '/api/v1/system/profile/mcp-connection' });
}

export function fetchMCPGrants() {
  return request<Api.User.MCPGrant[]>({ url: '/api/v1/mcp/grants' });
}

export function revokeMCPGrants(data: { grant_id?: string; client_id?: string; all?: boolean }) {
  return request<{ status: string }>({ url: '/api/v1/mcp/grants/revoke', method: 'post', data });
}

export function fetchMCPConfirmation(token: string) {
  return request<Api.User.MCPConfirmation>({ url: '/api/v1/mcp/confirmations/info', params: { token } });
}

export function decideMCPConfirmation(token: string, decision: 'approve' | 'reject') {
  return request<Api.User.MCPConfirmation>({
    url: '/api/v1/mcp/confirmations/decision',
    method: 'post',
    data: { token, decision }
  });
}

export function updateUserPreferences(revision: number, values: Record<string, unknown>) {
  return request<Api.Setting.Definition>({
    url: '/api/v1/system/profile/preferences/update',
    method: 'post',
    data: { revision, values }
  });
}

export function fetchUserList(params: Api.User.ListParams) {
  return request<Api.User.ListResult>({ url: USER_API.list, method: 'post', data: params });
}

export function fetchUser(id: string) {
  return request<Api.User.Record>({ url: USER_API.get, method: 'post', data: { id } });
}

export function createUser(data: Api.User.CreateParams) {
  return request<Api.User.Record>({ url: USER_API.create, method: 'post', data });
}

export function updateUser(data: Api.User.UpdateParams) {
  return request<Api.User.Record>({ url: USER_API.update, method: 'post', data });
}

export function updateUserStatus(id: string, status: Api.User.ActivationStatus) {
  return request<{ id: string; status: Api.User.ActivationStatus }>({
    url: USER_API.statusUpdate,
    method: 'post',
    data: { id, status }
  });
}

export function updateUserRoles(id: string, roleIds: string[]) {
  return request<{ id: string; roles: Api.User.RoleBrief[] }>({
    url: USER_API.rolesUpdate,
    method: 'post',
    data: { id, role_ids: roleIds }
  });
}

export function resetUserPassword(id: string, password: string) {
  return request<{ id: string }>({
    url: USER_API.passwordReset,
    method: 'post',
    data: { id, password }
  });
}
