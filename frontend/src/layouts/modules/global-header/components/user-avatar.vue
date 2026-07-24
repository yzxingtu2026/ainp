<script setup lang="ts">
import { computed } from 'vue';
import type { VNode } from 'vue';
import { useAuthStore } from '@/store/modules/auth';
import { useAttachmentAvatar } from '@/hooks/business/agent-avatar';
import { useRouterPush } from '@/hooks/common/router';
import { useSvgIcon } from '@/hooks/common/icon';
import { $t } from '@/locales';

defineOptions({
  name: 'UserAvatar'
});

const authStore = useAuthStore();
const { routerPushByKey, toLogin } = useRouterPush();
const { SvgIconVNode } = useSvgIcon();
const avatarAttachmentId = computed(() => authStore.userInfo.avatarAttachmentId);
const { url: avatarUrl } = useAttachmentAvatar(avatarAttachmentId);

function loginOrRegister() {
  toLogin();
}

type DropdownKey = 'system_profile' | 'logout';

type DropdownOption =
  | {
      key: DropdownKey;
      label: string;
      icon?: () => VNode;
    }
  | {
      type: 'divider';
      key: string;
    };

const options = computed(() => {
  const opts: DropdownOption[] = [
    {
      label: $t('route.system_profile'),
      key: 'system_profile',
      icon: SvgIconVNode({ icon: 'ph:user-circle', fontSize: 18 })
    },
    { type: 'divider', key: 'profile-divider' },
    {
      label: $t('common.logout'),
      key: 'logout',
      icon: SvgIconVNode({ icon: 'ph:sign-out', fontSize: 18 })
    }
  ];

  return opts;
});

function logout() {
  window.$dialog?.info({
    title: $t('common.tip'),
    content: $t('common.logoutConfirm'),
    positiveText: $t('common.confirm'),
    negativeText: $t('common.cancel'),
    onPositiveClick: () => {
      authStore.logout();
    }
  });
}

function handleDropdown(key: DropdownKey) {
  if (key === 'logout') {
    logout();
  } else {
    // If your other options are jumps from other routes, they will be directly supported here
    routerPushByKey(key);
  }
}
</script>

<template>
  <NButton v-if="!authStore.isLogin" quaternary @click="loginOrRegister">
    {{ $t('page.login.common.loginOrRegister') }}
  </NButton>
  <NDropdown v-else placement="bottom" trigger="click" :options="options" @select="handleDropdown">
    <div>
      <ButtonIcon>
        <span class="header-user-avatar" role="img" :aria-label="`${authStore.userInfo.userName}的头像`">
          <img v-if="avatarUrl" :src="avatarUrl" :alt="authStore.userInfo.userName" />
          <SvgIcon v-else icon="ph:user-circle" />
        </span>
        <span class="text-16px font-medium">{{ authStore.userInfo.userName }}</span>
      </ButtonIcon>
    </div>
  </NDropdown>
</template>

<style scoped>
.header-user-avatar {
  width: 32px;
  height: 32px;
  flex: none;
  display: grid;
  place-items: center;
  overflow: hidden;
  border-radius: 50%;
  font-size: 32px;
}

.header-user-avatar img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
</style>
