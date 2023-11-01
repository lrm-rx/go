import axios from '@/utils/axios';

export const signin = (username: string, password: string, ...rest: any) =>
  axios.post('/auth/signin', { username, password, ...rest });

export const signup = (username: string, password: string) =>
  axios.post('/auth/signup', { username, password });
