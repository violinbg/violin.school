import { CommonModule } from '@angular/common';
import { Component, computed, inject, OnInit, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { Router } from '@angular/router';
import { TranslatePipe, TranslateService } from '@ngx-translate/core';
import { ButtonModule } from 'primeng/button';
import { CardModule } from 'primeng/card';
import { InputNumberModule } from 'primeng/inputnumber';
import { InputTextModule } from 'primeng/inputtext';
import { TableModule } from 'primeng/table';
import { ToastModule } from 'primeng/toast';
import { MessageService } from 'primeng/api';
import {
  AdminService,
  AiSettingsResponse,
  AiUsageUserSummary,
  AiUserLimitOverridePatch,
  UpdateAiSettingsPatch,
} from '../core/services/admin.service';
import { AuthService } from '../core/services/auth.service';
import { AppHeaderComponent } from '../shared/components/app-header/app-header.component';
import { AppHeaderAction } from '../shared/components/app-header/app-header.models';

type OverrideMap = Record<string, number | null>;
const USD_TICKS_PER_DOLLAR = 10_000_000_000;

@Component({
  selector: 'vs-ai-settings',
  standalone: true,
  imports: [
    CommonModule,
    FormsModule,
    ButtonModule,
    CardModule,
    InputNumberModule,
    InputTextModule,
    TableModule,
    ToastModule,
    AppHeaderComponent,
    TranslatePipe,
  ],
  templateUrl: './ai-settings.component.html',
  styleUrl: './ai-settings.component.scss',
  providers: [MessageService],
})
export class AiSettingsComponent implements OnInit {
  readonly auth = inject(AuthService);
  private readonly adminSvc = inject(AdminService);
  private readonly router = inject(Router);
  private readonly messageService = inject(MessageService);
  private readonly translate = inject(TranslateService);

  readonly loading = signal(true);
  readonly saving = signal(false);
  readonly testing = signal(false);
  readonly data = signal<AiSettingsResponse | null>(null);

  readonly editBaseUrl = signal('');
  readonly editToken = signal('');
  readonly editGlobalLimit = signal(0);
  readonly editDefaultUserLimit = signal(0);
  readonly overrideInputs = signal<OverrideMap>({});

  private readonly baseline = signal<string>('');

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

  readonly isDirty = computed(() => this.currentStateSignature() !== this.baseline());

  ngOnInit(): void {
    this.load();
  }

  onHeaderAction(actionId: string): void {
    if (actionId === 'logout') this.logout();
    if (actionId === 'back') this.router.navigate(['/dashboard']);
  }

  logout(): void {
    this.auth.logout();
    this.router.navigate(['/']);
  }

  async load(): Promise<void> {
    this.loading.set(true);
    try {
      const res = await this.adminSvc.getAiSettings();
      this.data.set(res);
      this.editBaseUrl.set(res.settings.base_url);
      this.editToken.set('');
      this.editGlobalLimit.set(this.ticksToUsd(res.settings.monthly_global_limit_usd_ticks));
      this.editDefaultUserLimit.set(this.ticksToUsd(res.settings.monthly_default_user_limit_usd_ticks));
      const nextOverrides: OverrideMap = {};
      for (const item of res.settings.user_limit_overrides) {
        nextOverrides[item.user_id] = this.ticksToUsd(item.monthly_limit_usd_ticks);
      }
      this.overrideInputs.set(nextOverrides);
      this.baseline.set(this.currentStateSignature());
    } catch (error: any) {
      this.messageService.add({
        severity: 'error',
        summary: this.translate.instant('AI_SETTINGS.TOAST_ERROR'),
        detail: error?.error?.error || this.translate.instant('AI_SETTINGS.TOAST_LOAD_ERROR'),
      });
    } finally {
      this.loading.set(false);
    }
  }

  async save(): Promise<void> {
    if (!this.isDirty() || this.saving()) return;

    const current = this.data();
    if (!current) return;

    const patch: UpdateAiSettingsPatch = {};
    if (this.editBaseUrl() !== current.settings.base_url) {
      patch.base_url = this.editBaseUrl().trim();
    }
    if (this.editToken().trim() !== '') {
      patch.api_token = this.editToken().trim();
    }
    const currentGlobalLimitUsd = this.ticksToUsd(current.settings.monthly_global_limit_usd_ticks);
    if (this.editGlobalLimit() !== currentGlobalLimitUsd) {
      patch.monthly_global_limit_usd_ticks = this.usdToTicks(this.editGlobalLimit());
    }
    const currentDefaultUserLimitUsd = this.ticksToUsd(current.settings.monthly_default_user_limit_usd_ticks);
    if (this.editDefaultUserLimit() !== currentDefaultUserLimitUsd) {
      patch.monthly_default_user_limit_usd_ticks = this.usdToTicks(this.editDefaultUserLimit());
    }

    patch.user_limit_overrides = this.buildOverridePatch(current.usage.users, current.settings.user_limit_overrides);

    this.saving.set(true);
    try {
      await this.adminSvc.updateAiSettings(patch);
      this.messageService.add({
        severity: 'success',
        summary: this.translate.instant('AI_SETTINGS.TOAST_SAVED'),
        detail: this.translate.instant('AI_SETTINGS.TOAST_SETTINGS_SAVED'),
      });
      await this.load();
    } catch (error: any) {
      this.messageService.add({
        severity: 'error',
        summary: this.translate.instant('AI_SETTINGS.TOAST_ERROR'),
        detail: error?.error?.error || this.translate.instant('AI_SETTINGS.TOAST_SETTINGS_ERROR'),
      });
    } finally {
      this.saving.set(false);
    }
  }

  async testToken(): Promise<void> {
    this.testing.set(true);
    try {
      await this.adminSvc.testAiToken();
      this.messageService.add({
        severity: 'success',
        summary: this.translate.instant('AI_SETTINGS.TOAST_SUCCESS'),
        detail: this.translate.instant('AI_SETTINGS.TOAST_TEST_SUCCESS'),
      });
    } catch (error: any) {
      this.messageService.add({
        severity: 'error',
        summary: this.translate.instant('AI_SETTINGS.TOAST_ERROR'),
        detail: error?.error?.error || this.translate.instant('AI_SETTINGS.TOAST_TEST_ERROR'),
      });
    } finally {
      this.testing.set(false);
    }
  }

  setOverride(userId: string, value: number | null): void {
    const next = { ...this.overrideInputs() };
    next[userId] = value;
    this.overrideInputs.set(next);
  }

  clearOverride(userId: string): void {
    const next = { ...this.overrideInputs() };
    next[userId] = null;
    this.overrideInputs.set(next);
  }

  resolveOverride(userId: string): number | null {
    const value = this.overrideInputs()[userId];
    return typeof value === 'number' ? value : null;
  }

  formatUsd(value: number): string {
    return new Intl.NumberFormat('en-US', {
      style: 'currency',
      currency: 'USD',
      minimumFractionDigits: 2,
      maximumFractionDigits: 2,
    }).format(value);
  }

  formatUsdCost(value: number): string {
    return new Intl.NumberFormat('en-US', {
      style: 'currency',
      currency: 'USD',
      minimumFractionDigits: 4,
      maximumFractionDigits: 4,
    }).format(value);
  }

  formatUsdFromTicks(ticks: number): string {
    return this.formatUsd(this.ticksToUsd(ticks));
  }

  formatUsdCostFromTicks(ticks: number): string {
    return this.formatUsdCost(this.ticksToUsd(ticks));
  }

  formatTicks(value: number): string {
    return new Intl.NumberFormat('en-US').format(value);
  }

  private buildOverridePatch(users: AiUsageUserSummary[], existing: { user_id: string; monthly_limit_usd_ticks: number }[]): AiUserLimitOverridePatch[] {
    const existingMap = new Map(existing.map(item => [item.user_id, item.monthly_limit_usd_ticks]));
    const patch: AiUserLimitOverridePatch[] = [];
    for (const user of users) {
      const nextValue = this.resolveOverride(user.user_id);
      const prevValue = existingMap.get(user.user_id);
      if (nextValue === null && prevValue !== undefined) {
        patch.push({ user_id: user.user_id, monthly_limit_usd_ticks: null });
      } else if (typeof nextValue === 'number') {
        const nextTicks = this.usdToTicks(nextValue);
        if (nextTicks !== prevValue) {
          patch.push({ user_id: user.user_id, monthly_limit_usd_ticks: nextTicks });
        }
      }
    }
    return patch;
  }

  private usdToTicks(usd: number): number {
    if (!Number.isFinite(usd) || usd < 0) return 0;
    return Math.round(usd * USD_TICKS_PER_DOLLAR);
  }

  private ticksToUsd(ticks: number): number {
    if (!Number.isFinite(ticks) || ticks < 0) return 0;
    return ticks / USD_TICKS_PER_DOLLAR;
  }

  private currentStateSignature(): string {
    return JSON.stringify({
      base_url: this.editBaseUrl().trim(),
      has_token_update: this.editToken().trim() !== '',
      monthly_global_limit_usd_ticks: this.editGlobalLimit(),
      monthly_default_user_limit_usd_ticks: this.editDefaultUserLimit(),
      override_inputs: this.overrideInputs(),
    });
  }
}
