import { describe, expect, it } from 'vitest';
import { ApiError, errorKeyToTranslationKey } from './api';

describe('errorKeyToTranslationKey', () => {
  it('maps backend error keys onto i18n keys', () => {
    expect(errorKeyToTranslationKey('error:not-a-vog')).toBe('error_not_a_vog');
    expect(errorKeyToTranslationKey('error:validation-service-unavailable')).toBe('error_validation_service_unavailable');
    expect(errorKeyToTranslationKey(' error:internal ')).toBe('error_internal');
  });

  it('falls back to the default error', () => {
    expect(errorKeyToTranslationKey(undefined)).toBe('error_default');
    expect(errorKeyToTranslationKey('')).toBe('error_default');
  });
});

describe('ApiError', () => {
  it('exposes status, body and translation key', () => {
    const err = new ApiError(422, {
      error: 'error:validation-failed',
      message: 'rejected',
      validation: { code: 2, key: 'unknown_document', description: '', authentic: false, retryable: false },
    });
    expect(err.status).toBe(422);
    expect(err.message).toBe('rejected');
    expect(err.translationKey).toBe('error_validation_failed');
    expect(err.body.validation?.key).toBe('unknown_document');
  });
});
