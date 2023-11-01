<template>
  <!-- Delete Modal -->
  <div class="modal fade" ref="deleteRef" tabindex="-1" aria-labelledby="DeleteModalLabel" aria-hidden="true">
    <div class="modal-dialog">
      <div class="modal-content">
        <div class="modal-header">
          <h5 class="modal-title" id="DeleteModalLabel">删除</h5>
          <button type="button" class="btn-close" data-bs-dismiss="modal" aria-label="Close"></button>
        </div>
        <div class="modal-body">
          确定删除该记录吗？
        </div>
        <div class="modal-footer">
          <button type="button" class="btn btn-secondary" data-bs-dismiss="modal" @click="cancel()">取消</button>
          <button type="button" class="btn btn-primary" @click="deleteSubmit()">确定</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script lang="ts">
import { defineComponent, onMounted, ref, toRefs, watch } from "vue";
// import * as bootstrap from "bootstrap";
import { useModal } from "@/hooks";

export default defineComponent({
  props: {
    show: {
      type: Boolean,
      default: false,
    },
  },
  emits: ["delete"],
  setup(_props, { emit }) {
    const { show } = toRefs(_props);

    const { modalRef: deleteRef, hide } = useModal(show, "show");

    // 删除该条数据
    const deleteSubmit = async () => {
      // @delete
      emit("delete");
    };

    // 取消
    const cancel = () => {
      hide();
    };

    return {
      deleteRef,
      deleteSubmit,
      cancel,
    };
  },
});
</script>

<style scoped>
</style>