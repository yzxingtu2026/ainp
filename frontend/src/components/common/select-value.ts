export type SelectModelValue = string | number | Array<string | number> | null | undefined;
export type EmptyStringPolicy = 'auto' | 'preserve';
export type AppSelectOption = Record<string, unknown>;

export interface SelectValueOptions {
  emptyStringPolicy?: EmptyStringPolicy;
  valueField?: string;
  childrenField?: string;
}

export function optionContainsValue(
  options: AppSelectOption[] | undefined,
  value: string | number,
  valueField = 'value',
  childrenField = 'children'
): boolean {
  return (options || []).some(option => {
    const item = option as Record<string, unknown>;
    if (item[valueField] === value) return true;
    const children = item[childrenField];
    return Array.isArray(children)
      ? optionContainsValue(children as AppSelectOption[], value, valueField, childrenField)
      : false;
  });
}

export function normalizeSelectValue(
  value: SelectModelValue,
  options: AppSelectOption[] | undefined,
  config: SelectValueOptions = {}
): Exclude<SelectModelValue, undefined> {
  if (value === null || value === undefined) return null;
  if (value !== '' || config.emptyStringPolicy === 'preserve') return value;
  return optionContainsValue(options, '', config.valueField, config.childrenField) ? '' : null;
}
