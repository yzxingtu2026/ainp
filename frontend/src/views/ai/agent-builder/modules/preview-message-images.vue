<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { fetchAttachment } from '@/service/api';
import { formatFileSize, useAttachmentGridThumbs } from '@/hooks/business/attachment';

defineOptions({ name: 'AgentPreviewMessageImages' });

const props = defineProps<{
  parts: Api.AI.ConversationContentPart[];
}>();

const items = ref<Api.Attachment.Item[]>([]);
const itemById = computed<Record<string, Api.Attachment.Item>>(() =>
  Object.fromEntries(items.value.map(item => [item.id, item]))
);
let loadVersion = 0;
const thumbs = useAttachmentGridThumbs(items, {
  enabled: computed(() => items.value.length > 0)
});

watch(
  () => props.parts.map(part => part.image_attachment_id || '').join('|'),
  async () => {
    const version = ++loadVersion;
    const ids = [...new Set(props.parts.map(part => part.image_attachment_id).filter(Boolean) as string[])];
    const loaded = await Promise.all(
      ids.map(async id => {
        const { data } = await fetchAttachment(id);
        return data || null;
      })
    );
    if (version === loadVersion) items.value = loaded.filter(Boolean) as Api.Attachment.Item[];
  },
  { immediate: true }
);
</script>

<template>
  <div class="mt-8px flex flex-wrap gap-8px">
    <div
      v-for="part in parts"
      :key="`${part.image_attachment_id}:${part.attachment_revision}`"
      class="w-92px overflow-hidden rounded-8px border border-gray-200 bg-white/50 dark:border-gray-700 dark:bg-gray-900/40"
    >
      <div class="h-72px bg-gray-100 dark:bg-gray-800">
        <img
          v-if="part.image_attachment_id && thumbs.urls[part.image_attachment_id]"
          :src="thumbs.urls[part.image_attachment_id]"
          :alt="itemById[part.image_attachment_id]?.original_name || ''"
          class="h-full w-full object-cover"
        />
        <div v-else class="h-full w-full flex-col-center text-gray-400">
          <SvgIcon icon="mdi:image-outline" class="text-24px" />
        </div>
      </div>
      <div class="px-6px py-5px">
        <div class="truncate text-10px" :title="itemById[part.image_attachment_id || '']?.original_name">
          {{ itemById[part.image_attachment_id || '']?.original_name || part.image_attachment_id }}
        </div>
        <div v-if="itemById[part.image_attachment_id || '']" class="text-9px text-gray-500">
          {{ formatFileSize(itemById[part.image_attachment_id || ''].size) }}
        </div>
      </div>
    </div>
  </div>
</template>
