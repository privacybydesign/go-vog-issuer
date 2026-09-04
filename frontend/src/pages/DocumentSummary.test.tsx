import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import '../i18n';
import { DocumentInfo } from '../types';
import DocumentSummary, { formatDate } from './DocumentSummary';

const document: DocumentInfo = {
  reference_number: '9999012026032500922',
  issue_date: '2026-03-25',
  surname: 'Jansen',
  prefix: 'de',
  given_names: 'Jan',
  date_of_birth: '1991-05-14',
  place_of_birth: 'Barneveld',
  country_of_birth: 'Nederland',
  purpose: 'Werk',
  profile_codes: [],
  profiles: [],
};

describe('formatDate', () => {
  it('renders a backend YYYY-MM-DD date as DD-MM-YYYY', () => {
    expect(formatDate('1991-05-14')).toBe('14-05-1991');
  });
});

describe('DocumentSummary', () => {
  it('shows the date of birth as DD-MM-YYYY instead of the raw ISO value', () => {
    render(<DocumentSummary document={document} />);
    expect(screen.getByText('14-05-1991')).toBeInTheDocument();
    expect(screen.queryByText('1991-05-14')).not.toBeInTheDocument();
  });
});
