import { CommonModule } from '@angular/common';
import { Component, ElementRef, inject, Input, OnDestroy, OnInit, signal, ViewChild } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { NavigationStart, Router } from '@angular/router';
import { TranslatePipe, TranslateService } from '@ngx-translate/core';
import { MessageService } from 'primeng/api';
import { ButtonModule } from 'primeng/button';
import { CardModule } from 'primeng/card';
import { ToastModule } from 'primeng/toast';
import { Subscription } from 'rxjs';
import { filter } from 'rxjs/operators';
import {
  CommunicationConversation,
  CommunicationMessage,
  CommunicationService,
  CommunicationStreamEvent,
} from '../../../core/services/communication.service';

@Component({
  selector: 'vs-context-chat-bubble',
  standalone: true,
  imports: [CommonModule, FormsModule, ButtonModule, CardModule, ToastModule, TranslatePipe],
  templateUrl: './context-chat-bubble.component.html',
  styleUrl: './context-chat-bubble.component.scss',
  providers: [MessageService],
})
export class ContextChatBubbleComponent implements OnInit, OnDestroy {
  @Input() contextKey = 'dean-office-help';
  @Input() recipientKind = 'dean_office';
  @Input() recipientId = 'dean.taskford';
  @Input() titleKey = 'CONTEXT_CHAT.TITLE';

  private readonly communication = inject(CommunicationService);
  private readonly messageService = inject(MessageService);
  private readonly translate = inject(TranslateService);
  private readonly router = inject(Router);

  readonly open = signal(false);
  readonly loading = signal(false);
  readonly conversationId = signal<string | null>(null);
  readonly messages = signal<CommunicationMessage[]>([]);
  readonly sending = signal(false);
  readonly streamMessageId = signal<string | null>(null);
  readonly draftMessage = signal('');
  readonly convertingToMail = signal(false);
  readonly showInactivityPrompt = signal(false);
  @ViewChild('messageListContainer') private messageListContainer?: ElementRef<HTMLDivElement>;

  private streamSub: Subscription | null = null;
  private routeSub: Subscription | null = null;
  private inactivityTimer: ReturnType<typeof setTimeout> | null = null;
  private readonly inactivityTimeoutMs = 10 * 60 * 1000;

  ngOnInit(): void {
    this.routeSub = this.router.events
      .pipe(filter(event => event instanceof NavigationStart))
      .subscribe(() => this.closePanel());
    void this.ensureConversation();
  }

  ngOnDestroy(): void {
    this.stopStream();
    this.clearInactivityTimer();
    this.routeSub?.unsubscribe();
    this.routeSub = null;
  }

  toggleOpen(): void {
    const next = !this.open();
    this.open.set(next);
    if (next) {
      this.armInactivityTimer();
      this.scheduleScrollToBottom();
      return;
    }
    this.clearInactivityTimer();
  }

  closePanel(): void {
    this.open.set(false);
    this.clearInactivityTimer();
  }

  async sendMessage(): Promise<void> {
    const conversationId = this.conversationId();
    const content = this.draftMessage().trim();
    if (!conversationId || !content || this.sending() || this.streamMessageId()) return;

    this.sending.set(true);
    this.registerInteraction();
    try {
      const response = await this.communication.postMessage(conversationId, content);
      this.draftMessage.set('');
      this.messages.update(current => [...current, response.user_message, response.assistant_message]);
      this.scheduleScrollToBottom();
      this.streamAssistantMessage(conversationId, response.assistant_message.id, response.stream.url);
    } catch (error: any) {
      this.showError(error?.error?.error || this.translate.instant('CONTEXT_CHAT.ERROR_SEND'));
    } finally {
      this.sending.set(false);
    }
  }

  onComposerEnter(event: Event): void {
    const keyboardEvent = event as KeyboardEvent;
    if (keyboardEvent.shiftKey) return;
    keyboardEvent.preventDefault();
    void this.sendMessage();
  }

  async continueInMail(): Promise<void> {
    const conversationId = this.conversationId();
    if (!conversationId || this.convertingToMail()) return;

    this.convertingToMail.set(true);
    try {
      const conversation = await this.communication.convertConversationToMail(conversationId);
      this.closePanel();
      await this.router.navigate(['/mail'], { queryParams: { conversationId: conversation.id } });
    } catch (error: any) {
      this.showError(error?.error?.error || this.translate.instant('CONTEXT_CHAT.ERROR_CONTINUE_IN_MAIL'));
    } finally {
      this.convertingToMail.set(false);
    }
  }

  keepCurrentConversation(): void {
    this.showInactivityPrompt.set(false);
    this.registerInteraction();
  }

  async startNewConversation(): Promise<void> {
    this.showInactivityPrompt.set(false);
    this.stopStream();
    this.draftMessage.set('');
    this.messages.set([]);

    try {
      const conversation = await this.communication.createConversation({
        channelType: 'context_chat',
        contextKey: this.contextKey,
        recipientKind: this.recipientKind,
        recipientId: this.recipientId,
        title: '',
        summary: '',
      });
      this.conversationId.set(conversation.id);
      this.armInactivityTimer();
      this.scheduleScrollToBottom();
    } catch (error: any) {
      this.showError(error?.error?.error || this.translate.instant('CONTEXT_CHAT.ERROR_LOAD'));
    }
  }

  formatDate(value: string | null | undefined): string {
    if (!value) return this.translate.instant('CONTEXT_CHAT.DATE_UNKNOWN');
    return new Date(value).toLocaleString();
  }

  private async ensureConversation(): Promise<void> {
    this.loading.set(true);
    try {
      const existing = await this.communication.listConversations({
        channelType: 'context_chat',
        contextKey: this.contextKey,
        recipientKind: this.recipientKind,
        limit: 1,
      });
      const conversation = existing[0] || await this.communication.createConversation({
        channelType: 'context_chat',
        contextKey: this.contextKey,
        recipientKind: this.recipientKind,
        recipientId: this.recipientId,
        title: '',
        summary: '',
      });
      this.conversationId.set(conversation.id);
      await this.loadMessages(conversation.id);
    } catch (error: any) {
      this.showError(error?.error?.error || this.translate.instant('CONTEXT_CHAT.ERROR_LOAD'));
    } finally {
      this.loading.set(false);
    }
  }

  private async loadMessages(conversationId: string): Promise<void> {
    const response = await this.communication.listMessages(conversationId, 30, 0);
    this.messages.set(response.messages);
    this.scheduleScrollToBottom();
  }

  private streamAssistantMessage(conversationId: string, assistantMessageId: string, streamPath: string): void {
    this.stopStream();
    this.streamMessageId.set(assistantMessageId);

    this.streamSub = this.communication.streamAssistantResponse(streamPath).subscribe({
      next: event => this.applyStreamEvent(assistantMessageId, event),
      error: (error: any) => {
        this.patchAssistantMessage(assistantMessageId, {
          status: 'error',
          error_text: error?.message || this.translate.instant('CONTEXT_CHAT.ERROR_STREAM'),
        });
        this.showError(error?.message || this.translate.instant('CONTEXT_CHAT.ERROR_STREAM'));
        this.streamMessageId.set(null);
      },
      complete: () => {
        this.streamMessageId.set(null);
        this.streamSub = null;
        void this.loadMessages(conversationId);
      },
    });
  }

  private applyStreamEvent(messageId: string, event: CommunicationStreamEvent): void {
    if (event.type === 'message.delta') {
      this.registerInteraction();
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
      this.patchAssistantMessage(messageId, {
        status: 'completed',
        finish_reason: event.finish_reason ?? 'completed',
        error_text: '',
      });
      this.scheduleScrollToBottom();
      return;
    }
    if (event.type === 'message.error') {
      this.patchAssistantMessage(messageId, {
        status: 'error',
        error_text: event.error || this.translate.instant('CONTEXT_CHAT.ERROR_STREAM'),
      });
    }
  }

  private patchAssistantMessage(messageId: string, patch: Partial<CommunicationMessage>): void {
    this.messages.update(messages => messages.map(message => (message.id !== messageId ? message : { ...message, ...patch })));
  }

  private stopStream(): void {
    this.streamSub?.unsubscribe();
    this.streamSub = null;
    this.streamMessageId.set(null);
  }

  private showError(detail: string): void {
    this.messageService.add({
      severity: 'error',
      summary: this.translate.instant('CONTEXT_CHAT.TOAST_ERROR'),
      detail,
    });
  }

  registerInteraction(): void {
    if (!this.open()) return;
    this.armInactivityTimer();
  }

  private armInactivityTimer(): void {
    this.clearInactivityTimer();
    this.inactivityTimer = setTimeout(() => {
      this.open.set(false);
      this.showInactivityPrompt.set(true);
      this.clearInactivityTimer();
    }, this.inactivityTimeoutMs);
  }

  private clearInactivityTimer(): void {
    if (this.inactivityTimer) {
      clearTimeout(this.inactivityTimer);
      this.inactivityTimer = null;
    }
  }

  private scheduleScrollToBottom(): void {
    setTimeout(() => {
      const el = this.messageListContainer?.nativeElement;
      if (!el) return;
      el.scrollTop = el.scrollHeight;
    }, 0);
  }
}
