import { inject, Injectable } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { firstValueFrom, Observable } from 'rxjs';
import { AuthService } from './auth.service';

export interface DeanConversation {
  id: string;
  user_id: string;
  title: string;
  summary: string;
  status: string;
  last_message_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface DeanMessage {
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

export interface PostDeanMessageResponse {
  user_message: DeanMessage;
  assistant_message: DeanMessage;
  stream: {
    url: string;
    method: 'GET' | string;
    content_type: 'text/event-stream' | string;
  };
}

export interface DeanStreamEvent {
  type: string;
  delta?: string;
  content?: string;
  finish_reason?: string;
  error?: string;
}

@Injectable({ providedIn: 'root' })
export class DeanOfficeService {
  private readonly http = inject(HttpClient);
  private readonly auth = inject(AuthService);

  async listConversations(limit = 50, offset = 0): Promise<DeanConversation[]> {
    const response = await firstValueFrom(
      this.http.get<{ conversations: DeanConversation[] }>('/api/v1/dean-office/conversations', {
        params: { limit, offset },
      })
    );
    return response.conversations;
  }

  async createConversation(input?: { title?: string; summary?: string }): Promise<DeanConversation> {
    const response = await firstValueFrom(
      this.http.post<{ conversation: DeanConversation }>('/api/v1/dean-office/conversations', input ?? {})
    );
    return response.conversation;
  }

  async listMessages(conversationId: string, limit = 100, offset = 0): Promise<{ conversation: DeanConversation; messages: DeanMessage[] }> {
    return firstValueFrom(
      this.http.get<{ conversation: DeanConversation; messages: DeanMessage[] }>(
        `/api/v1/dean-office/conversations/${conversationId}/messages`,
        { params: { limit, offset } }
      )
    );
  }

  async postMessage(conversationId: string, content: string, provider = '', model = ''): Promise<PostDeanMessageResponse> {
    return firstValueFrom(
      this.http.post<PostDeanMessageResponse>(`/api/v1/dean-office/conversations/${conversationId}/messages`, {
        content,
        provider,
        model,
      })
    );
  }

  streamAssistantResponse(path: string): Observable<DeanStreamEvent> {
    return new Observable<DeanStreamEvent>(subscriber => {
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
    subscriber: { next: (value: DeanStreamEvent) => void; complete: () => void; error: (error: unknown) => void; closed: boolean }
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
        const payload = JSON.parse(rawData) as DeanStreamEvent;
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
