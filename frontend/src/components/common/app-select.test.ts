// @vitest-environment happy-dom

import { nextTick } from 'vue';
import { mount } from '@vue/test-utils';
import { NSelect } from 'naive-ui';
import { afterEach, describe, expect, it, vi } from 'vitest';
import AppSelect from './app-select.vue';
import { normalizeSelectValue, optionContainsValue } from './select-value';

const mounted: Array<ReturnType<typeof mount>> = [];

afterEach(() => {
  mounted.splice(0).forEach(wrapper => wrapper.unmount());
});

function mountSelect(props: Record<string, unknown> = {}, slots: Record<string, string> = {}) {
  const wrapper = mount(AppSelect, {
    props,
    slots
  });
  mounted.push(wrapper);
  return wrapper;
}

describe('AppSelect empty value contract', () => {
  it('normalizes only an unregistered empty string without mutating the model', async () => {
    const wrapper = mountSelect({ value: '', options: [{ label: 'Enabled', value: 'enabled' }] });
    expect(wrapper.findComponent(NSelect).props('value')).toBeNull();
    expect(wrapper.emitted('update:value')).toBeUndefined();

    await wrapper.setProps({ options: [{ label: 'All', value: '' }] });
    expect(wrapper.findComponent(NSelect).props('value')).toBe('');
  });

  it('preserves legal empty, zero, arrays, disabled options and historical non-empty values', () => {
    expect(normalizeSelectValue('', [{ label: 'All', value: '' }])).toBe('');
    expect(normalizeSelectValue(0, [{ label: 'Zero', value: 0 }])).toBe(0);
    expect(normalizeSelectValue([], [])).toEqual([]);
    expect(normalizeSelectValue('', [{ label: 'All', value: '', disabled: true }])).toBe('');
    expect(normalizeSelectValue('retired', [])).toBe('retired');
    expect(normalizeSelectValue(null, [])).toBeNull();
    expect(normalizeSelectValue(undefined, [])).toBeNull();
  });

  it('supports grouped options, custom fields and the preserve escape policy', () => {
    const options = [{ type: 'group', label: 'Filters', nodes: [{ text: 'All', code: '' }] }];
    expect(optionContainsValue(options, '', 'code', 'nodes')).toBe(true);
    expect(normalizeSelectValue('', options, { valueField: 'code', childrenField: 'nodes' })).toBe('');
    expect(normalizeSelectValue('', [], { emptyStringPolicy: 'preserve' })).toBe('');
  });
});

describe('AppSelect adapter contract', () => {
  it('forwards common props, named slots and the full update payload', async () => {
    const option = { label: 'Enabled', value: 'enabled' };
    const wrapper = mountSelect(
      { value: '', options: [option], clearable: true, disabled: true, loading: true, placeholder: 'Choose' },
      {
        header: '<span>Header</span>',
        action: '<span>Action</span>',
        empty: '<span>Empty</span>',
        arrow: '<span>Arrow</span>'
      }
    );
    const select = wrapper.findComponent(NSelect);
    expect(select.props()).toMatchObject({
      value: null,
      clearable: true,
      disabled: true,
      loading: true,
      placeholder: 'Choose'
    });
    expect(Object.keys(select.vm.$slots)).toEqual(expect.arrayContaining(['header', 'action', 'empty', 'arrow']));

    select.vm.$emit('update:value', 'enabled', option);
    await nextTick();
    expect(wrapper.emitted('update:value')).toEqual([['enabled', option]]);

    select.vm.$emit('update:value', null, null);
    await nextTick();
    expect(wrapper.emitted('update:value')?.at(-1)).toEqual([null, null]);
  });

  it('forwards the Select instance focus methods', () => {
    const wrapper = mountSelect();
    const select = wrapper.findComponent(NSelect);
    const focus = vi.spyOn(select.vm, 'focus');
    const blur = vi.spyOn(select.vm, 'blur');
    const focusInput = vi.spyOn(select.vm, 'focusInput');
    const blurInput = vi.spyOn(select.vm, 'blurInput');
    const instance = wrapper.vm as unknown as {
      focus: () => void;
      blur: () => void;
      focusInput: () => void;
      blurInput: () => void;
    };
    instance.focus();
    instance.blur();
    instance.focusInput();
    instance.blurInput();
    expect([focus, blur, focusInput, blurInput].every(method => method.mock.calls.length === 1)).toBe(true);
  });
});
