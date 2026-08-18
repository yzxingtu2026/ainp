// @vitest-environment happy-dom

import { nextTick } from 'vue';
import { mount } from '@vue/test-utils';
import { afterEach, describe, expect, it, vi } from 'vitest';
import WorkspaceComposer from './workspace-composer.vue';

vi.mock('@/locales', () => ({
  $t: (key: string) => key
}));

vi.mock('@/hooks/business/attachment', () => ({
  attachmentTypeIcon: () => 'mdi:file-outline',
  isImageContentType: (contentType: string) => contentType.startsWith('image/'),
  useAttachmentGridThumbs: () => ({ urls: {} })
}));

vi.mock('@/hooks/business/attachment-upload-queue', async () => {
  const { computed, ref } = await import('vue');
  return {
    useAttachmentUploadQueue: () => ({
      items: ref([]),
      busy: computed(() => false),
      enqueue: vi.fn(),
      remove: vi.fn(),
      retry: vi.fn(),
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

const mounted: Array<ReturnType<typeof mount>> = [];

afterEach(() => {
  mounted.splice(0).forEach(wrapper => wrapper.unmount());
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
