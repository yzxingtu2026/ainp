import { request } from '../request';

export function fetchSelfProfile() {
  return request<Api.Profile.SelfProfile>({ url: '/api/v1/system/profile' });
}

export function updateSelfProfile(payload: { display_name: string; avatar_attachment_id: string }) {
  return request<Api.Profile.SelfProfile>({
    url: '/api/v1/system/profile/update',
    method: 'post',
    data: payload
  });
}

export function updateSelfPassword(payload: { current_password: string; new_password: string }) {
  return request<{ reauthentication_required: boolean }>({
    url: '/api/v1/system/profile/password/update',
    method: 'post',
    data: payload
  });
}

export function fetchUserPreferences() {
  return request<Api.Setting.Definition>({ url: '/api/v1/system/profile/preferences' });
}

export function fetchMCPConnectionInfo() {
  return request<Api.Profile.MCPConnectionInfo>({ url: '/api/v1/system/profile/mcp-connection' });
}

export function fetchMCPGrants() {
  return request<Api.Profile.MCPGrant[]>({ url: '/api/v1/mcp/grants' });
}

export function revokeMCPGrants(data: { grant_id?: string; client_id?: string; all?: boolean }) {
  return request<{ status: string }>({ url: '/api/v1/mcp/grants/revoke', method: 'post', data });
}

export function fetchMCPConfirmation(token: string) {
  return request<Api.Profile.MCPConfirmation>({ url: '/api/v1/mcp/confirmations/info', params: { token } });
}

export function decideMCPConfirmation(token: string, decision: 'approve' | 'reject') {
  return request<Api.Profile.MCPConfirmation>({
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
