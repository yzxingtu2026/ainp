export interface SettingUpdatePayload {
  values: Record<string, unknown>;
  secrets: Record<string, string>;
  clearSecretFields: string[];
  hasSecretFields: boolean;
}

export function buildSettingUpdatePayload(
  definition: Api.Setting.Definition,
  formValues: Record<string, unknown>,
  secretInputs: Record<string, string>,
  clearedSecretFields: ReadonlySet<string>
): SettingUpdatePayload {
  const values: Record<string, unknown> = {};
  const secrets: Record<string, string> = {};
  let hasSecretFields = false;
  definition.fields.forEach(field => {
    if (field.type === 'secret') {
      hasSecretFields = true;
      const value = secretInputs[field.key];
      if (value) secrets[field.key] = value;
      return;
    }
    values[field.key] = formValues[field.key];
  });
  return { values, secrets, clearSecretFields: [...clearedSecretFields], hasSecretFields };
}
