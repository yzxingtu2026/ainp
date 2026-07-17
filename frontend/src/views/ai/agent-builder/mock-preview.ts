/** Typed Fake/Mock preview states for Agent Builder Phase A. Not persisted as real Conversation/Run. */

export type MockPreviewStatus = 'idle' | 'streaming' | 'waiting_confirmation' | 'stopped' | 'failed' | 'completed';

export interface MockPreviewMessage {
  id: string;
  role: 'user' | 'assistant' | 'tool' | 'system';
  text: string;
  kind?: 'text' | 'tool_call' | 'tool_result' | 'confirmation' | 'error';
}

export interface MockPreviewState {
  status: MockPreviewStatus;
  messages: MockPreviewMessage[];
  runSummary: string;
}

export function createIdleMockPreview(t: (key: string) => string): MockPreviewState {
  return {
    status: 'idle',
    messages: [
      {
        id: 'mock-system',
        role: 'system',
        kind: 'text',
        text: t('page.agentBuilder.mockBanner')
      }
    ],
    runSummary: t('page.agentBuilder.mockIdleSummary')
  };
}

export function createDemoMockPreview(userText: string, t: (key: string) => string): MockPreviewState {
  return {
    status: 'completed',
    messages: [
      {
        id: 'mock-system',
        role: 'system',
        kind: 'text',
        text: t('page.agentBuilder.mockBanner')
      },
      { id: 'mock-user', role: 'user', kind: 'text', text: userText },
      {
        id: 'mock-assistant',
        role: 'assistant',
        kind: 'text',
        text: t('page.agentBuilder.mockAssistantReply')
      },
      {
        id: 'mock-tool',
        role: 'tool',
        kind: 'tool_call',
        text: t('page.agentBuilder.mockToolCall')
      },
      {
        id: 'mock-confirm',
        role: 'system',
        kind: 'confirmation',
        text: t('page.agentBuilder.mockConfirmation')
      }
    ],
    runSummary: t('page.agentBuilder.mockCompletedSummary')
  };
}
