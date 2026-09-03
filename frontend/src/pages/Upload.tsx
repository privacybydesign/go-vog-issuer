import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useNavigate } from 'react-router-dom';
import { useAppContext } from '../AppContext';
import { ApiError, uploadVog } from '../api';
import {checkPdfFile, fileCheckErrorKey, formatBytes, MAX_UPLOAD_BYTES} from '../fileCheck';
import { UploadResponse } from '../types';
import FileDropzone from '../components/FileDropzone';
import DocumentSummary from './DocumentSummary';

export default function UploadPage() {
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const { upload, setUpload } = useAppContext();
  const [file, setFile] = useState<File | undefined>();
  const [fileError, setFileError] = useState<string | undefined>();
  const [busy, setBusy] = useState(false);
  const [errorMessage, setErrorMessage] = useState<string | undefined>();
  const [errorDetail, setErrorDetail] = useState<string | undefined>();

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
    if (!file || busy) {
      return;
    }
    setBusy(true);
    setErrorMessage(undefined);
    setErrorDetail(undefined);
    try {
      const response = await uploadVog(file);
      const result: UploadResponse = await response.json();
      setUpload(result);
    } catch (err) {
      if (err instanceof ApiError) {
        setErrorMessage(t(err.translationKey));
        if (err.body.validation) {
          setErrorDetail(t(`validation_${err.body.validation.key}`));
        }
      } else {
        navigate(`/${i18n.language}/error`);
      }
    } finally {
      setBusy(false);
    }
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
          {busy && (
            <div id="status-bar" className="alert alert-info" role="status">
              <div className="status-container">
                <div id="status">{t('upload_busy')}</div>
              </div>
            </div>
          )}
          <p>{t('upload_explanation')}</p>
          <label htmlFor="vog-file">{t('upload_file_label', { max: formatBytes(MAX_UPLOAD_BYTES) })}</label>
          <FileDropzone file={file} error={fileError} disabled={busy} onSelect={select} />
        </div>
      </main>
      <footer>
        <div className="actions">
          <Link to={`/${i18n.language}`} id="back-button">
            {t('back')}
          </Link>
          <button id="submit-button" type="submit" disabled={!file || busy}>{t('upload_button')}</button>
        </div>
      </footer>
    </form>
  );
}
