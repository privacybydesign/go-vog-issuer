import { ErrorResponse } from './types';

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

export function errorKeyToTranslationKey(errorKey: string | undefined): string {
  if (!errorKey) {
    return 'error_default';
  }
  return errorKey.trim().replaceAll('-', '_').replaceAll(':', '_').toLowerCase();
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

export async function uploadVog(file: File): Promise<Response> {
  const form = new FormData();
  form.append('file', file, file.name);
  const response = await fetch('/api/vog/upload', { method: 'POST', body: form });
  if (!response.ok) {
    throw await parseError(response);
  }
  return response;
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
