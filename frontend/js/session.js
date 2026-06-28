// ===== 会话管理逻辑 =====

function useSessions() {
    const { ref } = Vue;
    const { ElMessage, ElMessageBox } = ElementPlus;

    const sessionId = ref(localStorage.getItem('chatSessionId') || null);
    const currentSessionTitle = ref('智能对话');
    const sidebarSessions = ref([]);

    // 历史抽屉
    const historyVisible = ref(false);
    const historyLoading = ref(false);
    const historySessions = ref([]);
    const historyDetail = ref(null);
    const detailLoading = ref(false);

    // 加载侧边栏会话列表
    async function loadSidebarSessions() {
        try {
            const data = await apiLoadSessions();
            sidebarSessions.value = (data.sessions || []).slice(0, 20);
        } catch (e) {
            sidebarSessions.value = [];
        }
    }

    // 打开历史抽屉
    async function openHistoryDrawer() {
        historyVisible.value = true;
        historyDetail.value = null;
        historyLoading.value = true;
        try {
            const data = await apiLoadSessions();
            historySessions.value = data.sessions || [];
        } catch (e) {
            historySessions.value = [];
        } finally {
            historyLoading.value = false;
        }
    }

    // 加载会话消息详情
    async function loadSessionDetail(sid) {
        historyDetail.value = [];
        detailLoading.value = true;
        try {
            const data = await apiLoadSessionDetail(sid);
            historyDetail.value = data.messages || [];
        } catch (e) {
            historyDetail.value = [];
        } finally {
            detailLoading.value = false;
        }
    }

    // 点击侧边栏会话
    function openSessionDetail(sid, onSystemPromptLoad) {
        historyVisible.value = true;
        historyDetail.value = null;
        historyLoading.value = true;
        apiLoadSessions()
            .then(data => {
                historySessions.value = data.sessions || [];
                historyLoading.value = false;
                const sess = (data.sessions || []).find(s => s.id === sid);
                if (sess && onSystemPromptLoad) {
                    onSystemPromptLoad(sess.system_prompt || '');
                }
                loadSessionDetail(sid);
            })
            .catch(() => { historyLoading.value = false; });
    }

    // 重命名会话
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
            await apiRenameSession(s.id, newTitle);
            s.title = newTitle;
            ElMessage({ message: '重命名成功', type: 'success', duration: 1500 });
        } catch (e) {
            ElMessage({ message: '重命名失败', type: 'error', duration: 2000 });
        }
    }

    // 删除会话
    async function deleteSession(s, currentSessionId, onNewSession) {
        try {
            await ElMessageBox.confirm(`确定要删除会话「${s.title || '新对话'}」吗？`, '删除确认', {
                confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning'
            });
        } catch { return; }
        try {
            await apiDeleteSession(s.id);
            if (currentSessionId === s.id) {
                onNewSession && onNewSession();
            } else {
                loadSidebarSessions();
            }
            ElMessage({ message: '会话已删除', type: 'success', duration: 1500 });
        } catch (e) {
            ElMessage({ message: '删除失败', type: 'error', duration: 2000 });
        }
    }

    // 导出会话为 Markdown
    function exportSession(s) {
        if (!s || !s.id) return;
        // 同源 GET 携带 Cookie 认证，浏览器直接下载
        window.open('/api/sessions/export?session_id=' + encodeURIComponent(s.id) + '&format=md', '_blank');
    }

    return {
        sessionId, currentSessionTitle, sidebarSessions,
        historyVisible, historyLoading, historySessions, historyDetail, detailLoading,
        loadSidebarSessions, openHistoryDrawer, loadSessionDetail, openSessionDetail,
        renameSession, deleteSession, exportSession,
    };
}
