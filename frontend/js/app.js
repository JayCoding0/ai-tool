const { createApp, ref, computed, nextTick, onMounted, watch } = Vue;
const { ElMessage, ElMessageBox } = ElementPlus;

const APP_TEMPLATE = `
<div style="display:flex;height:100vh;overflow:hidden;">
    <!-- 侧边栏 -->
    <aside class="sidebar">
        <div class="sidebar-header">
            <div class="logo-wrap">
                <div class="logo-icon">🤖</div>
                <span class="logo-name">智能小助手</span>
            </div>
        </div>

        <button class="new-chat-btn" @click="newSession">
            <svg width="14" height="14" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2.5" d="M12 5v14M5 12h14"/>
            </svg>
            新建对话
        </button>

        <div class="sidebar-section">
            <div class="section-label">最近对话</div>
            <template v-if="sidebarSessions.length > 0">
                <div v-for="s in sidebarSessions" :key="s.id"
                    class="session-item"
                    @click="openSessionDetail(s.id)">
                    <svg width="14" height="14" fill="none" stroke="currentColor" viewBox="0 0 24 24" style="flex-shrink:0;opacity:0.5;">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.8" d="M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z"/>
                    </svg>
                    <span class="s-title">{{ s.title || '新对话' }}</span>
                    <div class="s-actions" @click.stop>
                        <button class="s-action-btn" title="重命名" @click.stop="renameSession(s)">
                            <svg width="12" height="12" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z"/></svg>
                        </button>
                        <button class="s-action-btn del" title="删除" @click.stop="deleteSession(s)">
                            <svg width="12" height="12" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"/></svg>
                        </button>
                    </div>
                </div>
            </template>
            <div v-else class="sidebar-empty">暂无历史记录</div>
        </div>

        <div class="sidebar-footer">
            <div class="model-row">
                <div class="model-row-label">当前模型</div>
                <el-select v-model="selectedModel" size="small" @change="onModelChange" placeholder="选择模型">
                    <el-option-group v-if="cloudModels.length > 0" label="☁️ 云端模型">
                        <el-option v-for="m in cloudModels" :key="m.name" :label="m.label" :value="m.name" />
                    </el-option-group>
                    <el-option-group v-if="localModels.length > 0" label="🖥️ 本地模型">
                        <el-option v-for="m in localModels" :key="m.name" :label="m.label" :value="m.name" />
                    </el-option-group>
                </el-select>
            </div>
            <div class="user-row" @click="username ? logout() : goLogin()">
                <div class="user-avatar" :style="username ? '' : 'background:linear-gradient(135deg,#b0b8c1,#8a9bb0);'">
                    <span v-if="username">{{ userAvatar }}</span>
                    <span v-else>
                        <svg width="16" height="16" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z"/></svg>
                    </span>
                </div>
                <div style="flex:1;min-width:0;">
                    <div class="user-name-text">{{ username || '游客模式' }}</div>
                    <div v-if="username" class="user-sub">
                        <span>⚡ {{ formatNumber(userTotalTokens) }} tokens</span>
                    </div>
                    <div v-else class="user-sub">点击去登录</div>
                </div>
                <button class="logout-btn" :title="username ? '退出登录' : '去登录'">
                    <svg width="15" height="15" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1"/>
                    </svg>
                </button>
            </div>
        </div>
    </aside>

    <!-- 主区域 -->
    <main class="main-area">
        <!-- 顶栏 -->
        <header class="chat-topbar">
            <div style="display:flex;align-items:center;gap:12px;">
                <span class="topbar-title">{{ currentSessionTitle }}</span>
                <div class="topbar-status">
                    <span class="status-dot" :style="{background: statusColor}"></span>
                    <span>{{ statusText }}</span>
                </div>
                <div v-if="roundCount > 0" class="topbar-round">
                    💬 {{ roundCount }} 轮对话
                </div>
            </div>
            <div class="topbar-actions">
                <button class="topbar-btn" @click="historyVisible = true">
                    <svg width="13" height="13" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"/>
                    </svg>
                    历史记录
                </button>
                <button class="topbar-btn" @click="skillsVisible = true" title="技能库">
                    <svg width="13" height="13" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z"/>
                    </svg>
                    Skills
                </button>
                <!-- 知识库选择器 -->
                <div style="position:relative;display:inline-block;">
                    <button class="topbar-btn" :class="{active: selectedKbId > 0}" @click="kbSelectorVisible = !kbSelectorVisible" title="选择知识库（RAG）">
                        📚 {{ selectedKbId > 0 ? (getSelectedKB() ? getSelectedKB().name : 'KB') : '知识库' }}
                    </button>
                    <div v-if="kbSelectorVisible" style="position:absolute;top:calc(100% + 6px);right:0;z-index:1000;background:#fff;border:1px solid #e8e8f0;border-radius:10px;box-shadow:0 4px 20px rgba(0,0,0,.12);min-width:220px;overflow:hidden;" @click.stop>
                        <div style="padding:8px 12px;font-size:11px;color:#999;border-bottom:1px solid #f0f0f0;font-weight:600;letter-spacing:.5px;">选择知识库（RAG）</div>
                        <div style="max-height:240px;overflow-y:auto;">
                            <div style="display:flex;align-items:center;gap:8px;padding:9px 14px;cursor:pointer;transition:background .15s;"
                                :style="selectedKbId === 0 ? 'background:#f0f0ff;' : ''"
                                @click="selectKnowledgeBase(0)">
                                <span style="font-size:14px;">🚫</span>
                                <div>
                                    <div style="font-size:13px;color:#333;font-weight:500;">不使用知识库</div>
                                    <div style="font-size:11px;color:#bbb;">纯模型回答</div>
                                </div>
                                <span v-if="selectedKbId === 0" style="margin-left:auto;color:#667eea;font-size:12px;">✓</span>
                            </div>
                            <div v-if="kbLoading" style="text-align:center;padding:20px;color:#bbb;font-size:12px;">加载中...</div>
                            <div v-else-if="knowledgeBases.length === 0" style="text-align:center;padding:16px;color:#bbb;font-size:12px;">
                                暂无知识库，<a href="/knowledge.html" target="_blank" style="color:#667eea;">去创建</a>
                            </div>
                            <div v-for="kb in knowledgeBases" :key="kb.id"
                                style="display:flex;align-items:center;gap:8px;padding:9px 14px;cursor:pointer;transition:background .15s;"
                                :style="selectedKbId === kb.id ? 'background:#f0f0ff;' : ''"
                                @click="selectKnowledgeBase(kb.id)">
                                <span style="font-size:14px;">📚</span>
                                <div style="flex:1;min-width:0;">
                                    <div style="font-size:13px;color:#333;font-weight:500;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">{{ kb.name }}</div>
                                    <div style="font-size:11px;color:#bbb;">{{ kb.doc_count }} 文档 · {{ kb.chunk_count }} 分块</div>
                                </div>
                                <span v-if="selectedKbId === kb.id" style="color:#667eea;font-size:12px;flex-shrink:0;">✓</span>
                            </div>
                        </div>
                        <div style="padding:8px 12px;border-top:1px solid #f0f0f0;">
                            <a href="/knowledge.html" target="_blank" style="font-size:12px;color:#667eea;text-decoration:none;">⚙️ 管理知识库</a>
                        </div>
                    </div>
                </div>
                <button class="topbar-btn" @click="openSystemPrompt" :class="{active: currentSystemPrompt}" :title="currentSystemPrompt ? 'System Prompt 已设置' : '设置 System Prompt'">
                    <svg width="13" height="13" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"/>
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"/>
                    </svg>
                    {{ currentSystemPrompt ? 'Prompt ✓' : 'System Prompt' }}
                </button>
                <button class="topbar-btn danger" @click="clearMessages">
                    <svg width="13" height="13" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"/>
                    </svg>
                    清空
                </button>
            </div>
        </header>

        <!-- 消息体 -->
        <div class="chat-body" ref="messagesEl">
            <div v-if="messages.length === 0" class="welcome-screen">
                <div class="welcome-icon">🤖</div>
                <div class="welcome-title">你好，我是智能小助手</div>
                <div class="welcome-sub">有什么我可以帮你的？</div>
                <div class="suggest-grid">
                    <div class="suggest-card" v-for="s in suggestions" :key="s.text" @click="useSuggestion(s.text)">
                        <div class="s-icon">{{ s.icon }}</div>
                        <div class="s-text">{{ s.text }}</div>
                    </div>
                </div>
            </div>
            <div v-else class="messages-list">
                <template v-for="(msg, idx) in messages" :key="idx">
                    <div v-if="msg.role === 'user'" class="msg-group">
                        <div class="msg-user">
                            <div class="msg-user-bubble">{{ msg.content }}</div>
                        </div>
                        <div class="msg-user-actions">
                            <button class="msg-action-btn" :class="{copied: msg.copied}" @click="copyMessage(msg)">
                                <svg width="11" height="11" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"/></svg>
                                {{ msg.copied ? '已复制' : '复制' }}
                            </button>
                        </div>
                    </div>
                    <div v-else class="msg-group">
                        <div class="msg-ai">
                            <div class="msg-ai-header">
                                <div class="ai-avatar">🤖</div>
                                <span class="ai-name">智能小助手</span>
                                <span v-if="msg.modelName" class="ai-model-tag">{{ msg.modelName }}</span>
                                <span v-if="msg.round" class="round-badge">第 {{ msg.round }} 轮</span>
                            </div>
                            <div v-if="msg.typing" class="msg-ai-content">
                                <div class="typing-indicator">
                                    <div class="typing-dot"></div>
                                    <div class="typing-dot"></div>
                                    <div class="typing-dot"></div>
                                </div>
                            </div>
                            <div v-else class="msg-ai-content">
                                <!-- ReAct 步骤列表 -->
                                <div v-if="msg.reactSteps && msg.reactSteps.length > 0" class="react-steps">
                                    <div class="react-summary" @click="msg.reactCollapsed = !msg.reactCollapsed; $forceUpdate()">
                                        <span class="react-summary-icon">⚡</span>
                                        <span class="react-summary-title">ReAct 推理链路</span>
                                        <div class="react-summary-badges">
                                            <span class="react-badge loop">
                                                🔄 {{ msg.reactSteps.filter(s => s.type === 'action').length }} 次工具调用
                                            </span>
                                            <span class="react-badge tool">
                                                🔁 第 {{ msg.reactSteps.filter(s=>s.type==='action').length > 0 ? Math.max(...msg.reactSteps.filter(s=>s.type==='action').map(s=>s.step||1)) : 0 }} 轮循环
                                            </span>
                                            <span v-if="msg.streaming" class="react-badge running">运行中...</span>
                                        </div>
                                        <span class="react-summary-toggle">{{ msg.reactCollapsed ? '▶ 展开' : '▼ 收起' }}</span>
                                    </div>
                                    <div v-show="!msg.reactCollapsed" class="react-timeline">
                                        <div v-for="(step, si) in msg.reactSteps" :key="si" class="react-step-item" :class="step.type">
                                            <!-- 思考步骤 -->
                                            <template v-if="step.type === 'thought'">
                                                <div class="react-step-header" @click="step.expanded = !step.expanded" style="cursor:pointer;">
                                                    <span class="react-step-icon thought-icon">🤔</span>
                                                    <span class="react-step-label">思考过程</span>
                                                    <span class="react-round-badge">第 {{ step.step }} 轮</span>
                                                    <span class="react-step-toggle">{{ step.expanded ? '▲' : '▼' }}</span>
                                                </div>
                                                <div v-if="step.expanded" class="react-step-body thought-body">{{ step.content }}</div>
                                            </template>
                                            <!-- 工具调用步骤 -->
                                            <template v-else-if="step.type === 'action'">
                                                <!-- call_agent 专属样式 -->
                                                <template v-if="step.isAgentCall">
                                                    <div class="react-step-header agent-call-header" @click="step.expanded = !step.expanded" style="cursor:pointer;">
                                                        <span class="react-step-icon">
                                                            <div v-if="step.status === 'calling'" class="tool-spinner" style="width:12px;height:12px;border-width:1.5px;flex-shrink:0;"></div>
                                                            <span v-else class="tool-done-icon">✓</span>
                                                        </span>
                                                        <span class="react-step-label">
                                                            <span v-if="step.status === 'calling'">🤖 正在调用子 Agent：</span>
                                                            <span v-else>🤖 已调用子 Agent：</span>
                                                            <strong class="agent-name-badge">{{ step.agentCallName }}</strong>
                                                        </span>
                                                        <span class="react-round-badge">第 {{ step.step }} 轮</span>
                                                        <span class="react-step-toggle">{{ step.expanded ? '▲' : '▼' }}</span>
                                                    </div>
                                                    <div v-if="step.expanded" class="react-step-body agent-call-body">
                                                        <div v-if="step.agentCallMsg" class="react-detail-block">
                                                            <div class="react-detail-label">📨 发送给子 Agent 的任务</div>
                                                            <div class="agent-task-text">{{ step.agentCallMsg }}</div>
                                                        </div>
                                                        <div v-if="step.result" class="react-detail-block">
                                                            <div class="react-detail-label">💬 子 Agent 回复</div>
                                                            <div class="agent-reply-text" v-html="renderMarkdown(step.result)"></div>
                                                        </div>
                                                        <div v-else-if="step.status === 'calling'" class="agent-waiting">
                                                            <div class="tool-spinner" style="width:10px;height:10px;border-width:1.5px;display:inline-block;vertical-align:middle;margin-right:6px;"></div>
                                                            等待子 Agent 响应...
                                                        </div>
                                                    </div>
                                                </template>
                                                <!-- 普通工具调用 -->
                                                <template v-else>
                                                    <div class="react-step-header" @click="step.expanded = !step.expanded" style="cursor:pointer;">
                                                        <span class="react-step-icon">
                                                            <div v-if="step.status === 'calling'" class="tool-spinner" style="width:12px;height:12px;border-width:1.5px;flex-shrink:0;"></div>
                                                            <span v-else class="tool-done-icon">✓</span>
                                                        </span>
                                                        <span class="react-step-label">
                                                            <span v-if="step.status === 'calling'">调用工具：</span>
                                                            <span v-else>已调用：</span>
                                                            <strong>{{ step.toolName }}</strong>
                                                            <span v-if="step.argsDisplay" class="react-step-args">{{ step.argsDisplay }}</span>
                                                        </span>
                                                        <span class="react-round-badge">第 {{ step.step }} 轮</span>
                                                        <span class="react-step-toggle">{{ step.expanded ? '▲' : '▼' }}</span>
                                                    </div>
                                                    <div v-if="step.expanded" class="react-step-body">
                                                        <div v-if="step.toolArgs" class="react-detail-block">
                                                            <div class="react-detail-label">📥 输入参数</div>
                                                            <pre class="react-detail-pre">{{ step.toolArgs }}</pre>
                                                        </div>
                                                        <div v-if="step.result" class="react-detail-block">
                                                            <div class="react-detail-label">📤 执行结果</div>
                                                            <pre class="react-detail-pre">{{ step.result }}</pre>
                                                        </div>
                                                    </div>
                                                </template>
                                            </template>
                                        </div>
                                    </div>
                                </div>
                                <!-- 思考块（模型原生 thinking） -->
                                <div v-if="msg.thinking" class="think-block">
                                    <div class="think-label">
                                        <svg width="11" height="11" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.663 17h4.673M12 3v1m6.364 1.636l-.707.707M21 12h-1M4 12H3m3.343-5.657l-.707-.707m2.828 9.9a5 5 0 117.072 0l-.548.547A3.374 3.374 0 0014 18.469V19a2 2 0 11-4 0v-.531c0-.895-.356-1.754-.988-2.386l-.548-.547z"/></svg>
                                        思考过程
                                    </div>
                                    {{ msg.thinking }}
                                </div>
                                <div v-if="msg.content" class="md-body" :class="{'streaming-cursor': msg.streaming}" v-html="renderMarkdown(msg.content)"></div>
                                <span v-else-if="msg.streaming && (!msg.reactSteps || msg.reactSteps.length === 0)" class="streaming-cursor"></span>
                            </div>
                            <div class="msg-ai-footer">
                                <span>{{ msg.time }}</span>
                                <span v-if="msg.totalTokens > 0" class="token-badge">
                                    <svg width="10" height="10" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z"/></svg>
                                    {{ msg.promptTokens }}↑ {{ msg.completionTokens }}↓ 共{{ msg.totalTokens }}
                                </span>
                                <span v-if="msg.sessionTotalTokens > 0" style="color:#bbb;">会话累计 {{ msg.sessionTotalTokens }} tokens</span>
                            </div>
                            <div v-if="!msg.typing && !msg.streaming" class="msg-actions">
                                <button class="msg-action-btn" :class="{copied: msg.copied}" @click="copyMessage(msg)">
                                    <svg width="11" height="11" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"/></svg>
                                    {{ msg.copied ? '已复制' : '复制' }}
                                </button>
                                <button class="msg-action-btn" @click="regenerate(idx)">
                                    <svg width="11" height="11" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"/></svg>
                                    重新生成
                                </button>
                            </div>
                        </div>
                    </div>
                </template>
            </div>
        </div>

        <!-- 输入区 -->
        <div class="chat-input-wrap">
            <div class="input-box">
                <textarea ref="inputEl" v-model="inputText"
                    rows="1"
                    placeholder="给智能小助手发消息..."
                    @keydown="onKeydown"
                    @input="autoResize"></textarea>
                <div class="input-actions">
                    <span class="input-left-tips">Enter 发送 · Shift+Enter 换行</span>
                    <div style="display:flex;align-items:center;gap:8px;">
                        <button v-if="sending" class="stop-btn" @click="stopGenerate">
                            <svg width="12" height="12" fill="currentColor" viewBox="0 0 24 24"><rect x="4" y="4" width="16" height="16" rx="2"/></svg>
                            停止生成
                        </button>
                        <button class="send-button" :disabled="!inputText.trim() || sending" @click="sendMessage">
                            <svg width="15" height="15" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2.2" d="M5 12h14M12 5l7 7-7 7"/>
                            </svg>
                        </button>
                    </div>
                </div>
            </div>
            <div class="input-disclaimer">内容由 AI 生成，仅供参考</div>
        </div>
    </main>

    <!-- System Prompt 设置弹窗 -->
    <div v-if="systemPromptVisible" style="position:fixed;inset:0;z-index:2000;display:flex;align-items:center;justify-content:center;">
        <div style="position:absolute;inset:0;background:rgba(0,0,0,0.5);" @click="systemPromptVisible = false"></div>
        <div style="position:relative;background:#fff;border-radius:12px;width:560px;max-width:90vw;box-shadow:0 8px 40px rgba(0,0,0,0.18);overflow:hidden;">
            <div style="display:flex;align-items:center;justify-content:space-between;padding:18px 24px 14px;border-bottom:1px solid #f0f0f0;">
                <span style="font-size:16px;font-weight:600;color:#1a1a1a;">设置 System Prompt</span>
                <span style="cursor:pointer;font-size:20px;color:#999;line-height:1;" @click="systemPromptVisible = false">×</span>
            </div>
            <div style="padding:20px 24px;">
                <div style="margin-bottom:12px;font-size:13px;color:#666;line-height:1.6;">
                    System Prompt 是给 AI 的"角色说明"，设置后本会话的 AI 将按照你的要求行事。<br>
                    <span style="color:#999;">留空则使用默认提示词（智能助手，中文回答）。</span>
                </div>
                <textarea
                    v-model="systemPromptInput"
                    rows="6"
                    maxlength="2000"
                    placeholder="例如：你是一个专业的 Go 语言工程师，只回答编程相关问题，回答时附带代码示例。"
                    style="width:100%;box-sizing:border-box;padding:10px 12px;border:1px solid #dcdfe6;border-radius:6px;font-size:13px;line-height:1.6;resize:none;outline:none;font-family:inherit;color:#333;transition:border-color 0.2s;"
                    @focus="$event.target.style.borderColor='#409eff'"
                    @blur="$event.target.style.borderColor='#dcdfe6'"
                ></textarea>
                <div style="text-align:right;font-size:12px;color:#999;margin-top:4px;">{{ (systemPromptInput||'').length }} / 2000</div>
            </div>
            <div style="display:flex;justify-content:flex-end;gap:10px;padding:14px 24px 18px;border-top:1px solid #f0f0f0;">
                <button @click="systemPromptVisible = false" style="padding:8px 18px;border:1px solid #dcdfe6;border-radius:6px;background:#fff;color:#606266;font-size:13px;cursor:pointer;">取消</button>
                <button @click="saveSystemPrompt" style="padding:8px 18px;border:none;border-radius:6px;background:#409eff;color:#fff;font-size:13px;cursor:pointer;font-weight:500;">保存</button>
            </div>
        </div>
    </div>

    <!-- 工具抽屉 -->
    <el-drawer v-model="skillsVisible" title="⚡ 工具 & Agent" size="500px" direction="rtl">
        <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:16px;">
            <div style="font-size:12px;color:#999;">模型将自主判断调用已启用的工具</div>
            <el-tag v-if="enabledCount > 0" size="small" type="success">{{ enabledCount }} 已启用</el-tag>
        </div>

        <div v-if="toolsLoading" style="text-align:center;padding:60px;color:#bbb;">
            <el-icon class="is-loading" style="font-size:24px;"><Loading /></el-icon>
        </div>
        <div v-else-if="multiAgentMode && agentList.length > 0" style="display:flex;flex-direction:column;gap:12px;">
            <div v-for="agent in agentList" :key="agent.name"
                :style="agent.is_master
                    ? 'border:1.5px solid #f0a020;border-radius:10px;overflow:hidden;background:#fff;box-shadow:0 1px 6px #f0a02019;'
                    : 'border:1.5px solid #409eff;border-radius:10px;overflow:hidden;background:#fff;box-shadow:0 1px 6px #409eff14;'">
                <div :style="agent.is_master
                    ? 'display:flex;align-items:center;justify-content:space-between;padding:10px 14px;background:#fff8ec;border-bottom:1px solid #f0d080;'
                    : 'display:flex;align-items:center;justify-content:space-between;padding:10px 14px;background:#f0f7ff;border-bottom:1px solid #c6dcff;'">
                    <div style="display:flex;align-items:center;gap:8px;">
                        <span style="font-size:20px;">{{ getAgentIcon(agent.name) }}</span>
                        <div>
                            <div :style="agent.is_master ? 'font-size:13px;font-weight:700;color:#b45309;' : 'font-size:13px;font-weight:700;color:#1d4ed8;'">
                                {{ agent.display_name || agent.name }}
                            </div>
                            <div style="font-size:11px;color:#888;margin-top:1px;max-width:240px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">{{ agent.description }}</div>
                        </div>
                    </div>
                    <div style="display:flex;align-items:center;gap:6px;">
                        <el-tag v-if="agent.is_master" size="small" effect="dark" color="#f59e0b" style="font-size:10px;border:none;color:#fff;">🎯 主调度</el-tag>
                        <el-tag v-else-if="agent.tools && agent.tools.length > 0" size="small" effect="plain" type="primary" style="font-size:10px;">{{ agent.tools.length }} 工具</el-tag>
                        <el-switch
                            v-if="!agent.is_master && agent.default_tools && agent.default_tools.length > 0"
                            :model-value="isAgentFullyEnabled(agent)"
                            @change="(val) => toggleAgentTools(agent, val)"
                            size="small"
                        />
                        <el-tag v-if="agent.is_master" size="small" effect="plain" style="font-size:10px;color:#92400e;border-color:#fcd34d;background:#fffbeb;">常驻</el-tag>
                    </div>
                </div>
                <div v-if="agent.tools && agent.tools.length > 0" style="padding:8px 10px;display:flex;flex-direction:column;gap:5px;">
                    <div v-for="tool in agent.tools" :key="tool.name"
                        :style="getToolItemStyle(agent.is_master, tool.name)">
                        <div style="display:flex;align-items:center;gap:8px;flex:1;min-width:0;">
                            <span style="font-size:15px;flex-shrink:0;">{{ getToolIcon(tool.name) }}</span>
                            <div style="min-width:0;">
                                <div style="font-size:12px;font-weight:500;color:#333;">{{ tool.display_name || tool.name }}</div>
                                <div style="font-size:10px;color:#bbb;font-family:monospace;" v-if="tool.display_name">{{ tool.name }}</div>
                                <div style="font-size:11px;color:#999;line-height:1.4;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;max-width:260px;" v-if="tool.description">{{ tool.description }}</div>
                            </div>
                        </div>
                        <el-switch
                            v-if="!agent.is_master"
                            :model-value="isToolEnabled(tool.name)"
                            @change="toggleTool(tool.name)"
                            size="small"
                            style="margin-left:10px;flex-shrink:0;"
                        />
                        <span v-else style="margin-left:10px;font-size:12px;color:#f59e0b;flex-shrink:0;" title="主调度工具，常驻启用">🔒</span>
                    </div>
                </div>
                <div v-else style="padding:12px 14px;font-size:12px;color:#bbb;text-align:center;">暂无工具</div>
            </div>
        </div>
        <div v-else-if="!multiAgentMode && availableTools.length > 0" style="display:flex;flex-direction:column;gap:8px;">
            <div v-for="tool in availableTools" :key="tool.name"
                style="display:flex;align-items:flex-start;justify-content:space-between;padding:10px 12px;background:#fff;border-radius:8px;border:1px solid transparent;"
                :style="isToolEnabled(tool.name) ? 'border-color:#67c23a;background:#f0f9eb;' : 'border-color:#eee;'">
                <div style="flex:1;min-width:0;">
                    <div style="font-size:13px;font-weight:500;color:#333;margin-bottom:2px;">
                        <span style="margin-right:5px;">{{ getToolIcon(tool.name) }}</span>{{ tool.display_name || tool.name }}
                    </div>
                    <div style="font-size:11px;color:#bbb;margin-bottom:2px;" v-if="tool.display_name">{{ tool.name }}</div>
                    <div style="font-size:11px;color:#999;line-height:1.5;">{{ tool.description }}</div>
                </div>
                <el-switch
                    :model-value="isToolEnabled(tool.name)"
                    @change="toggleTool(tool.name)"
                    style="margin-left:12px;flex-shrink:0;"
                />
            </div>
        </div>
        <div v-else style="text-align:center;padding:40px;color:#bbb;font-size:13px;">暂无已注册工具</div>

        <div style="margin-top:18px;padding-top:14px;border-top:1px dashed #eee;text-align:center;">
            <el-button size="small" plain @click="openImportDialog" style="font-size:12px;color:#666;">
                <el-icon style="margin-right:4px;"><Plus /></el-icon>导入其他工具
            </el-button>
        </div>
    </el-drawer>

    <!-- 导入其他工具对话框 -->
    <el-dialog v-model="importDialogVisible" title="导入其他工具到 Agent" width="480px" :close-on-click-modal="false">
        <div style="margin-bottom:14px;">
            <div style="font-size:12px;color:#666;margin-bottom:6px;font-weight:500;">导入到 Agent：</div>
            <el-select v-model="importTargetAgent" placeholder="请选择目标 Agent" style="width:100%;" size="default">
                <el-option
                    v-for="agent in agentList.filter(a => !a.is_master)"
                    :key="agent.name"
                    :label="(getAgentIcon(agent.name) + ' ' + (agent.display_name || agent.name))"
                    :value="agent.name"
                />
            </el-select>
        </div>
        <div style="font-size:12px;color:#666;margin-bottom:8px;font-weight:500;">
            选择要导入的工具
            <span v-if="importPendingTools.length > 0" style="color:#409eff;margin-left:6px;">已选 {{ importPendingTools.length }} 个</span>
        </div>
        <div v-if="importLoading" style="text-align:center;padding:40px;color:#bbb;">
            <el-icon class="is-loading" style="font-size:22px;"><Loading /></el-icon>
        </div>
        <div v-else-if="importableTools.length === 0" style="text-align:center;padding:30px;color:#bbb;font-size:13px;">
            所有已注册工具均已归属 Agent，暂无可导入工具
        </div>
        <div v-else style="display:flex;flex-direction:column;gap:6px;max-height:360px;overflow-y:auto;">
            <div v-for="tool in importableTools" :key="tool.name"
                style="display:flex;align-items:center;justify-content:space-between;padding:10px 12px;border-radius:8px;border:1px solid #eee;background:#fafafa;cursor:pointer;transition:all 0.15s;"
                :style="isImportPending(tool.name) ? 'border-color:#409eff;background:#ecf5ff;' : 'border-color:#eee;background:#fafafa;'"
                @click="toggleImportPending(tool)">
                <div style="display:flex;align-items:center;gap:8px;flex:1;min-width:0;">
                    <span style="font-size:16px;flex-shrink:0;">{{ getToolIcon(tool.name) }}</span>
                    <div style="min-width:0;">
                        <div style="font-size:13px;font-weight:500;color:#333;">{{ tool.display_name || tool.name }}</div>
                        <div style="font-size:10px;color:#bbb;font-family:monospace;" v-if="tool.display_name">{{ tool.name }}</div>
                        <div style="font-size:11px;color:#999;line-height:1.4;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;max-width:280px;" v-if="tool.description">{{ tool.description }}</div>
                    </div>
                </div>
                <el-checkbox
                    :model-value="isImportPending(tool.name)"
                    @change="toggleImportPending(tool)"
                    @click.stop
                    style="margin-left:10px;flex-shrink:0;"
                />
            </div>
        </div>
        <template #footer>
            <div style="display:flex;justify-content:flex-end;gap:8px;">
                <el-button @click="importDialogVisible = false" size="default">取消</el-button>
                <el-button
                    type="primary"
                    size="default"
                    :disabled="importPendingTools.length === 0 || !importTargetAgent"
                    @click="confirmImport">
                    确定导入（{{ importPendingTools.length }}）
                </el-button>
            </div>
        </template>
    </el-dialog>

    <!-- 历史记录抽屉 -->
    <el-drawer v-model="historyVisible" title="历史会话" size="440px" direction="rtl">
        <template v-if="!historyDetail">
            <div v-if="historyLoading" style="text-align:center;padding:60px;color:#bbb;">
                <el-icon class="is-loading" style="font-size:24px;"><Loading /></el-icon>
            </div>
            <div v-else-if="historySessions.length === 0" style="text-align:center;padding:60px;color:#bbb;font-size:13px;">暂无历史会话</div>
            <div v-else>
                <div v-for="s in historySessions" :key="s.id"
                    class="history-session-card"
                    @click="loadSessionDetail(s.id)">
                    <div class="history-session-icon">
                        <svg width="16" height="16" fill="none" stroke="#4e6ef2" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z"/>
                        </svg>
                    </div>
                    <div style="flex:1;min-width:0;">
                        <div style="font-size:13px;font-weight:500;color:#1a1a1a;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">{{ s.title || '新对话' }}</div>
                        <div style="font-size:11px;color:#bbb;margin-top:2px;">{{ formatTime(s.updated_at) }}</div>
                    </div>
                    <svg width="14" height="14" fill="none" stroke="#ccc" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"/>
                    </svg>
                </div>
            </div>
        </template>
        <template v-else>
            <div style="display:flex;align-items:center;gap:8px;margin-bottom:20px;">
                <el-button text @click="historyDetail = null" style="padding:0;">
                    <svg width="16" height="16" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7"/>
                    </svg>
                    返回列表
                </el-button>
            </div>
            <div v-if="detailLoading" style="text-align:center;padding:60px;color:#bbb;">
                <el-icon class="is-loading" style="font-size:24px;"><Loading /></el-icon>
            </div>
            <div v-else>
                <div v-for="(msg, i) in historyDetail" :key="i" style="margin-bottom:16px;">
                    <div v-if="msg.role === 'user'" style="display:flex;justify-content:flex-end;">
                        <div style="max-width:78%;background:#4e6ef2;color:#fff;padding:10px 14px;border-radius:16px 16px 4px 16px;font-size:13px;line-height:1.6;word-break:break-word;">{{ msg.content }}</div>
                    </div>
                    <div v-else>
                        <div style="display:flex;align-items:center;gap:7px;margin-bottom:6px;">
                            <div style="width:22px;height:22px;background:linear-gradient(135deg,#4e6ef2,#7c5cfc);border-radius:6px;display:flex;align-items:center;justify-content:center;font-size:11px;">🤖</div>
                            <span style="font-size:12px;font-weight:600;color:#1a1a1a;">智能小助手</span>
                        </div>
                        <div style="padding-left:29px;font-size:13px;line-height:1.7;color:#333;white-space:pre-wrap;word-break:break-word;">{{ msg.content }}</div>
                    </div>
                </div>
            </div>
        </template>
    </el-drawer>
</div>
`;

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
        const knowledgeModule = useKnowledge();

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
                            let argsObj = {};
                            try {
                                argsObj = JSON.parse(event.tool_args || '{}');
                                argsDisplay = Object.entries(argsObj).map(([k,v]) => `${k}: ${v}`).join(', ');
                            } catch(e) { argsDisplay = event.tool_args || ''; }

                            // call_agent 工具特殊处理：提取子 Agent 名称和任务描述
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
                                // call_agent 专属字段
                                isAgentCall,
                                agentCallName,
                                agentCallMsg,
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
