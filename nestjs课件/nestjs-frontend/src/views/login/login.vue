<template>
  <div class="container d-flex justify-content-center align-items-center vh-100">
    <div class="col-11 col-sm-8 col-lg-6 col-xl-4 col-xxl-4">
      <form class="border shadow-sm rounded p-4">
        <div class="mb-3">
          <label for="exampleInputEmail1" class="form-label">用户名：</label>
          <input type="email" :class="['form-control', {'is-invalid': loginInfo.usernameMsg}]" id="exampleInputEmail1" aria-describedby="emailHelp" v-model="loginInfo.username">
          <div class="invalid-feedback">
            {{loginInfo.usernameMsg}}
          </div>
        </div>
        <div class="mb-3">
          <label for="exampleInputPassword1" class="form-label">密码：</label>
          <input type="password" :class="['form-control', {'is-invalid': loginInfo.passwordMsg}]" id="exampleInputPassword1" v-model="loginInfo.password">
          <div class="invalid-feedback">
            {{loginInfo.passwordMsg}}
          </div>
        </div>
        <div class="mb-3 form-check">
          <input type="checkbox" class="form-check-input" id="exampleCheck1">
          <label class="form-check-label" for="exampleCheck1">记住我</label>
        </div>
        <div class="d-flex flex-column align-items-center px-1">
          <button type="submit" class="btn btn-primary w-100 mb-2 text-light" @click="submit()">登录</button>
          <router-link to="/reg" class="border rounded w-100 text-decoration-none text-center">
            <button type="submit" class="btn">注册</button>
          </router-link>
        </div>
      </form>
    </div>
  </div>
</template>

<script lang="ts">
import { defineComponent, reactive, computed } from "vue";
// import axios from "@/utils/axios";
import { useRouter } from "vue-router";
import { signin } from "@/api/login";
import { useUserStore } from "@/store/user";

interface LoginResponse {
  access_token: string;
}

export default defineComponent({
  setup() {
    // 获取路由实例
    const router = useRouter();
    const store = useUserStore();

    const loginInfo = reactive({
      username: "",
      usernameMsg: computed(() => {
        // if (
        //   loginInfo.username !== "" &&
        //   !/[A-Z0-9._%+-]+@[A-Z0-9-]+.+.[A-Z]{2,4}/gim.test(loginInfo.username)
        // ) {
        //   return "请输入正确的邮箱地址";
        // }
        return "";
      }),
      password: "",
      passwordMsg: computed(() => {
        if (loginInfo.password !== "" && loginInfo.password.length < 6) {
          return "密码的长度不能小于6位";
        }
        return "";
      }),
    });

    const submit = async () => {
      // console.log(loginInfo);
      const { username, password } = loginInfo;
      const res = (await signin(
        username,
        password
      )) as unknown as LoginResponse;
      // { access_token: xxx } -> 读取token信息并存储在localStorage中，方便进行读取和使用
      if (res.access_token) {
        store.$patch({
          token: res.access_token,
        });
      }
      router.push("/home");
      // axios.post("/auth/login", loginInfo).then((res) => {
      //   console.log(res);
      // });
    };

    return {
      loginInfo,
      submit,
    };
  },
});
</script>

<style scoped>
</style>