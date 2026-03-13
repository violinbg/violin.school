import { inject, Injectable } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { firstValueFrom } from 'rxjs';

export interface AdminSettings {
  registration_enabled: boolean;
  max_users: number;
  user_count: number;
}

export interface AiUserLimitOverride {
  user_id: string;
  username: string;
  full_name: string;
  monthly_limit_usd_ticks: number;
}

export interface AiSettings {
  base_url: string;
  has_token: boolean;
  monthly_global_limit_usd_ticks: number;
  monthly_default_user_limit_usd_ticks: number;
  user_limit_overrides: AiUserLimitOverride[];
}

export interface AiUsageUserSummary {
  user_id: string;
  username: string;
  full_name: string;
  cost_usd_ticks: number;
  total_tokens: number;
  effective_limit_usd_ticks: number;
}

export interface AiUsageSummary {
  month_start_utc: string;
  month_end_utc_exclusive: string;
  global_cost_usd_ticks: number;
  global_total_tokens: number;
  users: AiUsageUserSummary[];
}

export interface AiSettingsResponse {
  settings: AiSettings;
  usage: AiUsageSummary;
}

export interface AiUserLimitOverridePatch {
  user_id: string;
  monthly_limit_usd_ticks: number | null;
}

export interface UpdateAiSettingsPatch {
  base_url?: string;
  api_token?: string;
  monthly_global_limit_usd_ticks?: number;
  monthly_default_user_limit_usd_ticks?: number;
  user_limit_overrides?: AiUserLimitOverridePatch[];
}

@Injectable({ providedIn: 'root' })
export class AdminService {
  private readonly http = inject(HttpClient);

  async getSettings(): Promise<AdminSettings> {
    return firstValueFrom(this.http.get<AdminSettings>('/api/v1/admin/settings'));
  }

  async updateSettings(patch: Partial<Pick<AdminSettings, 'registration_enabled' | 'max_users'>>): Promise<void> {
    await firstValueFrom(this.http.patch('/api/v1/admin/settings', patch));
  }

  async getAiSettings(): Promise<AiSettingsResponse> {
    return firstValueFrom(this.http.get<AiSettingsResponse>('/api/v1/admin/ai/settings'));
  }

  async updateAiSettings(patch: UpdateAiSettingsPatch): Promise<void> {
    await firstValueFrom(this.http.patch('/api/v1/admin/ai/settings', patch));
  }

  async testAiToken(): Promise<void> {
    await firstValueFrom(this.http.post('/api/v1/admin/ai/test', {}));
  }

  async getAiUsage(): Promise<AiUsageSummary> {
    return firstValueFrom(this.http.get<AiUsageSummary>('/api/v1/admin/ai/usage'));
  }
}
