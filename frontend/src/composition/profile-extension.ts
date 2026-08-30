import type { Component } from 'vue';
import type { ProfileExtensionRegistration } from './contract';
import { RESERVED_PROFILE_TAB_IDS } from './define-app';

export const DEFAULT_PROFILE_TAB_ID = 'basic';

export function resolveProfileTabId(
  requested: unknown,
  extensions: readonly Pick<ProfileExtensionRegistration, 'id'>[]
): string {
  const value = typeof requested === 'string' ? requested : '';
  if (RESERVED_PROFILE_TAB_IDS.includes(value as (typeof RESERVED_PROFILE_TAB_IDS)[number])) return value;
  return extensions.some(extension => extension.id === value) ? value : DEFAULT_PROFILE_TAB_ID;
}

export function shouldReplaceProfileTabQuery(requested: unknown, resolved: string): boolean {
  return requested !== undefined && requested !== resolved;
}

export async function loadProfileExtensionComponent(registration: ProfileExtensionRegistration): Promise<Component> {
  const loaded = await registration.component();
  return 'default' in loaded ? loaded.default : loaded;
}
