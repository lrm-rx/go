import { onMounted, ref, toRefs, getCurrentInstance, watch, Ref } from 'vue';
import * as bootstrap from 'bootstrap';

export function useModal(refValue: Ref<boolean>, key: string) {
  // 1.定义modal与modalRef
  const modalRef = ref();
  // modal是实例
  const modal = ref();
  const vm = getCurrentInstance();

  // const { [key]: isShow } = toRefs(props);

  watch(
    () => refValue.value,
    () => {
      if (refValue.value) {
        show();
      } else {
        hide();
      }
    }
  );

  // 2.初始化modal
  onMounted(async () => {
    // 模态框创建
    modal.value = new bootstrap.Modal(modalRef.value, {
      backdrop: true,
    });

    modalRef.value.addEventListener('hidden.bs.modal', function () {
      // do something...
      vm?.emit(`update:${key}`, false);
    });
  });

  // 3.定义hide与show方法
  function show() {
    modal.value.show();
  }

  function hide() {
    modal.value.hide();
  }
  // 4.返回一个show显示与否的状态量
  return {
    modal,
    modalRef,
    show,
    hide,
  };
}
