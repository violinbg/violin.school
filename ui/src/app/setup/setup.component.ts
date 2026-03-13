import { Component, inject, signal } from '@angular/core';
import { Router } from '@angular/router';
import { FormsModule } from '@angular/forms';
import { HttpClient } from '@angular/common/http';
import { firstValueFrom } from 'rxjs';
import { InputTextModule } from 'primeng/inputtext';
import { PasswordModule } from 'primeng/password';
import { ButtonModule } from 'primeng/button';
import { MessageModule } from 'primeng/message';
import { AuthService } from '../core/services/auth.service';
import { TranslatePipe, TranslateService } from '@ngx-translate/core';

@Component({
  selector: 'vs-setup',
  standalone: true,
  imports: [FormsModule, InputTextModule, PasswordModule, ButtonModule, MessageModule, TranslatePipe],
  templateUrl: './setup.component.html',
  styleUrl: './setup.component.scss'
})
export class SetupComponent {
  private readonly http = inject(HttpClient);
  private readonly auth = inject(AuthService);
  private readonly router = inject(Router);
  private readonly translate = inject(TranslateService);

  username = '';
  fullName = '';
  password = '';
  confirmPassword = '';

  loading = signal(false);
  error = signal('');

  get passwordMismatch(): boolean {
    return this.confirmPassword.length > 0 && this.password !== this.confirmPassword;
  }

  async submit(): Promise<void> {
    if (this.passwordMismatch || !this.username || !this.fullName || !this.password) return;

    this.loading.set(true);
    this.error.set('');

    try {
      await firstValueFrom(
        this.http.post('/api/v1/setup', {
          username: this.username,
          full_name: this.fullName,
          password: this.password
        })
      );
      // Auto-login after setup.
      await this.auth.login(this.username, this.password);
      this.auth.isInitialized.set(true);
      this.router.navigate(['/']);
    } catch (err: any) {
      this.error.set(err?.error?.error ?? this.translate.instant('SETUP.ERROR'));
    } finally {
      this.loading.set(false);
    }
  }
}
