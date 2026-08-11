import { request } from '../request';

/** Setup 状态检查 */
export function fetchSetupStatus() {
  return request<Api.Setup.Status>({
    url: '/api/v1/setup/status'
  });
}

/** 环境检查 */
export function fetchSetupCheckEnv() {
  return request<Api.Setup.CheckEnv>({
    url: '/api/v1/setup/check-env',
    errorMode: 'inline'
  });
}

/** 测试数据库连通性 */
export function fetchSetupTestDB(dsn?: string) {
  return request<Api.Setup.TestDBResult>({
    url: '/api/v1/setup/test-db',
    method: 'post',
    data: { dsn: dsn || '' },
    errorMode: 'inline'
  });
}

/** 测试 Redis 连通性 */
export function fetchSetupTestRedis(addr: string, db: number) {
  return request<Api.Setup.TestDBResult>({
    url: '/api/v1/setup/test-redis',
    method: 'post',
    data: { addr, db },
    errorMode: 'inline'
  });
}

/** 初始化系统 */
export function fetchSetupInitialize(params: Api.Setup.InitParams) {
  return request<Api.Setup.InitResult>({
    url: '/api/v1/setup/initialize',
    method: 'post',
    data: params,
    errorMode: 'inline'
  });
}
