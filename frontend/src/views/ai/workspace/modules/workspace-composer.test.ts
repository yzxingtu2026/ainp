// @vitest-environment happy-dom

import { nextTick, ref } from 'vue';
import type { AttachmentQueueItem } from '@/hooks/business/attachment-upload-queue';
import { mount } from '@vue/test-utils';
import { afterEach, describe, expect, it, vi } from 'vitest';
import WorkspaceComposer from './workspace-composer.vue';

vi.mock('@/locales', () => ({
  $t: (key: string) => key
}));

vi.mock('@/hooks/business/attachment', () => ({
  attachmentTypeIcon: () => 'mdi:file-outline',
  canPreviewAttachment: (item: Pick<Api.Attachment.Item, 'content_type' | 'status'>) =>
    item.status === 'ready' &&
    (item.content_type.startsWith('image/') ||
      item.content_type === 'application/pdf' ||
      item.content_type.startsWith('video/')),
  formatFileSize: (size: number) => `${size} B`,
  isImageContentType: (contentType: string) => contentType.startsWith('image/'),
  useAttachmentGridThumbs: () => ({ urls: { 'img-1': 'blob:thumb-1' } })
}));

const queueItems = ref<AttachmentQueueItem[]>([]);
const enqueue = vi.fn();
const retry = vi.fn();
const remove = vi.fn();

vi.mock('@/hooks/business/attachment-upload-queue', async () => {
  const { computed } = await import('vue');
  return {
    useAttachmentUploadQueue: () => ({
      items: queueItems,
      busy: computed(() => queueItems.value.some(item => item.status === 'uploading')),
      enqueue,
      remove,
      retry,
      reset: vi.fn()
    })
  };
});

vi.mock('@/hooks/business/attachment-thumb-poll', () => ({
  useAttachmentThumbStatusPolling: vi.fn()
}));

vi.mock('@/components/common/attachment-picker.vue', () => ({
  default: { name: 'AttachmentPicker', template: '<div />' }
}));

vi.mock('@/components/common/attachment-preview.vue', () => ({
  default: {
    name: 'AttachmentPreview',
    props: ['show', 'attachment'],
    template: '<div data-test="attachment-preview" :data-id="attachment?.id" />'
  }
}));

const mounted: Array<ReturnType<typeof mount>> = [];

afterEach(() => {
  mounted.splice(0).forEach(wrapper => wrapper.unmount());
  queueItems.value = [];
  vi.clearAllMocks();
});

function mountComposer(props: Record<string, unknown> = {}) {
  const wrapper = mount(WorkspaceComposer, {
    props: {
      modelValue: '',
      attachments: [],
      canSend: true,
      ...props
    },
    global: {
      stubs: {
        AttachmentPicker: true,
        NSpin: true,
        SvgIcon: true
      }
    }
  });
  mounted.push(wrapper);
  return wrapper;
}

async function setScrollHeight(wrapper: ReturnType<typeof mount>, value: number) {
  const textarea = wrapper.get('textarea').element as HTMLTextAreaElement;
  Object.defineProperty(textarea, 'scrollHeight', { configurable: true, value });
  await nextTick();
  await nextTick();
  return textarea;
}

describe('WorkspaceComposer autosize', () => {
  it('starts at two rows and follows user/programmatic content changes', async () => {
    const wrapper = mountComposer({ density: 'workspace' });
    const textarea = await setScrollHeight(wrapper, 120);

    await wrapper.setProps({ modelValue: '第一行\n第二行\n第三行' });
    await nextTick();
    expect(wrapper.get('textarea').attributes('rows')).toBe('2');
    expect(textarea.style.height).toBe('120px');
    expect(wrapper.get('textarea').attributes('aria-label')).toBe('page.aiWorkspace.promptInput');
    expect(wrapper.attributes('style')).toContain('min(238px, 38dvh)');

    Object.defineProperty(textarea, 'scrollHeight', { configurable: true, value: 62 });
    await wrapper.setProps({ modelValue: '' });
    await nextTick();
    expect(textarea.style.height).toBe('62px');
  });

  it('uses the sidebar density cap and scrolls only after the cap', () => {
    const wrapper = mountComposer({ density: 'sidebar' });
    expect(wrapper.classes()).toContain('workspace-composer--sidebar');
    expect(wrapper.attributes('style')).toContain('min(194px, 32dvh)');
    expect(wrapper.get('textarea').attributes('rows')).toBe('2');
  });

  it('preserves Enter, Shift+Enter, and IME composing behavior', async () => {
    const wrapper = mountComposer();
    const textarea = wrapper.get('textarea');

    await textarea.trigger('keydown', { key: 'Enter' });
    await textarea.trigger('keydown', { key: 'Enter', shiftKey: true });
    await textarea.trigger('keydown', { key: 'Enter', isComposing: true });

    expect(wrapper.emitted('send')).toHaveLength(1);
  });
});

function queuedAttachment(status: 'uploading' | 'failed'): AttachmentQueueItem {
  return {
    localId: 'upload-1',
    file: new File(['image'], 'pasted.png', { type: 'image/png' }),
    status,
    progress: 42,
    error: status === 'failed' ? '上传失败，请重试' : '',
    attachment: null,
    abortController: null
  };
}

describe('WorkspaceComposer aurora states and controls', () => {
  it.each(['workspace', 'sidebar'] as const)('tracks content without changing the %s layout', async density => {
    const wrapper = mountComposer({ density, canSend: false });
    expect(wrapper.classes()).toContain(`workspace-composer--${density}`);
    expect(wrapper.classes()).not.toContain('workspace-composer--has-content');
    expect(wrapper.get('.workspace-composer__primary').attributes('disabled')).toBeDefined();
    expect(wrapper.get('.workspace-composer__primary').classes()).not.toContain('workspace-composer__primary--ready');
    await wrapper.setProps({ modelValue: '草稿', canSend: true });
    expect(wrapper.classes()).toContain('workspace-composer--has-content');
    expect(wrapper.get('.workspace-composer__primary').classes()).toContain('workspace-composer__primary--ready');
    await wrapper.get('.workspace-composer__primary').trigger('click');
    expect(wrapper.emitted('send')).toHaveLength(1);
    await wrapper.setProps({ modelValue: '  ' });
    expect(wrapper.classes()).not.toContain('workspace-composer--has-content');
    await wrapper.setProps({
      attachments: [
        {
          id: 'doc-1',
          content_type: 'text/plain',
          original_name: 'notes.txt',
          size: 5,
          status: 'ready',
          uploader_id: 'user-1',
          created_at: '',
          updated_at: ''
        }
      ]
    });
    expect(wrapper.classes()).toContain('workspace-composer--has-content');
    expect(wrapper.findAll('.workspace-composer__attachment')).toHaveLength(1);
    expect(wrapper.text()).toContain('TXT · 5 B');
  });

  it('opens AttachmentPreview for previewable image/PDF chips and keeps non-previewable docs static', async () => {
    const wrapper = mountComposer({
      attachments: [
        {
          id: 'img-1',
          content_type: 'image/png',
          original_name: 'screen.png',
          size: 120,
          status: 'ready',
          thumb_status: 'ready',
          uploader_id: 'user-1',
          created_at: '',
          updated_at: ''
        },
        {
          id: 'doc-pdf',
          content_type: 'application/pdf',
          original_name: 'report.pdf',
          size: 240,
          status: 'ready',
          uploader_id: 'user-1',
          created_at: '',
          updated_at: ''
        },
        {
          id: 'doc-xlsx',
          content_type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
          original_name: 'sheet.xlsx',
          size: 512,
          status: 'ready',
          uploader_id: 'user-1',
          created_at: '',
          updated_at: ''
        }
      ]
    });

    const previewTriggers = wrapper.findAll('.workspace-composer__attachment-preview');
    expect(previewTriggers).toHaveLength(2);
    await previewTriggers[1].trigger('click');
    await nextTick();
    expect(wrapper.get('[data-test="attachment-preview"]').attributes('data-id')).toBe('doc-pdf');
  });

  it('keeps the stop control available during a run, including while offline', async () => {
    const wrapper = mountComposer({ active: true, disabled: true, offline: true });
    expect(wrapper.classes()).toEqual(
      expect.arrayContaining(['workspace-composer--active', 'workspace-composer--offline'])
    );
    expect(wrapper.get('textarea').attributes('disabled')).toBeDefined();
    expect(wrapper.get('.workspace-composer__icon').attributes('disabled')).toBeDefined();
    const stop = wrapper.get('.workspace-composer__primary');
    expect(stop.classes()).toContain('workspace-composer__primary--stop');
    expect(stop.attributes('title')).toBe('page.aiWorkspace.stop');
    expect(stop.attributes('disabled')).toBeUndefined();
    await stop.trigger('click');
    expect(wrapper.emitted('stop')).toHaveLength(1);
    expect(wrapper.emitted('send')).toBeUndefined();
  });

  it('shows upload progress then failure, retaining retry/remove actions and editable text', async () => {
    queueItems.value = [queuedAttachment('uploading')];
    const wrapper = mountComposer();
    expect(wrapper.classes()).toContain('workspace-composer--uploading');
    expect(wrapper.text()).toContain('42%');
    expect(wrapper.get('textarea').attributes('disabled')).toBeUndefined();
    expect(wrapper.get('.workspace-composer__primary').attributes('disabled')).toBeDefined();
    await wrapper.get('textarea').trigger('keydown', { key: 'Enter' });
    expect(wrapper.emitted('send')).toBeUndefined();
    queueItems.value = [queuedAttachment('failed')];
    await nextTick();
    expect(wrapper.classes()).not.toContain('workspace-composer--uploading');
    expect(wrapper.classes()).toContain('workspace-composer--failed');
    expect(wrapper.get('.workspace-composer__attachment--failed').attributes('title')).toBe('上传失败，请重试');
    await wrapper.get('button[title="page.aiWorkspace.retryImageUpload"]').trigger('click');
    expect(retry).toHaveBeenCalledWith('upload-1');
    await wrapper.get('button[title="page.aiWorkspace.removeImage"]').trigger('click');
    expect(remove).toHaveBeenCalledWith('upload-1');
    queueItems.value = [];
    await nextTick();
    expect(wrapper.classes()).not.toContain('workspace-composer--failed');
  });

  it('reflects external failure and recovery without blocking a retry', async () => {
    const wrapper = mountComposer({ failed: true });
    expect(wrapper.classes()).toContain('workspace-composer--failed');
    await wrapper.get('textarea').trigger('keydown', { key: 'Enter' });
    expect(wrapper.emitted('send')).toHaveLength(1);
    await wrapper.setProps({ failed: false });
    expect(wrapper.classes()).not.toContain('workspace-composer--failed');
  });

  it.each([{ disabled: true }, { disabled: true, offline: true }, { active: true }, { canSend: false }])(
    'preserves send guards for %j',
    async props => {
      const wrapper = mountComposer(props);
      await wrapper.get('textarea').trigger('keydown', { key: 'Enter' });
      expect(wrapper.emitted('send')).toBeUndefined();
      if (props.disabled) expect(wrapper.classes()).toContain('workspace-composer--disabled');
    }
  );

  it('still queues pasted images and ignores image paste during a run', async () => {
    const wrapper = mountComposer();
    const file = new File(['image'], 'pasted.png', { type: 'image/png' });
    await wrapper.get('textarea').trigger('paste', {
      clipboardData: { items: [{ kind: 'file', type: 'image/png', getAsFile: () => file }] }
    });
    expect(enqueue).toHaveBeenCalledWith([expect.objectContaining({ type: 'image/png', size: file.size })]);
    await wrapper.setProps({ active: true });
    await wrapper.get('textarea').trigger('paste', {
      clipboardData: { items: [{ kind: 'file', type: 'image/png', getAsFile: () => file }] }
    });
    expect(enqueue).toHaveBeenCalledTimes(1);
  });
});
