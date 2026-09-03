import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useNavigate } from 'react-router-dom';
import { useAppContext } from '../AppContext';
import { ApiError, RetryProgress, UploadCancelled, isValidationServiceUnavailable, uploadVogWithRetry } from '../api';
import { checkPdfFile, fileCheckErrorKey, formatBytes, MAX_UPLOAD_BYTES } from '../fileCheck';
import FileDropzone from '../components/FileDropzone';
import DocumentSummary from './DocumentSummary';

/** A pending automatic retry: which attempt is next and when it starts. */
interface PendingRetry extends RetryProgress {
  retryAt: number;
}

export default function UploadPage() {
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const { upload, setUpload } = useAppContext();
  const [file, setFile] = useState<File | undefined>();
  const [fileError, setFileError] = useState<string | undefined>();
  const [busy, setBusy] = useState(false);
  const [retry, setRetry] = useState<PendingRetry | undefined>();
  const [now, setNow] = useState(() => Date.now());
  const [errorMessage, setErrorMessage] = useState<string | undefined>();
  const [errorDetail, setErrorDetail] = useState<string | undefined>();
  const abortRef = useRef<AbortController | undefined>(undefined);

  // Tick while a retry is pending so the countdown stays current.
  useEffect(() => {
    if (!retry) {
      return;
    }
    setNow(Date.now());
    const timer = setInterval(() => setNow(Date.now()), 250);
    return () => clearInterval(timer);
  }, [retry]);

  // Leaving the page cancels any upload or retry still in progress.
  useEffect(() => () => abortRef.current?.abort(), []);

  const select = async (picked: File | undefined) => {
    setErrorMessage(undefined);
    setErrorDetail(undefined);
    setFileError(undefined);
    if (!picked) {
      setFile(undefined);
      return;
    }
    const problem = await checkPdfFile(picked);
    if (problem) {
      setFile(undefined);
      setFileError(t(fileCheckErrorKey(problem)));
      return;
    }
    setFile(picked);
  };

  const submit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    if (busy) {
      return;
    }
    if (!file) {
      const dialog:any = document.getElementById('no-upload');
      dialog.showModal();
      return;
    }
    const controller = new AbortController();
    abortRef.current = controller;
    setBusy(true);
    setRetry(undefined);
    setErrorMessage(undefined);
    setErrorDetail(undefined);
    try {
      const result = await uploadVogWithRetry(file, {
        signal: controller.signal,
        onRetry: (progress) => setRetry({ ...progress, retryAt: Date.now() + progress.delayMs }),
      });
      setUpload(result);
    } catch (err) {
      if (err instanceof UploadCancelled) {
        return;
      }
      if (isValidationServiceUnavailable(err)) {
        setErrorMessage(t('upload_retry_failed'));
        if (err.body.validation) {
          setErrorDetail(t(`validation_${err.body.validation.key}`));
        }
      } else if (err instanceof ApiError) {
        setErrorMessage(t(err.translationKey));
        if (err.body.validation) {
          setErrorDetail(t(`validation_${err.body.validation.key}`));
        }
      } else {
        navigate(`/${i18n.language}/error`);
      }
    } finally {
      if (abortRef.current === controller) {
        abortRef.current = undefined;
      }
      setRetry(undefined);
      setBusy(false);
    }
  };

  const closeModal = () => {
    const dialog:any = document.getElementById('no-upload');
    dialog.close();
  }

  const stopRetrying = () => {
    abortRef.current?.abort();
  };

  const reset = () => {
    setUpload(undefined);
    setFile(undefined);
    setFileError(undefined);
    setErrorMessage(undefined);
    setErrorDetail(undefined);
  };

  if (upload) {
    return (
      <div id="container">
        <header>
          <h1>{t('upload_header')}</h1>
        </header>
        <main>
          <div className="sms-form">
            <div id="status-bar" className="alert alert-success" role="alert">
              <div className="status-container">
                <div id="status">{t('upload_success')}</div>
              </div>
            </div>
            <p>{t('upload_result_header')}</p>
            <DocumentSummary document={upload.document} />
          </div>
        </main>
        <footer>
          <div className="actions">
            <a href="#" id="back-button" onClick={(e) => { e.preventDefault(); reset(); }}>
              {t('upload_other_file')}
            </a>
            <button id="submit-button" type="button" onClick={() => navigate(`/${i18n.language}/verify`)}>
              {t('upload_continue')}
            </button>
          </div>
        </footer>
      </div>
    );
  }

  const secondsLeft = retry ? Math.max(0, Math.ceil((retry.retryAt - now) / 1000)) : 0;

  return (
    <form id="container" onSubmit={submit}>
      <header>
        <h1>{t('upload_header')}</h1>
      </header>
      <main>
        <div className="sms-form">
          {errorMessage && (
            <div id="status-bar" className="alert alert-danger" role="alert">
              <div className="status-container">
                <div id="status">
                  {errorMessage}
                  {errorDetail && <><br />{errorDetail}</>}
                </div>
              </div>
            </div>
          )}
          {busy && !retry && (
            <div id="status-bar" className="alert alert-info" role="status">
              <div className="status-container">
                <div id="status">{t('upload_busy')}</div>
              </div>
            </div>
          )}
          {busy && retry && (
            <div id="status-bar" className="alert alert-warning" role="status" aria-live="polite">
              <div className="status-container">
                <div id="status">
                  <div>{t('upload_service_down')}</div>
                  <div>
                    {secondsLeft > 0
                      ? t('upload_retrying_in', { seconds: secondsLeft, attempt: retry.attempt, max: retry.maxAttempts })
                      : t('upload_retrying_now', { attempt: retry.attempt, max: retry.maxAttempts })}
                  </div>
                  <button type="button" className="status-action" onClick={stopRetrying}>
                    {t('upload_retry_stop')}
                  </button>
                </div>
              </div>
            </div>
          )}
          <p>{t('upload_explanation')}</p>
          <p>{t('upload_validation_explanation')}</p>
          <label htmlFor="vog-file">{t('upload_file_label', { max: formatBytes(MAX_UPLOAD_BYTES) })}</label>
          <FileDropzone file={file} error={fileError} disabled={busy} onSelect={select} />
        </div>
      </main>
      <footer>
        <div className="actions">
          <Link to={`/${i18n.language}`} id="back-button">
            {t('back')}
          </Link>
          <button id="submit-button" type="submit" disabled={busy}>{t('upload_button')}</button>
        </div>
        <dialog id="no-upload">
          <p>{t('dialog_no_upload')}</p>
          <button onClick={closeModal}>OK</button>
        </dialog>
      </footer>
    </form>
  );
}
