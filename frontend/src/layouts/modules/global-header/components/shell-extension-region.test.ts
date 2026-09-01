import { describe, expect, it } from 'vitest';
import { mount } from '@vue/test-utils';
import ShellExtensionRegion from './shell-extension-region.vue';

const extensions = [
  {
    id: 'example.first',
    owner: 'example',
    placement: 'header.context' as const,
    order: 10,
    component: async () => ({})
  },
  {
    id: 'example.second',
    owner: 'example',
    placement: 'header.context' as const,
    order: 20,
    component: async () => ({})
  }
];

const global = {
  mocks: { $t: (key: string) => key },
  stubs: {
    ShellExtensionHost: {
      props: ['registration'],
      template: '<div class="extension-stub">{{ registration.id }}</div>'
    },
    SvgIcon: true,
    NText: { template: '<span><slot /></span>' },
    NButton: { template: '<button v-bind="$attrs"><slot name="icon" /></button>' },
    NPopover: { template: '<div><slot name="trigger" /><div data-testid="popover"><slot /></div></div>' }
  }
};

describe('shell extension region', () => {
  it('renders ordered extensions inline on wide layouts', () => {
    const wrapper = mount(ShellExtensionRegion, { props: { extensions, compact: false }, global });
    expect(wrapper.find('.shell-extension-inline').exists()).toBe(true);
    expect(wrapper.findAll('.extension-stub').map(item => item.text())).toEqual(['example.first', 'example.second']);
  });

  it('keeps all extensions available from an accessible compact trigger', () => {
    const wrapper = mount(ShellExtensionRegion, { props: { extensions, compact: true }, global });
    expect(wrapper.get('[data-testid="shell-extension-trigger"]').attributes('aria-label')).toBe(
      'common.businessContext'
    );
    expect(wrapper.findAll('.extension-stub').map(item => item.text())).toEqual(['example.first', 'example.second']);
  });
});
