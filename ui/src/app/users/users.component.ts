import { Component, computed, inject, OnInit, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { Router } from '@angular/router';
import { ButtonModule } from 'primeng/button';
import { CardModule } from 'primeng/card';
import { TableModule } from 'primeng/table';
import { ToastModule } from 'primeng/toast';
import { ConfirmDialogModule } from 'primeng/confirmdialog';
import { TagModule } from 'primeng/tag';
import { TooltipModule } from 'primeng/tooltip';
import { ToggleSwitchModule } from 'primeng/toggleswitch';
import { InputNumberModule } from 'primeng/inputnumber';
import { ProgressBarModule } from 'primeng/progressbar';
import { MessageService, ConfirmationService } from 'primeng/api';
import { AuthService } from '../core/services/auth.service';
import { UserService, User } from '../core/services/user.service';
import { AdminService, AdminSettings } from '../core/services/admin.service';
import { AppHeaderComponent } from '../shared/components/app-header/app-header.component';
import { AppHeaderAction } from '../shared/components/app-header/app-header.models';
import { CreateUserDialogComponent } from './create-user-dialog/create-user-dialog.component';
import { EditUserDialogComponent } from './edit-user-dialog/edit-user-dialog.component';
import { TranslatePipe, TranslateService } from '@ngx-translate/core';

@Component({
  selector: 'vs-users',
  standalone: true,
  imports: [
    CommonModule,
    FormsModule,
    ButtonModule,
    CardModule,
    TableModule,
    ToastModule,
    ConfirmDialogModule,
    TagModule,
    TooltipModule,
    ToggleSwitchModule,
    InputNumberModule,
    ProgressBarModule,
    AppHeaderComponent,
    CreateUserDialogComponent,
    EditUserDialogComponent,
    TranslatePipe,
  ],
  templateUrl: './users.component.html',
  styleUrl: './users.component.scss',
  providers: [MessageService, ConfirmationService],
})
export class UsersComponent implements OnInit {
  readonly auth = inject(AuthService);
  private readonly userSvc = inject(UserService);
  private readonly adminSvc = inject(AdminService);
  private readonly router = inject(Router);
  private readonly messageService = inject(MessageService);
  private readonly confirmService = inject(ConfirmationService);
  private readonly translate = inject(TranslateService);

  users = signal<User[]>([]);
  loading = signal(true);
  showCreateDialog = signal(false);
  showEditDialog = signal(false);
  editingUser = signal<User | null>(null);
  adminSettings = signal<AdminSettings | null>(null);
  savingSettings = signal(false);
  editRegistration = signal(false);
  editMaxUsers = signal(1);
  private readonly settingsBaseline = signal<{ registration_enabled: boolean; max_users: number } | null>(null);
  readonly settingsDirty = computed(() => {
    const baseline = this.settingsBaseline();
    if (!baseline) return false;
    return this.editRegistration() !== baseline.registration_enabled ||
      this.editMaxUsers() !== baseline.max_users;
  });

  readonly headerLeftAction: AppHeaderAction = {
    id: 'back',
    labelKey: 'HEADER.BACK',
    icon: 'pi pi-arrow-left',
    severity: 'secondary',
    outlined: true,
    size: 'small',
  };

  readonly headerActions: AppHeaderAction[] = [
    {
      id: 'logout',
      labelKey: 'HEADER.SIGN_OUT',
      icon: 'pi pi-sign-out',
      severity: 'secondary',
      outlined: true,
      size: 'small',
    },
  ];

  ngOnInit(): void {
    this.loadUsers();
    this.loadAdminSettings();
  }

  onHeaderAction(actionId: string): void {
    if (actionId === 'logout') this.logout();
    if (actionId === 'back') this.navigateBack();
  }

  logout(): void {
    this.auth.logout();
    this.router.navigate(['/']);
  }

  navigateBack(): void {
    this.router.navigate(['/dashboard']);
  }

  async loadAdminSettings(): Promise<void> {
    try {
      const settings = await this.adminSvc.getSettings();
      this.adminSettings.set(settings);
      this.initEditState(settings);
    } catch {
      // non-critical — ignore errors loading settings
    }
  }

  private initEditState(settings: AdminSettings): void {
    this.editRegistration.set(settings.registration_enabled);
    this.editMaxUsers.set(settings.max_users);
    this.settingsBaseline.set({
      registration_enabled: settings.registration_enabled,
      max_users: settings.max_users,
    });
  }

  async saveSettings(): Promise<void> {
    if (!this.settingsDirty() || this.savingSettings()) return;
    const maxUsers = this.editMaxUsers();
    if (!maxUsers || maxUsers < 1) return;
    this.savingSettings.set(true);
    try {
      const patch = {
        registration_enabled: this.editRegistration(),
        max_users: maxUsers,
      };
      await this.adminSvc.updateSettings(patch);
      const current = this.adminSettings();
      if (current) this.adminSettings.set({ ...current, ...patch });
      this.settingsBaseline.set({ ...patch });
      this.auth.setRegistrationEnabled(patch.registration_enabled);
      this.messageService.add({
        severity: 'success',
        summary: this.translate.instant('USERS.TOAST_SAVED'),
        detail: this.translate.instant('USERS.TOAST_SETTINGS_SAVED'),
      });
    } catch {
      this.messageService.add({
        severity: 'error',
        summary: this.translate.instant('USERS.TOAST_ERROR'),
        detail: this.translate.instant('USERS.TOAST_SETTINGS_ERROR'),
      });
    } finally {
      this.savingSettings.set(false);
    }
  }

  async loadUsers(): Promise<void> {
    this.loading.set(true);
    try {
      const users = await this.userSvc.listUsers();
      this.users.set(users);
    } catch (error: any) {
      const detail = error?.error?.error
        || error?.message
        || 'Failed to load users';
      this.messageService.add({
        severity: 'error',
        summary: this.translate.instant('USERS.TOAST_ERROR'),
        detail,
      });
    } finally {
      this.loading.set(false);
    }
  }

  openCreateDialog(): void {
    this.showCreateDialog.set(true);
  }

  openEditDialog(user: User): void {
    this.editingUser.set(user);
    this.showEditDialog.set(true);
  }

  confirmToggleStatus(user: User): void {
    const isDeactivating = user.active;
    this.confirmService.confirm({
      message: this.translate.instant(
        isDeactivating ? 'USERS.CONFIRM_DEACTIVATE_MSG' : 'USERS.CONFIRM_REACTIVATE_MSG',
        { username: user.username }
      ),
      header: this.translate.instant(
        isDeactivating ? 'USERS.CONFIRM_DEACTIVATE_HEADER' : 'USERS.CONFIRM_REACTIVATE_HEADER'
      ),
      icon: 'pi pi-exclamation-triangle',
      accept: async () => {
        await this.toggleUserStatus(user);
      },
    });
  }

  async toggleUserStatus(user: User): Promise<void> {
    try {
      await this.userSvc.toggleUserStatus(user.id, !user.active);
      this.messageService.add({
        severity: 'success',
        summary: this.translate.instant('USERS.TOAST_SUCCESS'),
        detail: this.translate.instant(user.active ? 'USERS.TOAST_DEACTIVATED' : 'USERS.TOAST_REACTIVATED', { username: user.username }),
      });
      await this.loadUsers();
    } catch (error: any) {
      this.messageService.add({
        severity: 'error',
        summary: this.translate.instant('USERS.TOAST_ERROR'),
        detail: error?.error?.error || this.translate.instant('USERS.TOAST_TOGGLE_ERROR'),
      });
    }
  }

  confirmDelete(user: User): void {
    if (user.id === this.auth.currentUser()?.id) {
      this.messageService.add({
        severity: 'error',
        summary: this.translate.instant('USERS.TOAST_CANNOT_DELETE_HEADER'),
        detail: this.translate.instant('USERS.TOAST_CANNOT_DELETE_DETAIL'),
      });
      return;
    }

    this.confirmService.confirm({
      message: this.translate.instant('USERS.CONFIRM_DELETE_MSG', { username: user.username }),
      header: this.translate.instant('USERS.CONFIRM_DELETE_HEADER'),
      icon: 'pi pi-exclamation-triangle',
      acceptButtonStyleClass: 'p-button-danger',
      accept: async () => {
        await this.deleteUser(user);
      },
    });
  }

  async deleteUser(user: User): Promise<void> {
    try {
      await this.userSvc.deleteUser(user.id);
      this.messageService.add({
        severity: 'success',
        summary: this.translate.instant('USERS.TOAST_SUCCESS'),
        detail: this.translate.instant('USERS.TOAST_DELETED', { username: user.username }),
      });
      await this.loadUsers();
    } catch (error: any) {
      this.messageService.add({
        severity: 'error',
        summary: this.translate.instant('USERS.TOAST_ERROR'),
        detail: error?.error?.error || this.translate.instant('USERS.TOAST_DELETE_ERROR'),
      });
    }
  }

  getRoleTag(role: string): { label: string; severity: 'danger' | 'info' } {
    return role === 'admin'
      ? { label: this.translate.instant('USERS.ROLE_ADMIN'), severity: 'danger' }
      : { label: this.translate.instant('USERS.ROLE_USER'), severity: 'info' };
  }

  getStatusTag(active: boolean): { label: string; severity: 'success' | 'warn' } {
    return active
      ? { label: this.translate.instant('USERS.STATUS_ACTIVE'), severity: 'success' }
      : { label: this.translate.instant('USERS.STATUS_INACTIVE'), severity: 'warn' };
  }

  formatDate(date: string | null): string {
    if (!date) return '—';
    return new Date(date).toLocaleDateString('en-US', { year: 'numeric', month: 'short', day: 'numeric' });
  }
}
