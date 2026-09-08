import { useTranslation } from 'react-i18next';
import { DocumentInfo } from '../types';

export function fullName(document: DocumentInfo): string {
  return [document.given_names, document.prefix, document.surname]
    .filter((part) => part && part.trim() !== '')
    .join(' ');
}

/** Renders a backend YYYY-MM-DD date as DD-MM-YYYY, regardless of viewer locale. */
export function formatDate(isoDate: string): string {
  const [year, month, day] = isoDate.split('-');
  return `${day}-${month}-${year}`;
}

export default function DocumentSummary({ document }: { document: DocumentInfo }) {
  const { t, i18n } = useTranslation();
  const english = i18n.language.startsWith('en');

  return (
    <table className="table">
      <tbody>
        <tr>
          <th>{t('field_reference_number')}</th>
          <td>{document.reference_number}</td>
        </tr>
        <tr>
          <th>{t('field_issue_date')}</th>
          <td>{document.issue_date}</td>
        </tr>
        <tr>
          <th>{t('field_name')}</th>
          <td>{fullName(document)}</td>
        </tr>
        <tr>
          <th>{t('field_date_of_birth')}</th>
          <td>{formatDate(document.date_of_birth)}</td>
        </tr>
        <tr>
          <th>{t('field_place_of_birth')}</th>
          <td>{[document.place_of_birth, document.country_of_birth].filter(Boolean).join(', ')}</td>
        </tr>
        <tr>
          <th>{t('field_purpose')}</th>
          <td>{document.purpose}</td>
        </tr>
        <tr>
          <th>{t('field_profile')}</th>
          <td>
            <ul className="profile-list">
              {document.profiles.map((profile) => (
                <li key={profile.code}>
                  <b>{profile.code}</b>: {english ? profile.description_en : profile.description}
                </li>
              ))}
            </ul>
          </td>
        </tr>
      </tbody>
    </table>
  );
}
