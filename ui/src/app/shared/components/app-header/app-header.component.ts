import { Component, EventEmitter, inject, Input, Output } from '@angular/core';
import { ButtonModule } from 'primeng/button';
import { TooltipModule } from 'primeng/tooltip';
import { SelectModule } from 'primeng/select';
import { FormsModule } from '@angular/forms';
import { TranslateService } from '@ngx-translate/core';
import { AppHeaderAction } from './app-header.models';
import { Language, LanguageService } from '../../../core/services/language.service';

@Component({
  selector: 'vs-app-header',
  standalone: true,
  imports: [ButtonModule, TooltipModule, SelectModule, FormsModule],
  templateUrl: './app-header.component.html',
  styleUrl: './app-header.component.scss',
})
export class AppHeaderComponent {
  private readonly translate = inject(TranslateService);
  readonly languageService = inject(LanguageService);

  @Input() title = '';
  @Input() titleKey = 'APP.TITLE';
  @Input() logoIcon = 'pi pi-graduation-cap';
  @Input() username: string | null = null;
  @Input() showUsername = true;
  @Input() leftAction: AppHeaderAction | null = null;
  @Input() actions: AppHeaderAction[] = [];
  @Input() sticky = true;
  @Input() showLanguageSelector = true;

  @Output() actionClick = new EventEmitter<string>();

  get visibleActions(): AppHeaderAction[] {
    return this.actions.filter(action => !action.hidden);
  }

  get resolvedTitle(): string {
    if (this.title) return this.title;
    return this.translate.instant(this.titleKey);
  }

  resolveLabel(action: AppHeaderAction): string {
    if (action.labelKey) return this.translate.instant(action.labelKey);
    return action.label ?? '';
  }

  resolveTooltip(action: AppHeaderAction): string | undefined {
    if (action.tooltipKey) return this.translate.instant(action.tooltipKey);
    return action.tooltip;
  }

  onLanguageChange(code: string): void {
    this.languageService.setLanguage(code);
  }

  getLanguage(code: string): Language | undefined {
    return this.languageService.languages.find(l => l.code === code);
  }

  onAction(action: AppHeaderAction): void {
    if (action.disabled || action.loading) {
      return;
    }
    this.actionClick.emit(action.id);
  }
}