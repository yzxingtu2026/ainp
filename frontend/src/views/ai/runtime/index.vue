<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { Icon } from '@iconify/vue';
import { $t } from '@/locales';
import { fetchAIAliases, fetchAIDeployments, generateAIModel } from '@/service/api';
import { attachmentTypeIcon, formatFileSize, isImageContentType, useAttachmentGridThumbs } from '@/hooks/business/attachment';
import { getAuthorization } from '@/service/request/shared';
import { localStg } from '@/utils/storage';
import { getServiceBaseURL } from '@/utils/service';

defineOptions({ name: 'AIModelRuntime' });

const aliases = ref<Api.AI.Alias[]>([]);
const deployments = ref<Api.AI.Deployment[]>([]);
const modelAlias = ref('');
const prompt = ref('');
const output = ref('');
const error = ref('');
const loading = ref(false);
const streaming = ref(false);
const pickerVisible = ref(false);
const images = ref<Api.Attachment.Item[]>([]);
const imageDetails = ref<Record<string, 'auto' | 'low' | 'high'>>({});
const thumbs = useAttachmentGridThumbs(images, { enabled: computed(() => images.value.length > 0) });
let controller: AbortController | null = null;
const selectedAlias = computed(() => aliases.value.find(item => item.code === modelAlias.value));
const selectedDeployment = computed(() => deployments.value.find(item => item.id === selectedAlias.value?.deployment_id));
const supportsVision = computed(() => Boolean(selectedDeployment.value?.capabilities_json.input_modalities?.includes('image') && selectedDeployment.value?.capabilities_json.features?.includes('vision')));
const disabled = computed(() => loading.value || !modelAlias.value || (!prompt.value.trim() && !images.value.length) || (images.value.length > 0 && !supportsVision.value));

function requestBody() {
  const content: Record<string, unknown>[] = [];
  if (prompt.value.trim()) content.push({ type: 'input_text', text: prompt.value.trim() });
  for (const image of images.value) content.push({ type: 'input_image', image_attachment_id: image.id, detail: imageDetails.value[image.id] || 'auto' });
  return { model_alias: modelAlias.value, media_policy: 'native_only', messages: [{ role: 'user', content }] };
}

async function loadAliases() {
  const [{ data }, { data: deploymentPage }] = await Promise.all([fetchAIAliases(), fetchAIDeployments({ page: 1, page_size: 100 })]);
  aliases.value = data || [];
  deployments.value = deploymentPage?.items || [];
  modelAlias.value ||= aliases.value[0]?.code || '';
}

function selectImages(items: Api.Attachment.Item[]) {
  const accepted = items.filter(item => item.status === 'ready' && isImageContentType(item.content_type)).slice(0, 4);
  if (accepted.length !== items.length) window.$message?.warning($t('aiRuntime.imagesOnly'));
  images.value = accepted;
  for (const image of images.value) imageDetails.value[image.id] ||= 'auto';
}

function removeImage(id: string) { images.value = images.value.filter(item => item.id !== id); delete imageDetails.value[id]; }

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
        <NFormItem :label="$t('aiRuntime.images')">
          <div class="w-full flex-col-stretch gap-10px">
            <NAlert v-if="modelAlias && !supportsVision" type="warning" :title="$t('aiRuntime.visionUnsupported')" />
            <div v-if="images.length" class="grid grid-cols-4 gap-10px lt-sm:grid-cols-2">
              <div v-for="item in images" :key="item.id" class="overflow-hidden rounded-6px border border-gray-200 dark:border-gray-700">
                <div class="relative h-96px bg-gray-50 dark:bg-gray-800">
                  <img v-if="thumbs.urls[item.id]" :src="thumbs.urls[item.id]" :alt="item.original_name" class="h-full w-full object-cover" />
                  <div v-else class="h-full w-full flex-col-center text-gray-400"><Icon :icon="attachmentTypeIcon(item.content_type)" class="text-28px" /></div>
                  <ButtonIcon class="absolute right-6px top-6px bg-white/90 dark:bg-gray-900/90" icon="mdi:close" :tooltip-content="$t('aiRuntime.removeImage')" @click="removeImage(item.id)" />
                </div>
                <div class="px-8px py-6px"><div class="truncate text-12px" :title="item.original_name">{{ item.original_name }}</div><div class="text-11px text-gray-500">{{ formatFileSize(item.size) }}</div>
                  <NSelect v-model:value="imageDetails[item.id]" class="mt-6px" size="tiny" :options="[{label:$t('aiRuntime.detailAuto'),value:'auto'},{label:$t('aiRuntime.detailLow'),value:'low'},{label:$t('aiRuntime.detailHigh'),value:'high'}]" />
                </div>
              </div>
            </div>
            <NButton secondary :disabled="loading || images.length >= 4 || !supportsVision" @click="pickerVisible = true"><template #icon><Icon icon="mdi:image-plus-outline" /></template>{{ $t('aiRuntime.chooseImages') }}</NButton>
          </div>
        </NFormItem>
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
  <AttachmentPicker v-model:show="pickerVisible" :selected-ids="images.map(item => item.id)" :max-count="4" @confirm="selectImages" />
</template>
