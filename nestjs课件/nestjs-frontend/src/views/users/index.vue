<template>
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
          <th scope="col">用户名</th>
          <th scope="col">角色</th>
          <th scope="col">性别</th>
          <th scope="col">头像</th>
          <th scope="col">地址</th>
          <th scope="col">操作</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="(item,index) in lists" :key="item.id">
          <th scope="row">{{(index+1)}}</th>
          <td>{{item.username}}</td>
          <td>{{item.roles.map(o=>o.name).join(',')}}</td>
          <td>{{item.profile && item.profile.gender}}</td>
          <td>{{item.profile && item.profile.photo}}</td>
          <td>{{item.profile && item.profile.address}}</td>
          <td>
            <button type="button" class="btn btn-secondary px-3" @click="openModal('edit', item)">
              <i class="far fa-edit me-2"></i>编辑</button>
            <button type="button" class="btn btn-danger px-3 ms-3" @click="openModal('delete', item)">
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

    <!-- Button trigger modal -->
    <!-- <button type="button" class="btn btn-primary" data-bs-toggle="modal" data-bs-target="#DeleteModal">
      Launch demo modal
    </button> -->

    <DeleteModal v-model:show="deleteShow" @delete="deleteSubmit"></DeleteModal>
    <EditAddModal v-model:show="editShow" :schema="formSchema" :msg="msg" @submit="editSubmit"></EditAddModal>
  </div>
</template>

<script lang="ts">
import { defineComponent, onMounted, reactive, ref } from "vue";

import axios from "@/utils/axios";

import { getAllUsers } from "@/api/user";

import DeleteModal from "@/components/modal/DeleteModal.vue";
import EditAddModal from "@/components/modal/EditAddModal.vue";
import { FormItem } from "@/components/Form.vue";

interface Profile {
  gender: number;
  address: string;
  photo: string;
}

interface RoleItem {
  id: number;
  name: string;
}

interface UserItem {
  id: number;
  username: string;
  password?: string;
  profile: Profile;
  roles: RoleItem[];
}

export default defineComponent({
  components: {
    DeleteModal,
    EditAddModal,
  },
  setup() {
    const lists = ref([] as UserItem[]);
    const tmpItem = ref({} as UserItem);

    const formSchema = [
      {
        field: "用户名",
        type: "input",
        prop: "username",
        value: "",
        attr: {
          placeholder: "请输入用户名",
        },
      },
      {
        field: "密码",
        type: "input",
        prop: "password",
        value: "",
        attr: {
          placeholder: "请输入登录密码",
        },
      },
      {
        field: "角色",
        type: "checkbox",
        prop: "roles",
        value: [],
        children: [
          {
            value: 1,
            field: "普通用户",
          },
          {
            value: 2,
            field: "管理员",
          },
          {
            value: 3,
            field: "测试用户",
          },
        ],
      },
      {
        field: "性别",
        type: "radio",
        prop: "gender",
        value: 0,
        children: [
          {
            value: 1,
            field: "男",
          },
          {
            value: 2,
            field: "女",
          },
        ],
      },
      {
        field: "头像",
        type: "input",
        prop: "photo",
        value: "",
        attr: {
          placeholder: "请输入头像链接",
        },
      },
      {
        field: "地址",
        type: "input",
        prop: "address",
        value: "",
        attr: {
          placeholder: "请输入地址",
        },
      },
    ] as FormItem[];

    const deleteShow = ref(false);
    const editShow = ref(false);

    const formValue = reactive({
      username: "",
      password: "",
      profile: {
        gender: 0,
        address: "",
        photo: "",
      } as Profile,
      roles: [] as RoleItem[],
    } as UserItem);

    let localType: string = "";

    // 模态框的控制handler
    const msg = ref("新增");

    // 获取数据
    const getData = async () => {
      const res = (await getAllUsers()) as unknown as UserItem[];
      if (res && res.length > 0) {
        lists.value = res;
      }
    };

    onMounted(async () => {
      // 初始化
      await getData();
    });

    // 控制模态框
    const openModal = (type: string, item?: UserItem) => {
      localType = type;
      tmpItem.value = item || ({} as UserItem);
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
      console.log(formValue);
      // 判断用户是新增，还是编辑
      // if (localType === "add") {
      //   // 发送对应的数据到接口
      //   const res = await axios.post("/user", formValue);
      //   console.log("🚀 ~ file: index.vue ~ line 238 ~ editSubmit ~ res", res);
      //   // 清空form表单
      //   Object.assign(formValue, {
      //     username: "",
      //     password: "",
      //     profile: {
      //       gender: 0,
      //       address: "",
      //       photo: "",
      //     } as Profile,
      //     roles: [] as RoleItem[],
      //   });
      //   // 关闭模态框
      //   editShow.value = false;
      // }
      // todo作业 编辑
      // 编辑的时候 —> 保存id信息 -> 传参把id代上
      // 编辑之前，填充数据到form表单上 -> formValue -> mapper -> roles id
    };

    // 删除该条数据
    const deleteSubmit = async () => {
      // 1.获取用户删除的item -> id
      const id = tmpItem.value.id;
      // 2.发送删除请求
      const res = (await axios.delete("/user/" + id)) as UserItem;
      // 3.请求成功之后关闭模态框
      console.log("🚀 ~ file: index.vue ~ line 272 ~ deleteSubmit ~ res", res);
      if (res.username === tmpItem.value.username) {
        deleteShow.value = false;
      }
      // 获取新的列表数据并更新
      await getData();
    };

    return {
      formValue,
      lists,
      openModal,
      msg,
      editSubmit,
      deleteSubmit,
      deleteShow,
      editShow,
      formSchema,
    };
  },
});
</script>

<style scoped>
</style>