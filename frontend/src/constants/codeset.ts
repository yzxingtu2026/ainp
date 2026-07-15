export const CODESET_KEYS = {
  activationStatus: 'common.activation_status',
  attachmentStatus: 'system.attachment_status'
} as const;

export const ACTIVATION_STATUS = {
  enabled: 'enabled',
  disabled: 'disabled'
} as const;

export type ActivationStatus = (typeof ACTIVATION_STATUS)[keyof typeof ACTIVATION_STATUS];
