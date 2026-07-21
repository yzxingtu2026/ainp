<script setup lang="ts">
import { computed, ref } from 'vue';
import { useRouter } from 'vue-router';
import { useAuthStore } from '@/store/modules/auth';
import { useRouteStore } from '@/store/modules/route';
import { useAiAssistantStore } from '@/store/modules/ai-assistant';
import { $t } from '@/locales';

defineOptions({ name: 'Home' });

const router = useRouter();
const authStore = useAuthStore();
const routeStore = useRouteStore();
const aiAssistantStore = useAiAssistantStore();
const input = ref('');
const focused = ref(false);
const navigating = ref(false);

const greetingPeriod = computed(() => {
  const hour = new Date().getHours();
  if (hour < 6) return $t('page.home.periodLateNight');
  if (hour < 12) return $t('page.home.periodMorning');
  if (hour < 18) return $t('page.home.periodAfternoon');
  return $t('page.home.periodEvening');
});

const greeting = computed(() =>
  $t('page.home.greeting', {
    period: greetingPeriod.value,
    name: authStore.userInfo.userName || $t('page.home.defaultUser')
  })
);

const suggestions = computed(() => [
  $t('page.home.suggestionFind'),
  $t('page.home.suggestionSummarize'),
  $t('page.home.suggestionPlan'),
  $t('page.home.suggestionHelp')
]);

function firstTrustedTarget(menu: App.Global.Menu): App.Global.Menu | null {
  if (menu.children?.length) {
    for (const child of menu.children) {
      const target = firstTrustedTarget(child);
      if (target) return target;
    }
  }
  return menu.routePath && String(menu.routePath).startsWith('/') ? menu : null;
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
            path: target.routePath
          }
        : null;
    })
    .filter((item): item is NonNullable<typeof item> => Boolean(item))
);

function useSuggestion(suggestion: string) {
  input.value = suggestion;
}

async function send() {
  if (!input.value.trim() || navigating.value) return;
  const pending = aiAssistantStore.queueWorkspaceMessage(input.value);
  if (!pending) return;
  navigating.value = true;
  await router.push('/ai/workspace');
}

function openModule(path: App.Global.RoutePath) {
  void router.push(path);
}
</script>

<template>
  <main class="ai-home">
    <section class="ai-home__hero">
      <div class="ai-home__mascot" :class="{ 'ai-home__mascot--focused': focused }">
        <AiMascot :state="navigating ? 'working' : focused ? 'thinking' : 'idle'" :size="136" />
      </div>
      <div class="ai-home__welcome">
        <h1>{{ greeting }}</h1>
        <p>{{ $t('page.home.welcomeSubtitle') }}</p>
      </div>

      <div class="ai-home__composer" :class="{ 'ai-home__composer--focused': focused }">
        <NInput
          v-model:value="input"
          type="textarea"
          :placeholder="$t('page.home.inputPlaceholder')"
          :autosize="{ minRows: 1, maxRows: 4 }"
          :disabled="navigating"
          @focus="focused = true"
          @blur="focused = false"
          @keydown.enter.exact.prevent="send"
        />
        <NTooltip>
          <template #trigger>
            <button
              type="button"
              class="ai-home__send"
              :disabled="!input.trim() || navigating"
              :aria-label="$t('page.home.send')"
              @click="send"
            >
              <SvgIcon :icon="navigating ? 'mdi:loading' : 'mdi:arrow-up'" :class="{ 'is-spinning': navigating }" />
            </button>
          </template>
          {{ $t('page.home.send') }}
        </NTooltip>
      </div>

      <div class="ai-home__suggestions">
        <button v-for="suggestion in suggestions" :key="suggestion" type="button" @click="useSuggestion(suggestion)">
          {{ suggestion }}
        </button>
      </div>
    </section>

    <section class="ai-home__modules" aria-labelledby="home-modules-title">
      <div class="ai-home__section-heading">
        <div>
          <h2 id="home-modules-title">{{ $t('page.home.modulesTitle') }}</h2>
          <p>{{ $t('page.home.modulesSubtitle') }}</p>
        </div>
        <span>{{ modules.length }}</span>
      </div>
      <div v-if="modules.length" class="ai-home__module-grid">
        <button
          v-for="module in modules"
          :key="module.key"
          type="button"
          class="ai-home__module"
          @click="openModule(module.path)"
        >
          <span class="ai-home__module-icon"><component :is="module.icon" /></span>
          <span class="ai-home__module-copy">
            <strong>{{ module.title }}</strong>
            <small>{{ module.description }}</small>
          </span>
          <SvgIcon icon="mdi:arrow-right" class="ai-home__module-arrow" />
        </button>
      </div>
      <NEmpty v-else size="small" :description="$t('page.home.noModules')" />
    </section>
  </main>
</template>

<style scoped>
.ai-home {
  min-height: 100%;
  padding: clamp(20px, 4vw, 52px);
  color: rgb(var(--base-text-color));
  background: rgb(var(--layout-bg-color));
}
.ai-home__hero {
  min-height: min(560px, 62vh);
  max-width: 920px;
  margin: 0 auto;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  text-align: center;
}
.ai-home__mascot {
  height: 142px;
  display: flex;
  align-items: center;
  transition: transform 0.2s ease;
}
.ai-home__mascot--focused {
  transform: translateY(-3px);
}
.ai-home__welcome h1 {
  margin: 14px 0 8px;
  font-size: clamp(26px, 3.2rem, 42px);
  line-height: 1.25;
  font-weight: 650;
  letter-spacing: 0;
}
.ai-home__welcome p {
  margin: 0;
  color: rgb(var(--base-text-color) / 62%);
  font-size: 15px;
}
.ai-home__composer {
  width: min(100%, 760px);
  margin-top: 30px;
  padding: 10px 10px 10px 18px;
  display: flex;
  align-items: flex-end;
  gap: 10px;
  border: 1px solid rgb(var(--base-text-color) / 15%);
  border-radius: 8px;
  background: rgb(var(--container-bg-color));
  box-shadow: 0 12px 36px rgb(0 0 0 / 6%);
  transition:
    border-color 0.2s ease,
    box-shadow 0.2s ease;
}
.ai-home__composer--focused {
  border-color: rgb(var(--primary-color));
  box-shadow: 0 12px 36px rgb(var(--primary-color) / 12%);
}
.ai-home__composer :deep(.n-input) {
  background: transparent;
}
.ai-home__composer :deep(.n-input__border),
.ai-home__composer :deep(.n-input__state-border) {
  display: none;
}
.ai-home__send {
  width: 40px;
  height: 40px;
  flex: none;
  display: grid;
  place-items: center;
  border: 0;
  border-radius: 6px;
  color: white;
  background: rgb(var(--primary-color));
  cursor: pointer;
  font-size: 20px;
}
.ai-home__send:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}
.ai-home__suggestions {
  width: min(100%, 760px);
  margin-top: 14px;
  display: flex;
  justify-content: center;
  flex-wrap: wrap;
  gap: 8px;
}
.ai-home__suggestions button {
  padding: 6px 10px;
  border: 1px solid rgb(var(--base-text-color) / 12%);
  border-radius: 6px;
  color: rgb(var(--base-text-color) / 68%);
  background: transparent;
  cursor: pointer;
  font-size: 12px;
}
.ai-home__suggestions button:hover {
  color: rgb(var(--primary-color));
  border-color: rgb(var(--primary-color) / 45%);
}
.ai-home__modules {
  max-width: 1180px;
  margin: 0 auto;
  padding-top: 28px;
  border-top: 1px solid rgb(var(--base-text-color) / 10%);
}
.ai-home__section-heading {
  margin-bottom: 18px;
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
}
.ai-home__section-heading h2 {
  margin: 0;
  font-size: 20px;
  font-weight: 650;
}
.ai-home__section-heading p {
  margin: 5px 0 0;
  color: rgb(var(--base-text-color) / 55%);
  font-size: 13px;
}
.ai-home__section-heading > span {
  color: rgb(var(--base-text-color) / 38%);
  font-size: 12px;
}
.ai-home__module-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
}
.ai-home__module {
  min-height: 92px;
  min-width: 0;
  padding: 16px;
  display: flex;
  align-items: center;
  gap: 13px;
  text-align: left;
  border: 1px solid rgb(var(--base-text-color) / 11%);
  border-radius: 8px;
  color: inherit;
  background: rgb(var(--container-bg-color));
  cursor: pointer;
  transition:
    transform 0.18s ease,
    border-color 0.18s ease,
    box-shadow 0.18s ease;
}
.ai-home__module:hover {
  transform: translateY(-2px);
  border-color: rgb(var(--primary-color) / 42%);
  box-shadow: 0 8px 22px rgb(0 0 0 / 6%);
}
.ai-home__module-icon {
  width: 40px;
  height: 40px;
  flex: none;
  display: grid;
  place-items: center;
  border-radius: 6px;
  color: rgb(var(--primary-color));
  background: rgb(var(--primary-color) / 10%);
  font-size: 22px;
}
.ai-home__module-copy {
  min-width: 0;
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 5px;
}
.ai-home__module-copy strong,
.ai-home__module-copy small {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.ai-home__module-copy strong {
  font-size: 14px;
  font-weight: 600;
}
.ai-home__module-copy small {
  color: rgb(var(--base-text-color) / 50%);
  font-size: 12px;
}
.ai-home__module-arrow {
  flex: none;
  color: rgb(var(--base-text-color) / 32%);
}
.is-spinning {
  animation: home-spin 1s linear infinite;
}
@keyframes home-spin {
  to {
    transform: rotate(360deg);
  }
}
@media (max-width: 960px) {
  .ai-home__module-grid {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
}
@media (max-width: 720px) {
  .ai-home {
    padding: 20px 14px 30px;
  }
  .ai-home__hero {
    min-height: 520px;
    justify-content: flex-start;
    padding-top: 34px;
  }
  .ai-home__welcome h1 {
    font-size: 28px;
  }
  .ai-home__module-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  .ai-home__module {
    min-height: 104px;
    padding: 14px;
    align-items: flex-start;
  }
  .ai-home__module-arrow {
    display: none;
  }
}
@media (prefers-reduced-motion: reduce) {
  .ai-home__mascot,
  .ai-home__module,
  .is-spinning {
    transition: none;
    animation: none;
  }
}
</style>
