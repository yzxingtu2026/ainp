<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import type { RegisteredAttachmentResourceType } from '@/constants/attachment';
import { ATTACHMENT_ACTIONS } from '@/constants/attachment';
import { useAuth } from '@/hooks/business/auth';
import { bindAttachment, deleteAttachment, listAttachmentsByResource, unbindAttachment } from '@/service/api';
import { $t } from '@/locales';

defineOptions({ name: 'AttachmentField' });

const props = withDefaults(
  defineProps<{
    resourceType: RegisteredAttachmentResourceType;
    resourceId?: string;
    field?: string;
    readonly?: boolean;
    multiple?: boolean;
    maxCount?: number;
  }>(),
  {
    field: '',
    readonly: false,
    multiple: true
  }
);

const { hasAuth } = useAuth();
const items = ref<Api.Attachment.Item[]>([]);
const loading = ref(false);
const pending = ref<Api.Attachment.Item[]>([]);
const pickerShow = ref(false);

const canPick = computed(
  () =>
    hasAuth(ATTACHMENT_ACTIONS.uploadPrepare) ||
    hasAuth(ATTACHMENT_ACTIONS.list) ||
    hasAuth(ATTACHMENT_ACTIONS.policyGet)
);
const canBind = computed(() => hasAuth(ATTACHMENT_ACTIONS.bind));
const canUnbind = computed(() => hasAuth(ATTACHMENT_ACTIONS.unbind));
const canDelete = computed(() => hasAuth(ATTACHMENT_ACTIONS.delete));

async function refresh() {
  if (!props.resourceId) {
    items.value = [...pending.value];
    return;
  }
  loading.value = true;
  try {
    const { data, error } = await listAttachmentsByResource({
      resource_type: props.resourceType,
      resource_id: props.resourceId,
      field: props.field
    });
    if (error) {
      window.$message?.error(error.message || $t('page.attachment.loadFailed'));
      return;
    }
    items.value = data?.items || [];
  } finally {
    loading.value = false;
  }
}

async function handlePicked(selected: Api.Attachment.Item[]) {
  for (const attachment of selected) {
    if (items.value.some(item => item.id === attachment.id) || pending.value.some(item => item.id === attachment.id)) {
      continue;
    }
    if (!props.resourceId) {
      pending.value = [...pending.value, attachment];
      items.value = [...pending.value];
      continue;
    }
    if (!canBind.value) {
      window.$message?.error($t('page.attachment.noBindPermission'));
      return;
    }
    const { error } = await bindAttachment({
      attachment_id: attachment.id,
      resource_type: props.resourceType,
      resource_id: props.resourceId,
      field: props.field
    });
    if (error) {
      window.$message?.error(error.message || $t('page.attachment.bindFailed'));
      return;
    }
  }
  if (props.resourceId) {
    window.$message?.success($t('page.attachment.uploadSuccess'));
    await refresh();
  }
}

async function handleUnbind(item: Api.Attachment.Item) {
  if (!props.resourceId) {
    pending.value = pending.value.filter(i => i.id !== item.id);
    items.value = [...pending.value];
    return;
  }
  const { error } = await unbindAttachment({
    attachment_id: item.id,
    resource_type: props.resourceType,
    resource_id: props.resourceId,
    field: props.field
  });
  if (error) {
    window.$message?.error(error.message || $t('page.attachment.unbindFailed'));
    return;
  }
  await refresh();
}

async function handleDelete(item: Api.Attachment.Item) {
  if (props.resourceId) {
    const { error: unbindErr } = await unbindAttachment({
      attachment_id: item.id,
      resource_type: props.resourceType,
      resource_id: props.resourceId,
      field: props.field
    });
    if (unbindErr) {
      window.$message?.error(unbindErr.message || $t('page.attachment.unbindFailed'));
      return;
    }
  } else {
    pending.value = pending.value.filter(i => i.id !== item.id);
  }
  const { error } = await deleteAttachment(item.id);
  if (error) {
    window.$message?.error(error.message || $t('page.attachment.deleteFailed'));
    await refresh();
    return;
  }
  await refresh();
}

async function bindPending(resourceId: string) {
  for (const item of pending.value) {
    const { error } = await bindAttachment({
      attachment_id: item.id,
      resource_type: props.resourceType,
      resource_id: resourceId,
      field: props.field
    });
    if (error) {
      throw new Error(error.message || 'bind_pending_failed');
    }
  }
  pending.value = [];
  await refresh();
}

defineExpose({ refresh, bindPending, pendingAttachments: pending });

onMounted(refresh);
watch(() => [props.resourceType, props.resourceId, props.field], refresh);
</script>

<template>
  <div class="flex-col gap-16px">
    <div v-if="!readonly" class="flex items-center gap-12px">
      <NButton type="primary" :disabled="!canPick" @click="pickerShow = true">
        <template #icon>
          <SvgIcon icon="mdi:paperclip" />
        </template>
        {{ $t('page.attachment.openPicker') }}
      </NButton>
      <NAlert v-if="!canPick" class="flex-1" type="warning" :title="$t('page.attachment.noUploadPermission')" />
    </div>
    <AttachmentList
      :items="items"
      :loading="loading"
      :readonly="readonly"
      :can-unbind="canUnbind"
      :can-delete="canDelete"
      @unbind="handleUnbind"
      @delete="handleDelete"
      @refresh="refresh"
    />
    <AttachmentPicker
      v-model:show="pickerShow"
      mode="append"
      :multiple="multiple"
      :max-count="maxCount"
      :locked-selected-ids="items.map(i => i.id)"
      @confirm="handlePicked"
    />
  </div>
</template>
