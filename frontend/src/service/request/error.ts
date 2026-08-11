import { isAxiosError } from 'axios';
import type { AxiosError } from 'axios';
import type { CustomAxiosRequestConfig, RequestErrorMode } from '@sa/axios';
import type { AinPEnvelope } from './type';

type ErrorConfig = CustomAxiosRequestConfig & { errorMode?: RequestErrorMode };

export function backendMessage(payload?: AinPEnvelope | null): string {
  const message = payload?.message || payload?.msg || '';
  return typeof message === 'string' ? message.trim() : '';
}

export function getRequestErrorMode(error: unknown): RequestErrorMode {
  if (!isAxiosError(error)) return 'global';
  return ((error as AxiosError).config as ErrorConfig | undefined)?.errorMode || 'global';
}

export function shouldShowGlobalRequestError(error: unknown): boolean {
  return getRequestErrorMode(error) === 'global';
}

/** Returns only a localized backend envelope message, never Axios' technical message. */
export function getRequestErrorMessage(error: unknown, fallback: string): string {
  if (!isAxiosError<AinPEnvelope>(error)) return fallback;
  return backendMessage(error.response?.data) || fallback;
}

export function isRequestError(error: unknown): boolean {
  return isAxiosError(error);
}

/** For explicit non-unified chains such as fetch/download; Axios text is still never exposed. */
export function getStandaloneErrorMessage(error: unknown, fallback: string): string {
  if (isAxiosError(error)) return getRequestErrorMessage(error, fallback);
  if (error instanceof Error && error.message.trim()) return error.message.trim();
  return fallback;
}
