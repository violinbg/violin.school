import { inject, Injectable } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { TranslateService } from '@ngx-translate/core';
import { firstValueFrom } from 'rxjs';

export interface Language {
  code: string;
  label: string;
  flag: string;
}

export const SUPPORTED_LANGUAGES: Language[] = [
  { code: 'en', label: 'English',    flag: '🇺🇸' },
  { code: 'bg', label: 'Български',  flag: '🇧🇬' },
  { code: 'es', label: 'Español',    flag: '🇪🇸' },
  { code: 'ja', label: '日本語',      flag: '🇯🇵' },
  { code: 'ko', label: '한국어',      flag: '🇰🇷' },
  { code: 'zh', label: '中文',        flag: '🇨🇳' },
];

const LANGUAGE_KEY = 'vr_language';

@Injectable({ providedIn: 'root' })
export class LanguageService {
  private readonly translate = inject(TranslateService);
  private readonly http = inject(HttpClient);

  readonly languages = SUPPORTED_LANGUAGES;

  /** Called once at app boot via APP_INITIALIZER. */
  init(): void {
    const saved = localStorage.getItem(LANGUAGE_KEY);
    const lang = this.isSupported(saved) ? saved! : 'en';
    this.translate.addLangs(SUPPORTED_LANGUAGES.map(l => l.code));
    this.translate.setDefaultLang('en');
    this.translate.use(lang);
  }

  /**
   * Switches the active language. Persists to localStorage immediately and,
   * if a token is present (user is logged in), syncs the preference to the API.
   */
  async setLanguage(code: string): Promise<void> {
    if (!this.isSupported(code)) return;
    this.translate.use(code);
    localStorage.setItem(LANGUAGE_KEY, code);

    const hasToken = !!localStorage.getItem('vr_token');
    if (hasToken) {
      try {
        await firstValueFrom(
          this.http.patch('/api/v1/profile/language', { language: code })
        );
      } catch {
        // Non-critical — language is already applied locally
      }
    }
  }

  /** Called after a successful login to apply the user's stored language preference. */
  applyUserLanguage(language: string | null | undefined): void {
    const lang = this.isSupported(language) ? language! : 'en';
    if (lang !== this.translate.currentLang) {
      this.translate.use(lang);
    }
    localStorage.setItem(LANGUAGE_KEY, lang);
  }

  get currentLang(): string {
    return this.translate.currentLang ?? 'en';
  }

  private isSupported(code: string | null | undefined): code is string {
    return !!code && SUPPORTED_LANGUAGES.some(l => l.code === code);
  }
}
