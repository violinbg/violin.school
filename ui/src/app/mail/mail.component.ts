import { CommonModule } from '@angular/common';
import { Component, ElementRef, inject, OnDestroy, OnInit, signal, ViewChild } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ActivatedRoute, Router } from '@angular/router';
import { TranslatePipe, TranslateService } from '@ngx-translate/core';
import { MessageService } from 'primeng/api';
import { ButtonModule } from 'primeng/button';
import { CardModule } from 'primeng/card';
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

@Component({
  selector: 'vs-mail',
  standalone: true,
  imports: [CommonModule, FormsModule, ButtonModule, CardModule, ToastModule, SelectModule, AppHeaderComponent, TranslatePipe],
  templateUrl: './mail.component.html',
  styleUrl: './mail.component.scss',
  providers: [MessageService],
})
export class MailComponent implements OnInit, OnDestroy {
  readonly auth = inject(AuthService);
  private readonly communication = inject(CommunicationService);
  private readonly router = inject(Router);
  private readonly route = inject(ActivatedRoute);
  private readonly messageService = inject(MessageService);
  private readonly translate = inject(TranslateService);

  readonly recipients: RecipientOption[] = [{ labelKey: 'MAIL.RECIPIENT_DEAN_OFFICE', kind: 'dean_office', id: 'dean.taskford' }];
  readonly selectedRecipient = signal<RecipientOption>(this.recipients[0]);

  readonly conversations = signal<CommunicationConversation[]>([]);
  readonly selectedConversationId = signal<string | null>(null);
  readonly messages = signal<CommunicationMessage[]>([]);
  readonly loadingConversations = signal(true);
  readonly loadingMessages = signal(false);
  readonly creatingConversation = signal(false);
  readonly sendingMessage = signal(false);
  readonly streamingMessageId = signal<string | null>(null);

  readonly conversationTitle = signal('');
  readonly conversationSummary = signal('');
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
      const conversations = await this.communication.listConversations({ channelType: 'mail_thread' });
      this.conversations.set(conversations);
      const selected = preferredConversationId || this.selectedConversationId() || conversations[0]?.id || null;
      this.selectedConversationId.set(selected);
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
    if (this.creatingConversation() || this.isStreaming()) return;
    this.creatingConversation.set(true);
    const recipient = this.selectedRecipient();
    try {
      const conversation = await this.communication.createConversation({
        channelType: 'mail_thread',
        recipientKind: recipient.kind,
        recipientId: recipient.id,
        title: this.conversationTitle().trim(),
        summary: this.conversationSummary().trim(),
      });
      this.conversationTitle.set('');
      this.conversationSummary.set('');
      this.conversations.set([conversation, ...this.conversations()]);
      this.selectedConversationId.set(conversation.id);
      await this.loadMessages(conversation.id);
    } catch (error: any) {
      this.showError(error?.error?.error || this.translate.instant('MAIL.ERROR_CREATE_CONVERSATION'));
    } finally {
      this.creatingConversation.set(false);
    }
  }

  async selectConversation(conversationId: string): Promise<void> {
    if (this.loadingMessages() || this.isStreaming() || conversationId === this.selectedConversationId()) return;
    this.selectedConversationId.set(conversationId);
    await this.loadMessages(conversationId);
  }

  async sendMessage(): Promise<void> {
    const conversationId = this.selectedConversationId();
    const content = this.draftMessage().trim();
    if (!conversationId || !content || this.sendingMessage() || this.isStreaming()) return;

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
}
