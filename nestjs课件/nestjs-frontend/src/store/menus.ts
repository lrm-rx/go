import { defineStore } from 'pinia';

import { MenuItem } from '@/components/types/SidebarNav';

export const useMenusStore = defineStore('menu', {
  state: () => ({
    menus: [
      {
        id: 1,
        name: '控制台',
        routeName: 'dashboard',
      },
      {
        id: 2,
        name: '用户管理',
        routeName: 'users',
      },
      {
        id: 2,
        name: '角色管理',
        routeName: 'roles',
      },
      {
        id: 3,
        name: '菜单管理',
        routeName: 'menus',
      },
    ] as MenuItem[],
  }),
  persist: true,
});
