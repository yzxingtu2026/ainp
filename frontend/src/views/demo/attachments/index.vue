<script setup lang="ts">
import { ATTACHMENT_RESOURCE_TYPES } from '@/constants/attachment';
import { $t } from '@/locales';

defineOptions({ name: 'DemoAttachments' });

const demoResourceId = 'demo-task-1';
</script>

<template>
  <div class="min-h-500px flex-col-stretch gap-16px overflow-hidden lt-sm:overflow-auto">
    <NCard :title="$t('page.attachment.demoTitle')" :bordered="false" class="card-wrapper">
      <NSpace vertical>
        <NAlert type="info" :title="$t('page.attachment.demoHint')">
          <div class="text-13px leading-22px">
            resource_type=<code>{{ ATTACHMENT_RESOURCE_TYPES.demoTask }}</code>
            ，resource_id=<code>{{ demoResourceId }}</code>
            ，field=<code>files</code>
            。主入口为「选择附件」弹层（我的上传 / 上传新文件）。
          </div>
        </NAlert>
        <AttachmentField
          :resource-type="ATTACHMENT_RESOURCE_TYPES.demoTask"
          :resource-id="demoResourceId"
          field="files"
          :multiple="true"
          :max-count="5"
        />
      </NSpace>
    </NCard>
  </div>
</template>
