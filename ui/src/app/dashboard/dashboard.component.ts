import { Component, inject } from '@angular/core';
import { Router } from '@angular/router';
import { CardModule } from 'primeng/card';
import { ButtonModule } from 'primeng/button';
import { AuthService } from '../core/services/auth.service';
import { AppHeaderComponent } from '../shared/components/app-header/app-header.component';
import { AppHeaderAction } from '../shared/components/app-header/app-header.models';
import { TranslatePipe } from '@ngx-translate/core';

@Component({
  selector: 'vs-dashboard',
  standalone: true,
  imports: [
    CardModule,
    ButtonModule,
    AppHeaderComponent,
    TranslatePipe,
  ],
  templateUrl: './dashboard.component.html',
  styleUrl: './dashboard.component.scss',
})
export class DashboardComponent {
  readonly auth = inject(AuthService);
  private readonly router = inject(Router);

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

  onHeaderAction(actionId: string): void {
    if (actionId === 'logout') this.logout();
  }

  logout(): void {
    this.auth.logout();
    this.router.navigate(['/']);
  }

  navigateTo(path: string): void {
    this.router.navigate([path]);
  }

  get isAdmin(): boolean {
    return this.auth.isAdmin();
  }
}
