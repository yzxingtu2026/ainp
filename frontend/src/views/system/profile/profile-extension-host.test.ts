import { afterEach, describe, expect, it, vi } from 'vitest';
import { flushPromises, mount } from '@vue/test-utils';
import ProfileExtensionHost from './profile-extension-host.vue';

const registration = {
  id: 'example.identity',
  owner: 'example',
  placement: 'tab' as const,
  label: 'Identity',
  component: () => Promise.resolve({ template: '<div data-testid="extension">loaded</div>' })
};

afterEach(() => vi.restoreAllMocks());

describe('profile extension host', () => {
  it('renders a lazy profile extension', async () => {
    const wrapper = mount(ProfileExtensionHost, { props: { registration } });
    await flushPromises();
    expect(wrapper.get('[data-testid="extension"]').text()).toBe('loaded');
    wrapper.unmount();
  });

  it('renders the scoped error boundary when lazy loading fails', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => undefined);
    const wrapper = mount(ProfileExtensionHost, {
      props: {
        registration: { ...registration, component: () => Promise.reject(new Error('load failed')) }
      },
      global: {
        stubs: { NResult: { template: '<div data-testid="load-error">load error</div>' } }
      }
    });
    await flushPromises();
    expect(wrapper.get('[data-testid="load-error"]').text()).toBe('load error');
    wrapper.unmount();
  });
});
