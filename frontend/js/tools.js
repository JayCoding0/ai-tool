// ===== 工具相关逻辑 =====

// 工具图标映射
const TOOL_ICON_MAP = {
    'calculate': '🧮',
    'get_current_time': '🕐',
    'http_request': '🌐',
    'mysql_query': '🗄️',
    'write_file': '📝',
    'execute_command': '💻',
    'weather': '🌤️',
    'get_weather': '🌤️',
    'ip_lookup': '🔍',
    'get_public_ip': '🌐',
    'file_explorer': '📁',
    'list_directory': '📁',
    'query_database': '🗄️',
    'web_search': '🔎',
    'send_email': '📧',
    'read_file': '📖',
    'call_agent': '🤖',
    'search': '🔎',
    'map_search': '🗺️',
};

// Agent 图标映射
const AGENT_ICON_MAP = {
    'master_agent': '🎯',
    'weather_agent': '🌤️',
    'search_agent': '🔎',
    'code_agent': '💻',
};

// Agent 中文名映射
const AGENT_DISPLAY_NAME_MAP = {
    'master_agent': '主调度',
    'weather_agent': '天气助手',
    'search_agent': '搜索助手',
    'code_agent': '代码助手',
};

function getToolIcon(name) {
    return TOOL_ICON_MAP[name] || '🔧';
}

function getAgentIcon(name) {
    return AGENT_ICON_MAP[name] || '🤖';
}

function getAgentDisplayName(name) {
    return AGENT_DISPLAY_NAME_MAP[name] || name;
}



// 创建工具状态（供 Vue setup 使用）
function useTools() {
    const { ref, computed } = Vue;
    const availableTools = ref([]);   // 所有已注册工具（扁平列表，兼容旧逻辑）
    const agentList = ref([]);        // Agent 列表（含各自工具）
    // 多 Agent 模式：enabledTools 只包含主 Agent 的工具（call_agent），始终发给后端
    // 单 Agent 模式：enabledTools 包含用户选择的工具
    const enabledTools = ref([]);
    // 多 Agent 模式：子 Agent 工具的启用状态（仅前端展示，不发给后端）
    const subAgentEnabledTools = ref([]);
    const toolsLoading = ref(false);
    const multiAgentMode = ref(false); // 是否为多 Agent 模式
    let initialized = false;           // 是否已完成首次初始化

    // 导入其他工具相关状态
    const importDialogVisible = ref(false);
    const importableTools = ref([]);   // 所有已注册但未归属任何 Agent 的工具
    const importLoading = ref(false);
    const importTargetAgent = ref(''); // 导入目标 Agent 名称
    // 导入对话框中临时选中的工具（待确认）
    const importPendingTools = ref([]); // [{name, display_name, description}]

    // 加载 Agent 列表（多 Agent 模式）或工具列表（单 Agent 模式）
    async function loadTools() {
        toolsLoading.value = true;
        try {
            // 优先尝试加载 Agent 列表
            const agentData = await apiLoadAgents();
            if (agentData.agents && agentData.agents.length > 0) {
                multiAgentMode.value = true;
                agentList.value = agentData.agents;
                // 将所有 Agent 的工具合并为扁平列表（去重）
                const toolMap = new Map();
                for (const agent of agentData.agents) {
                    for (const tool of (agent.tools || [])) {
                        if (!toolMap.has(tool.name)) {
                            toolMap.set(tool.name, tool);
                        }
                    }
                }
                availableTools.value = Array.from(toolMap.values());

                // 仅首次加载时初始化，之后保留用户选择
                if (!initialized) {
                    initialized = true;
                    // 多 Agent 模式：enabledTools 只保留主 Agent 的工具（call_agent）
                    const masterAgent = agentData.agents.find(a => a.is_master);
                    if (masterAgent) {
                        enabledTools.value = [...(masterAgent.default_tools || [])];
                    } else {
                        enabledTools.value = [];
                    }
                    // 子 Agent 工具默认全部启用（展示用）
                    const subTools = new Set();
                    for (const agent of agentData.agents) {
                        if (!agent.is_master) {
                            for (const toolName of (agent.default_tools || [])) {
                                subTools.add(toolName);
                            }
                        }
                    }
                    subAgentEnabledTools.value = Array.from(subTools);
                }
                // 恢复持久化的导入工具（每次加载都需要合并）
                // 注：工具配置已由后端从数据库恢复，此处无需再从 localStorage 恢复
            } else {
                // 回退到单 Agent 模式：直接加载工具列表
                multiAgentMode.value = false;
                const data = await apiLoadTools();
                availableTools.value = data.tools || [];
                if (!initialized) {
                    initialized = true;
                    enabledTools.value = availableTools.value.map(t => t.name);
                }
            }
        } catch (e) {
            // 回退到单 Agent 模式
            try {
                multiAgentMode.value = false;
                const data = await apiLoadTools();
                availableTools.value = data.tools || [];
                if (!initialized) {
                    initialized = true;
                    enabledTools.value = availableTools.value.map(t => t.name);
                }
            } catch (e2) {}
        }
        toolsLoading.value = false;
    }

    // 加载可导入的其他工具（所有已注册工具中，不在任何 Agent 工具列表里的）
    async function loadImportableTools() {
        importLoading.value = true;
        try {
            const data = await apiLoadTools();
            const allTools = data.tools || [];
            // 收集已归属 Agent 的工具名
            const agentToolNames = new Set();
            for (const agent of agentList.value) {
                for (const tool of (agent.tools || [])) {
                    agentToolNames.add(tool.name);
                }
            }
            // 过滤出未归属的工具
            importableTools.value = allTools.filter(t => !agentToolNames.has(t.name));
        } catch (e) {
            importableTools.value = [];
        }
        importLoading.value = false;
    }

    // 打开导入工具对话框
    async function openImportDialog() {
        importPendingTools.value = [];
        // 默认选中第一个非主 Agent
        const firstSub = agentList.value.find(a => !a.is_master);
        importTargetAgent.value = firstSub ? firstSub.name : (agentList.value[0]?.name || '');
        importDialogVisible.value = true;
        await loadImportableTools();
    }

    // 切换导入对话框中工具的待选状态
    function toggleImportPending(tool) {
        const idx = importPendingTools.value.findIndex(t => t.name === tool.name);
        if (idx === -1) {
            importPendingTools.value.push(tool);
        } else {
            importPendingTools.value.splice(idx, 1);
        }
    }

    function isImportPending(toolName) {
        return importPendingTools.value.some(t => t.name === toolName);
    }

    // 确认导入：将待选工具加入目标 Agent，并持久化到后端
    async function confirmImport() {
        if (!importTargetAgent.value || importPendingTools.value.length === 0) {
            importDialogVisible.value = false;
            return;
        }
        const targetAgent = agentList.value.find(a => a.name === importTargetAgent.value);
        if (targetAgent) {
            // 先更新前端状态
            if (!targetAgent._importedTools) targetAgent._importedTools = [];
            for (const tool of importPendingTools.value) {
                // 加入 tools 展示列表（去重）
                if (!targetAgent.tools) targetAgent.tools = [];
                if (!targetAgent.tools.find(t => t.name === tool.name)) {
                    targetAgent.tools.push({
                        name: tool.name,
                        display_name: tool.display_name || tool.name,
                        description: tool.description || '',
                    });
                }
                // 加入 default_tools（用于开关控制）
                if (!targetAgent.default_tools) targetAgent.default_tools = [];
                if (!targetAgent.default_tools.includes(tool.name)) {
                    targetAgent.default_tools.push(tool.name);
                }
                // 默认启用该工具（加入子 Agent 启用列表）
                if (!subAgentEnabledTools.value.includes(tool.name)) {
                    subAgentEnabledTools.value.push(tool.name);
                }
                // 记录到 _importedTools（用于标记）
                if (!targetAgent._importedTools.find(t => t.name === tool.name)) {
                    targetAgent._importedTools.push({
                        name: tool.name,
                        display_name: tool.display_name || tool.name,
                        description: tool.description || '',
                    });
                }
            }

            // 调用后端接口持久化（将该 Agent 的完整工具列表发给后端）
            try {
                await apiUpdateAgentTools(targetAgent.name, targetAgent.default_tools);
            } catch (e) {
                console.warn('保存 Agent 工具配置到后端失败，仅前端生效:', e);
            }
        }
        importPendingTools.value = [];
        importDialogVisible.value = false;
    }

    // 单 Agent 模式下切换工具
    function toggleTool(toolName) {
        if (multiAgentMode.value) {
            // 多 Agent 模式：切换子 Agent 工具的展示启用状态
            const idx = subAgentEnabledTools.value.indexOf(toolName);
            if (idx === -1) {
                subAgentEnabledTools.value.push(toolName);
            } else {
                subAgentEnabledTools.value.splice(idx, 1);
            }
        } else {
            const idx = enabledTools.value.indexOf(toolName);
            if (idx === -1) {
                enabledTools.value.push(toolName);
            } else {
                enabledTools.value.splice(idx, 1);
            }
        }
    }

    // 启用/禁用某个 Agent 的所有工具
    function toggleAgentTools(agent, enable) {
        if (multiAgentMode.value && !agent.is_master) {
            // 多 Agent 模式下子 Agent：只影响 subAgentEnabledTools
            for (const toolName of (agent.default_tools || [])) {
                const idx = subAgentEnabledTools.value.indexOf(toolName);
                if (enable && idx === -1) {
                    subAgentEnabledTools.value.push(toolName);
                } else if (!enable && idx !== -1) {
                    subAgentEnabledTools.value.splice(idx, 1);
                }
            }
        } else {
            for (const toolName of (agent.default_tools || [])) {
                const idx = enabledTools.value.indexOf(toolName);
                if (enable && idx === -1) {
                    enabledTools.value.push(toolName);
                } else if (!enable && idx !== -1) {
                    enabledTools.value.splice(idx, 1);
                }
            }
        }
    }

    // 计算工具列表项的样式（主 Agent 橙金色，子 Agent 蓝色）
    function getToolItemStyle(isMaster, toolName) {
        const base = 'display:flex;align-items:center;justify-content:space-between;padding:7px 10px;border-radius:7px;transition:all 0.15s;cursor:default;';
        const enabled = isToolEnabled(toolName);
        if (isMaster) {
            return base + (enabled ? 'background:var(--sidebar-hover);border:1px solid #fcd34d;' : 'background:var(--card-bg);border:1px solid transparent;');
        }
        return base + (enabled ? 'background:var(--primary-light);border:1px solid #93c5fd;' : 'background:var(--card-bg);border:1px solid transparent;');
    }

    // 判断工具是否启用（多 Agent 模式下子 Agent 工具看 subAgentEnabledTools）
    function isToolEnabled(toolName) {
        if (multiAgentMode.value) {
            // 主 Agent 工具看 enabledTools，子 Agent 工具看 subAgentEnabledTools
            if (enabledTools.value.includes(toolName)) return true;
            return subAgentEnabledTools.value.includes(toolName);
        }
        return enabledTools.value.includes(toolName);
    }

    // 判断某个 Agent 的工具是否全部启用
    function isAgentFullyEnabled(agent) {
        const tools = agent.default_tools || [];
        if (tools.length === 0) return false;
        if (multiAgentMode.value && !agent.is_master) {
            return tools.every(t => subAgentEnabledTools.value.includes(t));
        }
        return tools.every(t => enabledTools.value.includes(t));
    }

    // 判断某个 Agent 的工具是否部分启用（用于半选状态）
    function isAgentPartiallyEnabled(agent) {
        const tools = agent.default_tools || [];
        if (tools.length === 0) return false;
        let enabledCount;
        if (multiAgentMode.value && !agent.is_master) {
            enabledCount = tools.filter(t => subAgentEnabledTools.value.includes(t)).length;
        } else {
            enabledCount = tools.filter(t => enabledTools.value.includes(t)).length;
        }
        return enabledCount > 0 && enabledCount < tools.length;
    }

    // 已启用工具总数（多 Agent 模式：主 Agent 工具数 + 子 Agent 启用工具数）
    const enabledCount = computed(() => {
        if (multiAgentMode.value) {
            return enabledTools.value.length + subAgentEnabledTools.value.length;
        }
        return enabledTools.value.length;
    });

    return {
        availableTools, agentList, enabledTools, subAgentEnabledTools, toolsLoading,
        multiAgentMode, enabledCount,
        importDialogVisible, importableTools, importLoading,
        importTargetAgent, importPendingTools,
        loadTools, toggleTool, toggleAgentTools,
        isToolEnabled, isAgentFullyEnabled, isAgentPartiallyEnabled,
        getToolItemStyle,
        openImportDialog, toggleImportPending, isImportPending, confirmImport,
    };
}
