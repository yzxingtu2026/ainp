const local: App.I18n.Schema = {
  system: {
    title: 'Soybean 管理系统',
    updateTitle: '系统版本更新通知',
    updateContent: '检测到系统有新版本发布，是否立即刷新页面？',
    updateConfirm: '立即刷新',
    updateCancel: '稍后再说'
  },
  common: {
    action: '操作',
    add: '新增',
    addSuccess: '添加成功',
    backToHome: '返回首页',
    batchDelete: '批量删除',
    cancel: '取消',
    close: '关闭',
    check: '勾选',
    selectAll: '全选',
    expandColumn: '展开列',
    columnSetting: '列设置',
    config: '配置',
    confirm: '确认',
    delete: '删除',
    deleteSuccess: '删除成功',
    confirmDelete: '确认删除吗？',
    detail: '详情',
    edit: '编辑',
    warning: '警告',
    error: '错误',
    index: '序号',
    keywordSearch: '请输入关键词搜索',
    logout: '退出登录',
    logoutConfirm: '确认退出登录吗？',
    lookForward: '敬请期待',
    modify: '修改',
    modifySuccess: '修改成功',
    noData: '无数据',
    next: '下一步',
    operate: '操作',
    pleaseCheckValue: '请检查输入的值是否合法',
    refresh: '刷新',
    reset: '重置',
    search: '搜索',
    switch: '切换',
    tip: '提示',
    trigger: '触发',
    update: '更新',
    updateSuccess: '更新成功',
    userCenter: '个人中心',
    yesOrNo: {
      yes: '是',
      no: '否'
    }
  },
  request: {
    logout: '请求失败后登出用户',
    logoutMsg: '用户状态失效，请重新登录',
    logoutWithModal: '请求失败后弹出模态框再登出用户',
    logoutWithModalMsg: '用户状态失效，请重新登录',
    refreshToken: '请求的token已过期，刷新token',
    tokenExpired: 'token已过期'
  },
  concurrency: {
    keyRequired: '该操作需要提供幂等键',
    conflict: '相同幂等键已用于不同的请求',
    inProgress: '操作正在处理中，请勿重复提交',
    revisionConflict: '数据已被他人更新，请刷新后重试',
    resourceLocked: '资源正在被其他操作占用，请稍后重试'
  },
  theme: {
    themeDrawerTitle: '主题配置',
    tabs: {
      appearance: '外观',
      layout: '布局',
      general: '通用',
      preset: '预设'
    },
    appearance: {
      themeSchema: {
        title: '主题模式',
        light: '亮色模式',
        dark: '暗黑模式',
        auto: '跟随系统'
      },
      grayscale: '灰色模式',
      colourWeakness: '色弱模式',
      themeColor: {
        title: '主题颜色',
        primary: '主色',
        info: '信息色',
        success: '成功色',
        warning: '警告色',
        error: '错误色',
        followPrimary: '跟随主色'
      },
      themeRadius: {
        title: '主题圆角'
      },
      recommendColor: '应用推荐算法的颜色',
      recommendColorDesc: '推荐颜色的算法参照',
      preset: {
        title: '主题预设',
        apply: '应用',
        applySuccess: '预设应用成功',
        default: {
          name: '默认预设',
          desc: 'Soybean 默认主题预设'
        },
        dark: {
          name: '暗色预设',
          desc: '适用于夜间使用的暗色主题预设'
        },
        compact: {
          name: '紧凑型',
          desc: '适用于小屏幕的紧凑布局预设'
        },
        azir: {
          name: 'Azir的预设',
          desc: '是 Azir 比较喜欢的莫兰迪色系冷淡风'
        }
      }
    },
    layout: {
      layoutMode: {
        title: '布局模式',
        vertical: '左侧菜单模式',
        'vertical-mix': '左侧菜单混合模式',
        'vertical-hybrid-header-first': '左侧混合-顶部优先',
        horizontal: '顶部菜单模式',
        'top-hybrid-sidebar-first': '顶部混合-侧边优先',
        'top-hybrid-header-first': '顶部混合-顶部优先',
        vertical_detail: '左侧菜单布局，菜单在左，内容在右。',
        'vertical-mix_detail': '左侧双菜单布局，一级菜单在左侧深色区域，二级菜单在左侧浅色区域。',
        'vertical-hybrid-header-first_detail':
          '左侧混合布局，一级菜单在顶部，二级菜单在左侧深色区域，三级菜单在左侧浅色区域。',
        horizontal_detail: '顶部菜单布局，菜单在顶部，内容在下方。',
        'top-hybrid-sidebar-first_detail': '顶部混合布局，一级菜单在左侧，二级菜单在顶部。',
        'top-hybrid-header-first_detail': '顶部混合布局，一级菜单在顶部，二级菜单在左侧。'
      },
      tab: {
        title: '标签栏设置',
        visible: '显示标签栏',
        cache: '标签栏信息缓存',
        cacheTip: '离开页面后仍然保留标签栏信息',
        height: '标签栏高度',
        mode: {
          title: '标签栏风格',
          slider: '滑块风格',
          chrome: '谷歌风格',
          button: '按钮风格'
        },
        closeByMiddleClick: '鼠标中键关闭标签页',
        closeByMiddleClickTip: '启用后可以使用鼠标中键点击标签页进行关闭'
      },
      header: {
        title: '头部设置',
        height: '头部高度',
        breadcrumb: {
          visible: '显示面包屑',
          showIcon: '显示面包屑图标'
        }
      },
      sider: {
        title: '侧边栏设置',
        inverted: '深色侧边栏',
        width: '侧边栏宽度',
        collapsedWidth: '侧边栏折叠宽度',
        mixWidth: '混合布局侧边栏宽度',
        mixCollapsedWidth: '混合布局侧边栏折叠宽度',
        mixChildMenuWidth: '混合布局子菜单宽度',
        autoSelectFirstMenu: '自动选择第一个子菜单',
        autoSelectFirstMenuTip: '点击一级菜单时，自动选择并导航到第一个子菜单的最深层级'
      },
      footer: {
        title: '底部设置',
        visible: '显示底部',
        fixed: '固定底部',
        height: '底部高度',
        right: '底部居右'
      },
      content: {
        title: '内容区域设置',
        scrollMode: {
          title: '滚动模式',
          tip: '主题滚动仅 main 部分滚动，外层滚动可携带头部底部一起滚动',
          wrapper: '外层滚动',
          content: '主体滚动'
        },
        page: {
          animate: '页面切换动画',
          mode: {
            title: '页面切换动画类型',
            'fade-slide': '滑动',
            fade: '淡入淡出',
            'fade-bottom': '底部消退',
            'fade-scale': '缩放消退',
            'zoom-fade': '渐变',
            'zoom-out': '闪现',
            none: '无'
          }
        },
        fixedHeaderAndTab: '固定头部和标签栏'
      }
    },
    general: {
      title: '通用设置',
      watermark: {
        title: '水印设置',
        visible: '显示全屏水印',
        text: '自定义水印文本',
        enableUserName: '启用用户名水印',
        enableTime: '显示当前时间',
        timeFormat: '时间格式'
      },
      multilingual: {
        title: '多语言设置',
        visible: '显示多语言按钮'
      },
      globalSearch: {
        title: '全局搜索设置',
        visible: '显示全局搜索按钮'
      }
    },
    configOperation: {
      copyConfig: '复制配置',
      copySuccessMsg: '复制成功，请替换 src/theme/settings.ts 中的变量 themeSettings',
      resetConfig: '重置配置',
      resetSuccessMsg: '重置成功'
    }
  },
  route: {
    login: '登录',
    403: '无权限',
    404: '页面不存在',
    500: '服务器错误',
    'iframe-page': '外链页面',
    home: '首页',
    demo: '演示',
    demo_hello: 'Hello',
    demo_attachments: '附件联调',
    system: '系统',
    system_profile: '个人中心',
    system_users: '用户管理',
    system_roles: '角色管理',
    system_settings: '系统设置',
    system_attachments: '附件中心',
    setup: '安装向导'
  },
  page: {
    setting: {
      title: '系统设置',
      empty: '暂无可用设置',
      required: '必填',
      reset: '重置为默认',
      resetConfirm: '确认删除租户覆盖并恢复代码默认值？',
      resetSuccess: '已重置为默认值'
    },
    attachment: {
      demoTitle: '附件组件联调',
      demoHint: '在业务上下文中通过选择器上传/选择附件；非生产附件库搜索。',
      dropOrClick: '拖拽文件到此处，或点击选择',
      selectFiles: '选择文件',
      uploadHint: '单文件不超过 {size}，允许扩展名：{ext}',
      cancel: '取消上传',
      retry: '重试',
      empty: '暂无附件',
      preview: '预览',
      previewFailed: '预览加载失败',
      previewUnavailable: '暂无法预览',
      download: '下载',
      unbind: '解绑',
      confirmUnbind: '确认解除该附件与业务资源的关联？',
      confirmDelete: '确认删除该附件？删除后不可恢复。',
      uploadSuccess: '附件已关联',
      loadFailed: '加载附件列表失败',
      bindFailed: '绑定附件失败',
      unbindFailed: '解绑失败',
      deleteFailed: '删除失败',
      downloadFailed: '下载失败',
      noUploadPermission: '无上传权限',
      noBindPermission: '无绑定权限',
      openPicker: '选择附件',
      pickerTitle: '附件选择器',
      pickerMine: '我的上传',
      pickerUpload: '上传新文件',
      pickerConfirm: '确认选择',
      searchName: '按文件名搜索',
      maxCountReached: '已达到可选数量上限',
      loadingMore: '加载更多…',
      noMore: '没有更多了',
      loadMoreCap: '已加载较多结果，请缩小搜索范围或前往附件中心',
      centerTitle: '附件中心',
      detailTitle: '附件详情',
      fileName: '文件名',
      contentType: '类型',
      size: '大小',
      statusLabel: '状态',
      statusAll: '全部状态',
      uploader: '上传者',
      createdAt: '创建时间',
      links: '业务绑定',
      noLinks: '暂无业务绑定',
      status: {
        queued: '排队中',
        preparing: '准备中',
        uploading: '上传中',
        completing: '完成中',
        success: '成功',
        failed: '失败',
        cancelled: '已取消'
      },
      precheck: {
        size: '文件过大',
        ext: '扩展名不允许',
        mime: '类型不允许',
        empty: '空文件'
      }
    },
    user: {
      title: '用户管理',
      createTitle: '新建用户',
      editTitle: '编辑用户',
      rolesTitle: '为 {name} 分配角色',
      passwordTitle: '重置 {name} 的密码',
      username: '用户名',
      usernameInvalid: '用户名需以字母开头，3-64 位字母/数字/下划线',
      displayName: '显示名',
      status: '状态',
      roles: '角色',
      updatedAt: '更新时间',
      password: '密码',
      confirmPassword: '确认密码',
      passwordHint: '密码至少 8 位，需同时包含字母和数字',
      passwordMismatch: '两次输入密码不一致',
      assignRoles: '分配角色',
      resetPassword: '重置密码',
      enable: '启用',
      disable: '停用',
      confirmEnable: '确认启用该用户？',
      confirmDisable: '确认停用该用户？停用后其 Token 将立即失效。',
      enabled: '已启用',
      disabled: '已停用',
      builtin: '内置'
    },
    role: {
      title: '角色管理',
      createTitle: '新建角色',
      editTitle: '编辑角色',
      permissionTitle: '为 {name} 分配权限',
      permissionSearch: '搜索权限、Action 或说明',
      selectedCount: '已选择 {count} 项权限',
      code: '角色编码',
      name: '角色名称',
      description: '描述',
      status: '状态',
      type: '类型',
      builtin: '内置',
      custom: '自定义',
      enabled: '已启用',
      disabled: '已停用',
      userCount: '用户数',
      permissions: '分配权限',
      updatedAt: '更新时间',
      reloginRequired: '权限已更新，你当前角色的登录态已失效，请重新登录后再继续操作。',
      reloginNow: '重新登录'
    },
    login: {
      common: {
        loginOrRegister: '登录 / 注册',
        userNamePlaceholder: '请输入用户名',
        phonePlaceholder: '请输入手机号',
        codePlaceholder: '请输入验证码',
        passwordPlaceholder: '请输入密码',
        confirmPasswordPlaceholder: '请再次输入密码',
        codeLogin: '验证码登录',
        confirm: '确定',
        back: '返回',
        validateSuccess: '验证成功',
        loginSuccess: '登录成功',
        welcomeBack: '欢迎回来，{userName} ！'
      },
      pwdLogin: {
        title: '密码登录',
        rememberMe: '记住我',
        forgetPassword: '忘记密码？',
        register: '注册账号',
        otherAccountLogin: '其他账号登录',
        otherLoginMode: '其他登录方式',
        superAdmin: '超级管理员',
        admin: '管理员',
        user: '普通用户'
      },
      codeLogin: {
        title: '验证码登录',
        getCode: '获取验证码',
        reGetCode: '{time}秒后重新获取',
        sendCodeSuccess: '验证码发送成功',
        imageCodePlaceholder: '请输入图片验证码'
      },
      register: {
        title: '注册账号',
        agreement: '我已经仔细阅读并接受',
        protocol: '《用户协议》',
        policy: '《隐私权政策》'
      },
      resetPwd: {
        title: '重置密码'
      },
      bindWeChat: {
        title: '绑定微信'
      }
    },
    home: {
      branchDesc:
        '为了方便大家开发和更新合并，我们对main分支的代码进行了精简，只保留了首页菜单，其余内容已移至example分支进行维护。预览地址显示的内容即为example分支的内容。',
      greeting: '早安，{userName}, 今天又是充满活力的一天!',
      weatherDesc: '今日多云转晴，20℃ - 25℃!',
      projectCount: '项目数',
      todo: '待办',
      message: '消息',
      downloadCount: '下载量',
      registerCount: '注册量',
      schedule: '作息安排',
      study: '学习',
      work: '工作',
      rest: '休息',
      entertainment: '娱乐',
      visitCount: '访问量',
      turnover: '成交额',
      dealCount: '成交量',
      projectNews: {
        title: '项目动态',
        moreNews: '更多动态',
        desc1: 'Soybean 在2021年5月28日创建了开源项目 soybean-admin!',
        desc2: 'Yanbowe 向 soybean-admin 提交了一个bug，多标签栏不会自适应。',
        desc3: 'Soybean 准备为 soybean-admin 的发布做充分的准备工作!',
        desc4: 'Soybean 正在忙于为soybean-admin写项目说明文档！',
        desc5: 'Soybean 刚才把工作台页面随便写了一些，凑合能看了！'
      },
      creativity: '创意'
    },
    setup: {
      title: '系统安装向导',
      stepEnv: '环境检查',
      stepDatabase: '数据库配置',
      stepAdmin: '管理员创建',
      stepComplete: '安装完成',
      goVersion: 'Go 版本',
      os: '操作系统',
      dsnSource: 'DSN 来源',
      i18nLangs: 'i18n 语言',
      i18nKeys: 'i18n 词条数',
      dsnFromEnv: 'DSN 已通过环境变量注入，无需手动填写',
      dbHost: '数据库主机',
      dbHostPlaceholder: '127.0.0.1',
      dbPort: '端口',
      dbPortPlaceholder: '5432',
      dbName: '数据库名',
      dbNamePlaceholder: 'ainp',
      dbUser: '数据库用户',
      dbUserPlaceholder: 'ainp',
      dbPassword: '数据库密码',
      dbPasswordPlaceholder: '请输入数据库密码',
      dbConnectFail: '数据库连接失败',
      dbConnectOk: '数据库连接成功',
      testConnection: '测试连接',
      tenantName: '租户名称',
      tenantNamePlaceholder: '请输入租户名称',
      adminUsername: '管理员用户名',
      adminUsernamePlaceholder: '请输入管理员用户名',
      adminPassword: '管理员密码',
      adminPasswordPlaceholder: '请输入管理员密码',
      confirmPassword: '确认密码',
      confirmPasswordPlaceholder: '请再次输入密码',
      passwordMismatch: '两次输入密码不一致',
      passwordTooShort: '密码至少 8 位，需同时包含字母和数字',
      initialize: '开始初始化',
      completeTitle: '安装完成',
      completeDesc: '系统已成功初始化，请重启后端服务后前往登录页面使用',
      restartRequired: '需要重启后端服务以加载新配置，请重启后点击下方按钮前往登录',
      goLogin: '前往登录'
    }
  },
  form: {
    required: '不能为空',
    userName: {
      required: '请输入用户名',
      invalid: '用户名格式不正确'
    },
    phone: {
      required: '请输入手机号',
      invalid: '手机号格式不正确'
    },
    pwd: {
      required: '请输入密码',
      invalid: '密码至少 8 位，需同时包含字母和数字'
    },
    confirmPwd: {
      required: '请输入确认密码',
      invalid: '两次输入密码不一致'
    },
    code: {
      required: '请输入验证码',
      invalid: '验证码格式不正确'
    },
    email: {
      required: '请输入邮箱',
      invalid: '邮箱格式不正确'
    }
  },
  dropdown: {
    closeCurrent: '关闭',
    closeOther: '关闭其它',
    closeLeft: '关闭左侧',
    closeRight: '关闭右侧',
    closeAll: '关闭所有',
    pin: '固定标签',
    unpin: '取消固定'
  },
  icon: {
    themeConfig: '主题配置',
    themeSchema: '主题模式',
    lang: '切换语言',
    fullscreen: '全屏',
    fullscreenExit: '退出全屏',
    reload: '刷新页面',
    collapse: '折叠菜单',
    expand: '展开菜单',
    pin: '固定',
    unpin: '取消固定'
  },
  datatable: {
    itemCount: '共 {total} 条',
    fixed: {
      left: '左固定',
      right: '右固定',
      unFixed: '取消固定'
    }
  }
};

export default local;
