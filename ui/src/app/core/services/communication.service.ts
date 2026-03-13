import { inject, Injectable } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { firstValueFrom, Observable } from 'rxjs';
import { AuthService } from './auth.service';

export type CommunicationChannelType = 'mail_thread' | 'context_chat';
export type CommunicationRecipientKind = 'dean_office' | string;
export type CommunicationConversationStatus = 'active' | 'archived' | string;

export interface CommunicationConversation {
  id: string;
  user_id: string;
  channel_type: CommunicationChannelType | string;
  context_key: string;
  recipient_kind: CommunicationRecipientKind | string;
  recipient_id: string;
  title: string;
  summary: string;
  status: string;
  last_message_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface CommunicationMessage {
  id: string;
  conversation_id: string;
  user_id?: string;
  sequence: number;
  role: 'user' | 'assistant' | string;
  status: string;
  provider: string;
  model: string;
  content: string;
  finish_reason: string;
  provider_request_id: string;
  error_text: string;
  created_at: string;
  updated_at: string;
}

export interface PostCommunicationMessageResponse {
  user_message: CommunicationMessage;
  assistant_message: CommunicationMessage;
  stream: {
    url: string;
    method: 'GET' | string;
    content_type: 'text/event-stream' | string;
  };
}

export interface CommunicationStreamEvent {
  type: string;
  delta?: string;
  content?: string;
  finish_reason?: string;
  error?: string;
}

@Injectable({ providedIn: 'root' })
export class CommunicationService {
  private readonly http = inject(HttpClient);
  private readonly auth = inject(AuthService);

  async listConversations(
    input: {
      channelType?: CommunicationChannelType;
      contextKey?: string;
      recipientKind?: string;
      status?: CommunicationConversationStatus;
      limit?: number;
      offset?: number;
    } = {}
  ): Promise<CommunicationConversation[]> {
    const response = await firstValueFrom(
      this.http.get<{ conversations: CommunicationConversation[] }>('/api/v1/communications/conversations', {
        params: {
          channel_type: input.channelType ?? 'mail_thread',
          context_key: input.contextKey ?? '',
          recipient_kind: input.recipientKind ?? '',
          status: input.status ?? '',
          limit: input.limit ?? 50,
          offset: input.offset ?? 0,
        },
      })
    );
    return response.conversations;
  }

  async createConversation(input?: {
    channelType?: CommunicationChannelType;
    contextKey?: string;
    recipientKind?: string;
    recipientId?: string;
    title?: string;
    summary?: string;
  }): Promise<CommunicationConversation> {
    const response = await firstValueFrom(
      this.http.post<{ conversation: CommunicationConversation }>('/api/v1/communications/conversations', {
        channel_type: input?.channelType ?? 'mail_thread',
        context_key: input?.contextKey ?? '',
        recipient_kind: input?.recipientKind ?? 'dean_office',
        recipient_id: input?.recipientId ?? 'dean.taskford',
        title: input?.title ?? '',
        summary: input?.summary ?? '',
      })
    );
    return response.conversation;
  }

  async listMessages(conversationId: string, limit = 100, offset = 0): Promise<{ conversation: CommunicationConversation; messages: CommunicationMessage[] }> {
    return firstValueFrom(
      this.http.get<{ conversation: CommunicationConversation; messages: CommunicationMessage[] }>(
        `/api/v1/communications/conversations/${conversationId}/messages`,
        { params: { limit, offset } }
      )
    );
  }

  async postMessage(conversationId: string, content: string, provider = '', model = ''): Promise<PostCommunicationMessageResponse> {
    return firstValueFrom(
      this.http.post<PostCommunicationMessageResponse>(`/api/v1/communications/conversations/${conversationId}/messages`, {
        content,
        provider,
        model,
      })
    );
  }

  async convertConversationToMail(conversationId: string): Promise<CommunicationConversation> {
    const response = await firstValueFrom(
      this.http.post<{ conversation: CommunicationConversation }>(`/api/v1/communications/conversations/${conversationId}/convert-to-mail`, {})
    );
    return response.conversation;
  }

  async archiveConversation(conversationId: string): Promise<CommunicationConversation> {
    const response = await firstValueFrom(
      this.http.post<{ conversation: CommunicationConversation }>(`/api/v1/communications/conversations/${conversationId}/archive`, {})
    );
    return response.conversation;
  }

  async restoreConversation(conversationId: string): Promise<CommunicationConversation> {
    const response = await firstValueFrom(
      this.http.post<{ conversation: CommunicationConversation }>(`/api/v1/communications/conversations/${conversationId}/restore`, {})
    );
    return response.conversation;
  }

  async deleteConversation(conversationId: string): Promise<void> {
    await firstValueFrom(this.http.delete(`/api/v1/communications/conversations/${conversationId}`));
  }

  streamAssistantResponse(path: string): Observable<CommunicationStreamEvent> {
    return new Observable<CommunicationStreamEvent>(subscriber => {
      const controller = new AbortController();
      const endpoint = this.resolveApiUrl(path);

      this.consumeSseStream(endpoint, controller.signal, subscriber).catch(error => {
        if (!subscriber.closed) {
          subscriber.error(error);
        }
      });

      return () => controller.abort();
    });
  }

  private async consumeSseStream(
    endpoint: string,
    signal: AbortSignal,
    subscriber: { next: (value: CommunicationStreamEvent) => void; complete: () => void; error: (error: unknown) => void; closed: boolean }
  ): Promise<void> {
    const token = await this.ensureAccessToken();
    const response = await fetch(endpoint, {
      method: 'GET',
      headers: token ? { Authorization: `Bearer ${token}` } : undefined,
      signal,
    });

    if (!response.ok) {
      let message = `Streaming request failed with status ${response.status}`;
      try {
        const body = await response.json();
        if (typeof body?.error === 'string' && body.error.trim()) {
          message = body.error;
        }
      } catch {
        // ignore parse failures; keep default message
      }
      throw new Error(message);
    }

    const body = response.body;
    if (!body) {
      throw new Error('Streaming response body is empty');
    }

    const reader = body.getReader();
    const decoder = new TextDecoder();
    let buffer = '';
    let eventName = '';
    let dataLines: string[] = [];

    const emitCurrentEvent = (): void => {
      if (!eventName || dataLines.length === 0) return;
      const rawData = dataLines.join('\n');
      dataLines = [];

      try {
        const payload = JSON.parse(rawData) as CommunicationStreamEvent;
        if (!payload.type && eventName) {
          payload.type = eventName;
        }
        subscriber.next(payload);
      } catch {
        subscriber.next({
          type: eventName,
          error: 'Invalid stream payload',
        });
      }
    };

    try {
      while (!signal.aborted) {
        const { value, done } = await reader.read();
        if (done) break;

        buffer += decoder.decode(value, { stream: true });
        let boundary = buffer.indexOf('\n');
        while (boundary >= 0) {
          const rawLine = buffer.slice(0, boundary).replace(/\r$/, '');
          buffer = buffer.slice(boundary + 1);

          if (rawLine === '') {
            emitCurrentEvent();
            eventName = '';
          } else if (rawLine.startsWith('event:')) {
            eventName = rawLine.slice(6).trim();
          } else if (rawLine.startsWith('data:')) {
            dataLines.push(rawLine.slice(5).trim());
          }

          boundary = buffer.indexOf('\n');
        }
      }

      if (!signal.aborted) {
        if (eventName || dataLines.length > 0) {
          emitCurrentEvent();
        }
        subscriber.complete();
      }
    } finally {
      reader.releaseLock();
    }
  }

  private async ensureAccessToken(): Promise<string | null> {
    const existing = this.auth.getToken();
    if (existing) return existing;
    try {
      return await this.auth.refreshToken();
    } catch {
      return this.auth.getToken();
    }
  }

  private resolveApiUrl(path: string): string {
    if (/^https?:\/\//i.test(path)) return path;
    if (path.startsWith('/')) return path;
    return `/${path}`;
  }
}
