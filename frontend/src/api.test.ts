import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  ApiError,
  RetryProgress,
  UploadCancelled,
  VALIDATION_RETRY_DELAYS_MS,
  errorKeyToTranslationKey,
  isValidationServiceUnavailable,
  uploadVogWithRetry,
} from './api';
import { ValidationInfo } from './types';

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

const validation = (key: string, retryable: boolean): ValidationInfo => ({
  code: 0,
  key,
  description: '',
  authentic: false,
  retryable,
});

const unreachable = () => new ApiError(503, { error: 'error:validation-service-unavailable' });
const retryableCode = () =>
  new ApiError(503, { error: 'error:validation-failed', validation: validation('validation_unavailable', true) });
const rejected = () =>
  new ApiError(422, { error: 'error:validation-failed', validation: validation('unknown_document', false) });

describe('isValidationServiceUnavailable', () => {
  it('recognises an unreachable validatie.nl', () => {
    expect(isValidationServiceUnavailable(unreachable())).toBe(true);
  });

  it('recognises the retryable response codes of validatie.nl', () => {
    expect(isValidationServiceUnavailable(retryableCode())).toBe(true);
  });

  it('does not treat a rejected document or other errors as an outage', () => {
    expect(isValidationServiceUnavailable(rejected())).toBe(false);
    expect(isValidationServiceUnavailable(new ApiError(503, { error: 'error:internal' }))).toBe(false);
    expect(isValidationServiceUnavailable(new ApiError(500, { error: 'error:internal' }))).toBe(false);
    expect(isValidationServiceUnavailable(new Error('network'))).toBe(false);
    expect(isValidationServiceUnavailable(undefined)).toBe(false);
  });
});

describe('uploadVogWithRetry', () => {
  const file = new File(['%PDF-1.7 fake'], 'vog.pdf', { type: 'application/pdf' });
  const ok = { session_id: 's1', validation: validation('authentic', false), document: {} };

  /** Queues fetch answers: an ApiError becomes an error response, anything else a 200. */
  function fetchAnswering(...answers: unknown[]) {
    const fetchMock = vi.fn(async () => {
      const answer = answers.shift();
      if (answer instanceof ApiError) {
        return new Response(JSON.stringify(answer.body), { status: answer.status });
      }
      return new Response(JSON.stringify(answer), { status: 200 });
    });
    vi.stubGlobal('fetch', fetchMock);
    return fetchMock;
  }

  /** A wait that never really sleeps but records what it was asked. */
  function instantWait() {
    const waits: number[] = [];
    const wait = async (ms: number) => {
      waits.push(ms);
    };
    return { wait, waits };
  }

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('returns the parsed response when the first upload succeeds', async () => {
    const fetchMock = fetchAnswering(ok);
    const { wait, waits } = instantWait();
    const onRetry = vi.fn();

    const result = await uploadVogWithRetry(file, { wait, onRetry });

    expect(result.session_id).toBe('s1');
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(waits).toEqual([]);
    expect(onRetry).not.toHaveBeenCalled();
  });

  it('retries while validatie.nl is unavailable and reports every retry', async () => {
    const fetchMock = fetchAnswering(unreachable(), retryableCode(), ok);
    const { wait, waits } = instantWait();
    const progress: RetryProgress[] = [];

    const result = await uploadVogWithRetry(file, { wait, onRetry: (p) => progress.push(p) });

    expect(result.session_id).toBe('s1');
    expect(fetchMock).toHaveBeenCalledTimes(3);
    expect(waits).toEqual([VALIDATION_RETRY_DELAYS_MS[0], VALIDATION_RETRY_DELAYS_MS[1]]);
    expect(progress).toEqual([
      { attempt: 2, maxAttempts: 4, delayMs: VALIDATION_RETRY_DELAYS_MS[0] },
      { attempt: 3, maxAttempts: 4, delayMs: VALIDATION_RETRY_DELAYS_MS[1] },
    ]);
  });

  it('gives up after the last delay and throws the last error', async () => {
    const fetchMock = fetchAnswering(unreachable(), unreachable(), unreachable(), retryableCode());
    const { wait, waits } = instantWait();

    const failure = uploadVogWithRetry(file, { wait, delaysMs: [1, 2, 3] });

    await expect(failure).rejects.toMatchObject({ status: 503, body: { error: 'error:validation-failed' } });
    expect(fetchMock).toHaveBeenCalledTimes(4);
    expect(waits).toEqual([1, 2, 3]);
  });

  it('does not retry a rejected document or any other error', async () => {
    const fetchMock = fetchAnswering(rejected(), ok);
    const { wait, waits } = instantWait();
    const onRetry = vi.fn();

    await expect(uploadVogWithRetry(file, { wait, onRetry })).rejects.toMatchObject({ status: 422 });
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(waits).toEqual([]);
    expect(onRetry).not.toHaveBeenCalled();
  });

  it('stops when cancelled during the pause', async () => {
    const fetchMock = fetchAnswering(unreachable(), ok);
    const controller = new AbortController();
    // The default wait is used here: it must end as soon as the signal fires.
    const failure = uploadVogWithRetry(file, {
      signal: controller.signal,
      onRetry: () => controller.abort(),
    });

    await expect(failure).rejects.toBeInstanceOf(UploadCancelled);
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it('reports a cancelled request as a cancellation', async () => {
    const controller = new AbortController();
    vi.stubGlobal(
      'fetch',
      vi.fn(async (_url: string, init: RequestInit) => {
        controller.abort();
        throw init.signal?.reason ?? new DOMException('aborted', 'AbortError');
      }),
    );

    await expect(uploadVogWithRetry(file, { signal: controller.signal })).rejects.toBeInstanceOf(UploadCancelled);
  });
});
