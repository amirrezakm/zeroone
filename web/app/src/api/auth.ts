import { useQuery } from '@tanstack/react-query';
import { api, post } from './client';

export type Me = {
  ok: boolean;
  username: string;
  auth: '' | 'session' | 'token';
  admins_count: number;
  bootstrap_needed: boolean;
};

export function fetchMe(): Promise<Me> {
  return api<Me>('/api/me');
}

export function login(username: string, password: string) {
  return post<{ ok: boolean; username: string; expires_at: number }>('/api/login', { username, password });
}

export function logout() {
  return post<{ ok: boolean }>('/api/logout');
}

export function useMe() {
  return useQuery({
    queryKey: ['me'],
    queryFn: fetchMe,
    staleTime: 30_000,
    retry: false,
  });
}

export type AdminItem = {
  username: string;
  created_at: number;
  last_login: number;
};

export function useAdmins() {
  return useQuery({
    queryKey: ['admins'],
    queryFn: () => api<{ ok: boolean; admins: AdminItem[] }>('/api/admins'),
  });
}
