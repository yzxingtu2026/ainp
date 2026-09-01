import { afterEach, describe, expect, it, vi } from 'vitest';
import { flushPromises, mount } from '@vue/test-utils';
import ShellExtensionHost from './shell-extension-host.vue';

const registration = {
  id: 'example.context',
  owner: 'example',
  placement: 'header.context' as const,
  order: 10,
  component: () => Promise.resolve({ template: '<div data-testid="extension">loaded</div>' })
};

const errorStub = { template: '<div data-testid="extension-error">unavailable</div>' };

afterEach(() => vi.restoreAllMocks());

describe('shell extension host', () => {
  it('renders a compile-time lazy extension without platform props', async () => {
    const wrapper = mount(ShellExtensionHost, {
      props: { registration },
      global: { stubs: { ShellExtensionLoading: true, ShellExtensionError: errorStub } }
    });
    await flushPromises();
    expect(wrapper.get('[data-testid="extension"]').text()).toBe('loaded');
    expect(wrapper.get('[data-shell-extension-id="example.context"]').attributes()).not.toHaveProperty('auth');
    wrapper.unmount();
  });

  it('isolates lazy loading failure inside the extension host', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => undefined);
    const wrapper = mount(ShellExtensionHost, {
      props: { registration: { ...registration, component: () => Promise.reject(new Error('load failed')) } },
      global: { stubs: { ShellExtensionLoading: true, ShellExtensionError: errorStub } }
    });
    await flushPromises();
    expect(wrapper.get('[data-testid="extension-error"]').text()).toBe('unavailable');
    wrapper.unmount();
  });

  it('isolates extension render failure inside the extension host', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => undefined);
    const wrapper = mount(ShellExtensionHost, {
      props: {
        registration: {
          ...registration,
          component: () =>
            Promise.resolve({
              setup() {
                throw new Error('render failed');
              }
            })
        }
      },
      global: { stubs: { ShellExtensionLoading: true, ShellExtensionError: errorStub } }
    });
    await flushPromises();
    expect(wrapper.get('[data-testid="extension-error"]').text()).toBe('unavailable');
    wrapper.unmount();
  });
});
