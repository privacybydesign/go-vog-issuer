import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import '../i18n';
import { AppProvider } from '../AppContext';
import { ApiError, RetryOptions, UploadCancelled, uploadVogWithRetry } from '../api';
import UploadPage from './Upload';

vi.mock('../api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api')>();
  return { ...actual, uploadVogWithRetry: vi.fn() };
});

const uploadMock = vi.mocked(uploadVogWithRetry);

/** A controllable upload: the test decides when and how it ends. */
function pendingUpload() {
  let options: RetryOptions | undefined;
  let finish!: (result: unknown) => void;
  let fail!: (err: unknown) => void;
  uploadMock.mockImplementation((_file, opts) => {
    options = opts;
    return new Promise((resolve, reject) => {
      finish = resolve as (result: unknown) => void;
      fail = reject;
    });
  });
  return {
    get options() {
      return options!;
    },
    finish: (result: unknown) => finish(result),
    fail: (err: unknown) => fail(err),
  };
}

// A minimal file that passes the client-side PDF check.
const pdf = new File(['%PDF-1.7\n1 0 obj\nendobj\n%%EOF\n'], 'vog.pdf', { type: 'application/pdf' });

async function renderAndSubmit() {
  render(
    <AppProvider>
      <MemoryRouter initialEntries={['/nl/upload']}>
        <UploadPage />
      </MemoryRouter>
    </AppProvider>,
  );
  fireEvent.change(screen.getByLabelText(/^VOG \(PDF/), { target: { files: [pdf] } });
  const submit = screen.getByRole('button', { name: 'Uploaden en controleren' });
  await waitFor(() => expect(submit).toBeEnabled());
  fireEvent.click(submit);
  await waitFor(() => expect(uploadMock).toHaveBeenCalledTimes(1));
  return submit;
}

describe('UploadPage while validatie.nl is unavailable', () => {
  beforeEach(() => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
  });

  afterEach(() => {
    // vitest runs without globals, so Testing Library does not clean up by itself.
    cleanup();
    vi.useRealTimers();
    uploadMock.mockReset();
  });

  it('explains that the check runs against validatie.nl', async () => {
    pendingUpload();
    await renderAndSubmit();
    expect(screen.getByText(/We sturen de PDF naar validatie\.nl/)).toBeInTheDocument();
    expect(screen.getByRole('status')).toHaveTextContent('De VOG wordt gecontroleerd bij validatie.nl...');
  });

  it('announces the outage and counts down to the automatic retry', async () => {
    const upload = pendingUpload();
    await renderAndSubmit();

    await act(async () => upload.options.onRetry?.({ attempt: 2, maxAttempts: 4, delayMs: 5000 }));

    const status = await screen.findByRole('status');
    expect(status).toHaveTextContent('validatie.nl is op dit moment niet bereikbaar');
    expect(status).toHaveTextContent('Dit ligt niet aan je VOG.');
    expect(status).toHaveTextContent(/over 5 seconden \(poging 2 van 4\)/);

    await vi.advanceTimersByTimeAsync(2100);
    expect(status).toHaveTextContent(/over 3 seconden/);

    await vi.advanceTimersByTimeAsync(3000);
    expect(status).toHaveTextContent('De VOG wordt opnieuw gecontroleerd bij validatie.nl (poging 2 van 4)...');
  });

  it('lets the user stop retrying', async () => {
    const upload = pendingUpload();
    const submit = await renderAndSubmit();
    await act(async () => upload.options.onRetry?.({ attempt: 2, maxAttempts: 4, delayMs: 5000 }));

    fireEvent.click(await screen.findByRole('button', { name: 'Stop met proberen' }));
    expect(upload.options.signal?.aborted).toBe(true);

    upload.fail(new UploadCancelled());
    await waitFor(() => expect(submit).toBeEnabled());
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('shows a final message when validatie.nl stayed unavailable', async () => {
    const upload = pendingUpload();
    const submit = await renderAndSubmit();
    await act(async () => upload.options.onRetry?.({ attempt: 4, maxAttempts: 4, delayMs: 20000 }));

    upload.fail(new ApiError(503, { error: 'error:validation-service-unavailable' }));

    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent('validatie.nl bleef onbereikbaar');
    expect(alert).toHaveTextContent('Dit ligt niet aan je VOG.');
    expect(submit).toBeEnabled();
  });

  it('adds the validatie.nl explanation for a retryable response code', async () => {
    const upload = pendingUpload();
    await renderAndSubmit();
    upload.fail(
      new ApiError(503, {
        error: 'error:validation-failed',
        validation: { code: 3, key: 'validation_unavailable', description: '', authentic: false, retryable: true },
      }),
    );

    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent('validatie.nl bleef onbereikbaar');
    expect(alert).toHaveTextContent('De validatie is nu niet mogelijk');
  });

  it('does not present a rejected document as an outage', async () => {
    const upload = pendingUpload();
    await renderAndSubmit();
    upload.fail(
      new ApiError(422, {
        error: 'error:validation-failed',
        validation: { code: 2, key: 'unknown_document', description: '', authentic: false, retryable: false },
      }),
    );

    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent('De VOG is niet geaccepteerd door validatie.nl.');
    expect(alert).toHaveTextContent('Het document is niet bekend bij validatie.nl');
    expect(alert).not.toHaveTextContent('bleef onbereikbaar');
  });

  it('confirms the validatie.nl result on success', async () => {
    const upload = pendingUpload();
    await renderAndSubmit();
    upload.finish({
      session_id: 's1',
      validation: { code: 0, key: 'authentic', description: '', authentic: true, retryable: false },
      document: {
        reference_number: '1', issue_date: '2026-01-01', surname: 'Berg', prefix: 'van der', given_names: 'Anna',
        date_of_birth: '1980-02-03', place_of_birth: 'Utrecht', country_of_birth: 'NL', purpose: 'werk',
        profile_codes: [], profiles: [],
      },
    });

    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent('validatie.nl bevestigt dat de VOG echt en ongewijzigd is.');
  });
});
