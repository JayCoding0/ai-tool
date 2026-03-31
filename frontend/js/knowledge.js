// ===== 知识库（RAG）相关逻辑 =====

function useKnowledge() {
    const { ref } = Vue;

    const knowledgeBases = ref([]);          // 知识库列表
    const selectedKbId = ref(0);             // 当前选中的知识库 ID（0 = 不启用 RAG）
    const kbLoading = ref(false);
    const kbSelectorVisible = ref(false);    // 知识库选择下拉是否展开

    // 加载知识库列表
    async function loadKnowledgeBases() {
        kbLoading.value = true;
        try {
            const resp = await fetch('/api/knowledge/bases', { headers: getAuthHeaders() });
            if (!resp.ok) return;
            const data = await resp.json();
            knowledgeBases.value = data.knowledge_bases || [];
        } catch (e) {
            // 知识库服务未启用时静默失败
        } finally {
            kbLoading.value = false;
        }
    }

    // 选择知识库（传 0 表示取消选择）
    function selectKnowledgeBase(id) {
        selectedKbId.value = id;
        kbSelectorVisible.value = false;
    }

    // 当前选中的知识库对象
    function getSelectedKB() {
        if (!selectedKbId.value) return null;
        return knowledgeBases.value.find(kb => kb.id === selectedKbId.value) || null;
    }

    return {
        knowledgeBases,
        selectedKbId,
        kbLoading,
        kbSelectorVisible,
        loadKnowledgeBases,
        selectKnowledgeBase,
        getSelectedKB,
    };
}
