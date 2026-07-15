<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import type { RegisteredAttachmentResourceType } from '@/constants/attachment';
import { ATTACHMENT_ACTIONS } from '@/constants/attachment';
import { useAuth } from '@/hooks/business/auth';
import {
  bindAttachment,
  deleteAttachment,
  listAttachmentsByResource,
  unbindAttachment
} from '@/service/api';
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

const canUpload = computed(() => hasAuth(ATTACHMENT_ACTIONS.uploadPrepare));
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

async function handleUploaded(attachment: Api.Attachment.Item) {
  if (!props.resourceId) {
    // 临时上传：业务创建资源后再 bind；此处只展示本地 pending
    if (!pending.value.some(i => i.id === attachment.id)) {
      pending.value = [...pending.value, attachment];
    }
    items.value = [...pending.value];
    return;
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
  window.$message?.success($t('page.attachment.uploadSuccess'));
  await refresh();
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

/** 业务创建资源后调用，将临时附件正式绑定。 */
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
    <AttachmentUpload
      v-if="!readonly && canUpload"
      :multiple="multiple"
      :max-count="maxCount"
      :disabled="!canUpload"
      @success="handleUploaded"
    />
    <NAlert v-else-if="!readonly && !canUpload" type="warning" :title="$t('page.attachment.noUploadPermission')" />
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
  </div>
</template>
