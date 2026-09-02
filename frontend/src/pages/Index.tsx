import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { useAppContext } from '../AppContext';

export default function IndexPage() {
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const { setUpload } = useAppContext();

  const start = (e: React.FormEvent) => {
    e.preventDefault();
    setUpload(undefined);
    navigate(`/${i18n.language}/upload`);
  };

  return (
    <form id="container" onSubmit={start}>
      <header>
        <h1>{t('index_header')}</h1>
      </header>
      <main>
        <div className="sms-form">
          <p>{t('index_explanation')}</p>
          <b>{t('index_steps')}</b>
          <ol>
            <li>{t('index_step_1')}</li>
            <li>{t('index_step_2')}</li>
            <li>{t('index_step_3')}</li>
          </ol>
          <p className="details">{t('index_privacy')}</p>
        </div>
      </main>
      <footer>
        <div className="actions">
          <div></div>
          <button id="submit-button" type="submit">{t('index_start')}</button>
        </div>
      </footer>
    </form>
  );
}
