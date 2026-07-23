// @unocss-include
import { DARK_CLASS } from '@/constants/app';
import { AINP_LOGO_PATH } from '@/constants/brand';
import { $t } from '@/locales';
import { toggleHtmlClass } from '@/utils/common';
import { localStg } from '@/utils/storage';

export function setupLoading() {
  const darkMode = localStg.get('darkMode') || false;

  if (darkMode) {
    toggleHtmlClass(DARK_CLASS).add();
  }

  const loadingClasses = [
    'left-0 top-0',
    'left-0 bottom-0 animate-delay-500',
    'right-0 top-0 animate-delay-1000',
    'right-0 bottom-0 animate-delay-1500'
  ];

  const dot = loadingClasses
    .map(item => `<div class="absolute w-16px h-16px bg-primary rounded-8px animate-pulse ${item}"></div>`)
    .join('\n');

  const loading = `
<div class="fixed-center flex-col bg-layout">
  <div class="w-128px h-128px">
    <img src="${AINP_LOGO_PATH}" alt="AINP" class="size-full object-contain" />
  </div>
  <div class="w-56px h-56px my-36px">
    <div class="relative h-full animate-spin">
      ${dot}
    </div>
  </div>
  <h2 class="text-28px font-500 text-primary">${$t('system.title')}</h2>
</div>`;

  const app = document.getElementById('app');

  if (app) {
    app.innerHTML = loading;
  }
}
