import axios from '@/utils/axios';

export const getAllUsers = () => axios.get('/user');
