<template>
  <div>
    <div>
      <div class="mb-3">
        <button type="button" class="btn btn-primary px-3" @click="openModal('add')">
          <i class="fas fa-plus"></i>新增
        </button>
      </div>
      <table class="table table-bordered table-hover table-striped">
        <thead>
          <tr>
            <th scope="col">#</th>
            <th scope="col">菜单名称</th>
            <th scope="col">菜单路径</th>
            <th scope="col">排序</th>
            <th scope="col">操作策略</th>
            <th scope="col">操作</th>
          </tr>
        </thead>
        <tbody>
          <tr>
            <td>1</td>
            <td>首页</td>
            <td>/dashboard</td>
            <td>1</td>
            <td>READ,UPDATE</td>
            <td>
              <button type="button" class="btn btn-secondary px-3" @click="openModal('edit')">
                <i class="far fa-edit me-2"></i>编辑</button>
              <button type="button" class="btn btn-danger px-3 ms-3" @click="openModal('delete')">
                <i class="far fa-trash-alt me-2"></i>删除
              </button>
            </td>
          </tr>
        </tbody>
      </table>
      <nav aria-label="Page navigation example">
        <ul class="pagination">
          <li class="page-item"><a class="page-link" href="#">Previous</a></li>
          <li class="page-item"><a class="page-link" href="#">1</a></li>
          <li class="page-item"><a class="page-link" href="#">2</a></li>
          <li class="page-item"><a class="page-link" href="#">3</a></li>
          <li class="page-item"><a class="page-link" href="#">Next</a></li>
        </ul>
      </nav>
    </div>
    <DeleteModal v-model:show="deleteShow" @delete="deleteSubmit"></DeleteModal>
    <EditAddModal v-model:show="editShow" :schema="formSchema" :msg="msg" @submit="editSubmit"></EditAddModal>
  </div>
</template>

<script lang="ts">
import { defineComponent, ref } from "vue";

import DeleteModal from "@/components/modal/DeleteModal.vue";
import EditAddModal from "@/components/modal/EditAddModal.vue";
import { FormItem } from "@/components/Form.vue";

export default defineComponent({
  components: {
    DeleteModal,
    EditAddModal,
  },
  setup() {
    const deleteShow = ref(false);
    const editShow = ref(false);

    // 模态框的控制handler
    const msg = ref("新增");

    let localType = "";

    const formSchema = [
      {
        field: "菜单名",
        type: "input",
        prop: "name",
        value: "",
        attr: {
          placeholder: "请输入菜单名称",
        },
      },
      {
        field: "菜单路径",
        type: "input",
        prop: "path",
        value: "",
        attr: {
          placeholder: "请输入菜单路径",
        },
      },
      {
        field: "菜单排序",
        type: "input",
        prop: "order",
        value: "",
        attr: {
          placeholder: "请输入菜单排序",
        },
      },
      {
        field: "允许的操作策略",
        type: "checkbox",
        prop: "roles",
        value: [],
        children: [
          {
            value: 1,
            field: "CREATE",
          },
          {
            value: 2,
            field: "UPDATE",
          },
          {
            value: 3,
            field: "DELETE",
          },
          {
            value: 4,
            field: "READ",
          },
          {
            value: 5,
            field: "MANAGE",
          },
        ],
      },
    ] as FormItem[];

    const deleteSubmit = () => {
      console.log("delete");
    };

    // 控制模态框
    const openModal = (type: string) => {
      localType = type;
      // console.log(item);
      if (type === "delete") {
        deleteShow.value = true;
      } else if (type === "edit") {
        msg.value = "编辑";
        editShow.value = true;
      } else if (type === "add") {
        msg.value = "新增";
        editShow.value = true;
      }
    };

    const editSubmit = async (val: any) => {
      console.log("🚀 ~ file: index.vue ~ line 221 ~ editSubmit ~ val", val);
    };

    return {
      deleteShow,
      deleteSubmit,
      formSchema,
      editShow,
      msg,
      editSubmit,
      openModal,
    };
  },
});
</script>

<style scoped>
</style>