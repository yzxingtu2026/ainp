import { request } from '../request';

export function fetchCodeSets(keys: string[] = []) {
  return request<Api.CodeSet.ListResponse>({
    url: '/api/v1/codesets',
    params: keys.length > 0 ? { key: keys } : undefined
  });
}
