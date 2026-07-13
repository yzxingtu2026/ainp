<script setup lang="ts">
import { computed, ref } from 'vue';
import { getPaletteColorByNumber, mixColor } from '@sa/color';
import { useAppStore } from '@/store/modules/app';
import { useThemeStore } from '@/store/modules/theme';
import { $t } from '@/locales';
import EnvCheck from './modules/env-check.vue';
import Database from './modules/database.vue';
import Admin from './modules/admin.vue';
import Complete from './modules/complete.vue';

defineOptions({ name: 'Setup' });

const appStore = useAppStore();
const themeStore = useThemeStore();

const currentStep = ref(0);
const envData = ref<Api.Setup.CheckEnv | null>(null);
const dsnFromEnv = ref('');
const initResult = ref<Api.Setup.InitResult | null>(null);

const bgThemeColor = computed(() =>
  themeStore.darkMode ? getPaletteColorByNumber(themeStore.themeColor, 600) : themeStore.themeColor
);

const bgColor = computed(() => {
  const COLOR_WHITE = '#ffffff';
  const ratio = themeStore.darkMode ? 0.5 : 0.2;
  return mixColor(COLOR_WHITE, themeStore.themeColor, ratio);
});

const steps = computed(() => [
  { title: $t('page.setup.stepEnv') },
  { title: $t('page.setup.stepDatabase') },
  { title: $t('page.setup.stepAdmin') },
  { title: $t('page.setup.stepComplete') }
]);

function onEnvChecked(data: Api.Setup.CheckEnv) {
  envData.value = data;
  if (data.dsn_present) {
    dsnFromEnv.value = 'env-injected';
  }
}

function nextStep() {
  currentStep.value++;
}

function onInitialized(result: Api.Setup.InitResult) {
  initResult.value = result;
}
</script>

<template>
  <div class="flex-center min-h-screen wh-full" :style="{ backgroundColor: bgColor }">
    <div class="relative w-640px rounded-16px bg-layout p-40px shadow-lg">
      <LangSwitch
        class="absolute right-24px top-24px"
        :lang="appStore.locale"
        :lang-options="appStore.localeOptions"
        @change-lang="appStore.changeLocale"
      />
      <h2 class="mb-24px text-center text-24px font-bold">{{ $t('page.setup.title') }}</h2>
      <NSteps :current="currentStep + 1" size="small" class="mb-32px">
        <NStep v-for="(step, idx) in steps" :key="idx" :title="step.title" />
      </NSteps>

      <EnvCheck
        v-if="currentStep === 0"
        :bg-theme-color="bgThemeColor"
        @checked="onEnvChecked"
        @next="nextStep"
      />
      <Database
        v-if="currentStep === 1"
        :env-data="envData"
        :bg-theme-color="bgThemeColor"
        @next="nextStep"
      />
      <Admin
        v-if="currentStep === 2"
        :dsn-from-env="dsnFromEnv"
        :bg-theme-color="bgThemeColor"
        @next="nextStep"
        @initialized="onInitialized"
      />
      <Complete
        v-if="currentStep === 3"
        :result="initResult"
        :bg-theme-color="bgThemeColor"
      />
    </div>
  </div>
</template>

<style scoped></style>
