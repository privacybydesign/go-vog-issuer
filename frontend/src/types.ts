export interface ValidationInfo {
  code: number;
  key: string;
  description: string;
  authentic: boolean;
  retryable: boolean;
}

export interface ProfileInfo {
  code: string;
  risk_area: string;
  description: string;
  description_en: string;
}

export interface DocumentInfo {
  reference_number: string;
  issue_date: string;
  surname: string;
  prefix: string;
  given_names: string;
  date_of_birth: string;
  place_of_birth: string;
  country_of_birth: string;
  purpose: string;
  profile_codes: string[];
  profiles: ProfileInfo[];
}

export interface UploadResponse {
  session_id: string;
  validation: ValidationInfo;
  document: DocumentInfo;
}

export interface IdentityMatchInfo {
  source: string;
  matched: boolean;
  date_of_birth_match: boolean;
  surname_match: boolean;
  given_names_match: boolean;
  reasons?: string[];
}

export interface IssuanceResponse {
  jwt: string;
  irma_server_url: string;
  identity: IdentityMatchInfo;
}

/**
 * Outcome of comparing the disclosed identity with the VOG. On a match the
 * backend has already consumed the session and handed out the signed issuance
 * request, so it must travel with the outcome to the result page.
 */
export type MatchOutcome =
  | { matched: true; issuance: IssuanceResponse }
  | { matched: false; identity: IdentityMatchInfo };

export interface ErrorResponse {
  error: string;
  message?: string;
  validation?: ValidationInfo;
  identity?: IdentityMatchInfo;
}
