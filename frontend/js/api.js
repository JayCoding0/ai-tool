// ===== API 请求封装 =====

// 获取认证请求头
function getAuthHeaders() {
    const token = localStorage.getItem('auth_token');
    const headers = { 'Content-Type': 'application/json' };
    if (token) headers['Authorization'] = 'Bearer ' + token;
    return headers;
}

// 加载模型列表
async function apiLoadModels() {
    const resp = await fetch('/api/models');
    if (!resp.ok) throw new Error('加载模型失败');
    return await resp.json();
}

// 加载工具列表
async function apiLoadTools() {
    const resp = await fetch('/api/tools', { headers: getAuthHeaders() });
    if (!resp.ok) throw new Error('加载工具失败');
    return await resp.json();
}

// 加载会话列表
async function apiLoadSessions() {
    const resp = await fetch('/api/sessions', { headers: getAuthHeaders() });
    if (!resp.ok) throw new Error('加载会话失败');
    return await resp.json();
}

// 加载会话消息详情
async function apiLoadSessionDetail(sid) {
    const resp = await fetch('/api/history', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ session_id: sid })
    });
    if (!resp.ok) throw new Error('加载会话详情失败');
    return await resp.json();
}

// 重命名会话
async function apiRenameSession(sessionId, title) {
    const resp = await fetch('/api/sessions/rename', {
        method: 'POST',
        headers: getAuthHeaders(),
        body: JSON.stringify({ session_id: sessionId, title })
    });
    if (!resp.ok) throw new Error('重命名失败');
    return await resp.json();
}

// 删除会话
async function apiDeleteSession(sessionId) {
    const resp = await fetch('/api/sessions/delete', {
        method: 'POST',
        headers: getAuthHeaders(),
        body: JSON.stringify({ session_id: sessionId })
    });
    if (!resp.ok) throw new Error('删除失败');
    return await resp.json();
}

// 获取当前用户信息
async function apiGetUserInfo() {
    const resp = await fetch('/api/auth/me', { headers: getAuthHeaders() });
    if (!resp.ok) throw new Error('获取用户信息失败');
    return await resp.json();
}

// 退出登录
async function apiLogout() {
    const token = localStorage.getItem('auth_token');
    await fetch('/api/auth/logout', {
        method: 'POST',
        headers: token ? { 'Authorization': 'Bearer ' + token } : {}
    });
}

// 发送流式聊天请求（返回 Response 对象）
async function apiChatStream(payload, signal) {
    const resp = await fetch('/api/chat/stream', {
        method: 'POST',
        headers: getAuthHeaders(),
        signal,
        body: JSON.stringify(payload)
    });
    if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
    return resp;
}
