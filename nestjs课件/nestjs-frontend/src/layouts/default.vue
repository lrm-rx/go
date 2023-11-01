<template>
  <div class="container-fluid">
    <div class="row vh-100">
      <div class="col-3 col-xxl-2 px-0 d-flex flex-column">
        <div class="fs-4 text-light bg d-flex justify-content-center align-items-center" style="height: 64px">toimc通用业务框架</div>
        <!-- 左侧菜单栏组件 -->
        <SidebarNav :menus="store.menus" />
      </div>
      <div class="col-9 col-xxl-10">
        <div class="row vh-100">
          <div class="container-fluid px-0 d-flex flex-column">
            <!-- 头部导航 -->
            <HeaderNav />
            <Bread />
            <!-- 主体内容 -->
            <router-view class="bg-light flex-fill p-3"></router-view>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script lang="ts">
import { defineComponent } from "vue";
import { useMenusStore } from "@/store/menus";

import SidebarNav from "@/components/SidebarNav.vue";
import HeaderNav from "@/components/HeaderNav.vue";
import Bread from "@/components/Bread.vue";

export default defineComponent({
  components: {
    SidebarNav,
    HeaderNav,
    Bread,
  },
  setup() {
    const store = useMenusStore();
    return {
      store,
    };
  },
});
</script>

<style scoped lang="scss">
</style>