const { createApp, ref, computed, nextTick, onMounted, watch } = Vue;
const { ElMessage, ElMessageBox } = ElementPlus;

const app = createApp({
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
            sendMessage();
        }

        // System Prompt
        const systemPromptVisible = ref(false);
        const systemPromptInput = ref('');
        const systemPromptSaving = ref(false);
        const currentSystemPrompt = ref('');

        // 工具抽屉
        const skillsVisible = ref(false);

        // 停止生成控制器
        let abortController = null;

        // ===== 引入各模块 =====
        const toolsModule = useTools();
        const sessionsModule = useSessions();

        // ===== 工具方法 =====
        function formatTime(ts) {
            if (!ts) return '';
            return new Date(ts).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' });
        }

        function formatNumber(n) {
            if (!n) return '0';
            return n.toLocaleString('zh-CN');
        }

        function nowTime() {
            return new Date().toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' });
        }

        function scrollToBottom() {
            nextTick(() => {
                if (messagesEl.value) {
                    messagesEl.value.scrollTop = messagesEl.value.scrollHeight;
                }
            });
        }

        function setStatus(text, color) {
            statusText.value = text;
            statusColor.value = color;
        }

        // ===== Markdown 渲染 =====
        function renderMarkdown(content) {
            if (!content) return '';
            try {
                return marked.parse(content);
            } catch (e) {
                return content.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/\n/g,'<br>');
            }
        }

        // ===== 消息复制 =====
        function copyMessage(msg) {
            navigator.clipboard.writeText(msg.content || '').then(() => {
                msg.copied = true;
                setTimeout(() => { msg.copied = false; }, 2000);
            }).catch(() => {
                ElMessage({ message: '复制失败', type: 'error', duration: 1500 });
            });
        }

        // ===== 停止生成 =====
        function stopGenerate() {
            if (abortController) {
                abortController.abort();
                abortController = null;
            }
        }

        // ===== 重新生成 =====
        async function regenerate(msgIdx) {
            let userMsg = null;
            for (let i = msgIdx - 1; i >= 0; i--) {
                if (messages.value[i].role === 'user') {
                    userMsg = messages.value[i];
                    break;
                }
            }
            if (!userMsg) return;
            messages.value.splice(msgIdx);
            inputText.value = userMsg.content;
            messages.value.splice(msgIdx - 1);
            await sendMessage();
        }

        // ===== 初始化欢迎消息 =====
        function addWelcomeMessage() {
            roundCount.value = 0;
            messages.value = [{
                role: 'ai',
                content: '您好！我是智能小助手，很高兴为您服务。请问有什么可以帮您的？',
                time: nowTime(),
                typing: false
            }];
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
                // token 已失效，清除本地登录状态
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
            addWelcomeMessage();
            ElMessage({ message: '已创建新会话', type: 'success', duration: 1500 });
            sessionsModule.loadSidebarSessions();
        }

        function clearMessages() {
            messages.value = [{
                role: 'ai',
                content: '对话已清空，请继续提问。',
                time: nowTime(),
                typing: false
            }];
            ElMessage({ message: '已清空当前对话', type: 'success', duration: 1500 });
        }

        function openSessionDetail(sid) {
            sessionsModule.openSessionDetail(sid, (prompt) => {
                currentSystemPrompt.value = prompt;
            });
        }

        // ===== 刷新用户 token =====
        async function refreshUserTokens() {
            if (!username.value) return;
            try {
                const data = await apiGetUserInfo();
                if (data.total_tokens !== undefined) {
                    userTotalTokens.value = data.total_tokens;
                    localStorage.setItem('user_total_tokens', data.total_tokens);
                }
            } catch (e) {}
        }

        // ===== 发送消息 =====
        async function sendMessage() {
            const text = inputText.value.trim();
            if (!text || sending.value) return;

            roundCount.value++;
            const currentRound = roundCount.value;
            messages.value.push({
                role: 'user',
                content: text,
                time: nowTime(),
                round: currentRound,
            });
            inputText.value = '';
            nextTick(() => {
                if (inputEl.value) inputEl.value.style.height = 'auto';
            });
            scrollToBottom();

            sending.value = true;
            setStatus('思考中...', '#fbbf24');

            const typingIdx = messages.value.length;
            messages.value.push({ role: 'ai', typing: true, content: '', thinking: '', time: '' });
            scrollToBottom();

            try {
                abortController = new AbortController();
                const resp = await apiChatStream({
                    message: text,
                    session_id: sessionsModule.sessionId.value,
                    model_name: selectedModel.value,
                    system_prompt: currentSystemPrompt.value || undefined,
                    enabled_tools: toolsModule.enabledTools.value.length > 0 ? toolsModule.enabledTools.value : undefined
                }, abortController.signal);

                const reader = resp.body.getReader();
                const decoder = new TextDecoder();
                let buffer = '';
                let streamContent = '';
                let streamThinking = '';
                let modelName = '';

                messages.value.splice(typingIdx, 1, {
                    role: 'ai',
                    content: '',
                    thinking: '',
                    modelName: '',
                    time: nowTime(),
                    typing: false,
                    streaming: true,
                    round: currentRound,
                    reactCollapsed: false,
                });

                while (true) {
                    const { done, value } = await reader.read();
                    if (done) break;

                    buffer += decoder.decode(value, { stream: true });
                    const lines = buffer.split('\n');
                    buffer = lines.pop();

                    for (const line of lines) {
                        if (!line.startsWith('data: ')) continue;
                        const jsonStr = line.slice(6).trim();
                        if (!jsonStr) continue;
                        let event;
                        try { event = JSON.parse(jsonStr); } catch { continue; }

                        if (event.type === 'chunk') {
                            if (event.content) streamContent += event.content;
                            if (event.thinking) streamThinking += event.thinking;
                            if (event.model_name) modelName = event.model_name;
                            messages.value[typingIdx] = {
                                ...messages.value[typingIdx],
                                content: streamContent,
                                thinking: streamThinking || null,
                                modelName,
                                streaming: true,
                            };
                            scrollToBottom();
                        } else if (event.type === 'thought') {
                            const reactSteps = messages.value[typingIdx].reactSteps || [];
                            reactSteps.push({
                                type: 'thought',
                                step: event.step || reactSteps.length + 1,
                                content: event.content,
                                expanded: false,
                            });
                            messages.value[typingIdx] = { ...messages.value[typingIdx], reactSteps: [...reactSteps], streaming: true };
                            scrollToBottom();
                        } else if (event.type === 'tool_call') {
                            const reactSteps = messages.value[typingIdx].reactSteps || [];
                            let argsDisplay = '';
                            try {
                                const argsObj = JSON.parse(event.tool_args || '{}');
                                argsDisplay = Object.entries(argsObj).map(([k,v]) => `${k}: ${v}`).join(', ');
                            } catch(e) { argsDisplay = event.tool_args || ''; }
                            reactSteps.push({
                                type: 'action',
                                step: event.step || reactSteps.length + 1,
                                toolName: event.tool_display_name || event.tool_name,
                                toolRawName: event.tool_name,
                                toolCallId: event.tool_call_id || '',
                                toolArgs: event.tool_args || '',
                                argsDisplay,
                                status: 'calling',
                                result: '',
                                expanded: false,
                            });
                            messages.value[typingIdx] = { ...messages.value[typingIdx], reactSteps: [...reactSteps], streaming: true };
                            scrollToBottom();
                        } else if (event.type === 'tool_result') {
                            const reactSteps = messages.value[typingIdx].reactSteps || [];
                            const matchAction = event.tool_call_id
                                ? reactSteps.find(s => s.type === 'action' && s.toolCallId === event.tool_call_id && s.status === 'calling')
                                : [...reactSteps].reverse().find(s => s.type === 'action' && (s.toolRawName || s.toolName) === event.tool_name && s.status === 'calling');
                            if (matchAction) {
                                matchAction.status = 'done';
                                matchAction.result = event.tool_result || '';
                            }
                            messages.value[typingIdx] = { ...messages.value[typingIdx], reactSteps: [...reactSteps], streaming: true };
                            scrollToBottom();
                        } else if (event.type === 'done') {
                            if (event.session_id) {
                                sessionsModule.sessionId.value = event.session_id;
                                localStorage.setItem('chatSessionId', event.session_id);
                            }
                            const curMsg = messages.value[typingIdx];
                            messages.value[typingIdx] = {
                                role: 'ai',
                                content: streamContent,
                                thinking: streamThinking || null,
                                modelName: event.model_name || modelName,
                                time: nowTime(),
                                typing: false,
                                streaming: false,
                                round: currentRound,
                                reactSteps: curMsg?.reactSteps || [],
                                reactCollapsed: curMsg?.reactCollapsed || false,
                                promptTokens: event.prompt_tokens || 0,
                                completionTokens: event.completion_tokens || 0,
                                totalTokens: event.total_tokens || 0,
                                sessionTotalTokens: event.session_total_tokens || 0,
                            };
                            setStatus('已连接', '#34d399');
                            setTimeout(() => sessionsModule.loadSidebarSessions(), 1500);
                            if (username.value && event.total_tokens > 0) {
                                userTotalTokens.value += event.total_tokens;
                                localStorage.setItem('user_total_tokens', userTotalTokens.value);
                            }
                            refreshUserTokens();
                        } else if (event.type === 'error') {
                            throw new Error(event.error || '流式响应错误');
                        }
                    }
                }
            } catch (e) {
                if (e.name === 'AbortError') {
                    const cur = messages.value[typingIdx];
                    if (cur) messages.value[typingIdx] = { ...cur, streaming: false, typing: false };
                    setStatus('已连接', '#34d399');
                } else {
                    messages.value.splice(typingIdx, 1, {
                        role: 'ai',
                        content: '抱歉，发送消息时出现错误，请稍后重试。',
                        time: nowTime(),
                        typing: false
                    });
                    setStatus('连接错误', '#f87171');
                    ElMessage({ message: '发送失败，请检查网络连接', type: 'error', duration: 3000 });
                }
            } finally {
                abortController = null;
                sending.value = false;
                scrollToBottom();
            }
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
                sendMessage();
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
            addWelcomeMessage();
            checkConnection();
            loadModels();
            sessionsModule.loadSidebarSessions();
            loadUserInfo();
            toolsModule.loadTools();

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
            newSession, clearMessages, sendMessage, onKeydown, autoResize,
            onModelChange, goLogin, logout,
            openSystemPrompt, saveSystemPrompt,
            renderMarkdown, copyMessage, stopGenerate, regenerate,
            formatTime, formatNumber,
            // 会话模块
            ...sessionsModule,
            openSessionDetail,
            // 工具模块
            ...toolsModule,
            getToolIcon,
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
