// ===== 通用工具函数 =====

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

function scrollToBottom(messagesEl) {
    Vue.nextTick(() => {
        if (messagesEl && messagesEl.value) {
            messagesEl.value.scrollTop = messagesEl.value.scrollHeight;
        }
    });
}
