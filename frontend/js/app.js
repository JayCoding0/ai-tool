// ===== 应用入口（精简版） =====
// 模板、聊天逻辑、主题、工具函数已拆分到独立模块

const { createApp, ref, computed, nextTick, onMounted, watch } = Vue;
const { ElMessage, ElMessageBox } = ElementPlus;

const app = createApp({
    template: APP_TEMPLATE,
    setup() {
        // ===== 基础状态 =====
        const loading = ref(false);
        const messages = ref([]);
        const inputText = ref('');
        const sending = ref(false);
        const messagesEl = ref(null);
        const inputEl = ref(null);

        // 状态栏
        const statusText = ref('连接中...');
        const statusColor = ref('#9ca3af');

        // 模型
        const allModels = ref([]);
        const selectedModel = ref(localStorage.getItem('selectedModel') || '');
        const cloudModels = computed(() => allModels.value.filter(m => m.type !== 'local'));
        const localModels = computed(() => allModels.value.filter(m => m.type === 'local'));

        // 用户
        const username = ref(localStorage.getItem('username') || '');
        const userRole = ref(localStorage.getItem('user_role') || 'guest');
        const isAdmin = computed(() => userRole.value === 'admin');
        const isLoggedIn = computed(() => !!username.value);
        const userAvatar = computed(() => username.value ? username.value.charAt(0).toUpperCase() : '?');
        const userTotalTokens = ref(parseInt(localStorage.getItem('user_total_tokens') || '0', 10));

        // 对话轮次
        const roundCount = ref(0);

        // 欢迎页建议
        const suggestions = [
            { icon: '✍️', text: '帮我写一封工作邮件' },
            { icon: '💡', text: '给我一些学习编程的建议' },
            { icon: '🔍', text: '解释一下量子计算是什么' },
            { icon: '🎯', text: '帮我制定一个健身计划' },
        ];

        function useSuggestion(text) {
            inputText.value = text;
            chatModule.sendMessage();
        }

        // System Prompt
        const systemPromptVisible = ref(false);
        const systemPromptInput = ref('');
        const systemPromptSaving = ref(false);
        const currentSystemPrompt = ref('');

        // 工具抽屉
        const skillsVisible = ref(false);

        // 侧边栏搜索
        const sessionSearchQuery = ref('');

        // 回到底部按钮
        const showScrollBottom = ref(false);

        // ===== 引入各模块 =====
        const themeModule = useTheme();
        const toolsModule = useTools();
        const sessionsModule = useSessions();
        const knowledgeModule = useKnowledge();

        function setStatus(text, color) {
            statusText.value = text;
            statusColor.value = color;
        }

        // 聊天模块
        const chatModule = useChat({
            messages, sending, messagesEl, inputEl, inputText,
            roundCount, selectedModel, currentSystemPrompt,
            toolsModule, knowledgeModule, sessionsModule,
            username, userTotalTokens,
            setStatus,
        });

        // ===== 侧边栏搜索过滤 =====
        const filteredSidebarSessions = computed(() => {
            if (!sessionSearchQuery.value) return sessionsModule.sidebarSessions.value;
            const q = sessionSearchQuery.value.toLowerCase();
            return sessionsModule.sidebarSessions.value.filter(s =>
                (s.title || '新对话').toLowerCase().includes(q)
            );
        });

        // ===== 回到底部 =====
        function onChatScroll() {
            if (!messagesEl.value) return;
            const el = messagesEl.value;
            showScrollBottom.value = (el.scrollHeight - el.scrollTop - el.clientHeight) > 200;
        }

        function scrollToBottomSmooth() {
            if (messagesEl.value) {
                messagesEl.value.scrollTo({ top: messagesEl.value.scrollHeight, behavior: 'smooth' });
            }
            showScrollBottom.value = false;
        }

        // ===== 模型加载 =====
        async function loadModels() {
            try {
                const data = await apiLoadModels();
                if (data.models && data.models.length > 0) {
                    allModels.value = data.models;
                    const saved = localStorage.getItem('selectedModel');
                    if (saved && data.models.find(m => m.name === saved)) {
                        selectedModel.value = saved;
                    } else {
                        selectedModel.value = data.default_model || data.models[0].name;
                    }
                }
            } catch (e) {
                console.warn('加载模型列表失败', e);
            }
        }

        function onModelChange(val) {
            localStorage.setItem('selectedModel', val);
            const isLocal = val.includes(':');
            ElMessage({ message: isLocal ? `已切换到本地模型：${val}` : `已切换到云端模型：${val}`, type: 'info', duration: 2000 });
        }

        // ===== 认证 =====
        function goLogin() {
            window.location.href = '/login.html';
        }

        async function loadUserInfo() {
            if (!localStorage.getItem('auth_token')) return;
            try {
                const data = await apiGetUserInfo();
                if (data.total_tokens !== undefined) {
                    userTotalTokens.value = data.total_tokens;
                    localStorage.setItem('user_total_tokens', data.total_tokens);
                }
                if (data.role) {
                    userRole.value = data.role;
                    localStorage.setItem('user_role', data.role);
                }
            } catch (e) {
                localStorage.removeItem('auth_token');
                localStorage.removeItem('username');
                localStorage.removeItem('user_id');
                localStorage.removeItem('user_role');
                localStorage.removeItem('user_total_tokens');
                username.value = '';
                userRole.value = 'guest';
                userTotalTokens.value = 0;
            }
        }

        async function logout() {
            try { await apiLogout(); } catch (e) {}
            localStorage.removeItem('auth_token');
            localStorage.removeItem('username');
            localStorage.removeItem('user_id');
            localStorage.removeItem('user_role');
            localStorage.removeItem('chatSessionId');
            localStorage.removeItem('user_total_tokens');
            window.location.href = '/login.html';
        }

        // ===== 会话操作 =====
        function newSession() {
            sessionsModule.sessionId.value = null;
            localStorage.removeItem('chatSessionId');
            currentSystemPrompt.value = '';
            chatModule.addWelcomeMessage();
            ElMessage({ message: '已创建新会话', type: 'success', duration: 1500 });
            sessionsModule.loadSidebarSessions();
        }

        function openSessionDetail(sid) {
            sessionsModule.openSessionDetail(sid, (prompt) => {
                currentSystemPrompt.value = prompt;
            });
        }

        // ===== System Prompt =====
        function openSystemPrompt() {
            systemPromptInput.value = currentSystemPrompt.value;
            systemPromptVisible.value = true;
        }

        function saveSystemPrompt() {
            currentSystemPrompt.value = systemPromptInput.value;
            systemPromptVisible.value = false;
            ElMessage({
                message: systemPromptInput.value ? 'System Prompt 已设置，下次发起对话时生效' : 'System Prompt 已清空',
                type: 'success',
                duration: 2000
            });
        }

        // ===== 连接检测 =====
        async function checkConnection() {
            try {
                const resp = await fetch('/api/models');
                setStatus(resp.ok ? '已连接' : '连接错误', resp.ok ? '#34d399' : '#f87171');
            } catch (e) {
                setStatus('连接错误', '#f87171');
            }
        }

        // ===== 输入框 =====
        function onKeydown(e) {
            if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                chatModule.sendMessage();
            }
        }

        function autoResize(e) {
            const el = e.target;
            el.style.height = 'auto';
            el.style.height = Math.min(el.scrollHeight, 140) + 'px';
        }

        // ===== 生命周期 =====
        onMounted(() => {
            setupMarked();
            chatModule.addWelcomeMessage();
            checkConnection();
            loadModels();
            sessionsModule.loadSidebarSessions();
            loadUserInfo();
            toolsModule.loadTools();
            knowledgeModule.loadKnowledgeBases();

            // 点击页面其他区域关闭知识库下拉
            document.addEventListener('click', () => {
                knowledgeModule.kbSelectorVisible.value = false;
            });

            watch(sessionsModule.historyVisible, (val) => {
                if (val && !sessionsModule.historyDetail.value) {
                    sessionsModule.openHistoryDrawer();
                }
            });

            watch(skillsVisible, (val) => {
                if (val) toolsModule.loadTools();
            });
        });

        return {
            loading, messages, inputText, sending, messagesEl, inputEl,
            roundCount,
            statusText, statusColor,
            allModels, selectedModel, cloudModels, localModels,
            username, userAvatar, userTotalTokens, userRole, isAdmin, isLoggedIn,
            suggestions, useSuggestion,
            systemPromptVisible, systemPromptInput, systemPromptSaving, currentSystemPrompt,
            skillsVisible,
            sessionSearchQuery, filteredSidebarSessions,
            showScrollBottom, onChatScroll, scrollToBottomSmooth,
            // 主题模块
            ...themeModule,
            // 聊天模块
            sendMessage: chatModule.sendMessage,
            clearMessages: chatModule.clearMessages,
            renderMarkdown: chatModule.renderMarkdown,
            copyMessage: chatModule.copyMessage,
            stopGenerate: chatModule.stopGenerate,
            regenerate: chatModule.regenerate,
            groupReactSteps: chatModule.groupReactSteps,
            // 其他
            newSession, onKeydown, autoResize,
            onModelChange, goLogin, logout,
            openSystemPrompt, saveSystemPrompt,
            formatTime, formatNumber,
            getAgentDisplayName,
            // 会话模块
            ...sessionsModule,
            openSessionDetail,
            // 工具模块
            ...toolsModule,
            getToolIcon,
            getAgentIcon,
            // 知识库模块
            ...knowledgeModule,
        };
    }
});

// 注册 Element Plus 图标
for (const [key, component] of Object.entries(ElementPlusIconsVue)) {
    app.component(key, component);
}

const zhCn = ElementPlus.lang && ElementPlus.lang.zhCn ? ElementPlus.lang.zhCn : undefined;
app.use(ElementPlus, zhCn ? { locale: zhCn } : {});
app.mount('#app');
