import { Component, EventEmitter, inject, Output, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { InputTextModule } from 'primeng/inputtext';
import { PasswordModule } from 'primeng/password';
import { ButtonModule } from 'primeng/button';
import { DialogModule } from 'primeng/dialog';
import { MessageModule } from 'primeng/message';
import { TranslatePipe, TranslateService } from '@ngx-translate/core';
import { AuthService } from '../../core/services/auth.service';

@Component({
  selector: 'vs-login-dialog',
  standalone: true,
  imports: [FormsModule, DialogModule, InputTextModule, PasswordModule, ButtonModule, MessageModule, TranslatePipe],
  templateUrl: './login-dialog.component.html',
  styleUrl: './login-dialog.component.scss'
})
export class LoginDialogComponent {
  readonly auth = inject(AuthService);
  private readonly translate = inject(TranslateService);

  @Output() closed = new EventEmitter<void>();
  @Output() registerRequested = new EventEmitter<void>();

  visible = true;
  username = '';
  password = '';
  loading = signal(false);
  error = signal('');

  async submit(): Promise<void> {
    if (!this.username || !this.password) return;
    this.loading.set(true);
    this.error.set('');
    try {
      await this.auth.login(this.username, this.password);
      this.close();
    } catch {
      this.error.set(this.translate.instant('AUTH.LOGIN.ERROR_INVALID'));
    } finally {
      this.loading.set(false);
    }
  }

  onEnter(event: Event): void {
    event.preventDefault();
    void this.submit();
  }

  openRegister(): void {
    this.visible = false;
    this.closed.emit();
    this.registerRequested.emit();
  }

  close(): void {
    this.visible = false;
    this.closed.emit();
  }
}
