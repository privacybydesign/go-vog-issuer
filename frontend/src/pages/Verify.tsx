import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useNavigate } from 'react-router-dom';
import { useAppContext } from '../AppContext';
import { ApiError, issueVog } from '../api';
import { IssuanceResponse } from '../types';
import { fullName } from './DocumentSummary';

type Phase = 'idle' | 'disclosing' | 'matching';

export default function VerifyPage() {
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const { upload, setUpload, setOutcome } = useAppContext();
  const [phase, setPhase] = useState<Phase>('idle');
  const [errorMessage, setErrorMessage] = useState<string | undefined>();

  // Without an uploaded VOG there is nothing to verify against.
  useEffect(() => {
    if (!upload) {
      navigate(`/${i18n.language}/upload`, { replace: true });
    }
  }, [upload, navigate, i18n.language]);

  if (!upload) {
    return null;
  }

  const sessionId = upload.session_id;

  const handleApiError = (err: unknown) => {
    if (err instanceof ApiError) {
      if (err.body.error === 'error:identity-mismatch' && err.body.identity) {
        setOutcome({ matched: false, identity: err.body.identity });
        navigate(`/${i18n.language}/result`);
        return;
      }
      if (err.body.error === 'error:unknown-session') {
        setUpload(undefined);
        navigate(`/${i18n.language}/upload`);
        return;
      }
      setErrorMessage(t(err.translationKey));
      return;
    }
    navigate(`/${i18n.language}/error`);
  };

  // Step 2: the disclosure finished; ask the backend to compare the identity
  // with the VOG. Both outcomes are shown on the result page; on a match that
  // page also offers to add the VOG to the Yivi app.
  const matchIdentity = async () => {
    setPhase('matching');
    try {
      const response = await issueVog(sessionId);
      const result: IssuanceResponse = await response.json();
      setOutcome({ matched: true, issuance: result });
      navigate(`/${i18n.language}/result`);
    } catch (err) {
      setPhase('idle');
      handleApiError(err);
    }
  };

  // Step 1: disclosure of the identity. The backend starts the IRMA session
  // (it needs the requestor token to read the result), the app gets the
  // session pointer through yivi-frontend's default mapping.
  const startDisclosure = async () => {
    setErrorMessage(undefined);
    setOutcome(undefined);
    setPhase('disclosing');
    const yivi = await import('@privacybydesign/yivi-frontend');
    const disclosure = yivi.newPopup({
      language: i18n.language as 'en' | 'nl',
      session: {
        url: '/api/vog',
        start: {
          url: '/api/vog/start-disclosure',
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ session_id: sessionId }),
        },
        result: false,
      },
    });
    try {
      await disclosure.start();
    } catch (e) {
      setPhase('idle');
      if (e === 'Aborted') {
        setErrorMessage(t('verify_cancelled'));
      } else {
        console.error('Disclosure failed', e);
        setErrorMessage(t('verify_disclosure_error'));
      }
      return;
    }
    await matchIdentity();
  };

  const busy = phase !== 'idle';

  return (
    <div id="container">
      <header>
        <h1>{t('verify_header')}</h1>
      </header>
      <main>
        <div className="sms-form">
          {errorMessage && (
            <div id="status-bar" className="alert alert-danger" role="alert">
              <div className="status-container">
                <div id="status">{errorMessage}</div>
              </div>
            </div>
          )}
          {phase === 'matching' && (
            <div id="status-bar" className="alert alert-info" role="status">
              <div className="status-container">
                <div id="status">{t('verify_busy')}</div>
              </div>
            </div>
          )}
          <p>{t('verify_explanation')}</p>
          <table className="table">
            <tbody>
              <tr>
                <th>{t('field_name')}</th>
                <td>{fullName(upload.document)}</td>
              </tr>
              <tr>
                <th>{t('field_date_of_birth')}</th>
                <td>{upload.document.date_of_birth}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </main>
      <footer>
        <div className="actions">
          <Link to={`/${i18n.language}/upload`} id="back-button">
            {t('back')}
          </Link>
          <button id="submit-button" type="button" disabled={busy} onClick={startDisclosure}>
            {t('verify_start')}
          </button>
        </div>
      </footer>
    </div>
  );
}
