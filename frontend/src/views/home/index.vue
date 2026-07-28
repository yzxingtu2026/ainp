<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { fetchAIToolConfirmations } from '@/service/api';
import { useAuthStore } from '@/store/modules/auth';
import { useRouteStore } from '@/store/modules/route';
import { useTabStore } from '@/store/modules/tab';
import { formatDateTime } from '@/utils/datetime';
import { $t } from '@/locales';
import { useAssistantPersonalization } from '@/composables/use-assistant-personalization';
import HomeRobot from './modules/home-robot.vue';

defineOptions({ name: 'Home' });

type RobotMode = 'thinking' | 'jumping' | null;

interface ContinueItem {
  key: string;
  title: string;
  description: string;
  path: string;
  kind: 'confirmation' | 'page';
  icon?: string;
  menuIcon?: App.Global.Menu['icon'];
  meta?: string;
  risk?: string;
}

const router = useRouter();
const authStore = useAuthStore();
const routeStore = useRouteStore();
const tabStore = useTabStore();
const personalization = useAssistantPersonalization();
const confirmations = ref<Api.AI.ToolConfirmation[]>([]);
const robotMode = ref<RobotMode>(null);
const navigating = ref(false);
let navigationTimer: number | null = null;

const greeting = computed(() =>
  personalization.greeting.value || $t('page.aiWorkspace.greeting', { name: authStore.userInfo.userName || $t('page.home.defaultUser') })
);

function firstTrustedTarget(menu: App.Global.Menu): App.Global.Menu | null {
  if (menu.children?.length) {
    for (const child of menu.children) {
      const target = firstTrustedTarget(child);
      if (target) return target;
    }
  }
  return menu.routePath && String(menu.routePath).startsWith('/') ? menu : null;
}

function findMenuTrail(routeKey: string) {
  const trail: App.Global.Menu[] = [];

  function visit(items: App.Global.Menu[]): boolean {
    for (const item of items) {
      trail.push(item);
      if (item.routeKey === routeKey) return true;
      if (item.children?.length && visit(item.children)) return true;
      trail.pop();
    }
    return false;
  }

  return visit(routeStore.menus) ? [...trail] : [];
}

const modules = computed(() =>
  routeStore.menus
    .filter(menu => menu.children?.length)
    .map(menu => {
      const target = firstTrustedTarget(menu);
      return target
        ? {
            key: menu.key,
            title: menu.label,
            description: menu
              .children!.slice(0, 3)
              .map(child => child.label)
              .join(' · '),
            icon: menu.icon,
            path: String(target.routePath)
          }
        : null;
    })
    .filter((item): item is NonNullable<typeof item> => Boolean(item))
);

const continueItems = computed<ContinueItem[]>(() => {
  const pending: ContinueItem[] = confirmations.value
    .filter(item => item.status === 'pending')
    .slice(0, 2)
    .map(item => ({
      key: `confirmation:${item.id}`,
      title: $t('page.home.confirmationTitle'),
      description: item.tool_id,
      path: '/ai/workspace',
      kind: 'confirmation',
      icon: 'mdi:shield-check-outline',
      meta: formatDateTime(item.created_at, 'relative'),
      risk: item.risk
    }));

  const pages: ContinueItem[] = [...tabStore.tabs].reverse().flatMap(tab => {
    if (tab.routeKey === 'home') return [];
    const trail = findMenuTrail(tab.routeKey);
    const menu = trail.at(-1);
    if (!menu) return [];
    return [
      {
        key: `page:${tab.id}`,
        title: tab.newLabel || tab.label,
        description:
          trail
            .slice(0, -1)
            .map(item => item.label)
            .join(' / ') || $t('page.home.openPage'),
        path: tab.fullPath,
        kind: 'page' as const,
        menuIcon: menu.icon,
        meta: $t('page.home.opened')
      }
    ];
  });

  return [...pending, ...pages].slice(0, 4);
});

function riskLabel(risk?: string) {
  if (risk === 'high') return $t('page.home.riskHigh');
  if (risk === 'medium') return $t('page.home.riskMedium');
  return $t('page.home.riskLow');
}

function riskType(risk?: string) {
  if (risk === 'high') return 'error';
  if (risk === 'medium') return 'warning';
  return 'info';
}

function openItem(item: ContinueItem) {
  void router.push(item.path);
}

function openModule(path: string) {
  void router.push(path);
}

function setThinking(active: boolean) {
  if (!navigating.value) robotMode.value = active ? 'thinking' : null;
}

function startWork() {
  if (navigating.value) return;
  navigating.value = true;
  personalization.freeze();
  robotMode.value = 'jumping';
  const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  navigationTimer = window.setTimeout(() => void router.push('/ai/workspace'), reducedMotion ? 0 : 350);
}

function openSuggestion(prompt: string) {
  personalization.choose(prompt);
  startWork();
}

async function loadContinueWork() {
  const { data } = await fetchAIToolConfirmations();
  confirmations.value = data || [];
}

onMounted(() => {
  void loadContinueWork();
  void personalization.load(`${authStore.userInfo.userId || ''}`);
});
onBeforeUnmount(() => {
  if (navigationTimer !== null) window.clearTimeout(navigationTimer);
});
</script>

<template>
  <main class="home-page">
    <section class="home-welcome">
      <div class="home-welcome__copy">
        <p class="home-welcome__eyebrow">{{ $t('page.home.workspaceLabel') }}</p>
        <h1>{{ greeting }}</h1>
        <p class="home-welcome__subtitle">{{ $t('page.home.welcomeSubtitle') }}</p>
        <NButton
          type="primary"
          size="large"
          :loading="navigating"
          :disabled="navigating"
          class="home-welcome__start"
          @mouseenter="setThinking(true)"
          @mouseleave="setThinking(false)"
          @focus="setThinking(true)"
          @blur="setThinking(false)"
          @click="startWork"
        >
          <template #icon><SvgIcon icon="mdi:play" /></template>
          {{ $t('page.home.startWork') }}
        </NButton>
        <div v-if="personalization.suggestions.value.length" class="home-welcome__suggestions">
          <button
            v-for="suggestion in personalization.suggestions.value.slice(0, 4)"
            :key="suggestion.prompt"
            type="button"
            @click="openSuggestion(suggestion.prompt)"
          >
            <span>{{ suggestion.title }}</span>
            <SvgIcon icon="mdi:arrow-up-right" />
          </button>
        </div>
      </div>
      <HomeRobot :mode="robotMode" />
    </section>

    <section v-if="continueItems.length" class="home-section" aria-labelledby="continue-work-title">
      <div class="home-section__heading">
        <div>
          <h2 id="continue-work-title">{{ $t('page.home.continueTitle') }}</h2>
          <p>{{ $t('page.home.continueSubtitle') }}</p>
        </div>
        <span>{{ continueItems.length }}</span>
      </div>

      <div class="continue-panel">
        <div class="continue-list">
          <button
            v-for="item in continueItems"
            :key="item.key"
            type="button"
            class="continue-item"
            @click="openItem(item)"
          >
            <span class="continue-item__icon" aria-hidden="true">
              <component :is="item.menuIcon" v-if="item.menuIcon" />
              <SvgIcon v-else :icon="item.icon || 'mdi:file-document-outline'" />
            </span>
            <span class="continue-item__copy">
              <span class="continue-item__title-line">
                <strong>{{ item.title }}</strong>
                <NTag v-if="item.kind === 'confirmation'" size="small" :type="riskType(item.risk)">
                  {{ riskLabel(item.risk) }}
                </NTag>
              </span>
              <small>{{ item.description }}</small>
            </span>
            <span class="continue-item__meta">{{ item.meta }}</span>
            <SvgIcon icon="mdi:arrow-right" class="continue-item__arrow" />
          </button>
        </div>
      </div>
    </section>

    <section class="home-section" aria-labelledby="home-modules-title">
      <div class="home-section__heading">
        <div>
          <h2 id="home-modules-title">{{ $t('page.home.modulesTitle') }}</h2>
          <p>{{ $t('page.home.modulesSubtitle') }}</p>
        </div>
        <span>{{ modules.length }}</span>
      </div>
      <div v-if="modules.length" class="module-grid">
        <button
          v-for="module in modules"
          :key="module.key"
          type="button"
          class="module-card"
          @click="openModule(module.path)"
        >
          <span class="module-card__icon"><component :is="module.icon" /></span>
          <span class="module-card__copy">
            <strong>{{ module.title }}</strong>
            <small>{{ module.description }}</small>
          </span>
          <SvgIcon icon="mdi:arrow-right" class="module-card__arrow" />
        </button>
      </div>
      <NEmpty v-else size="small" :description="$t('page.home.noModules')" />
    </section>
  </main>
</template>

<style scoped>
.home-page {
  min-height: 100%;
  padding: 20px clamp(16px, 3vw, 40px) 36px;
  color: rgb(var(--base-text-color));
  background: rgb(var(--layout-bg-color));
}

.home-welcome {
  min-height: 246px;
  max-width: 1240px;
  margin: 0 auto;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 40px;
  border-bottom: 1px solid rgb(var(--base-text-color) / 9%);
}

.home-welcome__copy {
  min-width: 0;
  padding: 26px 0 32px;
}

.home-welcome__eyebrow {
  margin: 0 0 10px;
  color: rgb(var(--primary-color));
  font-size: 12px;
  font-weight: 650;
}

.home-welcome h1 {
  margin: 0;
  font-size: clamp(28px, 3vw, 40px);
  line-height: 1.25;
  font-weight: 650;
  letter-spacing: 0;
}

.home-welcome__subtitle {
  margin: 12px 0 24px;
  color: rgb(var(--base-text-color) / 58%);
  font-size: 15px;
}

.home-welcome__start {
  min-width: 132px;
  border-radius: 6px;
}

.home-welcome__suggestions {
  max-width: 660px;
  margin-top: 16px;
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
}

.home-welcome__suggestions button {
  min-width: 0;
  min-height: 40px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 8px 10px;
  border: 1px solid rgb(var(--base-text-color) / 12%);
  border-radius: 6px;
  background: rgb(var(--container-bg-color));
  color: inherit;
  text-align: left;
  cursor: pointer;
}

.home-welcome__suggestions span {
  min-width: 0;
  overflow-wrap: anywhere;
}

.home-section {
  max-width: 1240px;
  margin: 30px auto 0;
}

.home-section__heading {
  min-height: 44px;
  margin-bottom: 14px;
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
}

.home-section__heading h2 {
  margin: 0;
  font-size: 20px;
  line-height: 1.35;
  font-weight: 650;
}

.home-section__heading p {
  margin: 4px 0 0;
  color: rgb(var(--base-text-color) / 48%);
  font-size: 12px;
}

.home-section__heading > span {
  color: rgb(var(--base-text-color) / 35%);
  font-size: 12px;
}

.continue-panel {
  min-height: 154px;
  padding: 8px 20px;
  display: flex;
  flex-direction: column;
  justify-content: center;
  border: 1px solid rgb(var(--base-text-color) / 10%);
  border-radius: 8px;
  background: rgb(var(--container-bg-color));
}

.continue-list {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.continue-item {
  min-width: 0;
  min-height: 68px;
  padding: 12px 10px;
  display: flex;
  align-items: center;
  gap: 12px;
  text-align: left;
  border: 0;
  border-bottom: 1px solid rgb(var(--base-text-color) / 8%);
  color: inherit;
  background: transparent;
  cursor: pointer;
}

.continue-item:nth-child(odd) {
  padding-right: 20px;
  border-right: 1px solid rgb(var(--base-text-color) / 8%);
}

.continue-item:nth-child(even) {
  padding-left: 20px;
}

.continue-item:nth-last-child(-n + 2) {
  border-bottom: 0;
}

.continue-item:hover {
  background: rgb(var(--primary-color) / 4%);
}

.continue-item__icon,
.module-card__icon {
  width: 40px;
  height: 40px;
  flex: none;
  display: grid;
  place-items: center;
  border-radius: 6px;
  color: rgb(var(--primary-color));
  background: rgb(var(--primary-color) / 9%);
  font-size: 21px;
}

.continue-item__copy,
.module-card__copy {
  min-width: 0;
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.continue-item__title-line {
  min-width: 0;
  display: flex;
  align-items: center;
  gap: 8px;
}

.continue-item__title-line strong,
.module-card__copy strong {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 14px;
  font-weight: 600;
}

.continue-item__copy small,
.module-card__copy small {
  overflow: hidden;
  color: rgb(var(--base-text-color) / 48%);
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 12px;
}

.continue-item__meta {
  flex: none;
  color: rgb(var(--base-text-color) / 38%);
  font-size: 11px;
}

.continue-item__arrow,
.module-card__arrow {
  flex: none;
  color: rgb(var(--base-text-color) / 30%);
}

.module-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
}

.module-card {
  min-width: 0;
  min-height: 104px;
  padding: 18px 16px;
  display: flex;
  align-items: center;
  gap: 13px;
  text-align: left;
  border: 1px solid rgb(var(--base-text-color) / 10%);
  border-radius: 8px;
  color: inherit;
  background: rgb(var(--container-bg-color));
  cursor: pointer;
  transition:
    border-color 0.18s ease,
    box-shadow 0.18s ease,
    transform 0.18s ease;
}

.module-card:hover {
  border-color: rgb(var(--primary-color) / 42%);
  box-shadow: 0 8px 24px rgb(0 0 0 / 6%);
  transform: translateY(-2px);
}

@media (max-width: 1000px) {
  .module-grid {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
}

@media (max-width: 720px) {
  .home-page {
    padding: 12px 14px 28px;
  }

  .home-welcome {
    min-height: 268px;
    align-items: center;
    gap: 8px;
  }

  .home-welcome__copy {
    padding: 22px 0 26px;
  }

  .home-welcome h1 {
    font-size: 26px;
  }

  .home-welcome__subtitle {
    max-width: 260px;
    margin: 10px 0 20px;
    font-size: 13px;
  }

  .home-welcome__suggestions {
    grid-template-columns: 1fr;
  }

  .continue-panel {
    padding: 6px 12px;
  }

  .continue-list {
    grid-template-columns: 1fr;
  }

  .continue-item,
  .continue-item:nth-child(odd),
  .continue-item:nth-child(even) {
    padding: 12px 4px;
    border-right: 0;
    border-bottom: 1px solid rgb(var(--base-text-color) / 8%);
  }

  .continue-item:last-child {
    border-bottom: 0;
  }

  .continue-item__meta {
    display: none;
  }

  .module-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .module-card {
    min-height: 116px;
    padding: 14px;
    align-items: flex-start;
  }

  .module-card__arrow {
    display: none;
  }
}

@media (max-width: 430px) {
  .home-welcome {
    gap: 0;
  }

  .home-welcome__eyebrow {
    display: none;
  }

  .home-welcome h1 {
    font-size: 23px;
  }
}

@media (prefers-reduced-motion: reduce) {
  .module-card {
    transition: none;
  }
}
</style>
