const local: App.I18n.Schema = {
  system: {
    title: 'SoybeanAdmin',
    updateTitle: 'System Version Update Notification',
    updateContent: 'A new version of the system has been detected. Do you want to refresh the page immediately?',
    updateConfirm: 'Refresh immediately',
    updateCancel: 'Later'
  },
  common: {
    action: 'Action',
    add: 'Add',
    addSuccess: 'Add Success',
    backToHome: 'Back to home',
    batchDelete: 'Batch Delete',
    cancel: 'Cancel',
    close: 'Close',
    check: 'Check',
    selectAll: 'Select All',
    expandColumn: 'Expand Column',
    columnSetting: 'Column Setting',
    config: 'Config',
    confirm: 'Confirm',
    delete: 'Delete',
    deleteSuccess: 'Delete Success',
    confirmDelete: 'Are you sure you want to delete?',
    detail: 'Details',
    edit: 'Edit',
    warning: 'Warning',
    error: 'Error',
    index: 'Index',
    keywordSearch: 'Please enter keyword',
    logout: 'Logout',
    logoutConfirm: 'Are you sure you want to log out?',
    lookForward: 'Coming soon',
    modify: 'Modify',
    modifySuccess: 'Modify Success',
    noData: 'No Data',
    next: 'Next',
    operate: 'Operate',
    pleaseCheckValue: 'Please check whether the value is valid',
    refresh: 'Refresh',
    reset: 'Reset',
    search: 'Search',
    switch: 'Switch',
    tip: 'Tip',
    trigger: 'Trigger',
    update: 'Update',
    updateSuccess: 'Update Success',
    userCenter: 'User Center',
    yesOrNo: {
      yes: 'Yes',
      no: 'No'
    }
  },
  request: {
    logout: 'Logout user after request failed',
    logoutMsg: 'User status is invalid, please log in again',
    logoutWithModal: 'Pop up modal after request failed and then log out user',
    logoutWithModalMsg: 'User status is invalid, please log in again',
    refreshToken: 'The requested token has expired, refresh the token',
    tokenExpired: 'The requested token has expired'
  },
  concurrency: {
    keyRequired: 'Idempotency key is required for this operation',
    conflict: 'The same idempotency key was used for a different request',
    inProgress: 'Operation is in progress, please do not resubmit',
    revisionConflict: 'Data was updated by someone else, refresh and retry',
    resourceLocked: 'Resource is locked by another operation, please retry later'
  },
  theme: {
    themeDrawerTitle: 'Theme Configuration',
    tabs: {
      appearance: 'Appearance',
      layout: 'Layout',
      general: 'General',
      preset: 'Preset'
    },
    appearance: {
      themeSchema: {
        title: 'Theme Schema',
        light: 'Light',
        dark: 'Dark',
        auto: 'Follow System'
      },
      grayscale: 'Grayscale',
      colourWeakness: 'Colour Weakness',
      themeColor: {
        title: 'Theme Color',
        primary: 'Primary',
        info: 'Info',
        success: 'Success',
        warning: 'Warning',
        error: 'Error',
        followPrimary: 'Follow Primary'
      },
      themeRadius: {
        title: 'Theme Radius'
      },
      recommendColor: 'Apply Recommended Color Algorithm',
      recommendColorDesc: 'The recommended color algorithm refers to',
      preset: {
        title: 'Theme Presets',
        apply: 'Apply',
        applySuccess: 'Preset applied successfully',
        default: {
          name: 'Default Preset',
          desc: 'Default theme preset with balanced settings'
        },
        dark: {
          name: 'Dark Preset',
          desc: 'Dark theme preset for night time usage'
        },
        compact: {
          name: 'Compact Preset',
          desc: 'Compact layout preset for small screens'
        },
        azir: {
          name: "Azir's Preset",
          desc: 'It is a cold and elegant preset that Azir likes'
        }
      }
    },
    layout: {
      layoutMode: {
        title: 'Layout Mode',
        vertical: 'Vertical Mode',
        horizontal: 'Horizontal Mode',
        'vertical-mix': 'Vertical Mix Mode',
        'vertical-hybrid-header-first': 'Left Hybrid Header-First',
        'top-hybrid-sidebar-first': 'Top-Hybrid Sidebar-First',
        'top-hybrid-header-first': 'Top-Hybrid Header-First',
        vertical_detail: 'Vertical menu layout, with the menu on the left and content on the right.',
        'vertical-mix_detail':
          'Vertical mix-menu layout, with the primary menu on the dark left side and the secondary menu on the lighter left side.',
        'vertical-hybrid-header-first_detail':
          'Left hybrid layout, with the primary menu at the top, the secondary menu on the dark left side, and the tertiary menu on the lighter left side.',
        horizontal_detail: 'Horizontal menu layout, with the menu at the top and content below.',
        'top-hybrid-sidebar-first_detail':
          'Top hybrid layout, with the primary menu on the left and the secondary menu at the top.',
        'top-hybrid-header-first_detail':
          'Top hybrid layout, with the primary menu at the top and the secondary menu on the left.'
      },
      tab: {
        title: 'Tab Settings',
        visible: 'Tab Visible',
        cache: 'Tag Bar Info Cache',
        cacheTip: 'Keep the tab bar information after leaving the page',
        height: 'Tab Height',
        mode: {
          title: 'Tab Mode',
          slider: 'Slider',
          chrome: 'Chrome',
          button: 'Button'
        },
        closeByMiddleClick: 'Close Tab by Middle Click',
        closeByMiddleClickTip: 'Enable closing tabs by clicking with the middle mouse button'
      },
      header: {
        title: 'Header Settings',
        height: 'Header Height',
        breadcrumb: {
          visible: 'Breadcrumb Visible',
          showIcon: 'Breadcrumb Icon Visible'
        }
      },
      sider: {
        title: 'Sider Settings',
        inverted: 'Dark Sider',
        width: 'Sider Width',
        collapsedWidth: 'Sider Collapsed Width',
        mixWidth: 'Mix Sider Width',
        mixCollapsedWidth: 'Mix Sider Collapse Width',
        mixChildMenuWidth: 'Mix Child Menu Width',
        autoSelectFirstMenu: 'Auto Select First Submenu',
        autoSelectFirstMenuTip:
          'When a first-level menu is clicked, the first submenu is automatically selected and navigated to the deepest level'
      },
      footer: {
        title: 'Footer Settings',
        visible: 'Footer Visible',
        fixed: 'Fixed Footer',
        height: 'Footer Height',
        right: 'Right Footer'
      },
      content: {
        title: 'Content Area Settings',
        scrollMode: {
          title: 'Scroll Mode',
          tip: 'The theme scroll only scrolls the main part, the outer scroll can carry the header and footer together',
          wrapper: 'Wrapper',
          content: 'Content'
        },
        page: {
          animate: 'Page Animate',
          mode: {
            title: 'Page Animate Mode',
            fade: 'Fade',
            'fade-slide': 'Slide',
            'fade-bottom': 'Fade Zoom',
            'fade-scale': 'Fade Scale',
            'zoom-fade': 'Zoom Fade',
            'zoom-out': 'Zoom Out',
            none: 'None'
          }
        },
        fixedHeaderAndTab: 'Fixed Header And Tab'
      }
    },
    general: {
      title: 'General Settings',
      watermark: {
        title: 'Watermark Settings',
        visible: 'Watermark Full Screen Visible',
        text: 'Custom Watermark Text',
        enableUserName: 'Enable User Name Watermark',
        enableTime: 'Show Current Time',
        timeFormat: 'Time Format'
      },
      multilingual: {
        title: 'Multilingual Settings',
        visible: 'Display multilingual button'
      },
      globalSearch: {
        title: 'Global Search Settings',
        visible: 'Display GlobalSearch button'
      }
    },
    configOperation: {
      copyConfig: 'Copy Config',
      copySuccessMsg: 'Copy Success, Please replace the variable "themeSettings" in "src/theme/settings.ts"',
      resetConfig: 'Reset Config',
      resetSuccessMsg: 'Reset Success'
    }
  },
  route: {
    ai: 'AI Center',
    ai_models: 'Model Catalog',
    ai_providers: 'Model Providers',
    ai_runtime: 'Model Debugger',
    login: 'Login',
    403: 'No Permission',
    404: 'Page Not Found',
    500: 'Server Error',
    'iframe-page': 'Iframe',
    home: 'Home',
    demo: 'Demo',
    demo_hello: 'Hello',
    demo_attachments: 'Attachments',
    system: 'System',
    system_profile: 'Profile',
    system_users: 'Users',
    system_roles: 'Roles',
    system_settings: 'Settings',
    system_attachments: 'Attachments',
    'system_notification-templates': 'Notification Templates',
    notification: 'Notifications',
    notification_list: 'Notification List',
    operations: 'Operations',
    operations_jobs: 'Job Runs',
    operations_outbox: 'Outbox Events',
    'operations_notification-deliveries': 'Notification Deliveries',
    'operations_audit-logs': 'Audit Logs',
    operations_logs: 'Log Center',
    setup: 'Setup'
  },
  operations: {
    jobsTitle: 'Job Runs',
    outboxTitle: 'Outbox Events',
    jobDetail: 'Job Details',
    outboxDetail: 'Event Details',
    jobType: 'Job Type',
    eventType: 'Event Type',
    aggregate: 'Aggregate',
    statusLabel: 'Status',
    progress: 'Progress',
    attempts: 'Attempts',
    createdAt: 'Created At',
    updatedAt: 'Updated At',
    occurredAt: 'Occurred At',
    cancelJob: 'Cancel',
    retry: 'Retry',
    confirmAction: 'Confirm this operation?',
    success: 'Operation succeeded',
    error: 'Error',
    payload: 'Payload',
    handler: 'Handler',
    status: {
      pending_enqueue: 'Pending Enqueue',
      queued: 'Queued',
      running: 'Running',
      retrying: 'Retrying',
      succeeded: 'Succeeded',
      failed: 'Failed',
      dead: 'Dead',
      cancelled: 'Cancelled',
      pending: 'Pending',
      processing: 'Processing',
      published: 'Published'
    }
  },
  auditLog: {
    title: 'Audit Logs',
    detail: 'Audit Details',
    occurredAt: 'Occurred At',
    actor: 'Actor',
    userId: 'User ID',
    userKeyword: 'Username or display name',
    actorType: 'Actor Type',
    result: 'Result',
    success: 'Success',
    failure: 'Failure',
    resource: 'Resource',
    resourceType: 'Resource Type',
    resourceId: 'Resource ID',
    client: 'Client',
    summary: 'Summary',
    requestId: 'Request ID',
    permission: 'Permission',
    actions: { system_audit_log_list: 'Audit List', system_audit_log_get: 'Audit Details' }
  },
  logCenter: {
    title: 'Log Center',
    detail: 'Log Details',
    noProvider: 'No log source is available. Configure a provider with trusted tenant filtering.',
    timestamp: 'Timestamp',
    level: 'Level',
    source: 'Source',
    service: 'Service',
    component: 'Component',
    keyword: 'Keyword',
    message: 'Message',
    correlation: 'Correlation IDs',
    attributes: 'Attributes'
  },
  page: {
    setting: {
      title: 'Settings',
      empty: 'No settings available',
      required: 'Required',
      reset: 'Reset to defaults',
      resetConfirm: 'Delete tenant overrides and restore code defaults?',
      resetSuccess: 'Settings reset to defaults'
    },
    attachment: {
      demoTitle: 'Attachment Components',
      demoHint: 'Use the picker to upload or select attachments in a business context.',
      dropOrClick: 'Drop files here, or click to select',
      selectFiles: 'Select files',
      uploadHint: 'Max {size} per file. Allowed extensions: {ext}',
      cancel: 'Cancel upload',
      retry: 'Retry',
      empty: 'No attachments',
      preview: 'Preview',
      previewFailed: 'Failed to load preview',
      previewUnavailable: 'Preview unavailable',
      download: 'Download',
      unbind: 'Unbind',
      confirmUnbind: 'Unbind this attachment from the resource?',
      confirmDelete: 'Delete this attachment permanently?',
      uploadSuccess: 'Attachment linked',
      loadFailed: 'Failed to load attachments',
      bindFailed: 'Failed to bind attachment',
      unbindFailed: 'Failed to unbind',
      deleteFailed: 'Failed to delete',
      downloadFailed: 'Failed to download',
      noUploadPermission: 'No upload permission',
      noBindPermission: 'No bind permission',
      openPicker: 'Select attachments',
      pickerTitle: 'Attachment picker',
      pickerMine: 'My uploads',
      pickerUpload: 'Upload new',
      pickerConfirm: 'Confirm',
      searchName: 'Search by file name',
      maxCountReached: 'Selection limit reached',
      loadingMore: 'Loading more…',
      noMore: 'No more items',
      loadMoreCap: 'Loaded many results. Narrow the search or open Attachment Center',
      centerTitle: 'Attachment Center',
      detailTitle: 'Attachment details',
      fileName: 'File name',
      contentType: 'Type',
      size: 'Size',
      statusLabel: 'Status',
      statusAll: 'All statuses',
      uploader: 'Uploader',
      createdAt: 'Created at',
      links: 'Bindings',
      noLinks: 'No bindings',
      status: {
        queued: 'Queued',
        preparing: 'Preparing',
        uploading: 'Uploading',
        completing: 'Completing',
        success: 'Success',
        failed: 'Failed',
        cancelled: 'Cancelled'
      },
      precheck: {
        size: 'File too large',
        ext: 'Extension not allowed',
        mime: 'MIME type not allowed',
        empty: 'Empty file'
      }
    },
    user: {
      title: 'User Management',
      createTitle: 'Create User',
      editTitle: 'Edit User',
      rolesTitle: 'Assign roles for {name}',
      passwordTitle: 'Reset password for {name}',
      username: 'Username',
      usernameInvalid: 'Username must start with a letter and use 3-64 letters, digits or underscores',
      displayName: 'Display Name',
      status: 'Status',
      roles: 'Roles',
      updatedAt: 'Updated At',
      password: 'Password',
      confirmPassword: 'Confirm Password',
      passwordHint: 'Password must be 8+ characters and include letters and digits',
      passwordMismatch: 'The two passwords do not match',
      assignRoles: 'Assign Roles',
      resetPassword: 'Reset Password',
      enable: 'Enable',
      disable: 'Disable',
      confirmEnable: 'Enable this user?',
      confirmDisable: 'Disable this user? Existing tokens will become invalid immediately.',
      enabled: 'Enabled',
      disabled: 'Disabled',
      builtin: 'Built-in'
    },
    role: {
      title: 'Role Management',
      createTitle: 'Create Role',
      editTitle: 'Edit Role',
      permissionTitle: 'Permissions for {name}',
      permissionSearch: 'Search permissions, actions or descriptions',
      selectedCount: '{count} permissions selected',
      code: 'Role Code',
      name: 'Role Name',
      description: 'Description',
      status: 'Status',
      type: 'Type',
      builtin: 'Built-in',
      custom: 'Custom',
      enabled: 'Enabled',
      disabled: 'Disabled',
      userCount: 'Users',
      permissions: 'Permissions',
      updatedAt: 'Updated At',
      reloginRequired:
        'Permissions updated. Your current session is no longer valid because you hold this role. Please sign in again to continue.',
      reloginNow: 'Sign in again'
    },
    login: {
      common: {
        loginOrRegister: 'Login / Register',
        userNamePlaceholder: 'Please enter user name',
        phonePlaceholder: 'Please enter phone number',
        codePlaceholder: 'Please enter verification code',
        passwordPlaceholder: 'Please enter password',
        confirmPasswordPlaceholder: 'Please enter password again',
        codeLogin: 'Verification code login',
        confirm: 'Confirm',
        back: 'Back',
        validateSuccess: 'Verification passed',
        loginSuccess: 'Login successfully',
        welcomeBack: 'Welcome back, {userName} !'
      },
      pwdLogin: {
        title: 'Password Login',
        rememberMe: 'Remember me',
        forgetPassword: 'Forget password?',
        register: 'Register',
        otherAccountLogin: 'Other Account Login',
        otherLoginMode: 'Other Login Mode',
        superAdmin: 'Super Admin',
        admin: 'Admin',
        user: 'User'
      },
      codeLogin: {
        title: 'Verification Code Login',
        getCode: 'Get verification code',
        reGetCode: 'Reacquire after {time}s',
        sendCodeSuccess: 'Verification code sent successfully',
        imageCodePlaceholder: 'Please enter image verification code'
      },
      register: {
        title: 'Register',
        agreement: 'I have read and agree to',
        protocol: '《User Agreement》',
        policy: '《Privacy Policy》'
      },
      resetPwd: {
        title: 'Reset Password'
      },
      bindWeChat: {
        title: 'Bind WeChat'
      }
    },
    home: {
      branchDesc:
        'For the convenience of everyone in developing and updating the merge, we have streamlined the code of the main branch, only retaining the homepage menu, and the rest of the content has been moved to the example branch for maintenance. The preview address displays the content of the example branch.',
      greeting: 'Good morning, {userName}, today is another day full of vitality!',
      weatherDesc: 'Today is cloudy to clear, 20℃ - 25℃!',
      projectCount: 'Project Count',
      todo: 'Todo',
      message: 'Message',
      downloadCount: 'Download Count',
      registerCount: 'Register Count',
      schedule: 'Work and rest Schedule',
      study: 'Study',
      work: 'Work',
      rest: 'Rest',
      entertainment: 'Entertainment',
      visitCount: 'Visit Count',
      turnover: 'Turnover',
      dealCount: 'Deal Count',
      projectNews: {
        title: 'Project News',
        moreNews: 'More News',
        desc1: 'Soybean created the open source project soybean-admin on May 28, 2021!',
        desc2: 'Yanbowe submitted a bug to soybean-admin, the multi-tab bar will not adapt.',
        desc3: 'Soybean is ready to do sufficient preparation for the release of soybean-admin!',
        desc4: 'Soybean is busy writing project documentation for soybean-admin!',
        desc5: 'Soybean just wrote some of the workbench pages casually, and it was enough to see!'
      },
      creativity: 'Creativity'
    },
    setup: {
      title: 'Setup Wizard',
      stepEnv: 'Environment Check',
      stepDatabase: 'Database Config',
      stepAdmin: 'Admin Account',
      stepComplete: 'Complete',
      goVersion: 'Go Version',
      os: 'Operating System',
      dsnSource: 'DSN Source',
      i18nLangs: 'i18n Languages',
      i18nKeys: 'i18n Keys',
      dsnFromEnv: 'DSN has been injected via environment variable, no manual input needed',
      dbHost: 'DB Host',
      dbHostPlaceholder: '127.0.0.1',
      dbPort: 'Port',
      dbPortPlaceholder: '5432',
      dbName: 'Database Name',
      dbNamePlaceholder: 'ainp',
      dbUser: 'DB User',
      dbUserPlaceholder: 'ainp',
      dbPassword: 'DB Password',
      dbPasswordPlaceholder: 'Enter database password',
      dbConnectFail: 'Database connection failed',
      dbConnectOk: 'Database connection successful',
      testConnection: 'Test Connection',
      tenantName: 'Tenant Name',
      tenantNamePlaceholder: 'Please enter tenant name',
      adminUsername: 'Admin Username',
      adminUsernamePlaceholder: 'Please enter admin username',
      adminPassword: 'Admin Password',
      adminPasswordPlaceholder: 'Please enter admin password',
      confirmPassword: 'Confirm Password',
      confirmPasswordPlaceholder: 'Please enter password again',
      passwordMismatch: 'The two passwords are inconsistent',
      passwordTooShort: 'Password must be 8+ characters and include letters and digits',
      initialize: 'Initialize',
      completeTitle: 'Setup Complete',
      completeDesc: 'System has been successfully initialized. Please restart the backend server before logging in.',
      restartRequired:
        'The backend server needs to be restarted to load the new configuration. Please restart and then click the button below to proceed to login.',
      goLogin: 'Go to Login'
    }
  },
  form: {
    required: 'Cannot be empty',
    userName: {
      required: 'Please enter user name',
      invalid: 'User name format is incorrect'
    },
    phone: {
      required: 'Please enter phone number',
      invalid: 'Phone number format is incorrect'
    },
    pwd: {
      required: 'Please enter password',
      invalid: 'Password must be 8+ characters and include letters and digits'
    },
    confirmPwd: {
      required: 'Please enter password again',
      invalid: 'The two passwords are inconsistent'
    },
    code: {
      required: 'Please enter verification code',
      invalid: 'Verification code format is incorrect'
    },
    email: {
      required: 'Please enter email',
      invalid: 'Email format is incorrect'
    }
  },
  dropdown: {
    closeCurrent: 'Close Current',
    closeOther: 'Close Other',
    closeLeft: 'Close Left',
    closeRight: 'Close Right',
    closeAll: 'Close All',
    pin: 'Pin Tab',
    unpin: 'Unpin Tab'
  },
  icon: {
    themeConfig: 'Theme Configuration',
    themeSchema: 'Theme Schema',
    lang: 'Switch Language',
    fullscreen: 'Fullscreen',
    fullscreenExit: 'Exit Fullscreen',
    reload: 'Reload Page',
    collapse: 'Collapse Menu',
    expand: 'Expand Menu',
    pin: 'Pin',
    unpin: 'Unpin'
  },
  datatable: {
    itemCount: 'Total {total} items',
    fixed: {
      left: 'Left Fixed',
      right: 'Right Fixed',
      unFixed: 'Unfixed'
    }
  },
  notification: {
    title: 'Notifications',
    pageTitle: 'My Notifications',
    markRead: 'Mark as Read',
    markAllRead: 'Mark All Read',
    markAllReadSuccess: 'All marked as read',
    viewAll: 'View All',
    empty: 'No notifications',
    archive: 'Archive',
    archiveSuccess: 'Archived',
    unarchive: 'Unarchive',
    unarchiveSuccess: 'Unarchived',
    archived: 'Archived',
    unreadBadge: '{count} unread',
    filterAll: 'All',
    filterUnread: 'Unread',
    filterArchived: 'Include Archived',
    filterPlaceholder: 'Filter'
  },
  aiModel: {
    alias: 'Alias',
    bindAlias: 'Bind Alias',
    enabled: 'Enabled',
    disabled: 'Disabled',
    editTitle: 'Edit Model',
    createTitle: 'Create Model',
    deleteConfirm: 'Delete this model? This action cannot be undone.',
    aliasReferenced: 'Remove all Alias bindings before deleting this model',
    features: {
      streaming: 'Streaming',
      tool_calling: 'Tool Calling',
      parallel_tool_calls: 'Parallel Tool Calls',
      structured_output: 'Structured Output',
      vision: 'Vision',
      embeddings: 'Embeddings',
      image_generation: 'Image Generation',
      audio_input: 'Audio Input',
      audio_output: 'Audio Output'
    }
  },
  aiRuntime: {
    title: 'Model Debugger',
    modelAlias: 'Model Alias',
    prompt: 'Prompt',
    promptPlaceholder: 'Enter a test message',
    generate: 'Generate',
    stream: 'Stream',
    output: 'Output',
    emptyOutput: 'No output',
    requestFailed: 'Model request failed',
    images: 'Image input',
    chooseImages: 'Choose or upload images',
    removeImage: 'Remove image',
    imagesOnly: 'Only ready image attachments are supported',
    visionUnsupported: 'The selected model alias does not support image input',
    streamingUnsupported: 'Streaming is not enabled for the selected model alias',
    detailAuto: 'Auto detail',
    detailLow: 'Low detail',
    detailHigh: 'High detail'
  },
  toolConfirmation: {
    title: 'Sensitive Tool Confirmation',
    tool: 'Tool',
    arguments: 'Argument Summary',
    reject: 'Reject',
    approveOnce: 'Allow Once',
    risk: { low: 'Low Risk', medium: 'Medium Risk', high: 'High Risk', critical: 'Critical Risk' },
    sideEffect: {
      none: 'None',
      read: 'Read',
      write: 'Write',
      external: 'External',
      paid: 'Paid',
      destructive: 'Destructive'
    }
  },
  notificationAdmin: {
    templateTitle: 'Notification Template Management',
    deliveryTitle: 'Delivery Records',
    filterStatus: 'Filter status',
    filterChannel: 'Filter channel',
    tabAll: 'All',
    editTemplate: 'Edit Template Override',
    preview: 'Preview',
    previewTitle: 'Template Preview',
    render: 'Render',
    renderResult: 'Render Result',
    invalidJson: 'Invalid JSON format',
    resetConfirm: 'Reset to code default template?',
    retry: 'Retry',
    retryConfirm: 'Retry this delivery?',
    retrySuccess: 'Re-queued',
    noOverride: 'Default',
    overrideOn: 'Overridden',
    overrideOff: 'Disabled',
    colDefinition: 'Definition',
    colPriority: 'Priority',
    colChannel: 'Channel',
    colLocale: 'Locale',
    colDefaultSubject: 'Default Subject',
    colOverride: 'Override',
    colOverrideUpdated: 'Override Updated',
    colId: 'ID',
    colRecipient: 'Recipient',
    colStatus: 'Status',
    colAttempts: 'Attempts',
    colLastError: 'Last Error',
    colFailedAt: 'Failed At',
    colCreatedAt: 'Created At',
    formDefinitionKey: 'Definition Key',
    formSubject: 'Subject',
    formBody: 'Body',
    formEnabled: 'Enabled',
    formVariables: 'Variables (JSON)',
    formSource: 'Source',
    formTitle: 'InApp Title',
    formSummary: 'Summary',
    priorityLow: 'Low',
    priorityNormal: 'Normal',
    priorityHigh: 'High',
    priorityCritical: 'Critical',
    channelInApp: 'In-App',
    channelEmail: 'Email',
    channelSms: 'SMS',
    channelWebhook: 'Webhook',
    channelIm: 'IM',
    statusPending: 'Pending',
    statusQueued: 'Queued',
    statusSent: 'Sent',
    statusDelivered: 'Delivered',
    statusFailed: 'Failed',
    statusDead: 'Dead Letter',
    categoryUser: 'User',
    categorySecurity: 'Security',
    categorySystem: 'System',
    detailTitle: 'Delivery Detail',
    detailInfo: 'Basic Info',
    detailContent: 'Message Content',
    detailError: 'Error Info',
    detailDefinitionKey: 'Definition Key',
    detailLocale: 'Locale',
    detailProvider: 'Provider',
    detailExternalId: 'External ID',
    detailSentAt: 'Sent At',
    detailDeliveredAt: 'Delivered At',
    detailScheduledAt: 'Scheduled At',
    noContent: 'No content preview available'
  }
};

export default local;
