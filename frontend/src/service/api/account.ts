import { request } from '../request';

const ACCOUNT_API = {
  list: '/api/v1/system/accounts/list',
  get: '/api/v1/system/accounts/get',
  create: '/api/v1/system/accounts/create',
  update: '/api/v1/system/accounts/update',
  statusUpdate: '/api/v1/system/accounts/status/update',
  rolesUpdate: '/api/v1/system/accounts/roles/update',
  passwordReset: '/api/v1/system/accounts/password/reset'
} as const;

export function fetchAccountList(params: Api.Account.ListParams) {
  return request<Api.Account.ListResult>({ url: ACCOUNT_API.list, method: 'post', data: params });
}

export function fetchAccount(id: string) {
  return request<Api.Account.Record>({ url: ACCOUNT_API.get, method: 'post', data: { id } });
}

export function createAccount(data: Api.Account.CreateParams) {
  return request<Api.Account.Record>({ url: ACCOUNT_API.create, method: 'post', data });
}

export function updateAccount(data: Api.Account.UpdateParams) {
  return request<Api.Account.Record>({ url: ACCOUNT_API.update, method: 'post', data });
}

export function updateAccountStatus(id: string, status: Api.Account.ActivationStatus) {
  return request<{ id: string; status: Api.Account.ActivationStatus }>({
    url: ACCOUNT_API.statusUpdate,
    method: 'post',
    data: { id, status }
  });
}

export function updateAccountRoles(id: string, roleIds: string[]) {
  return request<{ id: string; roles: Api.Account.RoleBrief[] }>({
    url: ACCOUNT_API.rolesUpdate,
    method: 'post',
    data: { id, role_ids: roleIds }
  });
}

export function resetAccountPassword(id: string, password: string) {
  return request<{ id: string }>({
    url: ACCOUNT_API.passwordReset,
    method: 'post',
    data: { id, password }
  });
}
