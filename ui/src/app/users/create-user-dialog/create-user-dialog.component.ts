import { Component, EventEmitter, inject, Input, OnChanges, Output, SimpleChanges } from '@angular/core';
import { FormsModule, ReactiveFormsModule, FormBuilder, Validators } from '@angular/forms';
import { ButtonModule } from 'primeng/button';
import { DialogModule } from 'primeng/dialog';
import { InputTextModule } from 'primeng/inputtext';
import { SelectModule } from 'primeng/select';
import { MessageService } from 'primeng/api';
import { UserService, CreateUserRequest } from '../../core/services/user.service';
import { TranslatePipe, TranslateService } from '@ngx-translate/core';

@Component({
  selector: 'vs-create-user-dialog',
  standalone: true,
  imports: [FormsModule, ReactiveFormsModule, ButtonModule, DialogModule, InputTextModule, SelectModule, TranslatePipe],
  templateUrl: './create-user-dialog.component.html',
  styleUrl: './create-user-dialog.component.scss',
})
export class CreateUserDialogComponent implements OnChanges {
  private readonly userSvc = inject(UserService);
  private readonly messageService = inject(MessageService);
  private readonly fb = inject(FormBuilder);
  private readonly translate = inject(TranslateService);

  @Input() visible = false;

  @Output() visibleChange = new EventEmitter<boolean>();
  @Output() created = new EventEmitter<void>();

  form = this.fb.group({
    username: ['', [Validators.required, Validators.minLength(3)]],
    full_name: ['', Validators.required],
    password: ['', [Validators.required, Validators.minLength(8)]],
    role: ['user', Validators.required],
  });

  get roles() {
    return [
      { label: this.translate.instant('CREATE_USER_DIALOG.ROLE_ADMIN'), value: 'admin' },
      { label: this.translate.instant('CREATE_USER_DIALOG.ROLE_USER'), value: 'user' },
    ];
  }

  ngOnChanges(changes: SimpleChanges): void {
    if (changes['visible']?.currentValue === true) {
      this.form.reset({ role: 'user' });
    }
  }

  close(): void {
    this.visibleChange.emit(false);
  }

  async create(): Promise<void> {
    if (!this.form.valid) return;

    const req: CreateUserRequest = {
      username: this.form.get('username')?.value ?? '',
      full_name: this.form.get('full_name')?.value ?? '',
      password: this.form.get('password')?.value ?? '',
      role: (this.form.get('role')?.value as 'admin' | 'user') ?? 'user',
    };

    try {
      await this.userSvc.createUser(req);
      this.messageService.add({
        severity: 'success',
        summary: 'Success',
        detail: this.translate.instant('CREATE_USER_DIALOG.TOAST_SUCCESS', { username: req.username }),
      });
      this.close();
      this.created.emit();
    } catch (error: any) {
      this.messageService.add({
        severity: 'error',
        summary: 'Error',
        detail: error?.error?.error || this.translate.instant('CREATE_USER_DIALOG.TOAST_ERROR'),
      });
    }
  }
}
