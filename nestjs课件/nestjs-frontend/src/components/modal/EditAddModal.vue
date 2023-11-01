<template>
  <!-- Edit/Add Model -->
  <div class="modal fade" ref="editRef" tabindex="-1" aria-labelledby="EditAndAddModalLabel" aria-hidden="true">
    <div class="modal-dialog">
      <div class="modal-content">
        <div class="modal-header">
          <h5 class="modal-title" id="EditAndAddModalLabel">{{msg}}</h5>
          <button type="button" class="btn-close" data-bs-dismiss="modal" aria-label="Close"></button>
        </div>
        <div class="modal-body">
          <Form :schema="schema" @change="change" ref="formRef"></Form>
          <div class="modal-footer">
            <button type="button" class="btn btn-secondary" data-bs-dismiss="modal" @click="cancel">取消</button>
            <button type="button" class="btn btn-primary" @click="submit()">确定</button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script lang="ts">
import { defineComponent, PropType, ref, toRefs } from "vue";

import { useModal } from "@/hooks";

import Form, { FormItem } from "../Form.vue";

export default defineComponent({
  props: {
    show: {
      type: Boolean,
      default: false,
    },
    msg: {
      type: String,
      default: "新增",
    },
    schema: {
      type: Array as PropType<FormItem[]>,
      default: () => [],
    },
  },
  components: {
    Form,
  },
  emits: ["submit"],
  setup(_props, { emit }) {
    const { show } = toRefs(_props);
    const formRef = ref();

    const { modalRef: editRef, hide } = useModal(show, "show");

    const formData = ref();

    // 取消
    const cancel = () => {
      hide();
    };

    // 确定
    const submit = () => {
      emit("submit", formData.value);
      hide();
      formData.value = {};
      formRef.value?.clearForm();
    };

    return {
      editRef,
      cancel,
      submit,
      formRef,
      change: (val: any) => {
        formData.value = val;
      },
    };
  },
});
</script>

<style scoped>
</style>