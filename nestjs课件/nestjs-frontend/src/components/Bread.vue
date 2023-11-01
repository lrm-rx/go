<template>
  <nav aria-label="breadcrumb" class="border-bottom ps-3">
    <ol class="breadcrumb my-2">
      <li class="breadcrumb-item">
        <router-link :to="{ name:'home' }">首页</router-link>
      </li>
      <!-- <li class="breadcrumb-item "><a href="#">Library</a></li> -->
      <li class="breadcrumb-item active" aria-current="page">{{page}}</li>
    </ol>
  </nav>
</template>

<script lang="ts">
import { useMenusStore } from "@/store/menus";
import { computed, defineComponent } from "vue";
import { useRoute } from "vue-router";

export default defineComponent({
  setup() {
    const route = useRoute();

    const store = useMenusStore();

    return {
      page: computed(
        () => store.menus.find((o) => o.routeName === route.name)?.name
      ),
    };
  },
});
</script>

<style scoped>
</style>