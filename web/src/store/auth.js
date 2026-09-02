import { create } from 'zustand';
import { client } from '../api/client';

export const useAuthStore = create((set) => ({
  token: null,
  user: null,
  tenant: null,
  isAuthenticated: false,

  login: async (username, password) => {
    const res = await client.post('/api/auth/login', { username, password });
    const { token, user, tenant } = res.data;
    localStorage.setItem('sb_token', token);
    set({ token, user, tenant, isAuthenticated: true });
    return res.data;
  },

  logout: () => {
    localStorage.removeItem('sb_token');
    set({ token: null, user: null, tenant: null, isAuthenticated: false });
    window.location.href = '/login';
  },

  checkAuth: async () => {
    const token = localStorage.getItem('sb_token');
    if (!token) {
      set({ isAuthenticated: false });
      return;
    }
    try {
      const res = await client.get('/api/auth/me', {
        headers: { Authorization: `Bearer ${token}` },
      });
      set({
        token,
        user: res.data.user || res.data,
        tenant: res.data.tenant || null,
        isAuthenticated: true,
      });
    } catch {
      localStorage.removeItem('sb_token');
      set({ token: null, user: null, tenant: null, isAuthenticated: false });
    }
  },
}));
