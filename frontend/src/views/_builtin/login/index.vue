<script setup lang="ts">
import { computed } from 'vue';
import type { Component } from 'vue';
import { loginModuleRecord } from '@/constants/app';
import { useAppStore } from '@/store/modules/app';
import { useThemeStore } from '@/store/modules/theme';
import { $t } from '@/locales';
import PwdLogin from './modules/pwd-login.vue';
import CodeLogin from './modules/code-login.vue';
import Register from './modules/register.vue';
import ResetPwd from './modules/reset-pwd.vue';
import BindWechat from './modules/bind-wechat.vue';

interface Props {
  /** The login module */
  module?: UnionKey.LoginModule;
}

const props = defineProps<Props>();

const appStore = useAppStore();
const themeStore = useThemeStore();

interface LoginModule {
  label: App.I18n.I18nKey;
  component: Component;
}

const moduleMap: Record<UnionKey.LoginModule, LoginModule> = {
  'pwd-login': { label: loginModuleRecord['pwd-login'], component: PwdLogin },
  'code-login': { label: loginModuleRecord['code-login'], component: CodeLogin },
  register: { label: loginModuleRecord.register, component: Register },
  'reset-pwd': { label: loginModuleRecord['reset-pwd'], component: ResetPwd },
  'bind-wechat': { label: loginModuleRecord['bind-wechat'], component: BindWechat }
};

const activeModule = computed(() => moduleMap[props.module || 'pwd-login']);
</script>

<template>
  <div class="login-page" :class="{ 'is-dark': themeStore.darkMode }">
    <aside class="login-visual" aria-hidden="true">
      <img class="visual-image" src="/brand/ainp-login-visual.png" alt="" />
    </aside>

    <section class="login-panel">
      <div class="login-toolbar">
        <ThemeSchemaSwitch
          :theme-schema="themeStore.themeScheme"
          :show-tooltip="false"
          class="toolbar-control"
          @switch="themeStore.toggleThemeScheme"
        />
        <span v-if="themeStore.header.multilingual.visible" class="toolbar-divider"></span>
        <LangSwitch
          v-if="themeStore.header.multilingual.visible"
          :lang="appStore.locale"
          :lang-options="appStore.localeOptions"
          :show-tooltip="false"
          class="toolbar-control"
          @change-lang="appStore.changeLocale"
        />
      </div>

      <div class="login-content">
        <header class="brand-header">
          <SystemLogo class="brand-logo" />
          <span class="brand-name">{{ $t('system.title') }}</span>
        </header>

        <main class="login-main">
          <div class="login-heading">
            <h1>{{ $t(activeModule.label) }}</h1>
            <p>{{ $t('page.login.common.subtitle') }}</p>
          </div>

          <Transition :name="themeStore.page.animateMode" mode="out-in" appear>
            <component :is="activeModule.component" />
          </Transition>
        </main>

        <footer class="login-footer">AINP</footer>
      </div>
    </section>
  </div>
</template>

<style scoped>
.login-page {
  --login-panel: #f7f8fb;
  --login-surface: rgba(255, 255, 255, 0.72);
  --login-border: rgba(15, 23, 42, 0.12);
  --login-text: #111827;
  --login-muted: #64748b;
  min-height: 100%;
  display: grid;
  grid-template-columns: minmax(0, 1.18fr) minmax(480px, 0.82fr);
  overflow: hidden;
  color: var(--login-text);
  background: #090d15;
}

.login-page.is-dark {
  --login-panel: #0d121c;
  --login-surface: rgba(17, 24, 39, 0.64);
  --login-border: rgba(148, 163, 184, 0.18);
  --login-text: #f8fafc;
  --login-muted: #8b98ab;
}

.login-visual {
  position: relative;
  min-width: 0;
  overflow: hidden;
  isolation: isolate;
  background: #e9edf5;
}

.visual-image {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  object-fit: cover;
  object-position: 50% 48%;
  transition: filter 200ms ease;
}

.is-dark .visual-image {
  filter: brightness(0.38) saturate(1.08) contrast(1.06);
}

.login-panel {
  position: relative;
  min-width: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 88px clamp(48px, 6vw, 104px) 40px;
  background: var(--login-panel);
  border-left: 1px solid var(--login-border);
  transition:
    background-color 200ms ease,
    color 200ms ease;
}

.login-toolbar {
  position: absolute;
  top: 30px;
  right: clamp(28px, 4vw, 56px);
  display: flex;
  align-items: center;
  gap: 12px;
  color: var(--login-muted);
}

.toolbar-control {
  font-size: 19px;
}

.toolbar-divider {
  width: 1px;
  height: 18px;
  background: var(--login-border);
}

.login-content {
  width: min(100%, 440px);
}

.brand-header {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 58px;
}

.brand-logo {
  width: 44px;
  height: 44px;
  flex: 0 0 44px;
}

.brand-name {
  font-size: 24px;
  line-height: 1;
  font-weight: 650;
  color: var(--login-text);
}

.login-main {
  width: 100%;
}

.login-heading {
  margin-bottom: 30px;
}

.login-heading h1 {
  margin: 0;
  font-size: 34px;
  line-height: 1.25;
  font-weight: 650;
  color: var(--login-text);
}

.login-heading p {
  margin: 10px 0 0;
  font-size: 14px;
  line-height: 1.7;
  color: var(--login-muted);
}

.login-footer {
  margin-top: 54px;
  padding-top: 20px;
  border-top: 1px solid var(--login-border);
  font-size: 12px;
  color: var(--login-muted);
  letter-spacing: 0;
}

:deep(.n-form-item) {
  margin-bottom: 6px;
}

:deep(.n-input),
:deep(.n-button) {
  border-radius: 6px;
}

:deep(.n-input) {
  background: var(--login-surface);
}

:deep(.n-button--primary-type) {
  min-height: 46px;
  font-weight: 600;
}

@media (max-width: 980px) {
  .login-page {
    grid-template-columns: minmax(0, 0.9fr) minmax(440px, 1.1fr);
  }
}

@media (max-width: 760px) {
  .login-page {
    display: block;
    min-height: 100%;
    background: var(--login-panel);
  }

  .login-visual {
    display: none;
  }

  .login-panel {
    min-height: 100%;
    padding: 88px 24px 32px;
    border-left: 0;
  }

  .login-toolbar {
    top: 24px;
    right: 24px;
  }

  .brand-header {
    margin-bottom: 44px;
  }

  .login-heading h1 {
    font-size: 30px;
  }

  .login-footer {
    margin-top: 40px;
  }
}

@media (max-height: 720px) and (min-width: 761px) {
  .login-panel {
    align-items: flex-start;
    overflow-y: auto;
    padding-top: 76px;
  }

  .brand-header {
    margin-bottom: 34px;
  }

  .login-footer {
    margin-top: 32px;
  }
}
</style>
