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
    'ip_lookup': '🔍',
    'file_explorer': '📁',
    'query_database': '🗄️',
    'web_search': '🔎',
    'send_email': '📧',
    'read_file': '📖',
    'call_agent': '🤖',
    'search': '🔎',
    'map_search': '🗺️',
};

function getToolIcon(name) {
    return TOOL_ICON_MAP[name] || '🔧';
}

// 创建工具状态（供 Vue setup 使用）
function useTools() {
    const { ref } = Vue;
    const availableTools = ref([]);
    const enabledTools = ref([]);
    const toolsLoading = ref(false);

    async function loadTools() {
        toolsLoading.value = true;
        try {
            const data = await apiLoadTools();
            availableTools.value = data.tools || [];
            // 默认全部启用
            enabledTools.value = availableTools.value.map(t => t.name);
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
    }

    function isToolEnabled(toolName) {
        return enabledTools.value.includes(toolName);
    }

    return { availableTools, enabledTools, toolsLoading, loadTools, toggleTool, isToolEnabled };
}
