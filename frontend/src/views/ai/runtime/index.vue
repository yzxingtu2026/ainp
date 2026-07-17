<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { $t } from '@/locales';
import { fetchAIAliases, generateAIModel } from '@/service/api';
import { getAuthorization } from '@/service/request/shared';
import { localStg } from '@/utils/storage';
import { getServiceBaseURL } from '@/utils/service';

defineOptions({ name: 'AIModelRuntime' });

const aliases = ref<Api.AI.Alias[]>([]);
const modelAlias = ref('');
const prompt = ref('');
const output = ref('');
const error = ref('');
const loading = ref(false);
const streaming = ref(false);
let controller: AbortController | null = null;
const disabled = computed(() => loading.value || !modelAlias.value || !prompt.value.trim());

function requestBody() {
  return { model_alias: modelAlias.value, messages: [{ role: 'user', content: [{ type: 'input_text', text: prompt.value.trim() }] }] };
}

async function loadAliases() {
  const { data } = await fetchAIAliases();
  aliases.value = data || [];
  modelAlias.value ||= aliases.value[0]?.code || '';
}

async function generate() {
  loading.value = true; error.value = ''; output.value = '';
  try {
    const { data, error: requestError } = await generateAIModel(requestBody());
    if (requestError) throw requestError;
    output.value = data?.output?.filter(item => item.type === 'output_text').map(item => item.text || '').join('') || '';
  } catch (cause) { error.value = cause instanceof Error ? cause.message : $t('aiRuntime.requestFailed'); }
  finally { loading.value = false; }
}

async function stream() {
  loading.value = true; streaming.value = true; error.value = ''; output.value = '';
  controller = new AbortController();
  const isProxy = import.meta.env.DEV && import.meta.env.VITE_HTTP_PROXY === 'Y';
  const { baseURL } = getServiceBaseURL(import.meta.env, isProxy);
  try {
    const response = await fetch(`${baseURL}/api/v1/ai/models/stream`, { method: 'POST', signal: controller.signal, headers: { 'Content-Type': 'application/json', Authorization: getAuthorization() || '', 'Accept-Language': localStg.get('lang') || 'zh-CN' }, body: JSON.stringify(requestBody()) });
    if (!response.ok || !response.body) throw new Error($t('aiRuntime.requestFailed'));
    const reader = response.body.getReader(); const decoder = new TextDecoder(); let buffer = '';
    while (true) {
      const { done, value } = await reader.read(); if (done) break; buffer += decoder.decode(value, { stream: true });
      const frames = buffer.split('\n\n'); buffer = frames.pop() || '';
      for (const frame of frames) {
        const line = frame.split('\n').find(item => item.startsWith('data: ')); if (!line) continue;
        const event = JSON.parse(line.slice(6));
        if (event.type === 'output_text.delta') output.value += event.delta || '';
        if (event.type === 'response.failed') error.value = event.error?.message || $t('aiRuntime.requestFailed');
      }
    }
  } catch (cause) { if (!(cause instanceof DOMException && cause.name === 'AbortError')) error.value = cause instanceof Error ? cause.message : $t('aiRuntime.requestFailed'); }
  finally { loading.value = false; streaming.value = false; controller = null; }
}

function cancel() { controller?.abort(); }
onMounted(loadAliases);
</script>

<template>
  <div class="flex-col-stretch gap-16px">
    <NCard :title="$t('aiRuntime.title')" :bordered="false">
      <NForm label-placement="top">
        <NFormItem :label="$t('aiRuntime.modelAlias')"><NSelect v-model:value="modelAlias" :options="aliases.map(item => ({ label: item.code, value: item.code }))" /></NFormItem>
        <NFormItem :label="$t('aiRuntime.prompt')"><NInput v-model:value="prompt" type="textarea" :rows="6" :placeholder="$t('aiRuntime.promptPlaceholder')" /></NFormItem>
        <NSpace>
          <NButton type="primary" :loading="loading && !streaming" :disabled="disabled" @click="generate">{{ $t('aiRuntime.generate') }}</NButton>
          <NButton :loading="streaming" :disabled="disabled" @click="stream">{{ $t('aiRuntime.stream') }}</NButton>
          <NButton v-if="streaming" type="error" @click="cancel">{{ $t('common.cancel') }}</NButton>
        </NSpace>
      </NForm>
    </NCard>
    <NAlert v-if="error" type="error">{{ error }}</NAlert>
    <NCard :title="$t('aiRuntime.output')" :bordered="false"><NSpin :show="loading"><NEmpty v-if="!output" :description="$t('aiRuntime.emptyOutput')" /><pre v-else class="m-0 whitespace-pre-wrap break-words font-inherit">{{ output }}</pre></NSpin></NCard>
  </div>
</template>
