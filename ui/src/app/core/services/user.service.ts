import { inject, Injectable } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { firstValueFrom } from 'rxjs';

export interface User {
  id: string;
  username: string;
  full_name: string;
  role: string;
  active: boolean;
  created_at: string;
  updated_at: string;
  last_login: string | null;
}

export interface CreateUserRequest {
  username: string;
  full_name: string;
  password: string;
  role: 'admin' | 'user';
}

export interface UpdateUserRequest {
  full_name?: string;
  role?: 'admin' | 'user';
  password?: string;
}

export interface UserStatusRequest {
  active: boolean;
}

@Injectable({ providedIn: 'root' })
export class UserService {
  private readonly http = inject(HttpClient);

  async listUsers(): Promise<User[]> {
    const res = await firstValueFrom(
      this.http.get<{ users: User[] }>('/api/v1/users')
    );
    return res.users;
  }

  async createUser(req: CreateUserRequest): Promise<User> {
    return firstValueFrom(
      this.http.post<User>('/api/v1/users', req)
    );
  }

  async updateUser(id: string, req: UpdateUserRequest): Promise<User> {
    return firstValueFrom(
      this.http.put<User>(`/api/v1/users/${id}`, req)
    );
  }

  async deleteUser(id: string): Promise<void> {
    await firstValueFrom(
      this.http.delete<void>(`/api/v1/users/${id}`)
    );
  }

  async toggleUserStatus(id: string, active: boolean): Promise<User> {
    return firstValueFrom(
      this.http.patch<User>(`/api/v1/users/${id}/status`, { active })
    );
  }
}
