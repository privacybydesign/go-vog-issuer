import { DragEvent, useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { MAX_UPLOAD_BYTES, formatBytes } from '../fileCheck';

interface Props {
  /** The currently selected (and accepted) file, if any. */
  file?: File;
  /** Translated error message to show under the zone. */
  error?: string;
  disabled?: boolean;
  /** Called with the picked file, or undefined when the selection is cleared. */
  onSelect: (file: File | undefined) => void;
}

/**
 * Drop zone plus "choose file" button for the VOG PDF. The native file input
 * stays in the DOM (visually hidden) so keyboard users and screen readers get
 * the standard control, while pointer users get a large drop target.
 */
export default function FileDropzone({ file, error, disabled, onSelect }: Props) {
  const { t } = useTranslation();
  const inputRef = useRef<HTMLInputElement>(null);
  const [dragging, setDragging] = useState(false);
  const errorId = useId();
  const hintId = useId();

  const openPicker = () => {
    if (!disabled) {
      inputRef.current?.click();
    }
  };

  const pick = (picked: File | undefined) => {
    onSelect(picked);
    // Reset the input so picking the same file again re-triggers onChange.
    if (inputRef.current) {
      inputRef.current.value = '';
    }
  };

  const onDragOver = (e: DragEvent<HTMLDivElement>) => {
    e.preventDefault();
    if (!disabled) {
      e.dataTransfer.dropEffect = 'copy';
      setDragging(true);
    }
  };

  const onDrop = (e: DragEvent<HTMLDivElement>) => {
    e.preventDefault();
    setDragging(false);
    if (disabled) {
      return;
    }
    pick(e.dataTransfer.files?.[0]);
  };

  const className = [
    'dropzone',
    dragging ? 'dropzone-dragging' : '',
    error ? 'dropzone-error' : '',
    file ? 'dropzone-filled' : '',
    disabled ? 'dropzone-disabled' : '',
  ].join(' ').trim();

  return (
    <div className="dropzone-wrapper">
      <div
        className={className}
        onDragOver={onDragOver}
        onDragEnter={onDragOver}
        onDragLeave={() => setDragging(false)}
        onDrop={onDrop}
        onClick={file ? undefined : openPicker}
        data-testid="dropzone"
      >
        <input
          ref={inputRef}
          id="vog-file"
          className="dropzone-input"
          type="file"
          accept="application/pdf,.pdf"
          disabled={disabled}
          aria-describedby={error ? `${hintId} ${errorId}` : hintId}
          aria-invalid={error ? true : undefined}
          onChange={(e) => pick(e.target.files?.[0])}
          onClick={(e) => e.stopPropagation()}
        />

        {file ? (
          <div className="dropzone-file">
            <PdfIcon />
            <div className="dropzone-file-info">
              <div className="dropzone-file-name" title={file.name}>{file.name}</div>
              <div className="dropzone-file-meta">
                {formatBytes(file.size)} &middot; PDF &middot; <span className="dropzone-file-ready">{t('upload_file_ready')}</span>
              </div>
            </div>
            <button
              type="button"
              className="dropzone-remove"
              onClick={(e) => { e.stopPropagation(); pick(undefined); }}
              disabled={disabled}
              aria-label={t('upload_remove_file')}
              title={t('upload_remove_file')}
            >
              <CloseIcon />
            </button>
          </div>
        ) : (
          <div className="dropzone-empty">
            <UploadIcon />
            <div className="dropzone-title">{t('upload_drop_title')}</div>
            <div className="dropzone-or">{t('upload_drop_or')}</div>
            <button
              type="button"
              className="dropzone-button"
              onClick={(e) => { e.stopPropagation(); openPicker(); }}
              disabled={disabled}
            >
              {t('upload_choose_file')}
            </button>
          </div>
        )}
      </div>

      {file && (
        <button
          type="button"
          className="dropzone-change"
          onClick={openPicker}
          disabled={disabled}
        >
          {t('upload_change_file')}
        </button>
      )}

      <div id={hintId} className="dropzone-hint">
        {t('upload_file_requirements', { max: formatBytes(MAX_UPLOAD_BYTES) })}
      </div>
      {error && (
        <div id={errorId} className="dropzone-message" role="alert">
          <WarningIcon />
          <span>{error}</span>
        </div>
      )}
    </div>
  );
}

function UploadIcon() {
  return (
    <svg className="dropzone-icon" viewBox="0 0 48 48" width="48" height="48" aria-hidden="true" focusable="false">
      <path d="M14 6h14l10 10v24a2 2 0 0 1-2 2H14a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2z" fill="#FFFFFF" stroke="currentColor" strokeWidth="2" strokeLinejoin="round" />
      <path d="M28 6v10h10" fill="none" stroke="currentColor" strokeWidth="2" strokeLinejoin="round" />
      <path d="M24 34V22m0 0-5 5m5-5 5 5" fill="none" stroke="#E12747" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

function PdfIcon() {
  return (
    <svg className="dropzone-icon dropzone-icon-small" viewBox="0 0 48 48" width="40" height="40" aria-hidden="true" focusable="false">
      <path d="M14 6h14l10 10v24a2 2 0 0 1-2 2H14a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2z" fill="#FFFFFF" stroke="currentColor" strokeWidth="2" strokeLinejoin="round" />
      <path d="M28 6v10h10" fill="none" stroke="currentColor" strokeWidth="2" strokeLinejoin="round" />
      <rect x="8" y="24" width="24" height="12" rx="2" fill="#E12747" />
      <text x="20" y="33.5" textAnchor="middle" fontFamily="Alexandria, Verdana, Arial, sans-serif" fontWeight="bold" fontSize="8.5" fill="#FFFFFF">PDF</text>
    </svg>
  );
}

function CloseIcon() {
  return (
    <svg viewBox="0 0 20 20" width="16" height="16" aria-hidden="true" focusable="false">
      <path d="M5 5l10 10M15 5L5 15" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
    </svg>
  );
}

function WarningIcon() {
  return (
    <svg viewBox="0 0 20 20" width="18" height="18" aria-hidden="true" focusable="false">
      <path d="M10 2.5 18.5 17H1.5z" fill="currentColor" />
      <path d="M10 8v4.5" stroke="#FFFFFF" strokeWidth="1.8" strokeLinecap="round" />
      <circle cx="10" cy="14.8" r="1" fill="#FFFFFF" />
    </svg>
  );
}
