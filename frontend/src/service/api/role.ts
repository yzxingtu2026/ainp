import { request } from '../request';

const ROLE_API = {
  list: '/api/v1/system/roles/list',
  get: '/api/v1/system/roles/get',
  create: '/api/v1/system/roles/create',
  update: '/api/v1/system/roles/update',
  delete: '/api/v1/system/roles/delete',
  permissionsUpdate: '/api/v1/system/roles/permissions/update',
  permissionCatalog: '/api/v1/system/permissions/catalog'
} as const;

export function fetchRoleList(params: Api.Role.ListParams) {
  return request<Api.Role.ListResult>({ url: ROLE_API.list, method: 'post', data: params });
}

export function fetchRole(id: string) {
  return request<Api.Role.Record>({ url: ROLE_API.get, method: 'post', data: { id } });
}

export function createRole(data: Pick<Api.Role.Record, 'code' | 'name' | 'description' | 'status'> & { permissions: string[] }) {
  return request<Api.Role.Record>({ url: ROLE_API.create, method: 'post', data });
}

export function updateRole(data: Pick<Api.Role.Record, 'id' | 'name' | 'description' | 'status'>) {
  return request<Api.Role.Record>({ url: ROLE_API.update, method: 'post', data });
}

export function deleteRole(id: string) {
  return request<{ id: string }>({ url: ROLE_API.delete, method: 'post', data: { id } });
}

export function updateRolePermissions(id: string, permissions: string[]) {
  return request<{ id: string; permissions: string[] }>({
    url: ROLE_API.permissionsUpdate,
    method: 'post',
    data: { id, permissions }
  });
}

export function fetchPermissionCatalog() {
  return request<{ groups: Api.Role.PermissionGroup[] }>({ url: ROLE_API.permissionCatalog, method: 'post', data: {} });
}
