<script setup lang="ts">
import { ref, watch } from 'vue';
import { NButton, NEmpty, NInput, NList, NListItem, NModal, NSpace, NSpin, NTag, NThing } from 'naive-ui';
import { $t } from '@/locales';
import { fetchKnowledgeBases } from '@/service/api';

defineOptions({ name: 'AgentKnowledgePickerModal' });

const props = defineProps<{
  show: boolean;
  boundKnowledgeBaseIds: string[];
  canBind: boolean;
  bindKnowledge: (knowledgeBaseId: string) => Promise<void>;
}>();

const emit = defineEmits<{
  'update:show': [value: boolean];
  loaded: [items: Api.AI.KnowledgeBase[]];
}>();

const keyword = ref('');
const loading = ref(false);
const bindingId = ref('');
const candidates = ref<Api.AI.KnowledgeBase[]>([]);

watch(
  () => props.show,
  async visible => {
    if (!visible) return;
    keyword.value = '';
    await search();
  }
);

async function search() {
  loading.value = true;
  try {
    const { data } = await fetchKnowledgeBases({
      page: 1,
      page_size: 100,
      keyword: keyword.value.trim() || undefined,
      status: 'enabled'
    });
    candidates.value = (data?.items || []).filter(item => Boolean(item.current_index_version_id));
    emit('loaded', candidates.value);
  } finally {
    loading.value = false;
  }
}

async function onBind(knowledgeBaseId: string) {
  if (!props.canBind || props.boundKnowledgeBaseIds.includes(knowledgeBaseId) || bindingId.value) return;
  bindingId.value = knowledgeBaseId;
  try {
    await props.bindKnowledge(knowledgeBaseId);
  } finally {
    bindingId.value = '';
  }
}

function close() {
  emit('update:show', false);
}
</script>

<template>
  <NModal
    :show="show"
    preset="card"
    class="w-820px max-w-95vw"
    :title="$t('page.agentBuilder.knowledgePickerTitle')"
    :mask-closable="true"
    @update:show="emit('update:show', $event)"
  >
    <div class="mb-12px text-12px text-gray-500">{{ $t('page.agentBuilder.knowledgePickerHint') }}</div>
    <NSpace class="mb-12px">
      <NInput
        v-model:value="keyword"
        clearable
        class="w-360px max-w-full"
        :placeholder="$t('page.agentBuilder.knowledgeSearchPlaceholder')"
        @keyup.enter="search"
      />
      <NButton type="primary" :loading="loading" @click="search">{{ $t('common.search') }}</NButton>
    </NSpace>
    <NSpin :show="loading">
      <div class="max-h-60vh overflow-y-auto">
        <NEmpty v-if="candidates.length === 0" :description="$t('page.agentBuilder.noKnowledgeCandidates')" />
        <NList v-else bordered>
          <NListItem v-for="base in candidates" :key="base.id">
            <NThing :title="base.name" :description="base.description || $t('page.agentBuilder.noKnowledgeDescription')">
              <template #header-extra>
                <NSpace align="center">
                  <NTag size="small">{{ base.visibility }}</NTag>
                  <NTag size="small" type="success">{{ $t('page.agentBuilder.indexReady') }}</NTag>
                  <NButton
                    size="small"
                    type="primary"
                    :disabled="boundKnowledgeBaseIds.includes(base.id) || !canBind"
                    :loading="bindingId === base.id"
                    @click="onBind(base.id)"
                  >
                    {{ boundKnowledgeBaseIds.includes(base.id) ? $t('page.agentBuilder.bound') : $t('page.agentBuilder.bind') }}
                  </NButton>
                </NSpace>
              </template>
            </NThing>
            <div class="mt-4px text-12px text-gray-400">{{ base.code }}</div>
          </NListItem>
        </NList>
      </div>
    </NSpin>
    <template #footer>
      <NSpace justify="end">
        <NButton @click="close">{{ $t('common.close') }}</NButton>
      </NSpace>
    </template>
  </NModal>
</template>
