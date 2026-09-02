import { useTranslation } from 'react-i18next';
import { Link } from 'react-router-dom';

export default function DonePage() {
  const { t, i18n } = useTranslation();

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
