import axios from 'axios';
import { useUserStore } from '@/store/user';

const store = useUserStore();
// console.log('🚀 ~ file: axios.ts ~ line 4 ~ store', store.token);

const instance = axios.create({
  baseURL: '/api/v1',
  timeout: 60000,
});

// 添加请求拦截器
instance.interceptors.request.use(
  function (config) {
    // 在发送请求之前做些什么
    if (store.token) {
      config.headers!.Authorization = `Bearer ${store.token}`;
    }
    return config;
  },
  function (error) {
    // 对请求错误做些什么
    return Promise.reject(error);
  }
);

// 添加响应拦截器
instance.interceptors.response.use(
  function (response) {
    // 2xx 范围内的状态码都会触发该函数。
    // 对响应数据做点什么
    if (response.status.toString().startsWith('2')) {
      return response.data;
    }
    return response;
  },
  function (error) {
    // 超出 2xx 范围的状态码都会触发该函数。
    // 对响应错误做点什么
    return Promise.reject(error);
  }
);

export default instance;
