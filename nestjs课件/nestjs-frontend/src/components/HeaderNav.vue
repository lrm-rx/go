<template>
  <nav class="navbar navbar-expand-lg navbar-light border-bottom">
    <div class="container-fluid">
      <i class="fas fa-bars"></i>
      <ul class="navbar-nav d-flex justify-content-between align-items-center">
        <li class="nav-item me-3">
          <i class="fas fa-comments"></i>
        </li>
        <li class="nav-item me-3">
          <i class="fas fa-cog"></i>
        </li>
        <li class="nav-item me-3">
          <i class="far fa-envelope-open"></i>
        </li>
        <li class="nav-item ms-4">
          <div class="dropdown">
            <a href="#" class="dropdown-toggle" data-toggle="dropdown" id="dropdownMenuButton1" data-bs-toggle="dropdown" aria-expanded="false" data-bs-offset="10,20" style="height: 60px">
              <img alt="" class="avatar-circle" :src="header" width="38" height="38" />
            </a>
            <ul class="dropdown-menu dropdown-menu-lg-end my-0 py-0" style="background: #FFFFFF" aria-labelledby="dropdownMenuButton1">
              <li class="dropdwn-item px-3 py-2 text-sencondary bg-light">
                账号
              </li>
              <li class="dropdwn-item px-4 py-2">
                <i class="fas fa-comments me-3"></i>
                <span>更新消息</span>
              </li>
              <li class="dropdwn-item px-4 py-2">
                <i class="far fa-envelope-open me-3"></i>
                <span>评论信息</span>
              </li>
              <!-- 设置 -->
              <li class="dropdwn-item px-3 py-2 text-sencondary bg-light">
                设置
              </li>
              <li class="dropdwn-item px-4 py-2">
                <i class="fas fa-user me-3"></i>
                <span>个人信息</span>
              </li>
              <li class="dropdwn-item px-4 py-2">
                <i class="fas fa-credit-card me-3"></i>
                <span>支付信息</span>
              </li>
              <li class="dropdwn-item px-4 py-2">
                <i class="fas fa-cogs me-3"></i>
                <span>系统设置</span>
              </li>
              <li>
                <hr class="dropdown-divider">
              </li>
              <li class="dropdwn-item px-4 py-2">
                <i class="fas fa-sign-out-alt me-3"></i>
                <span>退出</span>
              </li>
            </ul>
          </div>
        </li>
      </ul>
    </div>
  </nav>
</template>

<script lang="ts">
import { defineComponent } from "vue";
import header from "@/assets/headers.jpg?url";

export default defineComponent({
  setup() {
    return {
      header,
    };
  },
});
</script>

<style scoped lang="scss">
i {
  font-size: 1.2rem;
  color: #6c737d;
}
</style>