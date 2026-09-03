import { ErrorResponse, UploadResponse } from './types';

/**
 * ApiError carries the structured error body of the backend so pages can
 * translate the error key and show validation / identity details.
 */
export class ApiError extends Error {
  status: number;
  body: ErrorResponse;

  constructor(status: number, body: ErrorResponse) {
    super(body.message || body.error);
    this.status = status;
    this.body = body;
  }

  /** i18n key derived from the backend error key, e.g. "error:not-a-vog" -> "error_not_a_vog". */
  get translationKey(): string {
    return errorKeyToTranslationKey(this.body.error);
  }
}

/** Thrown when an upload is cancelled through its AbortSignal. */
export class UploadCancelled extends Error {
  constructor() {
    super('upload cancelled');
    this.name = 'UploadCancelled';
  }
}

export function errorKeyToTranslationKey(errorKey: string | undefined): string {
  if (!errorKey) {
    return 'error_default';
  }
  return errorKey.trim().replaceAll('-', '_').replaceAll(':', '_').toLowerCase();
}

/**
 * True when the backend reports that validatie.nl (the GAAV validation
 * service) is unavailable: it could not be reached, or it answered with one of
 * its "try again later" response codes. The document itself is not the
 * problem in either case, so the upload can simply be repeated.
 */
export function isValidationServiceUnavailable(err: unknown): err is ApiError {
  if (!(err instanceof ApiError) || err.status !== 503) {
    return false;
  }
  if (err.body.error === 'error:validation-service-unavailable') {
    return true;
  }
  return err.body.error === 'error:validation-failed' && err.body.validation?.retryable === true;
}

async function parseError(response: Response): Promise<ApiError> {
  let body: ErrorResponse = { error: 'error:internal' };
  try {
    const parsed = await response.json();
    if (parsed && typeof parsed.error === 'string') {
      body = parsed;
    }
  } catch {
    // Not JSON (e.g. a proxy error page); keep the generic error.
  }
  return new ApiError(response.status, body);
}

export async function uploadVog(file: File, signal?: AbortSignal): Promise<Response> {
  const form = new FormData();
  form.append('file', file, file.name);
  const response = await fetch('/api/vog/upload', { method: 'POST', body: form, signal });
  if (!response.ok) {
    throw await parseError(response);
  }
  return response;
}

/**
 * Pauses between attempts when validatie.nl is unavailable, in milliseconds.
 * The backend already retries quick failures itself, so these are deliberately
 * long: they cover an outage of a minute or so without hammering the service.
 */
export const VALIDATION_RETRY_DELAYS_MS = [5000, 10000, 20000];

export interface RetryProgress {
  /** Number of the attempt about to be made, counting the first upload as 1. */
  attempt: number;
  /** Total number of attempts that will be made before giving up. */
  maxAttempts: number;
  /** Pause before that attempt starts. */
  delayMs: number;
}

export interface RetryOptions {
  /** Cancels the wait and the in-flight request; the promise then rejects with UploadCancelled. */
  signal?: AbortSignal;
  /** Pauses between attempts; the number of entries is the number of retries. */
  delaysMs?: number[];
  /** Called right before every pause, so the UI can announce the retry. */
  onRetry?: (progress: RetryProgress) => void;
  /** Replaceable for tests. */
  wait?: (ms: number, signal?: AbortSignal) => Promise<void>;
}

/**
 * Uploads the VOG and, while validatie.nl is unavailable, repeats the upload
 * after growing pauses. Any other error, including a rejection of the
 * document, is thrown at once. When every attempt fails, the last error is
 * thrown.
 */
export async function uploadVogWithRetry(file: File, options: RetryOptions = {}): Promise<UploadResponse> {
  const delays = options.delaysMs ?? VALIDATION_RETRY_DELAYS_MS;
  const wait = options.wait ?? sleep;
  const maxAttempts = delays.length + 1;

  for (let attempt = 1; ; attempt++) {
    throwIfCancelled(options.signal);
    try {
      const response = await uploadVog(file, options.signal);
      return (await response.json()) as UploadResponse;
    } catch (err) {
      // An aborted fetch rejects with an AbortError; report it as a cancellation.
      throwIfCancelled(options.signal);
      if (!isValidationServiceUnavailable(err) || attempt >= maxAttempts) {
        throw err;
      }
      const delayMs = delays[attempt - 1];
      options.onRetry?.({ attempt: attempt + 1, maxAttempts, delayMs });
      await wait(delayMs, options.signal);
    }
  }
}

function throwIfCancelled(signal?: AbortSignal) {
  if (signal?.aborted) {
    throw new UploadCancelled();
  }
}

function sleep(ms: number, signal?: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    if (signal?.aborted) {
      reject(new UploadCancelled());
      return;
    }
    const onAbort = () => {
      clearTimeout(timer);
      reject(new UploadCancelled());
    };
    const timer = setTimeout(() => {
      signal?.removeEventListener('abort', onAbort);
      resolve();
    }, ms);
    signal?.addEventListener('abort', onAbort, { once: true });
  });
}

export async function issueVog(sessionId: string): Promise<Response> {
  const response = await fetch('/api/vog/issue', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ session_id: sessionId }),
  });
  if (!response.ok) {
    throw await parseError(response);
  }
  return response;
}
