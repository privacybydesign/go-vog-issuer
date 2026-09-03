import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router-dom';
import { useAppContext } from '../AppContext';

export default function DonePage() {
  const { t, i18n } = useTranslation();
  const { setUpload, setOutcome } = useAppContext();

  // The credential is in the app: the flow is over, forget the VOG and the
  // (already used) issuance request.
  useEffect(() => {
    setUpload(undefined);
    setOutcome(undefined);
  }, [setUpload, setOutcome]);

  return (
    <div id="container">
      <header>
        <h1>{t('done_header')}</h1>
      </header>
      <main>
        <div className="sms-form">
          <div className="imageContainer">
            <img src="/images/done.png" alt="done" />
            <p>{t('thank_you')}</p>
          </div>
        </div>
      </main>
      <footer>
        <div className="actions">
          <Link to={`/${i18n.language}`} id="back-button">
            {t('again')}
          </Link>
          <div></div>
        </div>
      </footer>
    </div>
  );
}
