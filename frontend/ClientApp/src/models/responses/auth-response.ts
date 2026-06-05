export interface AuthCallbackResponse {
  access_token: string
  expires_in: number
  token_type: string
}

export interface LoginResponse {
  request_uri: string
  presentation_url: string
  state: string
  authorize_url: string
  expires_in: number
}

export interface LogoutResponse {
  logout_url: string
}

export interface LoginStatusResponse {
  state: string
  status: 'pending' | 'complete' | 'failed' | 'expired'
  expires_in: number
  redirect_uri?: string
  error_message?: string
}

export type LoginPollError = 'timeout' | 'not_found'

export const LOGIN_POLL_ERROR = {
  TIMEOUT: 'timeout',
  NOT_FOUND: 'not_found',
} as const satisfies Record<string, LoginPollError>

export type LoginPollStatus = LoginStatusResponse | LoginPollError | null

export function isLoginStatusResponse(value: LoginPollStatus): value is LoginStatusResponse {
  return typeof value === 'object' && value !== null && 'status' in value
}

export function isLoginPollError(value: LoginPollStatus): value is LoginPollError {
  return value === LOGIN_POLL_ERROR.TIMEOUT || value === LOGIN_POLL_ERROR.NOT_FOUND
}
