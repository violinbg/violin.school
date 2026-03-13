import { Component, EventEmitter, inject, Input, OnChanges, Output, signal } from '@angular/core';
import { FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';
import { CommonModule } from '@angular/common';
import { ButtonModule } from 'primeng/button';
import { DialogModule } from 'primeng/dialog';
import { InputTextModule } from 'primeng/inputtext';
import { PasswordModule } from 'primeng/password';
import { TooltipModule } from 'primeng/tooltip';
import { MessageService } from 'primeng/api';
import { TranslatePipe, TranslateService } from '@ngx-translate/core';
import { AuthService, CaptchaChallenge } from '../../core/services/auth.service';
import { firstValueFrom } from 'rxjs';

@Component({
  selector: 'vs-register-dialog',
  standalone: true,
  imports: [
    CommonModule,
    ReactiveFormsModule,
    DialogModule,
    InputTextModule,
    PasswordModule,
    ButtonModule,
    TooltipModule,
    TranslatePipe,
  ],
  templateUrl: './register-dialog.component.html',
  styleUrl: './register-dialog.component.scss',
})
export class RegisterDialogComponent implements OnChanges {
  @Input() visible = false;
  @Output() visibleChange = new EventEmitter<boolean>();
  @Output() registered = new EventEmitter<void>();

  private readonly fb = inject(FormBuilder);
  private readonly authService = inject(AuthService);
  private readonly messageService = inject(MessageService);
  private readonly translate = inject(TranslateService);

  captcha = signal<CaptchaChallenge | null>(null);
  loading = signal(false);

  form = this.fb.nonNullable.group({
    username: ['', [Validators.required, Validators.minLength(3)]],
    full_name: ['', Validators.required],
    password: ['', [Validators.required, Validators.minLength(8)]],
    captcha_answer: ['', Validators.required],
  });

  ngOnChanges(): void {
    if (this.visible) {
      this.form.reset();
      this.loadCaptcha();
    }
  }

  async loadCaptcha(): Promise<void> {
    try {
      const c = await firstValueFrom(this.authService.getCaptcha());
      this.captcha.set(c);
      this.form.controls.captcha_answer.reset('');
    } catch {
      this.messageService.add({
        severity: 'error',
        summary: this.translate.instant('USERS.TOAST_ERROR'),
        detail: this.translate.instant('AUTH.REGISTER.ERROR_CAPTCHA_LOAD'),
      });
    }
  }

  async submit(): Promise<void> {
    if (this.form.invalid) return;
    const c = this.captcha();
    if (!c) return;

    this.loading.set(true);
    try {
      await this.authService.register({
        ...this.form.getRawValue(),
        captcha_id: c.id,
      });
      this.messageService.add({
        severity: 'success',
        summary: this.translate.instant('AUTH.REGISTER.SUCCESS_TITLE'),
        detail: this.translate.instant('AUTH.REGISTER.SUCCESS_DETAIL'),
      });
      this.close();
      this.registered.emit();
    } catch (err: any) {
      const msg = err?.error?.error ?? this.translate.instant('AUTH.REGISTER.ERROR_FAILED');
      this.messageService.add({
        severity: 'error',
        summary: this.translate.instant('USERS.TOAST_ERROR'),
        detail: msg,
      });
      this.loadCaptcha();
    } finally {
      this.loading.set(false);
    }
  }

  close(): void {
    this.visibleChange.emit(false);
  }
}
