// ===== Vue 模板 =====
// 从 app.js 中拆分出来的模板字符串，同时优化了 UI 交互

const APP_TEMPLATE = `
<div style="display:flex;height:100vh;overflow:hidden;">
    <!-- 侧边栏 -->
    <aside class="sidebar">
        <div class="sidebar-header">
            <div class="logo-wrap">
                <div class="logo-icon">🤖</div>
                <span class="logo-name">智能Agent平台</span>
            </div>
        </div>

        <button class="new-chat-btn" @click="newSession">
            <svg width="14" height="14" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2.5" d="M12 5v14M5 12h14"/>
            </svg>
            新建对话
        </button>

        <!-- 侧边栏搜索 -->
        <div class="sidebar-search">
            <el-input
                v-model="sessionSearchQuery"
                placeholder="搜索会话..."
                size="small"
                clearable
                prefix-icon="Search"
            />
        </div>

        <div class="sidebar-section">
            <div class="section-label">最近对话</div>
            <template v-if="filteredSidebarSessions.length > 0">
                <div v-for="s in filteredSidebarSessions" :key="s.id"
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
                        <button class="s-action-btn" title="导出 Markdown" @click.stop="exportSession(s)">
                            <svg width="12" height="12" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v2a2 2 0 002 2h12a2 2 0 002-2v-2M7 10l5 5 5-5M12 15V3"/></svg>
                        </button>
                        <button class="s-action-btn del" title="删除" @click.stop="deleteSession(s)">
                            <svg width="12" height="12" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"/></svg>
                        </button>
                    </div>
                </div>
            </template>
            <div v-else-if="sessionSearchQuery" class="sidebar-empty">未找到匹配的会话</div>
            <div v-else class="sidebar-empty">暂无历史记录</div>
        </div>

        <div class="sidebar-footer">
            <details class="footer-nav-details">
                <summary class="footer-nav-summary">
                    <span class="fn-summary-label">🧩 扩展功能</span>
                    <span class="fn-summary-arrow">▾</span>
                </summary>
                <div class="footer-nav">
                    <a href="/knowledge.html" class="footer-nav-link">📚 知识库</a>
                    <a href="/workflow.html" class="footer-nav-link">🔀 工作流</a>
                    <a href="/eval.html" class="footer-nav-link">📊 评估</a>
                    <a href="/memory.html" class="footer-nav-link">🧠 记忆</a>
                    <a href="/cache.html" class="footer-nav-link">⚡ 缓存</a>
                    <a href="/trace.html" class="footer-nav-link">🔬 调用链</a>
                    <a href="/mcp.html" class="footer-nav-link">🔌 MCP</a>
                </div>
            </details>
            <div class="theme-toggle-row">
                <span>外观</span>
                <button class="theme-toggle-btn" @click="toggleTheme">
                    <span class="theme-icon">{{ isDarkTheme ? '☀️' : '🌙' }}</span>
                    <span>{{ isDarkTheme ? '浅色' : '深色' }}</span>
                </button>
            </div>
            <div class="model-row">
                <div class="model-row-label">当前模型</div>
                <el-select v-model="selectedModel" size="small" @change="onModelChange" placeholder="选择模型" popper-class="dark-select-popper">
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
                <div class="kb-selector-wrap">
                    <button class="topbar-btn" :class="{active: selectedKbId > 0}" @click.stop="kbSelectorVisible = !kbSelectorVisible" title="选择知识库（RAG）">
                        📚 {{ selectedKbId > 0 ? (getSelectedKB() ? getSelectedKB().name : 'KB') : '知识库' }}
                    </button>
                    <div v-if="kbSelectorVisible" class="kb-dropdown" @click.stop>
                        <div class="kb-dropdown-header">选择知识库（RAG）</div>
                        <div class="kb-dropdown-body">
                            <div class="kb-dropdown-item" :class="{selected: selectedKbId === 0}" @click="selectKnowledgeBase(0)">
                                <span class="kb-dropdown-icon">🚫</span>
                                <div>
                                    <div class="kb-dropdown-name">不使用知识库</div>
                                    <div class="kb-dropdown-desc">纯模型回答</div>
                                </div>
                                <span v-if="selectedKbId === 0" class="kb-dropdown-check">✓</span>
                            </div>
                            <div v-if="kbLoading" class="kb-dropdown-loading">加载中...</div>
                            <div v-else-if="knowledgeBases.length === 0" class="kb-dropdown-empty">
                                暂无知识库，<a href="/knowledge.html" target="_blank" style="color:var(--primary);">去创建</a>
                            </div>
                            <div v-for="kb in knowledgeBases" :key="kb.id"
                                class="kb-dropdown-item" :class="{selected: selectedKbId === kb.id}"
                                @click="selectKnowledgeBase(kb.id)">
                                <span class="kb-dropdown-icon">📚</span>
                                <div class="kb-dropdown-info">
                                    <div class="kb-dropdown-name">{{ kb.name }}</div>
                                    <div class="kb-dropdown-desc">{{ kb.doc_count }} 文档 · {{ kb.chunk_count }} 分块</div>
                                </div>
                                <span v-if="selectedKbId === kb.id" class="kb-dropdown-check">✓</span>
                            </div>
                        </div>
                        <div class="kb-dropdown-footer">
                            <a href="/knowledge.html" target="_blank" style="font-size:12px;color:var(--primary);text-decoration:none;">⚙️ 管理知识库</a>
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
        <div class="chat-body" ref="messagesEl" @scroll="onChatScroll">
            <div v-if="messages.length === 0" class="welcome-screen">
                <div class="welcome-icon">🤖</div>
                <div class="welcome-title">你好，欢迎使用智能Agent平台</div>
                <div class="welcome-sub">多Agent协同 · 工具调用 · 知识库RAG · 有什么可以帮你的？</div>
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
                                <span class="ai-name">智能Agent</span>
                                <span v-if="msg.modelName" class="ai-model-tag">{{ msg.modelName }}</span>
                                <span v-if="msg.ragKbName" class="rag-badge">📚 {{ msg.ragKbName }}</span>
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
                                        <!-- 按 Agent 分组显示 -->
                                        <template v-for="(group, gi) in groupReactSteps(msg.reactSteps)" :key="gi">
                                            <!-- Agent 分组卡片 -->
                                            <template v-if="group.type === 'agent-group'">
                                                <div class="react-agent-group">
                                                    <div class="react-agent-group-header" @click="group.collapsed = !group.collapsed; $forceUpdate()">
                                                        <span class="react-agent-group-icon">{{ getAgentIcon(group.agentName) }}</span>
                                                        <span class="react-agent-group-name">{{ getAgentDisplayName(group.agentName) }}</span>
                                                        <span class="react-agent-group-count">{{ group.steps.length }} 次调用</span>
                                                        <span class="react-agent-group-rounds">第 {{ group.rounds.join('、') }} 轮</span>
                                                        <span class="react-step-toggle">{{ group.collapsed ? '▶ 展开' : '▼ 收起' }}</span>
                                                    </div>
                                                    <div v-show="!group.collapsed" class="react-agent-group-body">
                                                        <div v-for="(step, si) in group.steps" :key="si" class="react-step-item action" style="margin-left:0;">
                                                    <div class="react-step-header agent-call-header" @click="step.expanded = !step.expanded" style="cursor:pointer;">
                                                        <span class="react-step-icon">
                                                            <div v-if="step.status === 'calling'" class="tool-spinner" style="width:12px;height:12px;border-width:1.5px;flex-shrink:0;"></div>
                                                            <span v-else class="tool-done-icon">✓</span>
                                                        </span>
                                                        <span class="react-step-label">
                                                            <span v-if="step.status === 'calling'">🤖 正在调用：</span>
                                                            <span v-else>🤖 已调用：</span>
                                                            <strong class="agent-name-badge">{{ getAgentDisplayName(step.agentCallName) }}</strong>
                                                        </span>
                                                        <span class="react-round-badge">第 {{ step.step }} 轮</span>
                                                        <span class="react-step-toggle">{{ step.expanded ? '▲' : '▼' }}</span>
                                                    </div>
                                                    <div v-if="step.expanded" class="react-step-body agent-call-body">
                                                        <div v-if="step.agentCallMsg" class="react-detail-block">
                                                            <div class="react-detail-label">📨 发送给子 Agent 的任务</div>
                                                            <div class="agent-task-text">{{ step.agentCallMsg }}</div>
                                                        </div>
                                                        <!-- 子 Agent 内部工具调用明细 -->
                                                        <div v-if="step.subSteps && step.subSteps.length > 0" class="react-detail-block">
                                                            <div class="sub-agent-detail-label">
                                                                <span class="sub-agent-detail-icon">🔧</span>
                                                                <span>工具调用链路</span>
                                                                <span class="sub-agent-detail-count">{{ step.subSteps.filter(s => s.type !== 'thought').length }} 次调用</span>
                                                            </div>
                                                            <div class="sub-steps-list">
                                                                <div v-for="(sub, si) in step.subSteps" :key="si" class="sub-step-item" :class="{'sub-step-thought': sub.type === 'thought', 'sub-step-done': sub.status === 'done', 'sub-step-calling': sub.status === 'calling'}">
                                                                    <!-- 子 Agent 思考过程 -->
                                                                    <template v-if="sub.type === 'thought'">
                                                                        <div class="sub-step-header sub-thought-header" @click="sub.expanded = !sub.expanded; $forceUpdate()" style="cursor:pointer;">
                                                                            <span class="sub-step-icon sub-thought-icon">💭</span>
                                                                            <span class="sub-step-label" style="color:#7c5cfc;font-weight:500;">思考过程</span>
                                                                            <span class="sub-step-toggle">{{ sub.expanded ? '▲' : '▼' }}</span>
                                                                        </div>
                                                                        <div v-if="sub.expanded" class="sub-step-body sub-thought-body">
                                                                            <div class="sub-thought-content">{{ sub.content }}</div>
                                                                        </div>
                                                                    </template>
                                                                    <!-- 子 Agent 工具调用 -->
                                                                    <template v-else>
                                                                    <div class="sub-step-header" @click="sub.expanded = !sub.expanded; $forceUpdate()" style="cursor:pointer;">
                                                                        <span class="sub-step-icon">
                                                                            <div v-if="sub.status === 'calling'" class="tool-spinner sub-tool-spinner"></div>
                                                                            <span v-else class="sub-tool-done-icon">✓</span>
                                                                        </span>
                                                                        <span class="sub-step-label">
                                                                            <span class="sub-tool-name-badge">{{ sub.toolName }}</span>
                                                                            <span v-if="sub.argsDisplay" class="sub-tool-args-preview">{{ sub.argsDisplay }}</span>
                                                                        </span>
                                                                        <span v-if="sub.status === 'calling'" class="sub-step-status calling">执行中</span>
                                                                        <span v-else class="sub-step-status done">已完成</span>
                                                                        <span class="sub-step-toggle">{{ sub.expanded ? '▲' : '▼' }}</span>
                                                                    </div>
                                                                    <div v-if="sub.expanded" class="sub-step-body">
                                                                        <div v-if="sub.toolArgs" class="sub-detail-section">
                                                                            <div class="sub-detail-label"><span class="sub-detail-label-icon">📥</span> 输入参数</div>
                                                                            <pre class="sub-detail-pre">{{ sub.toolArgs }}</pre>
                                                                        </div>
                                                                        <div v-if="sub.result" class="sub-detail-section">
                                                                            <div class="sub-detail-label"><span class="sub-detail-label-icon">📤</span> 执行结果</div>
                                                                            <pre class="sub-detail-pre">{{ sub.result }}</pre>
                                                                        </div>
                                                                    </div>
                                                                    </template>
                                                                </div>
                                                            </div>
                                                        </div>
                                                        <!-- 子 Agent 回复 -->
                                                        <div v-if="step.result" class="sub-agent-reply-block">
                                                            <div class="sub-agent-reply-header">
                                                                <span class="sub-agent-reply-icon">💬</span>
                                                                <span class="sub-agent-reply-title">{{ getAgentDisplayName(step.agentCallName) }} 回复</span>
                                                            </div>
                                                            <div class="sub-agent-reply-content md-body" v-html="renderMarkdown(step.result)"></div>
                                                        </div>
                                                        <div v-else-if="step.status === 'calling'" class="agent-waiting">
                                                            <div class="tool-spinner" style="width:12px;height:12px;border-width:1.5px;display:inline-block;vertical-align:middle;margin-right:8px;"></div>
                                                            <span>等待 <strong>{{ getAgentDisplayName(step.agentCallName) }}</strong> 响应中...</span>
                                                        </div>
                                                    </div>
                                                        </div>
                                                    </div>
                                                </div>
                                            </template>
                                            <!-- 思考步骤 -->
                                            <template v-else-if="group.type === 'thought'">
                                                <div class="react-step-item thought">
                                                <div class="react-step-header" @click="group.step.expanded = !group.step.expanded" style="cursor:pointer;">
                                                    <span class="react-step-icon thought-icon">🤔</span>
                                                    <span class="react-step-label">思考过程</span>
                                                    <span class="react-round-badge">第 {{ group.step.step }} 轮</span>
                                                    <span class="react-step-toggle">{{ group.step.expanded ? '▲' : '▼' }}</span>
                                                </div>
                                                <div v-if="group.step.expanded" class="react-step-body thought-body">{{ group.step.content }}</div>
                                                </div>
                                            </template>
                                            <!-- 普通工具调用 -->
                                            <template v-else-if="group.type === 'action'">
                                                <div class="react-step-item action">
                                                    <div class="react-step-header" @click="group.step.expanded = !group.step.expanded" style="cursor:pointer;">
                                                        <span class="react-step-icon">
                                                            <div v-if="group.step.status === 'calling'" class="tool-spinner" style="width:12px;height:12px;border-width:1.5px;flex-shrink:0;"></div>
                                                            <span v-else class="tool-done-icon">✓</span>
                                                        </span>
                                                        <span class="react-step-label">
                                                            <span v-if="group.step.status === 'calling'">调用工具：</span>
                                                            <span v-else>已调用：</span>
                                                            <strong>{{ group.step.toolName }}</strong>
                                                            <span v-if="group.step.argsDisplay" class="react-step-args">{{ group.step.argsDisplay }}</span>
                                                        </span>
                                                        <span class="react-round-badge">第 {{ group.step.step }} 轮</span>
                                                        <span class="react-step-toggle">{{ group.step.expanded ? '▲' : '▼' }}</span>
                                                    </div>
                                                    <div v-if="group.step.expanded" class="react-step-body">
                                                        <div v-if="group.step.toolArgs" class="react-detail-block">
                                                            <div class="react-detail-label">📥 输入参数</div>
                                                            <pre class="react-detail-pre">{{ group.step.toolArgs }}</pre>
                                                        </div>
                                                        <div v-if="group.step.result" class="react-detail-block">
                                                            <div class="react-detail-label">📤 执行结果</div>
                                                            <pre class="react-detail-pre">{{ group.step.result }}</pre>
                                                        </div>
                                                    </div>
                                                </div>
                                            </template>
                                        </template>
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

            <!-- 回到底部按钮 -->
            <transition name="fade">
                <button v-if="showScrollBottom" class="scroll-bottom-btn" @click="scrollToBottomSmooth">
                    <svg width="16" height="16" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 14l-7 7m0 0l-7-7m7 7V3"/>
                    </svg>
                </button>
            </transition>
        </div>

        <!-- 输入区 -->
        <div class="chat-input-wrap">
            <div class="input-box">
                <textarea ref="inputEl" v-model="inputText"
                    rows="1"
                    placeholder="给智能Agent发消息..."
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

    <!-- System Prompt 设置弹窗（改用 el-dialog） -->
    <el-dialog v-model="systemPromptVisible" title="设置 System Prompt" width="560px" :close-on-click-modal="true">
        <div class="sp-dialog-hint">
            System Prompt 是给 AI 的"角色说明"，设置后本会话的 AI 将按照你的要求行事。<br>
            <span style="color:var(--text-muted);">留空则使用默认提示词（智能助手，中文回答）。</span>
        </div>
        <el-input
            v-model="systemPromptInput"
            type="textarea"
            :rows="6"
            maxlength="2000"
            show-word-limit
            placeholder="例如：你是一个专业的 Go 语言工程师，只回答编程相关问题，回答时附带代码示例。"
        />
        <template #footer>
            <el-button @click="systemPromptVisible = false">取消</el-button>
            <el-button type="primary" @click="saveSystemPrompt">保存</el-button>
        </template>
    </el-dialog>

    <!-- 工具抽屉 -->
    <el-drawer v-model="skillsVisible" title="⚡ 工具 & Agent" size="500px" direction="rtl">
        <div class="tools-drawer-header">
            <div class="tools-drawer-hint">模型将自主判断调用已启用的工具</div>
            <el-tag v-if="enabledCount > 0" size="small" type="success">{{ enabledCount }} 已启用</el-tag>
        </div>

        <div v-if="toolsLoading" class="tools-loading">
            <el-icon class="is-loading" style="font-size:24px;"><Loading /></el-icon>
        </div>
        <div v-else-if="multiAgentMode && agentList.length > 0" class="agent-list">
            <div v-for="agent in agentList" :key="agent.name"
                class="agent-card" :class="{'agent-card-master': agent.is_master}">
                <div class="agent-card-header">
                    <div class="agent-card-info">
                        <span class="agent-card-icon">{{ getAgentIcon(agent.name) }}</span>
                        <div>
                            <div class="agent-card-name" :class="{'master': agent.is_master}">
                                {{ getAgentDisplayName(agent.name) }}
                            </div>
                            <div class="agent-card-desc">{{ agent.description }}</div>
                        </div>
                    </div>
                    <div class="agent-card-actions">
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
                <div v-if="agent.tools && agent.tools.length > 0" class="agent-tools-list">
                    <div v-for="tool in agent.tools" :key="tool.name"
                        class="agent-tool-item" :class="{'enabled': isToolEnabled(tool.name), 'master-tool': agent.is_master}">
                        <div class="agent-tool-info">
                            <span class="agent-tool-icon">{{ getToolIcon(tool.name) }}</span>
                            <div class="agent-tool-detail">
                                <div class="agent-tool-name">{{ tool.display_name || tool.name }}</div>
                                <div class="agent-tool-raw-name" v-if="tool.display_name">{{ tool.name }}</div>
                                <div class="agent-tool-desc" v-if="tool.description">{{ tool.description }}</div>
                            </div>
                        </div>
                        <el-switch
                            v-if="!agent.is_master"
                            :model-value="isToolEnabled(tool.name)"
                            @change="toggleTool(tool.name)"
                            size="small"
                            style="margin-left:10px;flex-shrink:0;"
                        />
                        <span v-else class="master-lock-icon" title="主调度工具，常驻启用">🔒</span>
                    </div>
                </div>
                <div v-else class="agent-no-tools">暂无工具</div>
            </div>
        </div>
        <div v-else-if="!multiAgentMode && availableTools.length > 0" class="single-tools-list">
            <div v-for="tool in availableTools" :key="tool.name"
                class="single-tool-item" :class="{'enabled': isToolEnabled(tool.name)}">
                <div class="single-tool-info">
                    <div class="single-tool-name">
                        <span class="single-tool-icon">{{ getToolIcon(tool.name) }}</span>{{ tool.display_name || tool.name }}
                    </div>
                    <div class="single-tool-raw" v-if="tool.display_name">{{ tool.name }}</div>
                    <div class="single-tool-desc">{{ tool.description }}</div>
                </div>
                <el-switch
                    :model-value="isToolEnabled(tool.name)"
                    @change="toggleTool(tool.name)"
                    style="margin-left:12px;flex-shrink:0;"
                />
            </div>
        </div>
        <div v-else class="tools-empty">暂无已注册工具</div>

        <div class="tools-import-section">
            <el-button size="small" plain @click="openImportDialog" style="font-size:12px;color:#666;">
                <el-icon style="margin-right:4px;"><Plus /></el-icon>导入其他工具
            </el-button>
        </div>
    </el-drawer>

    <!-- 导入其他工具对话框 -->
    <el-dialog v-model="importDialogVisible" title="导入其他工具到 Agent" width="480px" :close-on-click-modal="false">
        <div class="import-target-section">
            <div class="import-label">导入到 Agent：</div>
            <el-select v-model="importTargetAgent" placeholder="请选择目标 Agent" style="width:100%;" size="default" popper-class="dark-select-popper">
                <el-option
                    v-for="agent in agentList.filter(a => !a.is_master)"
                    :key="agent.name"
                    :label="(getAgentIcon(agent.name) + ' ' + getAgentDisplayName(agent.name))"
                    :value="agent.name"
                />
            </el-select>
        </div>
        <div class="import-tools-header">
            选择要导入的工具
            <span v-if="importPendingTools.length > 0" class="import-selected-count">已选 {{ importPendingTools.length }} 个</span>
        </div>
        <div v-if="importLoading" class="import-loading">
            <el-icon class="is-loading" style="font-size:22px;"><Loading /></el-icon>
        </div>
        <div v-else-if="importableTools.length === 0" class="import-empty">
            所有已注册工具均已归属 Agent，暂无可导入工具
        </div>
        <div v-else class="import-tools-list">
            <div v-for="tool in importableTools" :key="tool.name"
                class="import-tool-item" :class="{'selected': isImportPending(tool.name)}"
                @click="toggleImportPending(tool)">
                <div class="import-tool-info">
                    <span class="import-tool-icon">{{ getToolIcon(tool.name) }}</span>
                    <div class="import-tool-detail">
                        <div class="import-tool-name">{{ tool.display_name || tool.name }}</div>
                        <div class="import-tool-raw" v-if="tool.display_name">{{ tool.name }}</div>
                        <div class="import-tool-desc" v-if="tool.description">{{ tool.description }}</div>
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
            <div v-if="historyLoading" class="history-loading">
                <el-icon class="is-loading" style="font-size:24px;"><Loading /></el-icon>
            </div>
            <div v-else-if="historySessions.length === 0" class="history-empty">暂无历史会话</div>
            <div v-else>
                <div v-for="s in historySessions" :key="s.id"
                    class="history-session-card"
                    @click="loadSessionDetail(s.id)">
                    <div class="history-session-icon">
                        <svg width="16" height="16" fill="none" stroke="#4e6ef2" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z"/>
                        </svg>
                    </div>
                    <div class="history-session-info">
                        <div class="history-session-title">{{ s.title || '新对话' }}</div>
                        <div class="history-session-time">{{ formatTime(s.updated_at) }}</div>
                    </div>
                    <svg width="14" height="14" fill="none" stroke="currentColor" style="color:var(--text-muted);" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"/>
                    </svg>
                </div>
            </div>
        </template>
        <template v-else>
            <div class="history-back-row">
                <el-button text @click="historyDetail = null" style="padding:0;">
                    <svg width="16" height="16" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7"/>
                    </svg>
                    返回列表
                </el-button>
            </div>
            <div v-if="detailLoading" class="history-loading">
                <el-icon class="is-loading" style="font-size:24px;"><Loading /></el-icon>
            </div>
            <div v-else>
                <div v-for="(msg, i) in historyDetail" :key="i" class="history-msg-item">
                    <div v-if="msg.role === 'user'" class="history-msg-user">
                        <div class="history-msg-user-bubble">{{ msg.content }}</div>
                    </div>
                    <div v-else>
                        <div class="history-msg-ai-header">
                            <div class="history-msg-ai-avatar">🤖</div>
                            <span class="history-msg-ai-name">智能Agent</span>
                        </div>
                        <div class="history-msg-ai-content">{{ msg.content }}</div>
                    </div>
                </div>
            </div>
        </template>
    </el-drawer>
</div>
`;
