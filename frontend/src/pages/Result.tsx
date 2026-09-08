import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useNavigate } from 'react-router-dom';
import { useAppContext } from '../AppContext';
import { IdentityMatchInfo, IssuanceResponse } from '../types';
import { formatDate, fullName } from './DocumentSummary';

/** Translation key for the credential the identity was disclosed with. */
export function identitySourceKey(source: string): string {
  return `source_${source}`;
}

/**
 * Shows whether the disclosed identity matched the VOG. On a match it explains
 * that the VOG can now be added to the Yivi app as a card and offers to do so;
 * on a mismatch it lists what differed and offers another attempt.
 */
export default function ResultPage() {
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const { upload, outcome } = useAppContext();
  const [issuing, setIssuing] = useState(false);
  const [errorMessage, setErrorMessage] = useState<string | undefined>();

  // Without an outcome there is nothing to show: back to the upload, which
  // continues where the user left off.
  useEffect(() => {
    if (!outcome || !upload) {
      navigate(`/${i18n.language}/upload`, { replace: true });
    }
  }, [outcome, upload, navigate, i18n.language]);

  if (!outcome || !upload) {
    return null;
  }

  const source = (identity: IdentityMatchInfo) => {
    const key = identitySourceKey(identity.source);
    return i18n.exists(key) ? t(key) : identity.source;
  };

  const person = (identity: IdentityMatchInfo) => (
    <table className="table">
      <tbody>
        <tr>
          <th>{t('field_name')}</th>
          <td>{fullName(upload.document)}</td>
        </tr>
        <tr>
          <th>{t('field_date_of_birth')}</th>
          <td>{formatDate(upload.document.date_of_birth)}</td>
        </tr>
        <tr>
          <th>{t('field_identity_source')}</th>
          <td>{source(identity)}</td>
        </tr>
      </tbody>
    </table>
  );

  // Hand the signed issuance request to the Yivi app. The backend consumed
  // the session when it handed out the request, so this is the only copy.
  const addToApp = async (issuance: IssuanceResponse) => {
    setErrorMessage(undefined);
    setIssuing(true);
    const yivi = await import('@privacybydesign/yivi-frontend');
    const popup = yivi.newPopup({
      language: i18n.language as 'en' | 'nl',
      session: {
        url: issuance.irma_server_url,
        start: {
          method: 'POST',
          headers: { 'Content-Type': 'text/plain' },
          body: issuance.jwt,
        },
        result: false,
      },
    });
    try {
      await popup.start();
      // The done page clears the flow state; clearing it here would trigger
      // the redirect above before the navigation lands.
      navigate(`/${i18n.language}/done`);
    } catch (e) {
      setIssuing(false);
      setErrorMessage(e === 'Aborted' ? t('verify_cancelled') : t('issue_error'));
    }
  };

  // The verify page discards the old outcome when a new disclosure starts.
  const retry = () => navigate(`/${i18n.language}/verify`);

  if (!outcome.matched) {
    const identity = outcome.identity;
    return (
      <div id="container">
        <header>
          <h1>{t('result_mismatch_header')}</h1>
        </header>
        <main>
          <div className="sms-form">
            <div id="status-bar" className="alert alert-danger" role="alert">
              <div className="status-container">
                <div id="status">
                  <b>{t('mismatch_header')}</b>
                  <ul className="mismatch-list">
                    {!identity.date_of_birth_match && <li>{t('mismatch_date_of_birth')}</li>}
                    {!identity.surname_match && <li>{t('mismatch_surname')}</li>}
                    {!identity.given_names_match && <li>{t('mismatch_given_names')}</li>}
                  </ul>
                </div>
              </div>
            </div>
            <div className="imageContainer">
              <img src="/images/fail.png" alt="" />
            </div>
            <p>{t('result_mismatch_explanation')}</p>
            {person(identity)}
            <p>{t('result_mismatch_retry_explanation')}</p>
          </div>
        </main>
        <footer>
          <div className="actions">
            <Link to={`/${i18n.language}`} id="back-button">
              {t('back')}
            </Link>
            <button id="submit-button" type="button" onClick={retry}>
              {t('verify_retry')}
            </button>
          </div>
        </footer>
      </div>
    );
  }

  const issuance = outcome.issuance;
  return (
    <div id="container">
      <header>
        <h1>{t('result_match_header')}</h1>
      </header>
      <main>
        <div className="sms-form">
          {errorMessage && (
            <div id="status-bar" className="alert alert-danger" role="alert">
              <div className="status-container">
                <div id="status">
                  {errorMessage}
                  <br />
                  {t('result_issue_error_hint')}
                </div>
              </div>
            </div>
          )}
          {issuing && (
            <div id="status-bar" className="alert alert-info" role="status">
              <div className="status-container">
                <div id="status">{t('result_issuing')}</div>
              </div>
            </div>
          )}
          {!issuing && !errorMessage && (
            <div id="status-bar" className="alert alert-success" role="status">
              <div className="status-container">
                <div id="status">{t('result_match')}</div>
              </div>
            </div>
          )}
          <div className="imageContainer">
            <img src="/images/done.png" alt="" />
          </div>
          {person(issuance.identity)}
          <h2>{t('result_card_header')}</h2>
          <p>{t('result_card_explanation')}</p>
          <p>{t('result_card_usage')}</p>
          <p>{t('result_card_how')}</p>
          <p className="details">{t('result_card_privacy')}</p>
        </div>
      </main>
      <footer>
        <div className="actions">
          {errorMessage ? (
            <Link to={`/${i18n.language}`} id="back-button">
              {t('result_restart')}
            </Link>
          ) : (
            <div></div>
          )}
          <button id="submit-button" type="button" disabled={issuing} onClick={() => addToApp(issuance)}>
            {t('result_issue_button')}
          </button>
        </div>
      </footer>
    </div>
  );
}
