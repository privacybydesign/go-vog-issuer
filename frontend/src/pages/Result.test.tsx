import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import '../i18n';
import { AppProvider } from '../AppContext';
import { IdentityMatchInfo, MatchOutcome, UploadResponse } from '../types';
import ResultPage from './Result';

const popupStart = vi.fn();
const newPopup = vi.fn((_options: unknown) => ({ start: popupStart }));
vi.mock('@privacybydesign/yivi-frontend', () => ({ newPopup: (options: unknown) => newPopup(options) }));

const upload: UploadResponse = {
  session_id: 'session-1',
  validation: { code: 0, key: 'authentic', description: '', authentic: true, retryable: false },
  document: {
    reference_number: '9999012026032500922',
    issue_date: '2026-03-25',
    surname: 'Jansen',
    prefix: 'de',
    given_names: 'Jan',
    date_of_birth: '1980-01-01',
    place_of_birth: 'Utrecht',
    country_of_birth: 'Nederland',
    purpose: 'Werk',
    profile_codes: ['84'],
    profiles: [],
  },
};

const matchedIdentity: IdentityMatchInfo = {
  source: 'passport',
  matched: true,
  date_of_birth_match: true,
  surname_match: true,
  given_names_match: true,
};

function renderResult(outcome?: MatchOutcome, withUpload = true) {
  render(
    <AppProvider initialUpload={withUpload ? upload : undefined} initialOutcome={outcome}>
      <MemoryRouter initialEntries={['/nl/result']}>
        <Routes>
          <Route path="/nl/result" element={<ResultPage />} />
          <Route path="/nl/upload" element={<div>upload page</div>} />
          <Route path="/nl/verify" element={<div>verify page</div>} />
          <Route path="/nl/done" element={<div>done page</div>} />
        </Routes>
      </MemoryRouter>
    </AppProvider>,
  );
}

describe('ResultPage', () => {
  afterEach(() => {
    cleanup();
    newPopup.mockClear();
    popupStart.mockReset();
  });

  it('returns to the upload when there is no outcome to show', async () => {
    renderResult(undefined);
    await waitFor(() => expect(screen.getByText('upload page')).toBeInTheDocument());
  });

  describe('when the identity matches', () => {
    const outcome: MatchOutcome = {
      matched: true,
      issuance: { jwt: 'signed.jwt', irma_server_url: 'https://irma.example.org', identity: matchedIdentity },
    };

    it('confirms the match and explains that the VOG can be added to the Yivi app as a card', () => {
      renderResult(outcome);
      expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('Identiteit komt overeen');
      expect(screen.getByRole('status')).toHaveTextContent('Ja, je identiteit komt overeen met de VOG.');
      expect(screen.getByText('Jan de Jansen')).toBeInTheDocument();
      expect(screen.getByText('Paspoort')).toBeInTheDocument();
      expect(screen.getByRole('heading', { level: 2 })).toHaveTextContent('De VOG als kaartje in je Yivi-app');
      expect(screen.getByText(/Je kunt de VOG nu als kaartje \(credential\) toevoegen aan je Yivi-app/)).toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'VOG toevoegen aan Yivi-app' })).toBeEnabled();
    });

    it('hands the signed issuance request to the Yivi app and moves on to the done page', async () => {
      popupStart.mockResolvedValue(undefined);
      renderResult(outcome);
      fireEvent.click(screen.getByRole('button', { name: 'VOG toevoegen aan Yivi-app' }));

      await waitFor(() => expect(newPopup).toHaveBeenCalledTimes(1));
      expect(newPopup).toHaveBeenCalledWith(
        expect.objectContaining({
          language: 'nl',
          session: expect.objectContaining({
            url: 'https://irma.example.org',
            start: expect.objectContaining({ method: 'POST', body: 'signed.jwt' }),
          }),
        }),
      );
      await waitFor(() => expect(screen.getByText('done page')).toBeInTheDocument());
    });

    it('reports a cancelled issuance and lets the user try again or start over', async () => {
      popupStart.mockRejectedValue('Aborted');
      renderResult(outcome);
      fireEvent.click(screen.getByRole('button', { name: 'VOG toevoegen aan Yivi-app' }));

      await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('Geannuleerd.'));
      expect(screen.getByRole('button', { name: 'VOG toevoegen aan Yivi-app' })).toBeEnabled();
      expect(screen.getByRole('link', { name: 'Opnieuw beginnen' })).toHaveAttribute('href', '/nl');
    });
  });

  describe('when the identity does not match', () => {
    const outcome: MatchOutcome = {
      matched: false,
      identity: {
        source: 'driving_licence',
        matched: false,
        date_of_birth_match: true,
        surname_match: true,
        given_names_match: false,
      },
    };

    it('says so, lists what differed and offers another attempt', async () => {
      renderResult(outcome);
      expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('Identiteit komt niet overeen');
      expect(screen.getByRole('alert')).toHaveTextContent('De voornamen verschillen.');
      expect(screen.getByRole('alert')).not.toHaveTextContent('De achternaam verschilt.');
      expect(screen.getByText(/De VOG is daarom niet toegevoegd aan je Yivi-app/)).toBeInTheDocument();
      expect(screen.getByText('Rijbewijs')).toBeInTheDocument();

      fireEvent.click(screen.getByRole('button', { name: 'Opnieuw proberen met een ander gegeven' }));
      await waitFor(() => expect(screen.getByText('verify page')).toBeInTheDocument());
    });
  });
});
