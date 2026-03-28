// ===== 聊天核心逻辑（SSE 流处理、ReAct 步骤等） =====

function useChat(options) {
    const { ref, computed, nextTick } = Vue;
    const { ElMessage } = ElementPlus;

    const {
        messages, sending, messagesEl, inputEl, inputText,
        roundCount, selectedModel, currentSystemPrompt,
        toolsModule, knowledgeModule, sessionsModule,
        username, userTotalTokens,
        setStatus,
    } = options;

    let abortController = null;

    // ===== Markdown 渲染 =====
    function renderMarkdown(content) {
        if (!content) return '';
        try {
            return marked.parse(content);
        } catch (e) {
            return content.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/\n/g,'<br>');
        }
    }

    // ===== ReAct 步骤分组（同一 Agent 多次调用合并） =====
    function groupReactSteps(steps) {
        if (!steps || steps.length === 0) return [];
        const groups = [];
        const agentGroupMap = new Map();

        for (const step of steps) {
            if (step.type === 'action' && step.isAgentCall) {
                const name = step.agentCallName;
                if (agentGroupMap.has(name)) {
                    const group = agentGroupMap.get(name);
                    group.steps.push(step);
                    if (!group.rounds.includes(step.step)) {
                        group.rounds.push(step.step);
                    }
                } else {
                    const group = {
                        type: 'agent-group',
                        agentName: name,
                        steps: [step],
                        rounds: [step.step],
                        collapsed: false,
                    };
                    agentGroupMap.set(name, group);
                    groups.push(group);
                }
            } else if (step.type === 'thought') {
                groups.push({ type: 'thought', step });
            } else {
                groups.push({ type: 'action', step });
            }
        }
        return groups;
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
            content: '您好！我是智能Agent平台助手，很高兴为您服务。请问有什么可以帮您的？',
            time: nowTime(),
            typing: false
        }];
    }

    // ===== 清空消息 =====
    function clearMessages() {
        if (abortController) {
            abortController.abort();
            abortController = null;
        }
        sending.value = false;
        setStatus('已连接', '#34d399');
        roundCount.value = 0;
        messages.value = [{
            role: 'ai',
            content: '对话已清空，请继续提问。',
            time: nowTime(),
            typing: false
        }];
        ElMessage({ message: '已清空当前对话', type: 'success', duration: 1500 });
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

        // 若工具列表还在加载中，等待加载完成再发送
        if (toolsModule.toolsLoading.value) {
            await new Promise(resolve => {
                const stop = Vue.watch(toolsModule.toolsLoading, (loading) => {
                    if (!loading) { stop(); resolve(); }
                });
            });
        }

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
        scrollToBottom(messagesEl);

        sending.value = true;
        setStatus('思考中...', '#fbbf24');

        const typingIdx = messages.value.length;
        messages.value.push({ role: 'ai', typing: true, content: '', thinking: '', time: '' });
        scrollToBottom(messagesEl);

        try {
            abortController = new AbortController();
            const ragKbName = knowledgeModule.selectedKbId.value
                ? (knowledgeModule.getSelectedKB()?.name || '知识库')
                : null;
            const resp = await apiChatStream({
                message: text,
                session_id: sessionsModule.sessionId.value,
                model_name: selectedModel.value,
                system_prompt: currentSystemPrompt.value || undefined,
                enabled_tools: toolsModule.enabledTools.value.length > 0 ? toolsModule.enabledTools.value : undefined,
                knowledge_base_id: knowledgeModule.selectedKbId.value || undefined
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
                ragKbName,
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

                    // 防御：若消息已被清空，停止处理
                    if (!messages.value[typingIdx]) break;

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
                        scrollToBottom(messagesEl);
                    } else if (event.type === 'thought') {
                        const reactSteps = messages.value[typingIdx].reactSteps || [];

                        if (event.parent_tool_call_id) {
                            const parentStep = reactSteps.find(s => s.type === 'action' && s.isAgentCall && s.toolCallId === event.parent_tool_call_id);
                            if (parentStep) {
                                if (!parentStep.subSteps) parentStep.subSteps = [];
                                parentStep.subSteps.push({
                                    type: 'thought',
                                    step: event.step || parentStep.subSteps.length + 1,
                                    content: event.content,
                                    expanded: false,
                                });
                                messages.value[typingIdx] = { ...messages.value[typingIdx], reactSteps: [...reactSteps], streaming: true };
                                scrollToBottom(messagesEl);
                                continue;
                            }
                        }

                        reactSteps.push({
                            type: 'thought',
                            step: event.step || reactSteps.length + 1,
                            content: event.content,
                            expanded: false,
                        });
                        messages.value[typingIdx] = { ...messages.value[typingIdx], reactSteps: [...reactSteps], streaming: true };
                        scrollToBottom(messagesEl);
                    } else if (event.type === 'tool_call') {
                        const reactSteps = messages.value[typingIdx].reactSteps || [];
                        let argsDisplay = '';
                        let argsObj = {};
                        try {
                            argsObj = JSON.parse(event.tool_args || '{}');
                            argsDisplay = Object.entries(argsObj).map(([k,v]) => `${k}: ${v}`).join(', ');
                        } catch(e) { argsDisplay = event.tool_args || ''; }

                        if (event.parent_tool_call_id) {
                            const parentStep = reactSteps.find(s => s.type === 'action' && s.isAgentCall && s.toolCallId === event.parent_tool_call_id);
                            if (parentStep) {
                                if (!parentStep.subSteps) parentStep.subSteps = [];
                                parentStep.subSteps.push({
                                    type: 'action',
                                    step: event.step || parentStep.subSteps.length + 1,
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
                                scrollToBottom(messagesEl);
                                continue;
                            }
                        }

                        const isAgentCall = event.tool_name === 'call_agent';
                        const agentCallName = isAgentCall ? (argsObj.agent_name || '') : '';
                        const agentCallMsg  = isAgentCall ? (argsObj.message || '') : '';

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
                            isAgentCall,
                            agentCallName,
                            agentCallMsg,
                            subSteps: [],
                        });
                        if (isAgentCall) {
                            reactSteps[reactSteps.length - 1].expanded = true;
                        }
                        messages.value[typingIdx] = { ...messages.value[typingIdx], reactSteps: [...reactSteps], streaming: true };
                        scrollToBottom(messagesEl);
                    } else if (event.type === 'tool_result') {
                        const reactSteps = messages.value[typingIdx].reactSteps || [];

                        if (event.parent_tool_call_id) {
                            const parentStep = reactSteps.find(s => s.type === 'action' && s.isAgentCall && s.toolCallId === event.parent_tool_call_id);
                            if (parentStep && parentStep.subSteps) {
                                const subMatch = event.tool_call_id
                                    ? parentStep.subSteps.find(s => s.toolCallId === event.tool_call_id && s.status === 'calling')
                                    : [...parentStep.subSteps].reverse().find(s => (s.toolRawName || s.toolName) === event.tool_name && s.status === 'calling');
                                if (subMatch) {
                                    subMatch.status = 'done';
                                    subMatch.result = event.tool_result || '';
                                }
                                messages.value[typingIdx] = { ...messages.value[typingIdx], reactSteps: [...reactSteps], streaming: true };
                                scrollToBottom(messagesEl);
                                continue;
                            }
                        }

                        const matchAction = event.tool_call_id
                            ? reactSteps.find(s => s.type === 'action' && s.toolCallId === event.tool_call_id && s.status === 'calling')
                            : [...reactSteps].reverse().find(s => s.type === 'action' && (s.toolRawName || s.toolName) === event.tool_name && s.status === 'calling');
                        if (matchAction) {
                            matchAction.status = 'done';
                            let resultText = event.tool_result || '';
                            if (matchAction.isAgentCall) {
                                resultText = resultText.replace(/^\[.*?的回复\]\n?/, '');
                            }
                            matchAction.result = resultText;
                        }
                        messages.value[typingIdx] = { ...messages.value[typingIdx], reactSteps: [...reactSteps], streaming: true };
                        scrollToBottom(messagesEl);
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
                            ragKbName: curMsg?.ragKbName || null,
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
            scrollToBottom(messagesEl);
        }
    }

    return {
        renderMarkdown,
        groupReactSteps,
        copyMessage,
        stopGenerate,
        regenerate,
        addWelcomeMessage,
        clearMessages,
        sendMessage,
        refreshUserTokens,
    };
}
