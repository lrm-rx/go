import { createRouter, createWebHashHistory, RouteRecordRaw } from 'vue-router';
import { App } from 'vue';

const routes = [
  {
    path: '/login',
    component: () => import('@/views/login/login.vue'),
  },
  {
    path: '/reg',
    component: () => import('@/views/login/reg.vue'),
  },
  // 内容页
  // 首页
  {
    path: '/home',
    component: () => import('@/layouts/default.vue'),
    name: 'home',
    redirect: '/home/dashboard',
    children: [
      {
        name: 'dashboard',
        path: 'dashboard',
        component: () => import('@/views/dashboard/index.vue'),
        meta: {
          icon: 'fas fa-home',
        },
      },
      {
        name: 'users',
        path: 'users',
        component: () => import('@/views/users/index.vue'),
        meta: {
          icon: 'fas fa-user',
        },
      },
      {
        name: 'menus',
        path: 'menus',
        component: () => import('@/views/menus/index.vue'),
        meta: {
          icon: 'fas fa-bars',
        },
      },
      {
        name: 'roles',
        path: 'roles',
        component: () => import('@/views/roles/index.vue'),
        meta: {
          icon: 'fas fa-tools',
        },
      },
    ],
  },
] as RouteRecordRaw[];

const router = createRouter({
  history: createWebHashHistory(),
  routes,
});

export function setupRouter(app: App<Element>) {
  app.use(router);
}
