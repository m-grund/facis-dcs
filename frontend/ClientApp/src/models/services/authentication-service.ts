import type { LoginPollStatus } from '@/models/responses/auth-response'

export interface AuthenticationService {
  login: () => Promise<{
    presentationUrl: string
    state: string
    authorizeUrl: string
    expiresIn: number
  } | null>
  loginRenew: (state: string) => Promise<{
    presentationUrl: string
    state: string
    authorizeUrl: string
    expiresIn: number
  } | null>
  loginPollStatus: (state: string) => Promise<LoginPollStatus>
  refresh: () => Promise<boolean>
  logout: () => void
}
