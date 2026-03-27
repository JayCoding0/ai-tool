const { createApp, ref, computed, nextTick, onMounted, watch } = Vue;
const { ElMessage, ElMessageBox } = ElementPlus;

// ===== Markdown 渲染配置 =====
function setupMarked() {
    const renderer = new marked.Renderer();
    // 代码块：添加语言标签 + 复制按钮
    renderer.code = function(code, language) {
        const lang = language || 'plaintext';
        let highlighted = '';
        try {
            if (hljs.getLanguage(lang)) {
                highlighted = hljs.highlight(code, { language: lang }).value;
            } else {
                highlighted = hljs.highlightAuto(code).value;
            }
        } catch (e) {
            highlighted = code.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
        }
        const escapedCode = code.replace(/`/g, '\\`').replace(/\$/g, '\\$');
        return `<div class="code-block-wrap">
  <div class="code-block-header">
    <span class="code-block-lang">${lang}</span>
    <button class="code-copy-btn" onclick="copyCode(this, \`${escapedCode}\`)">  
      <svg width="12" height="12" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"/></svg>
      复制
    </button>
  </div>
  <pre><code class="hljs">${highlighted}</code></pre>
</div>`;
    };
    marked.setOptions({ renderer, breaks: true, gfm: true });
}

// 全局代码复制函数（供 v-html 内联调用）
window.copyCode = function(btn, code) {
    navigator.clipboard.writeText(code).then(() => {
        btn.classList.add('copied');
        btn.innerHTML = `<svg width="12" height="12" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/></svg> 已复制`;
        setTimeout(() => {
            btn.classList.remove('copied');
            btn.innerHTML = `<svg width="12" height="12" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"/></svg> 复制`;
        }, 2000);
    }).catch(() => {});
};

// 注册所有 Element Plus 图标
const app = createApp({
    setup() {
        // ===== 状态 =====
        const loading = ref(false);
        const messages = ref([]);
        const inputText = ref('');
        const sending = ref(false);
        const messagesEl = ref(null);
        const inputEl = ref(null);

        // 状态栏
        const statusText = ref('连接中...');
        const statusColor = ref('#9ca3af');

        // 会话
        const sessionId = ref(localStorage.getItem('chatSessionId') || null);

        // 模型
        const allModels = ref([]);
        const selectedModel = ref(localStorage.getItem('selectedModel') || '');
        const cloudModels = computed(() => allModels.value.filter(m => m.type !== 'local'));
        const localModels = computed(() => allModels.value.filter(m => m.type === 'local'));

        // 用户
        const username = ref(localStorage.getItem('username') || '');
        const userRole = ref(localStorage.getItem('user_role') || 'guest'); // guest/user/admin
        const isAdmin = computed(() => userRole.value === 'admin');
        const isLoggedIn = computed(() => !!username.value);
        const userAvatar = computed(() => username.value ? username.value.charAt(0).toUpperCase() : '?');
        const userTotalTokens = ref(parseInt(localStorage.getItem('user_total_tokens') || '0', 10));

        // 侧边栏会话
        const sidebarSessions = ref([]);
        const currentSessionTitle = ref('智能对话');

        // 对话轮次计数（每发一次用户消息+1）
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

        // 历史抽屉
        const historyVisible = ref(false);
        const historyLoading = ref(false);
        const historySessions = ref([]);
        const historyDetail = ref(null);
        const detailLoading = ref(false);

        // System Prompt
        const systemPromptVisible = ref(false);
        const systemPromptInput = ref('');
        const systemPromptSaving = ref(false);
        const currentSystemPrompt = ref(''); // 当前会话已保存的 system prompt

        // Skills 技能库
        const skillsVisible = ref(false);
        const skillsLoading = ref(false);
        const skillsList = ref([]);
        const skillEditVisible = ref(false);
        const editingSkill = ref(null); // null=新建，否则=编辑
        const skillSaving = ref(false);
        const skillForm = ref({ name: '', description: '', icon: '🤖', system_prompt: '', tools: [], is_public: false });
        const skillEmojis = ['🤖', '💻', '🌐', '📊', '✍️', '🗄️', '📋', '🔍', '🎯', '🧠', '⚡', '🎨', '📚', '🔧', '🚀'];
        const mySkills = computed(() => skillsList.value);

        // 停止生成控制器
        let abortController = null;

        // 工具图标映射
        const toolIconMap = {
            'calculate': '🧮',
            'get_current_time': '🕐',
            'http_request': '🌐',
            'mysql_query': '🗄️',
            'write_file': '📝',
            'execute_command': '💻',
            'weather': '🌤️',
            'ip_lookup': '🔍',
            'file_explorer': '📁',
            'query_database': '🗄️',
            'web_search': '🔎',
            'send_email': '📧',
            'read_file': '📖',
        };
        function getToolIcon(name) {
            return toolIconMap[name] || '🔧';
        }

        // 工具列表（自动注册，前端控制启用）
        const availableTools = ref([]);  // 所有已注册工具
        const enabledTools = ref(JSON.parse(localStorage.getItem('enabledTools') || '[]')); // 已启用工具名称列表
        const toolsLoading = ref(false);

        async function loadTools() {
            toolsLoading.value = true;
            try {
                const resp = await fetch('/api/tools', { headers: getAuthHeaders() });
                if (resp.ok) {
                    const data = await resp.json();
                    availableTools.value = data.tools || [];
                    // 只有工具列表非空时才过滤，避免接口异常时误清空已启用工具
                    if (availableTools.value.length > 0) {
                        const validNames = availableTools.value.map(t => t.name);
                        const filtered = enabledTools.value.filter(n => validNames.includes(n));
                        if (filtered.length !== enabledTools.value.length) {
                            enabledTools.value = filtered;
                            localStorage.setItem('enabledTools', JSON.stringify(filtered));
                        }
                    }
                }
            } catch (e) {}
            toolsLoading.value = false;
        }

        function toggleTool(toolName) {
            const idx = enabledTools.value.indexOf(toolName);
            if (idx === -1) {
                enabledTools.value.push(toolName);
            } else {
                enabledTools.value.splice(idx, 1);
            }
            localStorage.setItem('enabledTools', JSON.stringify(enabledTools.value));
        }

        function isToolEnabled(toolName) {
            return enabledTools.value.includes(toolName);
        }

        // ===== 工具方法 =====
        function getAuthHeaders() {
            const token = localStorage.getItem('auth_token');
            const headers = { 'Content-Type': 'application/json' };
            if (token) headers['Authorization'] = 'Bearer ' + token;
            return headers;
        }

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
            // 找到该 AI 消息前最近的用户消息
            let userMsg = null;
            for (let i = msgIdx - 1; i >= 0; i--) {
                if (messages.value[i].role === 'user') {
                    userMsg = messages.value[i];
                    break;
                }
            }
            if (!userMsg) return;
            // 删除该 AI 消息及之后的所有消息
            messages.value.splice(msgIdx);
            // 重新发送
            inputText.value = userMsg.content;
            // 移除最后一条用户消息（sendMessage 会重新添加）
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
                const resp = await fetch('/api/models');
                if (!resp.ok) return;
                const data = await resp.json();
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

        // 加载当前用户信息（含累计 token）
        async function loadUserInfo() {
            if (!localStorage.getItem('auth_token')) return;
            try {
                const resp = await fetch('/api/auth/me', { headers: getAuthHeaders() });
                if (!resp.ok) {
                    // token 已失效，清除本地登录状态，切换为游客模式
                    localStorage.removeItem('auth_token');
                    localStorage.removeItem('username');
                    localStorage.removeItem('user_id');
                    localStorage.removeItem('user_role');
                    localStorage.removeItem('user_total_tokens');
                    username.value = '';
                    userRole.value = 'guest';
                    userTotalTokens.value = 0;
                    return;
                }
                const data = await resp.json();
                if (data.total_tokens !== undefined) {
                    userTotalTokens.value = data.total_tokens;
                    localStorage.setItem('user_total_tokens', data.total_tokens);
                }
                if (data.role) {
                    userRole.value = data.role;
                    localStorage.setItem('user_role', data.role);
                }
            } catch (e) {}
        }

        async function logout() {
            const token = localStorage.getItem('auth_token');
            try {
                await fetch('/api/auth/logout', {
                    method: 'POST',
                    headers: token ? { 'Authorization': 'Bearer ' + token } : {}
                });
            } catch (e) {}
            localStorage.removeItem('auth_token');
            localStorage.removeItem('username');
            localStorage.removeItem('user_id');
            localStorage.removeItem('user_role');
            localStorage.removeItem('chatSessionId');
            localStorage.removeItem('user_total_tokens');
            window.location.href = '/login.html';
        }

        // ===== 会话管理 =====
        function newSession() {
            sessionId.value = null;
            localStorage.removeItem('chatSessionId');
            currentSystemPrompt.value = '';
            addWelcomeMessage();
            ElMessage({ message: '已创建新会话', type: 'success', duration: 1500 });
            loadSidebarSessions();
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

        // ===== 侧边栏历史 =====
        async function loadSidebarSessions() {
            try {
                const resp = await fetch('/api/sessions', { headers: getAuthHeaders() });
                if (!resp.ok) throw new Error();
                const data = await resp.json();
                sidebarSessions.value = (data.sessions || []).slice(0, 20);
            } catch (e) {
                sidebarSessions.value = [];
            }
        }

        // ===== 历史抽屉 =====
        async function openHistoryDrawer() {
            historyVisible.value = true;
            historyDetail.value = null;
            historyLoading.value = true;
            try {
                const resp = await fetch('/api/sessions', { headers: getAuthHeaders() });
                if (!resp.ok) throw new Error();
                const data = await resp.json();
                historySessions.value = data.sessions || [];
            } catch (e) {
                historySessions.value = [];
            } finally {
                historyLoading.value = false;
            }
        }

        async function loadSessionDetail(sid) {
            historyDetail.value = [];
            detailLoading.value = true;
            try {
                const resp = await fetch('/api/history', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ session_id: sid })
                });
                if (!resp.ok) throw new Error();
                const data = await resp.json();
                historyDetail.value = data.messages || [];
            } catch (e) {
                historyDetail.value = [];
            } finally {
                detailLoading.value = false;
            }
        }

        function openSessionDetail(sid) {
            historyVisible.value = true;
            historyDetail.value = null;
            historyLoading.value = true;
            fetch('/api/sessions', { headers: getAuthHeaders() })
                .then(r => r.json())
                .then(data => {
                    historySessions.value = data.sessions || [];
                    historyLoading.value = false;
                    // 加载该会话的 system prompt
                    const sess = (data.sessions || []).find(s => s.id === sid);
                    if (sess) currentSystemPrompt.value = sess.system_prompt || '';
                    loadSessionDetail(sid);
                })
                .catch(() => { historyLoading.value = false; });
        }

        // 每次收到 done 事件后刷新用户 token 总数
        async function refreshUserTokens() {
            if (!username.value) return;
            try {
                const resp = await fetch('/api/auth/me', { headers: getAuthHeaders() });
                if (!resp.ok) return;
                const data = await resp.json();
                if (data.total_tokens !== undefined) {
                    userTotalTokens.value = data.total_tokens;
                    localStorage.setItem('user_total_tokens', data.total_tokens);
                }
            } catch (e) {}
        }

        // ===== 会话重命名 =====
        async function renameSession(s) {
            let newTitle = '';
            try {
                const result = await ElMessageBox.prompt('请输入新的会话名称', '重命名会话', {
                    confirmButtonText: '确定',
                    cancelButtonText: '取消',
                    inputValue: s.title || '',
                    inputValidator: (val) => val && val.trim() ? true : '名称不能为空',
                });
                newTitle = result.value.trim();
            } catch { return; }
            try {
                const resp = await fetch('/api/sessions/rename', {
                    method: 'POST',
                    headers: getAuthHeaders(),
                    body: JSON.stringify({ session_id: s.id, title: newTitle })
                });
                if (!resp.ok) throw new Error();
                s.title = newTitle;
                ElMessage({ message: '重命名成功', type: 'success', duration: 1500 });
            } catch (e) {
                ElMessage({ message: '重命名失败', type: 'error', duration: 2000 });
            }
        }

        // ===== 会话删除 =====
        async function deleteSession(s) {
            try {
                await ElMessageBox.confirm(`确定要删除会话「${s.title || '新对话'}」吗？`, '删除确认', {
                    confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning'
                });
            } catch { return; }
            try {
                const resp = await fetch('/api/sessions/delete', {
                    method: 'POST',
                    headers: getAuthHeaders(),
                    body: JSON.stringify({ session_id: s.id })
                });
                if (!resp.ok) throw new Error();
                // 如果删除的是当前会话，新建会话
                if (sessionId.value === s.id) {
                    newSession();
                } else {
                    loadSidebarSessions();
                }
                ElMessage({ message: '会话已删除', type: 'success', duration: 1500 });
            } catch (e) {
                ElMessage({ message: '删除失败', type: 'error', duration: 2000 });
            }
        }

        // ===== 发送消息 =====
        async function sendMessage() {
            const text = inputText.value.trim();
            if (!text || sending.value) return;

            // 添加用户消息
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
                if (inputEl.value) {
                    inputEl.value.style.height = 'auto';
                }
            });
            scrollToBottom();

            sending.value = true;
            setStatus('思考中...', '#fbbf24');

            // 添加 AI 消息占位（流式填充）
            const typingIdx = messages.value.length;
            messages.value.push({ role: 'ai', typing: true, content: '', thinking: '', time: '' });
            scrollToBottom();

            try {
                abortController = new AbortController();
                const resp = await fetch('/api/chat/stream', {
                    method: 'POST',
                    headers: getAuthHeaders(),
                    signal: abortController.signal,
                    body: JSON.stringify({
                        message: text,
                        session_id: sessionId.value,
                        model_name: selectedModel.value,
                        system_prompt: currentSystemPrompt.value || undefined,
                        enabled_tools: enabledTools.value.length > 0 ? enabledTools.value : undefined
                    })
                });
                if (!resp.ok) throw new Error(`HTTP ${resp.status}`);

                const reader = resp.body.getReader();
                const decoder = new TextDecoder();
                let buffer = '';
                let streamContent = '';
                let streamThinking = '';
                let modelName = '';

                // 切换为流式模式（去掉打字动画）
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
                    buffer = lines.pop(); // 保留未完整的行

                    for (const line of lines) {
                        if (!line.startsWith('data: ')) continue;
                        const jsonStr = line.slice(6).trim();
                        if (!jsonStr) continue;
                        let event;
                        try { event = JSON.parse(jsonStr); } catch { continue; }

                        if (event.type === 'chunk') {
                            if (event.content) {
                                streamContent += event.content;
                            }
                            if (event.thinking) {
                                streamThinking += event.thinking;
                            }
                            if (event.model_name) modelName = event.model_name;
                            // 实时更新消息内容
                            messages.value[typingIdx] = {
                                ...messages.value[typingIdx],
                                content: streamContent,
                                thinking: streamThinking || null,
                                modelName: modelName,
                                streaming: true,
                            };
                            scrollToBottom();
                        } else if (event.type === 'thought') {
                            // AI 思考过程（工具调用前的推理）
                            const reactSteps = messages.value[typingIdx].reactSteps || [];
                            reactSteps.push({
                                type: 'thought',
                                step: event.step || reactSteps.length + 1,
                                content: event.content,
                                expanded: false,
                            });
                            messages.value[typingIdx] = {
                                ...messages.value[typingIdx],
                                reactSteps: [...reactSteps],
                                streaming: true,
                            };
                            scrollToBottom();
                        } else if (event.type === 'tool_call') {
                            // 工具调用中：展示进度提示，记录工具调用历史
                            const reactSteps = messages.value[typingIdx].reactSteps || [];
                            // 解析工具参数用于展示
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
                            messages.value[typingIdx] = {
                                ...messages.value[typingIdx],
                                reactSteps: [...reactSteps],
                                streaming: true,
                            };
                            scrollToBottom();
                        } else if (event.type === 'tool_result') {
                            // 工具结果返回：用 tool_call_id 精确匹配，降级为 tool_name 兜底
                            const reactSteps = messages.value[typingIdx].reactSteps || [];
                            const matchAction = event.tool_call_id
                                ? reactSteps.find(s => s.type === 'action' && s.toolCallId === event.tool_call_id && s.status === 'calling')
                                : [...reactSteps].reverse().find(s => s.type === 'action' && (s.toolRawName || s.toolName) === event.tool_name && s.status === 'calling');
                            if (matchAction) {
                                matchAction.status = 'done';
                                matchAction.result = event.tool_result || '';
                            }
                            messages.value[typingIdx] = {
                                ...messages.value[typingIdx],
                                reactSteps: [...reactSteps],
                                streaming: true,
                            };
                            scrollToBottom();
                        } else if (event.type === 'done') {
                            if (event.session_id) {
                                sessionId.value = event.session_id;
                                localStorage.setItem('chatSessionId', event.session_id);
                            }
                            // 最终更新，带 token 信息（保留 reactSteps 调用链路）
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
                            // 延迟刷新侧边栏（等待标题生成）
                            setTimeout(() => loadSidebarSessions(), 1500);
                            // 立即累加本次 token 到头像下方，实现实时显示
                            if (username.value && event.total_tokens > 0) {
                                userTotalTokens.value += event.total_tokens;
                                localStorage.setItem('user_total_tokens', userTotalTokens.value);
                            }
                            // 异步刷新精确值（与数据库同步）
                            refreshUserTokens();
                        } else if (event.type === 'error') {
                            throw new Error(event.error || '流式响应错误');
                        }
                    }
                }
            } catch (e) {
                if (e.name === 'AbortError') {
                    // 用户主动停止，保留已生成内容
                    const cur = messages.value[typingIdx];
                    if (cur) {
                        messages.value[typingIdx] = { ...cur, streaming: false, typing: false };
                    }
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
        async function openSystemPrompt() {
            systemPromptInput.value = currentSystemPrompt.value;
            systemPromptVisible.value = true;
        }

        function saveSystemPrompt() {
            // 直接保存到内存，发送消息时会随请求一起带上
            currentSystemPrompt.value = systemPromptInput.value;
            systemPromptVisible.value = false;
            ElMessage({
                message: systemPromptInput.value ? 'System Prompt 已设置，下次发起对话时生效' : 'System Prompt 已清空',
                type: 'success',
                duration: 2000
            });
        }

        // ===== Skills 技能库 =====
        async function loadSkills() {
            skillsLoading.value = true;
            try {
                const resp = await fetch('/api/skills', { headers: getAuthHeaders() });
                if (!resp.ok) throw new Error();
                const data = await resp.json();
                skillsList.value = data.skills || [];
            } catch (e) {
                skillsList.value = [];
            } finally {
                skillsLoading.value = false;
            }
        }

        async function applySkill(sk) {
            if (!sk) return;
            // 如果有当前会话，持久化到数据库
            if (sessionId.value) {
                try {
                    await fetch('/api/skills/apply', {
                        method: 'POST',
                        headers: getAuthHeaders(),
                        body: JSON.stringify({ skill_id: sk.id, session_id: sessionId.value })
                    });
                } catch (e) {}
            }
            // 更新内存中的 system prompt
            currentSystemPrompt.value = sk.system_prompt;
            // 如果技能绑定了工具，同步启用这些工具
            if (sk.tools && sk.tools.length > 0) {
                const validNames = availableTools.value.map(t => t.name);
                const toolsToEnable = sk.tools.filter(n => validNames.includes(n));
                enabledTools.value = toolsToEnable;
                localStorage.setItem('enabledTools', JSON.stringify(toolsToEnable));
            }
            skillsVisible.value = false;
            ElMessage({ message: `✅ 已应用技能「${sk.name}」`, type: 'success', duration: 2000 });
        }

        function openCreateSkill() {
            editingSkill.value = null;
            skillForm.value = { name: '', description: '', icon: '🤖', system_prompt: '', is_public: false };
            skillEditVisible.value = true;
        }

        function editSkill(sk) {
            editingSkill.value = sk;
            skillForm.value = {
                name: sk.name,
                description: sk.description,
                icon: sk.icon,
                system_prompt: sk.system_prompt,
                tools: sk.tools ? [...sk.tools] : [],
                is_public: sk.is_public,
            };
            skillEditVisible.value = true;
        }

        async function saveSkill() {
            if (!skillForm.value.name.trim()) {
                ElMessage({ message: '请填写技能名称', type: 'warning' }); return;
            }
            if (!skillForm.value.system_prompt.trim()) {
                ElMessage({ message: '请填写 System Prompt', type: 'warning' }); return;
            }
            skillSaving.value = true;
            try {
                let resp;
                if (editingSkill.value) {
                    resp = await fetch(`/api/skills/update?id=${editingSkill.value.id}`, {
                        method: 'POST',
                        headers: getAuthHeaders(),
                        body: JSON.stringify(skillForm.value)
                    });
                } else {
                    resp = await fetch('/api/skills/create', {
                        method: 'POST',
                        headers: getAuthHeaders(),
                        body: JSON.stringify(skillForm.value)
                    });
                }
                if (!resp.ok) {
                    const err = await resp.json();
                    throw new Error(err.error || '保存失败');
                }
                skillEditVisible.value = false;
                await loadSkills();
                ElMessage({ message: editingSkill.value ? '技能已更新' : '技能已创建', type: 'success' });
            } catch (e) {
                ElMessage({ message: e.message || '保存失败', type: 'error' });
            } finally {
                skillSaving.value = false;
            }
        }

        async function deleteSkill(sk) {
            try {
                await ElMessageBox.confirm(`确定要删除技能「${sk.name}」吗？`, '删除确认', {
                    confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning'
                });
            } catch { return; }
            try {
                const resp = await fetch('/api/skills/delete', {
                    method: 'POST',
                    headers: getAuthHeaders(),
                    body: JSON.stringify({ id: sk.id })
                });
                if (!resp.ok) throw new Error();
                await loadSkills();
                ElMessage({ message: '技能已删除', type: 'success' });
            } catch (e) {
                ElMessage({ message: '删除失败', type: 'error' });
            }
        }

        // admin 下载技能
        function adminDownloadSkill(sk) {
            const url = `/api/admin/skills/download?id=${sk.id}`;
            const a = document.createElement('a');
            a.href = url;
            a.download = `skill_${sk.id}_${sk.name}.json`;
            document.body.appendChild(a);
            a.click();
            document.body.removeChild(a);
        }

        // admin 上传技能（触发文件选择）
        function adminUploadSkillTrigger() {
            const input = document.createElement('input');
            input.type = 'file';
            input.accept = '.json';
            input.onchange = async (e) => {
                const file = e.target.files[0];
                if (!file) return;
                try {
                    const text = await file.text();
                    const skillData = JSON.parse(text);
                    const resp = await fetch('/api/admin/skills/upload', {
                        method: 'POST',
                        headers: getAuthHeaders(),
                        body: JSON.stringify(skillData)
                    });
                    if (!resp.ok) {
                        const err = await resp.json();
                        throw new Error(err.error || '上传失败');
                    }
                    await loadSkills();
                    ElMessage({ message: '技能已导入', type: 'success' });
                } catch (e) {
                    ElMessage({ message: e.message || '导入失败', type: 'error' });
                }
            };
            input.click();
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
            setupMarked(); // 初始化 Markdown 渲染器
            addWelcomeMessage();
            checkConnection();
            loadModels();
            loadSidebarSessions();
            loadUserInfo();
            loadTools(); // 启动时加载所有已注册工具

            // 监听 historyVisible
            watch(historyVisible, (val) => {
                if (val && !historyDetail.value) {
                    openHistoryDrawer();
                }
            });

            // 监听 skillsVisible，打开时加载技能列表和工具列表
            watch(skillsVisible, (val) => {
                if (val) {
                    loadSkills();
                    loadTools();
                }
            });
        });

        return {
            loading, messages, inputText, sending, messagesEl, inputEl,
            roundCount,
            statusText, statusColor,
            allModels, selectedModel, cloudModels, localModels,
            username, userAvatar, userTotalTokens, userRole, isAdmin, isLoggedIn,
            sidebarSessions, currentSessionTitle,
            suggestions, useSuggestion,
            historyVisible, historyLoading, historySessions, historyDetail, detailLoading,
            systemPromptVisible, systemPromptInput, systemPromptSaving, currentSystemPrompt,
            skillsVisible, skillsLoading, skillsList, mySkills,
            skillEditVisible, editingSkill, skillSaving, skillForm, skillEmojis,
            newSession, clearMessages, sendMessage, onKeydown, autoResize,
            onModelChange, goLogin, logout,
            openSessionDetail, loadSessionDetail,
            openSystemPrompt, saveSystemPrompt,
            applySkill, openCreateSkill, editSkill, saveSkill, deleteSkill,
            adminDownloadSkill, adminUploadSkillTrigger,
            availableTools, enabledTools, toolsLoading, loadTools, toggleTool, isToolEnabled, getToolIcon,
            renderMarkdown, copyMessage, stopGenerate, regenerate,
            renameSession, deleteSession,
            formatTime, formatNumber
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