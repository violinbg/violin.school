import { Component, inject, signal } from '@angular/core';
import { Router } from '@angular/router';
import { CardModule } from 'primeng/card';
import { ButtonModule } from 'primeng/button';
import { TagModule } from 'primeng/tag';
import { ToastModule } from 'primeng/toast';
import { MessageService } from 'primeng/api';
import { TranslatePipe, TranslateService } from '@ngx-translate/core';
import { AuthService } from '../core/services/auth.service';
import { LoginDialogComponent } from '../auth/login-dialog/login-dialog.component';
import { RegisterDialogComponent } from '../auth/register-dialog/register-dialog.component';
import { AboutDialogComponent } from './about-dialog/about-dialog.component';
import { AppHeaderComponent } from '../shared/components/app-header/app-header.component';
import { AppHeaderAction } from '../shared/components/app-header/app-header.models';

@Component({
  selector: 'vs-home',
  standalone: true,
  providers: [MessageService],
  imports: [CardModule, ButtonModule, TagModule, ToastModule, TranslatePipe, LoginDialogComponent, RegisterDialogComponent, AboutDialogComponent, AppHeaderComponent],
  templateUrl: './home.component.html',
  styleUrl: './home.component.scss'
})
export class HomeComponent {
  readonly auth = inject(AuthService);
  private readonly router = inject(Router);
  private readonly translate = inject(TranslateService);

  showLoginDialog = signal(false);
  showAboutDialog = signal(false);
  showRegisterDialog = signal(false);

  readonly features = [
    {
      icon: 'pi pi-book',
      titleKey: 'HOME.FEATURES.COURSES_TITLE',
      descriptionKey: 'HOME.FEATURES.COURSES_DESC',
      route: '/dashboard' as string | null,
      comingSoon: false,
    },
    {
      icon: 'pi pi-video',
      titleKey: 'HOME.FEATURES.LESSONS_TITLE',
      descriptionKey: 'HOME.FEATURES.LESSONS_DESC',
      route: null as string | null,
      comingSoon: true,
    },
    {
      icon: 'pi pi-chart-bar',
      titleKey: 'HOME.FEATURES.PROGRESS_TITLE',
      descriptionKey: 'HOME.FEATURES.PROGRESS_DESC',
      route: null as string | null,
      comingSoon: true,
    },
    {
      icon: 'pi pi-users',
      titleKey: 'HOME.FEATURES.COMMUNITY_TITLE',
      descriptionKey: 'HOME.FEATURES.COMMUNITY_DESC',
      route: null as string | null,
      comingSoon: true,
    }
  ];

  get headerUsername(): string | null {
    return this.auth.isLoggedIn() ? (this.auth.currentUser()?.full_name ?? null) : null;
  }

  get headerActions(): AppHeaderAction[] {
    if (this.auth.isLoggedIn()) {
      return [
        {
          id: 'dashboard',
          labelKey: 'HEADER.DASHBOARD',
          icon: 'pi pi-th-large',
          size: 'small',
        },
        {
          id: 'logout',
          labelKey: 'HEADER.SIGN_OUT',
          icon: 'pi pi-sign-out',
          severity: 'secondary',
          outlined: true,
          size: 'small',
        },
      ];
    }

    return [
      {
        id: 'signin',
        labelKey: 'HEADER.SIGN_IN',
        icon: 'pi pi-sign-in',
        severity: 'secondary',
        outlined: true,
        size: 'small',
      },
    ];
  }

  onHeaderAction(actionId: string): void {
    if (actionId === 'dashboard') {
      this.goToDashboard();
      return;
    }

    if (actionId === 'logout') {
      this.logout();
      return;
    }

    if (actionId === 'signin') {
      this.showLoginDialog.set(true);
    }
  }

  goToDashboard(): void {
    this.router.navigate(['/dashboard']);
  }

  logout(): void {
    this.auth.logout();
  }

  showLearnMore(): void {
    this.showAboutDialog.set(true);
  }

  onFeatureClick(route: string | null): void {
    if (!route) return;

    if (!this.auth.isLoggedIn()) {
      this.showLoginDialog.set(true);
      return;
    }

    this.router.navigate([route]);
  }

  onFeatureKeydown(event: KeyboardEvent, route: string | null): void {
    if (event.key !== 'Enter' && event.key !== ' ') return;
    event.preventDefault();
    this.onFeatureClick(route);
  }
}
