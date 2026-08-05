/** The global namespace for the app */
declare namespace App {
  /** Theme namespace */
  namespace Theme {
    type ColorPaletteNumber = import('@sa/color').ColorPaletteNumber;

    /** NaiveUI theme overrides that can be specified in preset */
    type NaiveUIThemeOverride = import('naive-ui').GlobalThemeOverrides;

    /** Theme setting */
    interface ThemeSetting {
      /** Theme scheme */
      themeScheme: UnionKey.ThemeScheme;
      /** grayscale mode */
      grayscale: boolean;
      /** colour weakness mode */
      colourWeakness: boolean;
      /** Whether to recommend color */
      recommendColor: boolean;
      /** Theme color */
      themeColor: string;
      /** Theme radius */
      themeRadius: number;
      /** Other color */
      otherColor: OtherColor;
      /** Whether info color is followed by the primary color */
      isInfoFollowPrimary: boolean;
      /** Layout */
      layout: {
        /** Layout mode */
        mode: UnionKey.ThemeLayoutMode;
        /** Scroll mode */
        scrollMode: UnionKey.ThemeScrollMode;
      };
      /** Page */
      page: {
        /** Whether to show the page transition */
        animate: boolean;
        /** Page animate mode */
        animateMode: UnionKey.ThemePageAnimateMode;
      };
      /** Header */
      header: {
        /** Header height */
        height: number;
        /** Header breadcrumb */
        breadcrumb: {
          /** Whether to show the breadcrumb */
          visible: boolean;
          /** Whether to show the breadcrumb icon */
          showIcon: boolean;
        };
        /** Multilingual */
        multilingual: {
          /** Whether to show the multilingual */
          visible: boolean;
        };
        globalSearch: {
          /** Whether to show the GlobalSearch */
          visible: boolean;
        };
      };
      /** Tab */
      tab: {
        /** Whether to show the tab */
        visible: boolean;
        /**
         * Whether to cache the tab
         *
         * If cache, the tabs will get from the local storage when the page is refreshed
         */
        cache: boolean;
        /** Tab height */
        height: number;
        /** Tab mode */
        mode: UnionKey.ThemeTabMode;
        /** Whether to close tab by middle click */
        closeTabByMiddleClick: boolean;
      };
      /** Fixed header and tab */
      fixedHeaderAndTab: boolean;
      /** Sider */
      sider: {
        /** Inverted sider */
        inverted: boolean;
        /** Sider width */
        width: number;
        /** Collapsed sider width */
        collapsedWidth: number;
        /** Sider width when the layout is 'vertical-mix', 'top-hybrid-sidebar-first', or 'top-hybrid-header-first' */
        mixWidth: number;
        /**
         * Collapsed sider width when the layout is 'vertical-mix', 'top-hybrid-sidebar-first', or
         * 'top-hybrid-header-first'
         */
        mixCollapsedWidth: number;
        /** Child menu width when the layout is 'vertical-mix', 'top-hybrid-sidebar-first', or 'top-hybrid-header-first' */
        mixChildMenuWidth: number;
        /** Whether to auto select the first submenu */
        autoSelectFirstMenu: boolean;
      };
      /** Footer */
      footer: {
        /** Whether to show the footer */
        visible: boolean;
        /** Whether fixed the footer */
        fixed: boolean;
        /** Footer height */
        height: number;
        /**
         * Whether float the footer to the right when the layout is 'top-hybrid-sidebar-first' or
         * 'top-hybrid-header-first'
         */
        right: boolean;
      };
      /** Watermark */
      watermark: {
        /** Whether to show the watermark */
        visible: boolean;
        /** Watermark text */
        text: string;
        /** Whether to use user name as watermark text */
        enableUserName: boolean;
        /** Whether to use current time as watermark text */
        enableTime: boolean;
        /** Time format for watermark text */
        timeFormat: string;
      };
      /** define some theme settings tokens, will transform to css variables */
      tokens: {
        light: ThemeSettingToken;
        dark?: {
          [K in keyof ThemeSettingToken]?: Partial<ThemeSettingToken[K]>;
        };
      };
    }

    interface OtherColor {
      info: string;
      success: string;
      warning: string;
      error: string;
    }

    interface ThemeColor extends OtherColor {
      primary: string;
    }

    type ThemeColorKey = keyof ThemeColor;

    type ThemePaletteColor = {
      [key in ThemeColorKey | `${ThemeColorKey}-${ColorPaletteNumber}`]: string;
    };

    type BaseToken = Record<string, Record<string, string>>;

    interface ThemeSettingTokenColor {
      /** the progress bar color, if not set, will use the primary color */
      nprogress?: string;
      container: string;
      layout: string;
      inverted: string;
      'base-text': string;
    }

    interface ThemeSettingTokenBoxShadow {
      header: string;
      sider: string;
      tab: string;
    }

    interface ThemeSettingToken {
      colors: ThemeSettingTokenColor;
      boxShadow: ThemeSettingTokenBoxShadow;
    }

    type ThemeTokenColor = ThemePaletteColor & ThemeSettingTokenColor;

    /** Theme token CSS variables */
    type ThemeTokenCSSVars = {
      colors: ThemeTokenColor & { [key: string]: string };
      boxShadow: ThemeSettingTokenBoxShadow & { [key: string]: string };
    };
  }

  /** Global namespace */
  namespace Global {
    type VNode = import('vue').VNode;
    type RouteLocationNormalizedLoaded = import('vue-router').RouteLocationNormalizedLoaded;
    type RouteKey = import('@elegant-router/types').RouteKey;
    type RouteMap = import('@elegant-router/types').RouteMap;
    type RoutePath = import('@elegant-router/types').RoutePath;
    type LastLevelRouteKey = import('@elegant-router/types').LastLevelRouteKey;

    /** The router push options */
    type RouterPushOptions = {
      query?: Record<string, string>;
      params?: Record<string, string>;
      force?: boolean;
    };

    /** The global header props */
    interface HeaderProps {
      /** Whether to show the logo */
      showLogo?: boolean;
      /** Whether to show the menu toggler */
      showMenuToggler?: boolean;
      /** Whether to show the menu */
      showMenu?: boolean;
    }

    /** The global menu */
    type Menu = {
      /**
       * The menu key
       *
       * Equal to the route key
       */
      key: string;
      /** The menu label */
      label: string;
      /** The menu i18n key */
      i18nKey?: I18n.I18nKey | null;
      /** The route key */
      routeKey: RouteKey;
      /** The route path */
      routePath: RoutePath;
      /** The menu icon */
      icon?: () => VNode;
      /** The menu children */
      children?: Menu[];
    };

    type Breadcrumb = Omit<Menu, 'children'> & {
      options?: Breadcrumb[];
    };

    /** Tab route */
    type TabRoute = Pick<RouteLocationNormalizedLoaded, 'name' | 'path' | 'meta'> &
      Partial<Pick<RouteLocationNormalizedLoaded, 'fullPath' | 'query' | 'matched'>>;

    /** The global tab */
    type Tab = {
      /** The tab id */
      id: string;
      /** The tab label */
      label: string;
      /**
       * The new tab label
       *
       * If set, the tab label will be replaced by this value
       */
      newLabel?: string;
      /**
       * The old tab label
       *
       * when reset the tab label, the tab label will be replaced by this value
       */
      oldLabel?: string;
      /** The tab route key */
      routeKey: LastLevelRouteKey;
      /** The tab route path */
      routePath: RouteMap[LastLevelRouteKey];
      /** The tab route full path */
      fullPath: string;
      /** The tab fixed index */
      fixedIndex?: number | null;
      /**
       * Tab icon
       *
       * Iconify icon
       */
      icon?: string;
      /**
       * Tab local icon
       *
       * Local icon
       */
      localIcon?: string;
      /** I18n key */
      i18nKey?: I18n.I18nKey | null;
    };

    /** Form rule */
    type FormRule = import('naive-ui').FormItemRule;

    /** The global dropdown key */
    type DropdownKey = 'closeCurrent' | 'closeOther' | 'closeLeft' | 'closeRight' | 'closeAll' | 'pin' | 'unpin';
  }

  /**
   * I18n namespace
   *
   * Locales type
   */
  namespace I18n {
    type RouteKey = import('@elegant-router/types').RouteKey;

    type LangType = 'en-US' | 'zh-CN';

    type LangOption = {
      label: string;
      key: LangType;
    };

    type I18nRouteKey = Exclude<RouteKey, 'root' | 'not-found'>;

    type FormMsg = {
      required: string;
      invalid: string;
    };

    type Schema = {
      system: {
        title: string;
        updateTitle: string;
        updateContent: string;
        updateConfirm: string;
        updateCancel: string;
      };
      common: {
        action: string;
        add: string;
        addSuccess: string;
        backToHome: string;
        back: string;
        batchDelete: string;
        cancel: string;
        close: string;
        check: string;
        selectAll: string;
        expandColumn: string;
        columnSetting: string;
        config: string;
        confirm: string;
        delete: string;
        deleteSuccess: string;
        confirmDelete: string;
        detail: string;
        edit: string;
        warning: string;
        error: string;
        index: string;
        keywordSearch: string;
        logout: string;
        logoutConfirm: string;
        lookForward: string;
        modify: string;
        modifySuccess: string;
        noData: string;
        next: string;
        operate: string;
        pleaseCheckValue: string;
        refresh: string;
        reset: string;
        search: string;
        switch: string;
        tip: string;
        trigger: string;
        update: string;
        updateSuccess: string;
        userCenter: string;
        yesOrNo: {
          yes: string;
          no: string;
        };
      };
      request: {
        logout: string;
        logoutMsg: string;
        logoutWithModal: string;
        logoutWithModalMsg: string;
        refreshToken: string;
        tokenExpired: string;
      };
      concurrency: {
        keyRequired: string;
        conflict: string;
        inProgress: string;
        revisionConflict: string;
        resourceLocked: string;
      };
      theme: {
        themeDrawerTitle: string;
        tabs: {
          appearance: string;
          layout: string;
          general: string;
          preset: string;
        };
        appearance: {
          themeSchema: { title: string } & Record<UnionKey.ThemeScheme, string>;
          grayscale: string;
          colourWeakness: string;
          themeColor: {
            title: string;
            followPrimary: string;
          } & Record<Theme.ThemeColorKey, string>;
          recommendColor: string;
          recommendColorDesc: string;
          themeRadius: {
            title: string;
          };
          preset: {
            title: string;
            apply: string;
            applySuccess: string;
            [key: string]:
              | {
                  name: string;
                  desc: string;
                }
              | string;
          };
        };
        layout: {
          layoutMode: { title: string } & Record<UnionKey.ThemeLayoutMode, string> & {
              [K in `${UnionKey.ThemeLayoutMode}_detail`]: string;
            };
          tab: {
            title: string;
            visible: string;
            cache: string;
            cacheTip: string;
            height: string;
            mode: { title: string } & Record<UnionKey.ThemeTabMode, string>;
            closeByMiddleClick: string;
            closeByMiddleClickTip: string;
          };
          header: {
            title: string;
            height: string;
            breadcrumb: {
              visible: string;
              showIcon: string;
            };
          };
          sider: {
            title: string;
            inverted: string;
            width: string;
            collapsedWidth: string;
            mixWidth: string;
            mixCollapsedWidth: string;
            mixChildMenuWidth: string;
            autoSelectFirstMenu: string;
            autoSelectFirstMenuTip: string;
          };
          footer: {
            title: string;
            visible: string;
            fixed: string;
            height: string;
            right: string;
          };
          content: {
            title: string;
            scrollMode: { title: string; tip: string } & Record<UnionKey.ThemeScrollMode, string>;
            page: {
              animate: string;
              mode: { title: string } & Record<UnionKey.ThemePageAnimateMode, string>;
            };
            fixedHeaderAndTab: string;
          };
        };
        general: {
          title: string;
          watermark: {
            title: string;
            visible: string;
            text: string;
            enableUserName: string;
            enableTime: string;
            timeFormat: string;
          };
          multilingual: {
            title: string;
            visible: string;
          };
          globalSearch: {
            title: string;
            visible: string;
          };
        };
        configOperation: {
          copyConfig: string;
          copySuccessMsg: string;
          resetConfig: string;
          resetSuccessMsg: string;
        };
      };
      menu: {
        ai: {
          root: { title: string };
          providers: { title: string };
          models: { title: string };
          agents: { title: string };
          agent_slots: { title: string };
          knowledge: { title: string };
          workspace: { title: string };
          agent_builder: { title: string };
        };
      };
      route: Record<I18nRouteKey, string>;
      page: {
        knowledge: {
          title: string;
          subtitle: string;
          createBase: string;
          editBase: string;
          search: string;
          allStatus: string;
          selectBase: string;
          name: string;
          code: string;
          status: string;
          updatedAt: string;
          description: string;
          visibility: string;
          private: string;
          tenant: string;
          embeddingAlias: string;
          embeddingAliasPlaceholder: string;
          embeddingAliasLoadFailed: string;
          currentAliasUnavailable: string;
          chunking: string;
          chunkSize: string;
          chunkOverlap: string;
          topK: string;
          minScore: string;
          enable: string;
          disable: string;
          documents: string;
          addDocument: string;
          documentName: string;
          contentType: string;
          ingestionStatus: string;
          retry: string;
          reindex: string;
          retryRequested: string;
          retrievalTest: string;
          retrievalRequiresEnabled: string;
          retrievalIndexNotReady: string;
          queryPlaceholder: string;
          run: string;
          noResults: string;
          previewCitation: string;
          noHeading: string;
          citationAvailable: string;
          citationStale: string;
          noEmbedding: string;
          noEmbeddingHint: string;
          loadFailed: string;
          loadDocumentsFailed: string;
          permissionRevoked: string;
          saveFailed: string;
          statusFailed: string;
          deleteFailed: string;
          addDocumentFailed: string;
          retryFailed: string;
          retrievalFailed: string;
          citationFailed: string;
          confirmDeleteBase: string;
          confirmDeleteDocument: string;
          validation: { name: string };
        };
        aiAgents: {
          title: string;
          create: string;
          openBuilder: string;
          duplicate: string;
          reconcile: string;
          keyword: string;
          managed: string;
          enable: string;
          disable: string;
          publishFirstVersion: string;
          resultCount: string;
          avatarLabel: string;
          openAgent: string;
          moreActions: string;
          noDescription: string;
          hasDraft: string;
          builtinManaged: string;
          unpublished: string;
          updatedAt: string;
          empty: string;
          emptyFiltered: string;
          clearFilters: string;
          loadFailed: string;
          loadFailedHint: string;
          statusConfirmTitle: string;
          statusConfirm: string;
          sourceAll: string;
          statusAll: string;
          colName: string;
          colCode: string;
          colSource: string;
          colVisibility: string;
          colStatus: string;
          colVersion: string;
          source: Record<'builtin' | 'tenant' | 'personal', string>;
          visibility: Record<'private' | 'tenant', string>;
          status: Record<'draft' | 'enabled' | 'disabled', string>;
        };
        aiWorkspace: {
          title: string;
          assistant: string;
          newConversation: string;
          searchConversations: string;
          recent: string;
          archived: string;
          emptyRecent: string;
          emptyArchived: string;
          restore: string;
          rename: string;
          archive: string;
          actions: string;
          openConversations: string;
          collapseConversations: string;
          runDetails: string;
          memory: string;
          newChat: string;
          greeting: string;
          ready: string;
          readyShort: string;
          expandWorkspace: string;
          unavailable: string;
          suggestionPriority: string;
          suggestionExceptions: string;
          suggestionFeature: string;
          suggestionNext: string;
          reconnecting: string;
          offline: string;
          disclaimer: string;
          noRun: string;
          status: string;
          steps: string;
          modelCalls: string;
          toolCalls: string;
          renameTitle: string;
          save: string;
          addImage: string;
          removeImage: string;
          imageUploading: string;
          retryImageUpload: string;
          imageLimit: string;
          clipboardImageUnsupported: string;
          placeholder: string;
          stop: string;
          send: string;
          slotUnavailable: string;
          slotResolveFailed: string;
          createFailed: string;
          sendFailed: string;
          runFailed: string;
          unavailableGeneric: string;
          reconnectFallback: string;
          diagnosticContextWindow: string;
          runFailure: string;
          runFailureSimple: string;
        };
        agentBuilder: {
          back: string;
          createTitle: string;
          editTitle: string;
          dirty: string;
          clean: string;
          draftSavedHint: string;
          showLeft: string;
          hideLeft: string;
          showRight: string;
          hideRight: string;
          versions: string;
          versionLabel: string;
          noVersions: string;
          saveMeta: string;
          createPublish: string;
          publishVersion: string;
          panePrompt: string;
          paneConfig: string;
          panePreview: string;
          groupBasic: string;
          editAvatar: string;
          avatarUpdated: string;
          avatarImageOnly: string;
          avatarUpdateFailed: string;
          tenantPlaceholder: string;
          builtinBaseTitle: string;
          systemManaged: string;
          builtinBaseHint: string;
          supplementTitle: string;
          supplementHint: string;
          supplementPlaceholder: string;
          customInstructionsTitle: string;
          customInstructionsHint: string;
          restoreBuiltin: string;
          restoreBuiltinTitle: string;
          restoreBuiltinContent: string;
          restoreBuiltinConfirm: string;
          instructionTooLong: string;
          name: string;
          code: string;
          description: string;
          source: string;
          sourcePersonal: string;
          sourceTenant: string;
          visibility: string;
          visibilityPrivate: string;
          visibilityTenant: string;
          modelAlias: string;
          modelAliasPlaceholder: string;
          generationParams: string;
          temperature: string;
          topP: string;
          maxOutputTokens: string;
          memoryPolicy: string;
          memoryInherit: string;
          memoryInheritedHint: string;
          memoryEnabled: string;
          memoryDisabled: string;
          memoryModel: string;
          memoryConsolidationModel: string;
          memoryConsolidationFallback: string;
          memoryAutoSave: string;
          memoryAgentScope: string;
          memoryApproval: string;
          memoryApprovalAlways: string;
          memoryApprovalSensitive: string;
          memoryApprovalNever: string;
          tools: string;
          openToolPicker: string;
          toolPickerTitle: string;
          toolPickerHint: string;
          toolSearchPlaceholder: string;
          boundTools: string;
          noBoundTools: string;
          noToolCandidates: string;
          requiredTool: string;
          requiredToolReadonly: string;
          conditionalLoading: string;
          toolDiscovery: string;
          bind: string;
          bound: string;
          unbind: string;
          toolsAfterCreate: string;
          knowledge: string;
          openKnowledgePicker: string;
          knowledgePickerTitle: string;
          knowledgePickerHint: string;
          knowledgeSearchPlaceholder: string;
          boundKnowledge: string;
          noBoundKnowledge: string;
          noKnowledgeCandidates: string;
          noKnowledgeDescription: string;
          indexReady: string;
          knowledgeAfterCreate: string;
          diagnostics: string;
          diagnosticsHealthy: string;
          diagnosticsUnhealthy: string;
          diagnosticsAfterCreate: string;
          previewLiveTag: string;
          previewIdleSummary: string;
          previewPlaceholder: string;
          runPreview: string;
          stopPreview: string;
          clearPreview: string;
          chooseImages: string;
          removeImage: string;
          previewImageLimits: string;
          previewVisionUnsupported: string;
          toolCall: string;
          unknownTool: string;
          toolCallId: string;
          toolRequest: string;
          noToolRequest: string;
          toolResponse: string;
          toolResponseReceived: string;
          waitingToolResponse: string;
          previewBlocked: string;
          previewNeedInput: string;
          previewNeedAgent: string;
          previewAgentDisabled: string;
          previewSubscribeFailed: string;
          previewConversationTitle: string;
          previewConversationFailed: string;
          previewMaterializeFailed: string;
          previewSendFailed: string;
          previewRunFailed: string;
          previewFailed: string;
          leaveTitle: string;
          leaveContent: string;
          leaveConfirm: string;
          loadFailed: string;
          metaSaved: string;
          metaAndPromptSaved: string;
          createSuccess: string;
          publishSuccess: string;
          publishFailed: string;
          revisionConflict: string;
        };
        aiRun: {
          progress: string;
          cancel: string;
          resume: string;
          retry: string;
          status: Record<Api.AI.AgentRunStatus, string>;
        };
        setting: {
          title: string;
          empty: string;
          required: string;
          reset: string;
          resetConfirm: string;
          resetSuccess: string;
          navigationIndex: {
            title: string;
            summary: string;
            rebuild: string;
            rebuildQueued: string;
            states: Record<Api.Setting.NavigationIndexState, string>;
          };
          toolIndex: {
            title: string;
            summary: string;
            rebuild: string;
            rebuildQueued: string;
            states: Record<Api.Setting.NavigationIndexState, string>;
          };
        };
        attachment: {
          demoTitle: string;
          demoHint: string;
          dropOrClick: string;
          selectFiles: string;
          uploadHint: string;
          cancel: string;
          retry: string;
          empty: string;
          preview: string;
          previewFailed: string;
          previewUnavailable: string;
          download: string;
          unbind: string;
          confirmUnbind: string;
          confirmDelete: string;
          uploadSuccess: string;
          loadFailed: string;
          bindFailed: string;
          unbindFailed: string;
          deleteFailed: string;
          downloadFailed: string;
          noUploadPermission: string;
          noBindPermission: string;
          openPicker: string;
          pickerTitle: string;
          pickerMine: string;
          pickerUpload: string;
          pickerConfirm: string;
          pickerAssociate: string;
          alreadyAssociated: string;
          searchName: string;
          maxCountReached: string;
          loadingMore: string;
          noMore: string;
          loadMoreCap: string;
          centerTitle: string;
          detailTitle: string;
          fileName: string;
          contentType: string;
          size: string;
          statusLabel: string;
          statusAll: string;
          uploader: string;
          createdAt: string;
          links: string;
          noLinks: string;
          status: {
            queued: string;
            preparing: string;
            uploading: string;
            completing: string;
            success: string;
            failed: string;
            cancelled: string;
          };
          precheck: {
            size: string;
            ext: string;
            mime: string;
            empty: string;
          };
        };
        account: {
          title: string;
          createTitle: string;
          editTitle: string;
          rolesTitle: string;
          passwordTitle: string;
          username: string;
          usernameInvalid: string;
          displayName: string;
          status: string;
          roles: string;
          updatedAt: string;
          password: string;
          confirmPassword: string;
          passwordHint: string;
          passwordMismatch: string;
          assignRoles: string;
          resetPassword: string;
          enable: string;
          disable: string;
          confirmEnable: string;
          confirmDisable: string;
          enabled: string;
          disabled: string;
          builtin: string;
        };
        role: {
          title: string;
          createTitle: string;
          editTitle: string;
          permissionTitle: string;
          permissionSearch: string;
          selectedCount: string;
          superAdmin: string;
          superAdminDescription: string;
          allPermissions: string;
          code: string;
          name: string;
          description: string;
          status: string;
          type: string;
          builtin: string;
          custom: string;
          enabled: string;
          disabled: string;
          userCount: string;
          permissions: string;
          updatedAt: string;
          reloginRequired: string;
          reloginNow: string;
        };
        login: {
          visual: {
            tagline: string;
            intelligentCollaboration: string;
            trustedExecution: string;
            flexibleExtension: string;
          };
          common: {
            loginOrRegister: string;
            subtitle: string;
            userNamePlaceholder: string;
            phonePlaceholder: string;
            codePlaceholder: string;
            passwordPlaceholder: string;
            confirmPasswordPlaceholder: string;
            codeLogin: string;
            confirm: string;
            back: string;
            validateSuccess: string;
            loginSuccess: string;
            welcomeBack: string;
          };
          pwdLogin: {
            title: string;
            rememberMe: string;
            forgetPassword: string;
            register: string;
            otherAccountLogin: string;
            otherLoginMode: string;
            superAdmin: string;
            admin: string;
            user: string;
          };
          codeLogin: {
            title: string;
            getCode: string;
            reGetCode: string;
            sendCodeSuccess: string;
            imageCodePlaceholder: string;
          };
          register: {
            title: string;
            agreement: string;
            protocol: string;
            policy: string;
          };
          resetPwd: {
            title: string;
          };
          bindWeChat: {
            title: string;
          };
        };
        home: {
          greeting: string;
          periodLateNight: string;
          periodMorning: string;
          periodAfternoon: string;
          periodEvening: string;
          defaultUser: string;
          workspaceLabel: string;
          welcomeSubtitle: string;
          startWork: string;
          continueTitle: string;
          continueSubtitle: string;
          confirmationTitle: string;
          openPage: string;
          opened: string;
          riskHigh: string;
          riskMedium: string;
          riskLow: string;
          modulesTitle: string;
          modulesSubtitle: string;
          noModules: string;
        };
        setup: {
          title: string;
          stepEnv: string;
          stepDatabase: string;
          stepRedisJobs: string;
          stepAdmin: string;
          stepComplete: string;
          goVersion: string;
          os: string;
          dsnSource: string;
          i18nLangs: string;
          i18nKeys: string;
          dsnFromEnv: string;
          dbHost: string;
          dbHostPlaceholder: string;
          dbPort: string;
          dbPortPlaceholder: string;
          dbName: string;
          dbNamePlaceholder: string;
          dbUser: string;
          dbUserPlaceholder: string;
          dbPassword: string;
          dbPasswordPlaceholder: string;
          dbConnectFail: string;
          dbConnectOk: string;
          testConnection: string;
          enableJobs: string;
          redisAddr: string;
          redisDb: string;
          embeddedWorker: string;
          redisPasswordEnv: string;
          testRedis: string;
          redisConnectOk: string;
          redisConnectFail: string;
          redisStatus: string;
          redisReachable: string;
          redisUnavailable: string;
          tenantName: string;
          tenantNamePlaceholder: string;
          adminUsername: string;
          adminUsernamePlaceholder: string;
          adminPassword: string;
          adminPasswordPlaceholder: string;
          confirmPassword: string;
          confirmPasswordPlaceholder: string;
          passwordMismatch: string;
          passwordTooShort: string;
          initialize: string;
          completeTitle: string;
          completeDesc: string;
          restartRequired: string;
          jobsWorkerHint: string;
          jobsEmbeddedHint: string;
          jobsDisabledHint: string;
          goLogin: string;
        };
      };
      form: {
        required: string;
        userName: FormMsg;
        phone: FormMsg;
        pwd: FormMsg;
        confirmPwd: FormMsg;
        code: FormMsg;
        email: FormMsg;
      };
      dropdown: Record<Global.DropdownKey, string>;
      icon: {
        themeConfig: string;
        themeSchema: string;
        lang: string;
        fullscreen: string;
        fullscreenExit: string;
        reload: string;
        collapse: string;
        expand: string;
        pin: string;
        unpin: string;
      };
      notification: {
        title: string;
        pageTitle: string;
        markRead: string;
        markAllRead: string;
        markAllReadSuccess: string;
        viewAll: string;
        empty: string;
        archive: string;
        archiveSuccess: string;
        unarchive: string;
        unarchiveSuccess: string;
        archived: string;
        unreadBadge: string;
        filterAll: string;
        filterUnread: string;
        filterArchived: string;
        filterPlaceholder: string;
      };
      operations: {
        jobsTitle: string;
        outboxTitle: string;
        jobDetail: string;
        outboxDetail: string;
        jobType: string;
        eventType: string;
        aggregate: string;
        statusLabel: string;
        progress: string;
        attempts: string;
        createdAt: string;
        updatedAt: string;
        occurredAt: string;
        cancelJob: string;
        retry: string;
        confirmAction: string;
        success: string;
        error: string;
        payload: string;
        handler: string;
        status: {
          pending_enqueue: string;
          queued: string;
          running: string;
          retrying: string;
          succeeded: string;
          failed: string;
          dead: string;
          cancelled: string;
          pending: string;
          processing: string;
          published: string;
        };
      };
      auditLog: {
        title: string;
        detail: string;
        occurredAt: string;
        actor: string;
        userId: string;
        userKeyword: string;
        actorType: string;
        result: string;
        success: string;
        failure: string;
        resource: string;
        resourceType: string;
        resourceId: string;
        client: string;
        summary: string;
        requestId: string;
        permission: string;
        actions: { system_audit_log_list: string; system_audit_log_get: string };
      };
      logCenter: {
        title: string;
        detail: string;
        noProvider: string;
        timestamp: string;
        level: string;
        source: string;
        service: string;
        component: string;
        keyword: string;
        message: string;
        correlation: string;
        attributes: string;
      };
      aiModel: {
        alias: string;
        bindAlias: string;
        enabled: string;
        disabled: string;
        editTitle: string;
        createTitle: string;
        deleteConfirm: string;
        aliasReferenced: string;
        inputModalities: string;
        outputModalities: string;
        capabilities: string;
        visionNeedsImage: string;
        modalities: Record<(typeof import('@/constants/ai').MODEL_MODALITIES)[number], string>;
        features: Record<(typeof import('@/constants/ai').MODEL_FEATURES)[number], string>;
        structuredOutputStrategies: Record<
          (typeof import('@/constants/ai').STRUCTURED_OUTPUT_STRATEGIES)[number],
          string
        >;
      };
      chat: {
        thinking: string;
        thinkingThinking: string;
        thinkingContext: string;
        thinkingToolSearch: string;
        thinkingToolLoad: string;
        thinkingToolExecute: string;
        thinkingFinalize: string;
        thinkingSlow: string;
        imageAttachment: string;
        toolProcessed: string;
        toolCall: string;
        toolRunning: string;
        toolSuccess: string;
        toolFailed: string;
        toolRehydrated: string;
        toolWaiting: string;
        toolArguments: string;
        toolResult: string;
        toolTechnicalDetails: string;
        toolIdentifier: string;
        toolCallIdentifier: string;
        navigationSearching: string;
        navigationProcessed: string;
        navigationEmpty: string;
        navigationOpen: string;
        navigationUnavailable: string;
        navigationSearchError: string;
        navigationOffline: string;
        navigationDegraded: string;
      };
      toolConfirmation: {
        title: string;
        tool: string;
        arguments: string;
        callId: string;
        technicalDetails: string;
        fallbackSummary: string;
        reject: string;
        approveOnce: string;
        risk: Record<'low' | 'medium' | 'high' | 'critical', string>;
        sideEffect: Record<'none' | 'read' | 'write' | 'external' | 'paid' | 'destructive', string>;
      };
      notificationAdmin: {
        templateTitle: string;
        deliveryTitle: string;
        filterStatus: string;
        filterChannel: string;
        tabAll: string;
        editTemplate: string;
        preview: string;
        previewTitle: string;
        render: string;
        renderResult: string;
        invalidJson: string;
        resetConfirm: string;
        retry: string;
        retryConfirm: string;
        retrySuccess: string;
        noOverride: string;
        overrideOn: string;
        overrideOff: string;
        colDefinition: string;
        colPriority: string;
        colChannel: string;
        colLocale: string;
        colDefaultSubject: string;
        colOverride: string;
        colOverrideUpdated: string;
        colId: string;
        colRecipient: string;
        colStatus: string;
        colAttempts: string;
        colLastError: string;
        colFailedAt: string;
        colCreatedAt: string;
        formDefinitionKey: string;
        formSubject: string;
        formBody: string;
        formEnabled: string;
        formVariables: string;
        formSource: string;
        formTitle: string;
        formSummary: string;
        priorityLow: string;
        priorityNormal: string;
        priorityHigh: string;
        priorityCritical: string;
        channelInApp: string;
        channelEmail: string;
        channelSms: string;
        channelWebhook: string;
        channelIm: string;
        statusPending: string;
        statusQueued: string;
        statusSent: string;
        statusDelivered: string;
        statusFailed: string;
        statusDead: string;
        categoryUser: string;
        categorySecurity: string;
        categorySystem: string;
        detailTitle: string;
        detailInfo: string;
        detailContent: string;
        detailError: string;
        detailDefinitionKey: string;
        detailLocale: string;
        detailProvider: string;
        detailExternalId: string;
        detailSentAt: string;
        detailDeliveredAt: string;
        detailScheduledAt: string;
        noContent: string;
      };
      datatable: {
        itemCount: string;
        fixed: {
          left: string;
          right: string;
          unFixed: string;
        };
      };
    };

    type GetI18nKey<T extends Record<string, unknown>, K extends keyof T = keyof T> = K extends string
      ? T[K] extends Record<string, unknown>
        ? `${K}.${GetI18nKey<T[K]>}`
        : K
      : never;

    type I18nKey = GetI18nKey<Schema>;

    type TranslateOptions<Locales extends string> = import('vue-i18n').TranslateOptions<Locales>;

    interface $T {
      (key: I18nKey): string;
      (key: I18nKey, plural: number, options?: TranslateOptions<LangType>): string;
      (key: I18nKey, defaultMsg: string, options?: TranslateOptions<I18nKey>): string;
      (key: I18nKey, list: unknown[], options?: TranslateOptions<I18nKey>): string;
      (key: I18nKey, list: unknown[], plural: number): string;
      (key: I18nKey, list: unknown[], defaultMsg: string): string;
      (key: I18nKey, named: Record<string, unknown>, options?: TranslateOptions<LangType>): string;
      (key: I18nKey, named: Record<string, unknown>, plural: number): string;
      (key: I18nKey, named: Record<string, unknown>, defaultMsg: string): string;
    }
  }

  /** Service namespace */
  namespace Service {
    /** Other baseURL key */
    type OtherBaseURLKey = 'demo';

    interface ServiceConfigItem {
      /** The backend service base url */
      baseURL: string;
      /** The proxy pattern of the backend service base url */
      proxyPattern: string;
    }

    interface OtherServiceConfigItem extends ServiceConfigItem {
      key: OtherBaseURLKey;
    }

    /** The backend service config */
    interface ServiceConfig extends ServiceConfigItem {
      /** Other backend service config */
      other: OtherServiceConfigItem[];
    }

    interface SimpleServiceConfig extends Pick<ServiceConfigItem, 'baseURL'> {
      other: Record<OtherBaseURLKey, string>;
    }

    /** The backend service response data */
    type Response<T = unknown> = {
      /** The backend service response code */
      code: string;
      /** The backend service response message */
      msg: string;
      /** The backend service response data */
      data: T;
    };

    /** The demo backend service response data */
    type DemoResponse<T = unknown> = {
      /** The backend service response code */
      status: string;
      /** The backend service response message */
      message: string;
      /** The backend service response data */
      result: T;
    };
  }
}
