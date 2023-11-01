import { defineStore } from 'pinia';

// 如何更新与保存token状态
export const useUserStore = defineStore('users', {
  state: () => ({
    token: '',
  }),
  persist: true,
});
