import { CommonModule } from '@angular/common';
import { Component, inject } from '@angular/core';
import { Router } from '@angular/router';
import { ButtonModule } from 'primeng/button';
import { CardModule } from 'primeng/card';
import { TranslatePipe } from '@ngx-translate/core';
import { AuthService } from '../core/services/auth.service';
import { AppHeaderComponent } from '../shared/components/app-header/app-header.component';
import { AppHeaderAction } from '../shared/components/app-header/app-header.models';
import { ContextChatBubbleComponent } from '../shared/components/context-chat-bubble/context-chat-bubble.component';

@Component({
  selector: 'vs-dean-office',
  standalone: true,
  imports: [CommonModule, CardModule, ButtonModule, TranslatePipe, AppHeaderComponent, ContextChatBubbleComponent],
  templateUrl: './dean-office.component.html',
  styleUrl: './dean-office.component.scss',
})
export class DeanOfficeComponent {
  readonly auth = inject(AuthService);
  private readonly router = inject(Router);

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

  onHeaderAction(actionId: string): void {
    if (actionId === 'logout') {
      this.auth.logout();
      this.router.navigate(['/']);
      return;
    }
    if (actionId === 'back') {
      this.router.navigate(['/dashboard']);
      return;
    }
  }

  navigateToMail(): void {
    this.router.navigate(['/mail']);
  }
}
