import { CommonModule } from '@angular/common';
import { Component, ElementRef, inject, OnDestroy, OnInit, signal, ViewChild } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ActivatedRoute, Router } from '@angular/router';
import { TranslatePipe, TranslateService } from '@ngx-translate/core';
import { ConfirmationService, MessageService } from 'primeng/api';
import { ButtonModule } from 'primeng/button';
import { CardModule } from 'primeng/card';
import { ConfirmDialogModule } from 'primeng/confirmdialog';
import { DialogModule } from 'primeng/dialog';
import { SelectModule } from 'primeng/select';
import { ToastModule } from 'primeng/toast';
import { Subscription } from 'rxjs';
import { AuthService } from '../core/services/auth.service';
import {
  CommunicationConversation,
  CommunicationMessage,
  CommunicationService,
  CommunicationStreamEvent,
} from '../core/services/communication.service';
import { AppHeaderComponent } from '../shared/components/app-header/app-header.component';
import { AppHeaderAction } from '../shared/components/app-header/app-header.models';

interface RecipientOption {
  labelKey: string;
  kind: string;
  id: string;
}

type MailFolder = 'inbox' | 'trash';

@Component({
  selector: 'vs-mail',
  standalone: true,
  imports: [
    CommonModule,
    FormsModule,
    ButtonModule,
    CardModule,
    ToastModule,
    SelectModule,
    DialogModule,
    ConfirmDialogModule,
    AppHeaderComponent,
    TranslatePipe,
  ],
  templateUrl: './mail.component.html',
  styleUrl: './mail.component.scss',
  providers: [MessageService, ConfirmationService],
})
export class MailComponent implements OnInit, OnDestroy {
  readonly auth = inject(AuthService);
  private readonly communication = inject(CommunicationService);
  private readonly router = inject(Router);
  private readonly route = inject(ActivatedRoute);
  private readonly messageService = inject(MessageService);
  private readonly confirmService = inject(ConfirmationService);
  private readonly translate = inject(TranslateService);

  readonly recipients: RecipientOption[] = [{ labelKey: 'MAIL.RECIPIENT_DEAN_OFFICE', kind: 'dean_office', id: 'dean.taskford' }];
  readonly selectedRecipient = signal<RecipientOption>(this.recipients[0]);
  readonly folder = signal<MailFolder>('inbox');
  readonly showComposeDialog = signal(false);

  readonly conversations = signal<CommunicationConversation[]>([]);
  readonly selectedConversationId = signal<string | null>(null);
  readonly messages = signal<CommunicationMessage[]>([]);
  readonly loadingConversations = signal(true);
  readonly loadingMessages = signal(false);
  readonly creatingConversation = signal(false);
  readonly sendingMessage = signal(false);
  readonly streamingMessageId = signal<string | null>(null);

  readonly composeSubject = signal('');
  readonly composeMessage = signal('');
  readonly draftMessage = signal('');
  @ViewChild('messageListContainer') private messageListContainer?: ElementRef<HTMLDivElement>;

  private streamSub: Subscription | null = null;

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
    const preferredFolder = this.route.snapshot.queryParamMap.get('folder')?.trim().toLowerCase();
    if (preferredFolder === 'trash') {
      this.folder.set('trash');
    }
    const preferredConversationId = this.route.snapshot.queryParamMap.get('conversationId')?.trim() || undefined;
    void this.loadConversations(preferredConversationId);
  }

  ngOnDestroy(): void {
    this.stopStream();
  }

  onHeaderAction(actionId: string): void {
    if (actionId === 'logout') {
      this.auth.logout();
      this.router.navigate(['/']);
      return;
    }
    if (actionId === 'back') {
      this.router.navigate(['/dashboard']);
    }
  }

  async loadConversations(preferredConversationId?: string): Promise<void> {
    this.loadingConversations.set(true);
    try {
      const conversations = await this.communication.listConversations({
        channelType: 'mail_thread',
        status: this.folder() === 'trash' ? 'archived' : 'active',
      });
      this.conversations.set(conversations);
      const requestedSelection = preferredConversationId || this.selectedConversationId();
      const selected = requestedSelection && conversations.some(conversation => conversation.id === requestedSelection)
        ? requestedSelection
        : (conversations[0]?.id || null);
      this.selectedConversationId.set(selected);
      this.updateQueryParams({ conversationId: selected, folder: this.folder() });
      if (selected) {
        await this.loadMessages(selected);
      } else {
        this.messages.set([]);
      }
    } catch (error: any) {
      this.showError(error?.error?.error || this.translate.instant('MAIL.ERROR_LOAD_CONVERSATIONS'));
    } finally {
      this.loadingConversations.set(false);
    }
  }

  async createConversation(): Promise<void> {
    if (this.folder() === 'trash' || this.creatingConversation() || this.isStreaming()) return;
    const content = this.composeMessage().trim();
    if (!content) {
      this.showError(this.translate.instant('MAIL.COMPOSE_MESSAGE_REQUIRED'));
      return;
    }

    this.creatingConversation.set(true);
    const recipient = this.selectedRecipient();
    const subject = this.composeSubject().trim();
    const summary = this.buildSnippet(content, 240);
    try {
      const conversation = await this.communication.createConversation({
        channelType: 'mail_thread',
        recipientKind: recipient.kind,
        recipientId: recipient.id,
        title: subject,
        summary,
      });
      const response = await this.communication.postMessage(conversation.id, content);
      const nextConversation: CommunicationConversation = {
        ...conversation,
        title: conversation.title || subject,
        summary: conversation.summary || summary,
        last_message_at: response.user_message.created_at || conversation.last_message_at,
      };
      this.composeSubject.set('');
      this.composeMessage.set('');
      this.showComposeDialog.set(false);
      this.conversations.set([nextConversation, ...this.conversations().filter(item => item.id !== nextConversation.id)]);
      this.selectedConversationId.set(nextConversation.id);
      this.messages.set([response.user_message, response.assistant_message]);
      this.updateQueryParams({ conversationId: nextConversation.id, folder: this.folder() });
      this.scheduleScrollToBottom();
      this.streamAssistantMessage(nextConversation.id, response.assistant_message.id, response.stream.url);
    } catch (error: any) {
      this.showError(error?.error?.error || this.translate.instant('MAIL.ERROR_CREATE_SEND_CONVERSATION'));
    } finally {
      this.creatingConversation.set(false);
    }
  }

  async selectConversation(conversationId: string): Promise<void> {
    if (this.loadingMessages() || this.isStreaming() || conversationId === this.selectedConversationId()) return;
    this.selectedConversationId.set(conversationId);
    this.updateQueryParams({ conversationId, folder: this.folder() });
    await this.loadMessages(conversationId);
  }

  async selectFolder(folder: MailFolder): Promise<void> {
    if (folder === this.folder()) return;
    this.folder.set(folder);
    this.showComposeDialog.set(false);
    this.selectedConversationId.set(null);
    this.messages.set([]);
    this.updateQueryParams({ folder, conversationId: null });
    await this.loadConversations();
  }

  openComposeDialog(): void {
    if (this.folder() === 'trash' || this.isStreaming()) return;
    this.showComposeDialog.set(true);
  }

  closeComposeDialog(): void {
    if (this.creatingConversation()) return;
    this.showComposeDialog.set(false);
  }

  selectedConversation(): CommunicationConversation | null {
    const conversationId = this.selectedConversationId();
    if (!conversationId) return null;
    return this.conversations().find(conversation => conversation.id === conversationId) || null;
  }

  onDeleteConversationClick(): void {
    const conversation = this.selectedConversation();
    if (!conversation || this.isStreaming()) return;

    if (this.folder() === 'trash') {
      this.confirmService.confirm({
        header: this.translate.instant('MAIL.CONFIRM_DELETE_HEADER'),
        message: this.translate.instant('MAIL.CONFIRM_DELETE_MESSAGE'),
        icon: 'pi pi-exclamation-triangle',
        acceptButtonStyleClass: 'p-button-danger',
        accept: async () => {
          await this.deleteConversation(conversation.id);
        },
      });
      return;
    }

    this.confirmService.confirm({
      header: this.translate.instant('MAIL.CONFIRM_ARCHIVE_HEADER'),
      message: this.translate.instant('MAIL.CONFIRM_ARCHIVE_MESSAGE'),
      icon: 'pi pi-exclamation-triangle',
      accept: async () => {
        await this.archiveConversation(conversation.id);
      },
    });
  }

  onRestoreConversationClick(): void {
    const conversation = this.selectedConversation();
    if (!conversation || this.folder() !== 'trash' || this.isStreaming()) return;

    this.confirmService.confirm({
      header: this.translate.instant('MAIL.CONFIRM_RESTORE_HEADER'),
      message: this.translate.instant('MAIL.CONFIRM_RESTORE_MESSAGE'),
      icon: 'pi pi-exclamation-triangle',
      accept: async () => {
        await this.restoreConversation(conversation.id);
      },
    });
  }

  async sendMessage(): Promise<void> {
    const conversationId = this.selectedConversationId();
    const content = this.draftMessage().trim();
    if (this.folder() === 'trash' || !conversationId || !content || this.sendingMessage() || this.isStreaming()) return;

    this.sendingMessage.set(true);
    try {
      const response = await this.communication.postMessage(conversationId, content);
      this.draftMessage.set('');
      this.messages.update(current => [...current, response.user_message, response.assistant_message]);
      this.scheduleScrollToBottom();
      this.streamAssistantMessage(conversationId, response.assistant_message.id, response.stream.url);
    } catch (error: any) {
      this.showError(error?.error?.error || this.translate.instant('MAIL.ERROR_SEND_MESSAGE'));
    } finally {
      this.sendingMessage.set(false);
    }
  }

  isSelected(conversationId: string): boolean {
    return this.selectedConversationId() === conversationId;
  }

  isStreaming(messageId?: string): boolean {
    const current = this.streamingMessageId();
    if (!current) return false;
    if (!messageId) return true;
    return current === messageId;
  }

  formatDate(value: string | null | undefined): string {
    if (!value) return this.translate.instant('MAIL.DATE_UNKNOWN');
    return new Date(value).toLocaleString();
  }

  resolveRecipientLabel(conversation: CommunicationConversation): string {
    if (conversation.recipient_kind === 'dean_office') {
      return this.translate.instant('MAIL.RECIPIENT_DEAN_OFFICE');
    }
    return conversation.recipient_kind;
  }

  resolveConversationSubject(conversation: CommunicationConversation): string {
    return conversation.title?.trim() || this.translate.instant('MAIL.UNTITLED');
  }

  resolveConversationPreview(conversation: CommunicationConversation): string {
    const source = conversation.summary?.trim()
      || conversation.title?.trim()
      || this.translate.instant('MAIL.NO_PREVIEW');
    return this.buildSnippet(source, 88);
  }

  resolveAvatarInitials(conversation: CommunicationConversation): string {
    const recipient = this.resolveRecipientLabel(conversation).trim();
    if (!recipient) return this.translate.instant('MAIL.AVATAR_FALLBACK');
    const parts = recipient.split(/\s+/).filter(Boolean);
    if (parts.length >= 2) {
      return `${parts[0][0] || ''}${parts[parts.length - 1][0] || ''}`.toUpperCase();
    }
    if (parts[0].length >= 2) {
      return `${parts[0][0]}${parts[0][parts[0].length - 1]}`.toUpperCase();
    }
    return `${parts[0][0] || this.translate.instant('MAIL.AVATAR_FALLBACK')}`.toUpperCase();
  }

  trackConversation(index: number, conversation: CommunicationConversation): string {
    return conversation.id || `${index}`;
  }

  trackMessage(index: number, message: CommunicationMessage): string {
    return message.id || `${index}`;
  }

  private async loadMessages(conversationId: string): Promise<void> {
    this.loadingMessages.set(true);
    this.stopStream();
    try {
      const response = await this.communication.listMessages(conversationId);
      this.messages.set(response.messages);
      this.scheduleScrollToBottom();
    } catch (error: any) {
      this.messages.set([]);
      this.showError(error?.error?.error || this.translate.instant('MAIL.ERROR_LOAD_MESSAGES'));
    } finally {
      this.loadingMessages.set(false);
    }
  }

  private streamAssistantMessage(conversationId: string, assistantMessageId: string, streamPath: string): void {
    this.stopStream();
    this.streamingMessageId.set(assistantMessageId);
    this.streamSub = this.communication.streamAssistantResponse(streamPath).subscribe({
      next: event => this.applyStreamEvent(assistantMessageId, event),
      error: (error: any) => {
        this.patchAssistantMessage(assistantMessageId, {
          status: 'error',
          error_text: error?.message || this.translate.instant('MAIL.ERROR_STREAM'),
        });
        this.showError(error?.message || this.translate.instant('MAIL.ERROR_STREAM'));
        this.streamingMessageId.set(null);
      },
      complete: () => {
        this.streamingMessageId.set(null);
        this.streamSub = null;
        void this.loadConversations(conversationId);
      },
    });
  }

  private applyStreamEvent(messageId: string, event: CommunicationStreamEvent): void {
    if (event.type === 'message.start') {
      this.patchAssistantMessage(messageId, { status: 'streaming', error_text: '' });
      return;
    }
    if (event.type === 'message.delta') {
      const patch: Partial<CommunicationMessage> = { status: 'streaming' };
      if (typeof event.content === 'string') {
        patch.content = event.content;
      }
      this.patchAssistantMessage(messageId, patch);
      if (!event.content && event.delta) {
        this.messages.update(messages =>
          messages.map(message => (message.id !== messageId ? message : { ...message, content: `${message.content || ''}${event.delta}` }))
        );
      }
      this.scheduleScrollToBottom();
      return;
    }
    if (event.type === 'message.done') {
      const patch: Partial<CommunicationMessage> = {
        status: 'completed',
        finish_reason: event.finish_reason ?? 'completed',
        error_text: '',
      };
      if (typeof event.content === 'string') {
        patch.content = event.content;
      }
      this.patchAssistantMessage(messageId, patch);
      this.scheduleScrollToBottom();
      return;
    }
    if (event.type === 'message.error') {
      this.patchAssistantMessage(messageId, {
        status: 'error',
        error_text: event.error || this.translate.instant('MAIL.ERROR_STREAM'),
      });
      if (event.error) {
        this.showError(event.error);
      }
    }
  }

  private patchAssistantMessage(messageId: string, patch: Partial<CommunicationMessage>): void {
    this.messages.update(messages => messages.map(message => (message.id !== messageId ? message : { ...message, ...patch })));
  }

  private stopStream(): void {
    this.streamSub?.unsubscribe();
    this.streamSub = null;
    this.streamingMessageId.set(null);
  }

  private showError(detail: string): void {
    this.messageService.add({
      severity: 'error',
      summary: this.translate.instant('MAIL.TOAST_ERROR'),
      detail,
    });
  }

  private updateQueryParams(params: { folder?: MailFolder; conversationId?: string | null }): void {
    void this.router.navigate([], {
      relativeTo: this.route,
      queryParams: {
        folder: params.folder ?? this.folder(),
        conversationId: params.conversationId ?? this.selectedConversationId(),
      },
      queryParamsHandling: 'merge',
      replaceUrl: true,
    });
  }

  onComposerEnter(event: Event): void {
    const keyboardEvent = event as KeyboardEvent;
    if (keyboardEvent.shiftKey) return;
    keyboardEvent.preventDefault();
    void this.sendMessage();
  }

  private scheduleScrollToBottom(): void {
    setTimeout(() => {
      const el = this.messageListContainer?.nativeElement;
      if (!el) return;
      el.scrollTop = el.scrollHeight;
    }, 0);
  }

  private async archiveConversation(conversationId: string): Promise<void> {
    const nextConversationId = this.nextConversationId(conversationId);
    try {
      await this.communication.archiveConversation(conversationId);
      this.messageService.add({
        severity: 'success',
        summary: this.translate.instant('MAIL.TOAST_SUCCESS'),
        detail: this.translate.instant('MAIL.TOAST_ARCHIVED'),
      });
      await this.loadConversations(nextConversationId || undefined);
    } catch (error: any) {
      this.showError(error?.error?.error || this.translate.instant('MAIL.ERROR_ARCHIVE_CONVERSATION'));
    }
  }

  private async restoreConversation(conversationId: string): Promise<void> {
    const nextConversationId = this.nextConversationId(conversationId);
    try {
      await this.communication.restoreConversation(conversationId);
      this.messageService.add({
        severity: 'success',
        summary: this.translate.instant('MAIL.TOAST_SUCCESS'),
        detail: this.translate.instant('MAIL.TOAST_RESTORED'),
      });
      await this.loadConversations(nextConversationId || undefined);
    } catch (error: any) {
      this.showError(error?.error?.error || this.translate.instant('MAIL.ERROR_RESTORE_CONVERSATION'));
    }
  }

  private async deleteConversation(conversationId: string): Promise<void> {
    const nextConversationId = this.nextConversationId(conversationId);
    try {
      await this.communication.deleteConversation(conversationId);
      this.messageService.add({
        severity: 'success',
        summary: this.translate.instant('MAIL.TOAST_SUCCESS'),
        detail: this.translate.instant('MAIL.TOAST_DELETED'),
      });
      await this.loadConversations(nextConversationId || undefined);
    } catch (error: any) {
      this.showError(error?.error?.error || this.translate.instant('MAIL.ERROR_DELETE_CONVERSATION'));
    }
  }

  private nextConversationId(excludedConversationId: string): string | null {
    const remaining = this.conversations().filter(conversation => conversation.id !== excludedConversationId);
    return remaining[0]?.id || null;
  }

  private buildSnippet(value: string, maxLength: number): string {
    const compact = value.replace(/\s+/g, ' ').trim();
    if (compact.length <= maxLength) return compact;
    return `${compact.slice(0, Math.max(0, maxLength - 1)).trimEnd()}…`;
  }
}
