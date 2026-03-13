import { inject, Injectable, signal } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { firstValueFrom, Observable } from 'rxjs';
import { LanguageService } from './language.service';

export interface CurrentUser {
  id: string;
  username: string;
  full_name: string;
  role: string;
  language?: string;
}

export interface CaptchaChallenge {
  id: string;
  question: string;
}

export interface RegisterRequest {
  username: string;
  full_name: string;
  password: string;
  captcha_id: string;
  captcha_answer: string;
}

const TOKEN_KEY = 'vs_token';
const REFRESH_TOKEN_KEY = 'vs_refresh_token';

@Injectable({ providedIn: 'root' })
export class AuthService {
  private readonly http = inject(HttpClient);
  private readonly languageService = inject(LanguageService);

  readonly isInitialized = signal(false);
  readonly isLoggedIn = signal(false);
  readonly currentUser = signal<CurrentUser | null>(null);
  readonly registrationEnabled = signal(false);

  private refreshPromise: Promise<string> | null = null;

  /** Called once at app boot via APP_INITIALIZER. */
  async init(): Promise<void> {
    const status = await firstValueFrom(
      this.http.get<{ initialized: boolean; registration_enabled: boolean }>('/api/v1/setup/status')
    ).catch(() => ({ initialized: false, registration_enabled: false }));

    this.isInitialized.set(status.initialized);
    this.registrationEnabled.set(status.registration_enabled ?? false);

    const token = localStorage.getItem(TOKEN_KEY);
    if (!token) return;

    const user = await firstValueFrom(
      this.http.get<CurrentUser>('/api/v1/auth/me')
    ).catch(() => null);

    if (user) {
      this.isLoggedIn.set(true);
      this.currentUser.set(user);
      this.languageService.applyUserLanguage(user.language);
    } else {
      localStorage.removeItem(TOKEN_KEY);
      localStorage.removeItem(REFRESH_TOKEN_KEY);
    }
  }

  private storeTokens(accessToken: string, refreshToken: string, user: CurrentUser): void {
    localStorage.setItem(TOKEN_KEY, accessToken);
    localStorage.setItem(REFRESH_TOKEN_KEY, refreshToken);
    this.isLoggedIn.set(true);
    this.currentUser.set(user);
    this.languageService.applyUserLanguage(user.language);
  }

  async login(username: string, password: string): Promise<void> {
    const res = await firstValueFrom(
      this.http.post<{ access_token: string; refresh_token: string; user: CurrentUser }>('/api/v1/auth/login', {
        username,
        password
      })
    );
    this.storeTokens(res.access_token, res.refresh_token, res.user);
  }

  setRegistrationEnabled(value: boolean): void {
    this.registrationEnabled.set(value);
  }

  getCaptcha(): Observable<CaptchaChallenge> {
    return this.http.get<CaptchaChallenge>('/api/v1/auth/captcha');
  }

  async register(req: RegisterRequest): Promise<void> {
    const res = await firstValueFrom(
      this.http.post<{ access_token: string; refresh_token: string; user: CurrentUser }>('/api/v1/auth/register', req)
    );
    this.storeTokens(res.access_token, res.refresh_token, res.user);
  }

  /**
   * Silently refreshes the access token using the stored refresh token.
   * Concurrent callers share the same in-flight request (deduplication).
   * Returns the new access token, or throws if refresh fails.
   */
  refreshToken(): Promise<string> {
    if (this.refreshPromise) return this.refreshPromise;

    this.refreshPromise = this._doRefresh().finally(() => {
      this.refreshPromise = null;
    });

    return this.refreshPromise;
  }

  private async _doRefresh(): Promise<string> {
    const refreshToken = localStorage.getItem(REFRESH_TOKEN_KEY);
    if (!refreshToken) throw new Error('No refresh token available');

    const res = await firstValueFrom(
      this.http.post<{ access_token: string; refresh_token: string }>('/api/v1/auth/refresh', {
        refresh_token: refreshToken
      })
    );

    // Guard: logout() may have been called while the request was in-flight.
    // If the session was cleared, discard the new tokens instead of restoring them.
    if (!this.isLoggedIn()) {
      throw new Error('Session ended during refresh');
    }

    localStorage.setItem(TOKEN_KEY, res.access_token);
    localStorage.setItem(REFRESH_TOKEN_KEY, res.refresh_token);
    return res.access_token;
  }

  logout(): void {
    const refreshToken = localStorage.getItem(REFRESH_TOKEN_KEY);
    // Null out any in-flight refresh so its .finally() cleanup is a no-op and
    // the isLoggedIn() guard in _doRefresh discards the response.
    this.refreshPromise = null;
    localStorage.removeItem(TOKEN_KEY);
    localStorage.removeItem(REFRESH_TOKEN_KEY);
    this.isLoggedIn.set(false);
    this.currentUser.set(null);
    if (refreshToken) {
      // Best-effort revocation — don't await or block on failure.
      this.http.post('/api/v1/auth/logout', { refresh_token: refreshToken }).subscribe({ error: () => {} });
    }
  }

  isAdmin(): boolean {
    const user = this.currentUser();
    return user?.role === 'admin' || false;
  }

  getToken(): string | null {
    return localStorage.getItem(TOKEN_KEY);
  }
}
