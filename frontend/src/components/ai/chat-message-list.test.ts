// @vitest-environment happy-dom

import { flushPromises, mount } from '@vue/test-utils';
import { afterEach, describe, expect, it, vi } from 'vitest';
import ChatMessageList from './chat-message-list.vue';

vi.mock('@/locales', () => ({
  $t: (key: string) => key
}));

vi.mock('@/hooks/business/agent-avatar', async () => {
  const { ref } = await import('vue');
  return {
    useAttachmentAvatar: () => ({ url: ref(null) })
  };
});

const thumbUrls: Record<string, string> = {
  'att-img-ready': 'blob:controlled-thumb-url'
};

vi.mock('@/hooks/business/attachment', () => ({
  attachmentTypeIcon: () => 'mdi:file-outline',
  canPreviewAttachment: (item: Pick<Api.Attachment.Item, 'content_type' | 'status'>) =>
    item.status === 'ready' && (item.content_type.startsWith('image/') || item.content_type === 'application/pdf'),
  formatFileSize: (size: number) => `${size} B`,
  isImageContentType: (contentType: string) => contentType.startsWith('image/'),
  useAttachmentGridThumbs: () => ({ urls: thumbUrls })
}));

vi.mock('@/hooks/business/attachment-thumb-poll', () => ({
  useAttachmentThumbStatusPolling: vi.fn()
}));

vi.mock('@/components/common/attachment-preview.vue', () => ({
  default: {
    name: 'AttachmentPreview',
    props: ['show', 'attachment'],
    template: '<div data-test="preview-modal" :data-id="attachment?.id" />'
  }
}));

const fetchAttachment = vi.fn();
vi.mock('@/service/api', () => ({
  fetchAttachment: (...args: unknown[]) => fetchAttachment(...args),
  approveAIToolConfirmation: vi.fn(),
  rejectAIToolConfirmation: vi.fn()
}));

const mounted: Array<ReturnType<typeof mount>> = [];

afterEach(() => {
  mounted.splice(0).forEach(wrapper => wrapper.unmount());
  vi.clearAllMocks();
});

describe('ChatMessageList multimodal history, streaming caret, and turn actions', () => {
  it('renders controlled image thumbnails, document chips, AttachmentPreview trigger, and safe fallback without leaking IDs', async () => {
    fetchAttachment.mockImplementation(async (id: string) => {
      if (id === 'att-img-ready') {
        return {
          data: {
            id: 'att-img-ready',
            original_name: 'architecture.png',
            content_type: 'image/png',
            size: 2048,
            sha256: 'rev-img-1',
            status: 'ready',
            thumb_status: 'ready',
            uploader_id: 'u-1',
            created_at: '',
            updated_at: ''
          },
          error: null
        };
      }
      if (id === 'att-pdf-ready') {
        return {
          data: {
            id: 'att-pdf-ready',
            original_name: 'contract.pdf',
            content_type: 'application/pdf',
            size: 4096,
            sha256: 'rev-pdf-1',
            status: 'ready',
            uploader_id: 'u-1',
            created_at: '',
            updated_at: ''
          },
          error: null
        };
      }
      return {
        data: {
          id,
          original_name: 'secret.png',
          content_type: 'image/png',
          size: 100,
          sha256: 'mismatched-revision',
          status: 'ready',
          uploader_id: 'u-1',
          created_at: '',
          updated_at: ''
        },
        error: null
      };
    });

    const messages: Api.AI.ConversationMessage[] = [
      {
        id: 'msg-user-1',
        conversation_id: 'conv-1',
        role: 'user',
        status: 'committed',
        sequence: 1,
        created_at: '2026-11-13T10:00:00Z',
        content: [
          { type: 'input_text', text: '请审阅架构图与合同' },
          { type: 'input_image', image_attachment_id: 'att-img-ready', attachment_revision: 'rev-img-1' },
          { type: 'input_image', image_attachment_id: 'secret-att-id-999', attachment_revision: 'expected-rev' },
          { type: 'input_file', file_attachment_id: 'att-pdf-ready', attachment_revision: 'rev-pdf-1' },
          { type: 'input_file', file_attachment_id: 'secret-doc-id-888', attachment_revision: 'expected-doc-rev' }
        ]
      },
      {
        id: 'msg-assistant-1',
        conversation_id: 'conv-1',
        role: 'assistant',
        status: 'committed',
        sequence: 2,
        run_id: 'run-settled-1',
        created_at: '2026-11-13T10:00:05Z',
        content: [{ type: 'output_text', text: '审阅完成，一切正常。' }]
      }
    ];

    const wrapper = mount(ChatMessageList, {
      props: {
        messages,
        agentName: '默认运维助手',
        failedRunId: 'run-settled-1',
        canRetryFailedRun: true
      },
      global: {
        stubs: {
          AiMascot: true,
          MarkdownRenderer: {
            name: 'MarkdownRenderer',
            props: ['content', 'streaming'],
            template: '<div class="md-stub" :data-streaming="String(Boolean(streaming))">{{ content }}</div>'
          },
          NAlert: true,
          NButton: true,
          NSpin: true,
          SvgIcon: true,
          ToolActivityGroup: true,
          ToolConfirmationCard: true
        }
      }
    });
    mounted.push(wrapper);
    await flushPromises();

    const userBubble = wrapper.get('.message__user-bubble');
    expect(userBubble.classes()).not.toContain('message__content');
    const thumbImg = wrapper.get('.user-image-card img');
    expect(thumbImg.attributes('src')).toBe('blob:controlled-thumb-url');
    expect(wrapper.text()).toContain('contract.pdf');
    expect(wrapper.text()).toContain('page.aiWorkspace.imageUnavailable');
    expect(wrapper.text()).toContain('page.aiWorkspace.documentUnavailable');
    expect(wrapper.html()).not.toContain('secret-att-id-999');
    expect(wrapper.html()).not.toContain('secret-doc-id-888');

    await wrapper.get('.user-image-card').trigger('click');
    expect(wrapper.get('[data-test="preview-modal"]').attributes('data-id')).toBe('att-img-ready');

    await wrapper.get('button[aria-label="chat.viewRunDetails"]').trigger('click');
    expect(wrapper.emitted('openRun')).toEqual([['run-settled-1']]);

    await wrapper.get('button[aria-label="chat.retryRun"]').trigger('click');
    expect(wrapper.emitted('retryRun')).toEqual([['run-settled-1']]);
  });

  it('activates streaming caret only on the live trailing text segment and clears it once settled', async () => {
    const wrapper = mount(ChatMessageList, {
      props: {
        messages: [],
        activeRunId: 'run-live-1',
        streamingText: '正在流式输出内容'
      },
      global: {
        stubs: {
          AiMascot: true,
          MarkdownRenderer: {
            name: 'MarkdownRenderer',
            props: ['content', 'streaming'],
            template: '<div class="md-stub" :data-streaming="String(Boolean(streaming))">{{ content }}</div>'
          },
          NAlert: true,
          NButton: true,
          NSpin: true,
          SvgIcon: true,
          ToolActivityGroup: true,
          ToolConfirmationCard: true
        }
      }
    });
    mounted.push(wrapper);

    expect(wrapper.get('.md-stub').attributes('data-streaming')).toBe('true');

    await wrapper.setProps({
      activeRunId: '',
      streamingText: '',
      messages: [
        {
          id: 'msg-done',
          conversation_id: 'conv-1',
          role: 'assistant',
          status: 'committed',
          sequence: 1,
          run_id: 'run-live-1',
          created_at: '',
          content: [{ type: 'output_text', text: '正在流式输出内容' }]
        }
      ]
    });

    expect(wrapper.get('.md-stub').attributes('data-streaming')).toBe('false');
  });
});
