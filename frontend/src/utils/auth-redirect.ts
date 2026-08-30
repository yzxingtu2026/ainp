const METHOD_PATTERN = /^[a-z][a-z0-9_-]{0,63}$/;
const ICON_PATTERN = /^[a-z0-9]+:[a-z0-9][a-z0-9-]*$/;
const CALLBACK_STORAGE_KEY = 'ainp.auth.redirect';

export interface RedirectAuthContext {
  method: string;
  challengeId: string;
  redirect: string;
}

export function isSafeInternalRedirect(value: unknown): value is string {
  if (typeof value !== 'string' || !value.startsWith('/') || value.startsWith('//')) return false;
  try {
    const parsed = new URL(value, 'https://ainp.invalid');
    return parsed.origin === 'https://ainp.invalid' && !parsed.username && !parsed.password;
  } catch {
    return false;
  }
}

export function safeInternalRedirect(value: unknown): string {
  return isSafeInternalRedirect(value) ? value : '';
}

export function safeAuthorizeURL(value: unknown): string {
  if (typeof value !== 'string') return '';
  try {
    const parsed = new URL(value);
    if (!['https:', 'http:'].includes(parsed.protocol) || parsed.username || parsed.password) return '';
    if (parsed.protocol === 'http:' && !['localhost', '127.0.0.1', '[::1]'].includes(parsed.hostname)) return '';
    return parsed.toString();
  } catch {
    return '';
  }
}

export function normalizeRedirectMethods(methods: readonly Api.Auth.AuthMethod[]): Api.Auth.AuthMethod[] {
  const seen = new Set<string>();
  return methods
    .filter(item => {
      const { method, capabilities } = item;
      if (!METHOD_PATTERN.test(method) || seen.has(method) || !capabilities.redirect || !capabilities.challenge)
        return false;
      if (capabilities.icon && !ICON_PATTERN.test(capabilities.icon)) return false;
      if (capabilities.order !== undefined && (!Number.isSafeInteger(capabilities.order) || capabilities.order < 0))
        return false;
      seen.add(method);
      return true;
    })
    .sort(
      (a, b) => (a.capabilities.order ?? 1000) - (b.capabilities.order ?? 1000) || a.method.localeCompare(b.method)
    );
}

export function saveRedirectAuthContext(context: RedirectAuthContext) {
  sessionStorage.setItem(CALLBACK_STORAGE_KEY, JSON.stringify(context));
}

export function takeRedirectAuthContext(): RedirectAuthContext | null {
  const raw = sessionStorage.getItem(CALLBACK_STORAGE_KEY);
  sessionStorage.removeItem(CALLBACK_STORAGE_KEY);
  if (!raw) return null;
  try {
    const parsed = JSON.parse(raw) as Partial<RedirectAuthContext>;
    if (!METHOD_PATTERN.test(parsed.method || '') || typeof parsed.challengeId !== 'string') return null;
    return { method: parsed.method!, challengeId: parsed.challengeId, redirect: safeInternalRedirect(parsed.redirect) };
  } catch {
    return null;
  }
}

export function isValidAuthMethod(method: unknown): method is string {
  return typeof method === 'string' && METHOD_PATTERN.test(method);
}
