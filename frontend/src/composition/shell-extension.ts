import type { Component } from 'vue';
import type { ShellExtensionRegistration } from './contract';

export async function loadShellExtensionComponent(registration: ShellExtensionRegistration): Promise<Component> {
  const loaded = await registration.component();
  return 'default' in loaded ? loaded.default : loaded;
}
