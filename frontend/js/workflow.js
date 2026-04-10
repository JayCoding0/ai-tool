// workflow.js — 工作流编排页面 JS 逻辑（从 workflow.html 拆分）
const { createApp, ref, reactive, computed, onMounted, watch, nextTick } = Vue;
createApp({
    setup() {
        // ===== 视图状态 =====
        const view = ref('list'); // 'list' | 'editor'

        // ===== 列表视图状态 =====
        const workflows = ref([]);
        const listLoading = ref(false);
        const showCreateDialog = ref(false);
        const creating = ref(false);
        const createForm = reactive({ name: '', description: '' });

        const statusLabel = (s) => ({ draft: '草稿', published: '已发布', archived: '已归档' }[s] || s);

        // 获取认证头
        function getAuthHeaders() {
            const token = localStorage.getItem('auth_token');
            const headers = { 'Content-Type': 'application/json' };
            if (token) headers['Authorization'] = 'Bearer ' + token;
            return headers;
        }

        // 加载工作流列表
        const loadWorkflows = async () => {
            listLoading.value = true;
            try {
                const r = await fetch('/api/workflows', { headers: getAuthHeaders() });
                const data = await r.json();
                workflows.value = data.workflows || [];
            } catch (e) {
                ElementPlus.ElMessage.error('加载工作流列表失败');
            } finally {
                listLoading.value = false;
            }
        };

        // 创建工作流
        const createWorkflow = async () => {
            if (!createForm.name.trim()) {
                ElementPlus.ElMessage.warning('请输入工作流名称');
                return;
            }
            creating.value = true;
            try {
                const r = await fetch('/api/workflows', {
                    method: 'POST',
                    headers: getAuthHeaders(),
                    body: JSON.stringify({
                        name: createForm.name,
                        description: createForm.description,
                        nodes: [
                            { id: 'start', type: 'start', name: '开始', config: {}, position: { x: 100, y: 200 } },
                            { id: 'end', type: 'end', name: '结束', config: {}, position: { x: 600, y: 200 } }
                        ],
                        edges: [],
                        variables: []
                    })
                });
                if (!r.ok) {
                    const err = await r.json();
                    throw new Error(err.error || '创建失败');
                }
                const wf = await r.json();
                ElementPlus.ElMessage.success('工作流创建成功');
                showCreateDialog.value = false;
                createForm.name = '';
                createForm.description = '';
                await loadWorkflows();
                // 自动打开编辑器
                const created = workflows.value.find(w => w.name === wf.name) || wf;
                openEditor(created);
            } catch (e) {
                ElementPlus.ElMessage.error(e.message);
            } finally {
                creating.value = false;
            }
        };

        // 删除工作流
        const deleteWorkflow = async (wf) => {
            try {
                await ElementPlus.ElMessageBox.confirm(`确定删除工作流「${wf.name}」？`, '确认删除', { type: 'warning' });
                const r = await fetch(`/api/workflows/${wf.id}`, { method: 'DELETE', headers: getAuthHeaders() });
                if (!r.ok) throw new Error('删除失败');
                ElementPlus.ElMessage.success('已删除');
                await loadWorkflows();
            } catch (e) {
                if (e !== 'cancel' && e.message !== 'cancel') ElementPlus.ElMessage.error(e.message || '删除失败');
            }
        };

        // 发布工作流
        const publishWorkflow = async (wf) => {
            try {
                const r = await fetch(`/api/workflows/${wf.id}/publish`, { method: 'POST', headers: getAuthHeaders() });
                if (!r.ok) {
                    const err = await r.json();
                    throw new Error(err.error || '发布失败');
                }
                ElementPlus.ElMessage.success('工作流已发布');
                await loadWorkflows();
            } catch (e) {
                ElementPlus.ElMessage.error(e.message);
            }
        };

        // ===== 快速执行 =====
        const showExecDialog = ref(false);
        const execWorkflow = ref(null);
        const execInputs = reactive({});
        const execEvents = ref([]);
        const executing = ref(false);
        const execFinalOutput = ref('');

        const quickExecute = (wf) => {
            execWorkflow.value = wf;
            execEvents.value = [];
            execFinalOutput.value = '';
            // 初始化输入
            Object.keys(execInputs).forEach(k => delete execInputs[k]);
            if (wf.variables) {
                wf.variables.forEach(v => {
                    execInputs[v.name] = v.default_value || '';
                });
            }
            showExecDialog.value = true;
        };

        const doExecute = async () => {
            if (!execWorkflow.value) return;
            executing.value = true;
            execEvents.value = [];
            execFinalOutput.value = '';
            nodeExecStatus.value = {};

            try {
                const r = await fetch(`/api/workflows/${execWorkflow.value.id}/execute`, {
                    method: 'POST',
                    headers: getAuthHeaders(),
                    body: JSON.stringify({ inputs: { ...execInputs } })
                });

                const reader = r.body.getReader();
                const decoder = new TextDecoder();
                let buffer = '';

                while (true) {
                    const { done, value } = await reader.read();
                    if (done) break;
                    buffer += decoder.decode(value, { stream: true });
                    const lines = buffer.split('\n');
                    buffer = lines.pop();
                    for (const line of lines) {
                        if (line.startsWith('data: ')) {
                            try {
                                const evt = JSON.parse(line.slice(6));
                                execEvents.value.push(evt);
                                handleExecEvent(evt);
                            } catch (e) {}
                        }
                    }
                }
            } catch (e) {
                ElementPlus.ElMessage.error('执行失败: ' + e.message);
            } finally {
                executing.value = false;
            }
        };

        // ===== 编辑器视图状态 =====
        const currentWorkflow = ref(null);
        const editorNodes = ref([]);
        const editorEdges = ref([]);
        const editorVariables = ref([]);
        const selectedNodeId = ref(null);
        const selectedEdgeId = ref(null);
        const saving = ref(false);
        const canvasRef = ref(null);
        const svgRef = ref(null);

        // 缩放和平移
        const zoom = ref(1);
        const panX = ref(0);
        const panY = ref(0);
        const svgViewBox = computed(() => {
            const w = 2000 / zoom.value;
            const h = 1200 / zoom.value;
            const x = -panX.value / zoom.value;
            const y = -panY.value / zoom.value;
            return `${x} ${y} ${w} ${h}`;
        });

        // 节点拖拽
        const draggingNode = ref(null);
        const dragOffset = reactive({ x: 0, y: 0 });

        // 连线拖拽
        const draggingEdge = ref(false);
        const edgeFrom = ref(null);
        const edgeMouse = reactive({ x: 0, y: 0 });

        // 节点执行状态
        const nodeExecStatus = ref({});

        // ===== 多选 =====
        const selectedNodeIds = ref([]);

        // ===== 框选 =====
        const selectionBox = ref(null);
        let isSelecting = false;

        // ===== 对齐辅助线 =====
        const alignGuides = ref([]);
        const SNAP_THRESHOLD = 8;

        // ===== 撤销/重做 =====
        const undoStack = ref([]);
        const redoStack = ref([]);
        const MAX_HISTORY = 50;

        const canUndo = computed(() => undoStack.value.length > 0);
        const canRedo = computed(() => redoStack.value.length > 0);

        // 保存快照到撤销栈
        const saveSnapshot = () => {
            const snapshot = {
                nodes: JSON.parse(JSON.stringify(editorNodes.value)),
                edges: JSON.parse(JSON.stringify(editorEdges.value))
            };
            undoStack.value.push(snapshot);
            if (undoStack.value.length > MAX_HISTORY) undoStack.value.shift();
            redoStack.value = []; // 新操作清空重做栈
        };

        const undo = () => {
            if (!canUndo.value) return;
            // 保存当前状态到重做栈
            redoStack.value.push({
                nodes: JSON.parse(JSON.stringify(editorNodes.value)),
                edges: JSON.parse(JSON.stringify(editorEdges.value))
            });
            const snapshot = undoStack.value.pop();
            editorNodes.value = snapshot.nodes;
            editorEdges.value = snapshot.edges;
            selectedNodeId.value = null;
            selectedEdgeId.value = null;
            selectedNodeIds.value = [];
        };

        const redo = () => {
            if (!canRedo.value) return;
            undoStack.value.push({
                nodes: JSON.parse(JSON.stringify(editorNodes.value)),
                edges: JSON.parse(JSON.stringify(editorEdges.value))
            });
            const snapshot = redoStack.value.pop();
            editorNodes.value = snapshot.nodes;
            editorEdges.value = snapshot.edges;
            selectedNodeId.value = null;
            selectedEdgeId.value = null;
            selectedNodeIds.value = [];
        };

        // ===== 连线动画 =====
        const edgeAnimations = ref({});

        // 编辑器执行
        const showEditorExec = ref(false);
        const editorExecInputs = reactive({});
        const editorExecEvents = ref([]);
        const editorExecuting = ref(false);
        const editorExecOutput = ref('');

        // 可用工具和 Agent 列表（从后端加载）
        const availableTools = ref([]);
        const availableAgents = ref([]);
        const toolListLoading = ref(false);
        const agentListLoading = ref(false);

        const loadAvailableTools = async () => {
            toolListLoading.value = true;
            try {
                const r = await fetch('/api/tools', { headers: getAuthHeaders() });
                const data = await r.json();
                availableTools.value = (data.tools || []).filter(t => t.name !== 'call_agent');
            } catch (e) {
                console.error('加载工具列表失败', e);
            } finally {
                toolListLoading.value = false;
            }
        };

        const loadAvailableAgents = async () => {
            agentListLoading.value = true;
            try {
                const r = await fetch('/api/agents', { headers: getAuthHeaders() });
                const data = await r.json();
                availableAgents.value = (data.agents || []).filter(a => !a.is_master);
            } catch (e) {
                console.error('加载 Agent 列表失败', e);
            } finally {
                agentListLoading.value = false;
            }
        };

        // 节点类型定义
        const nodeTypes = [
            { type: 'llm', icon: '🤖', label: 'LLM 对话', desc: '调用大语言模型' },
            { type: 'tool', icon: '🔧', label: '工具调用', desc: '调用已注册工具' },
            { type: 'agent', icon: '🤝', label: '子 Agent', desc: '调用子 Agent' },
            { type: 'template', icon: '📝', label: '模板转换', desc: '文本拼接/格式化' },
            { type: 'http', icon: '🌐', label: 'HTTP 请求', desc: '发起 HTTP 请求' },
            { type: 'condition', icon: '🔀', label: '条件分支', desc: '按条件走不同分支' },
            { type: 'parallel', icon: '⚡', label: '并行汇聚', desc: '合并多个并行分支' },
            { type: 'code', icon: '💻', label: '代码执行', desc: 'JavaScript 沙箱执行' },
            { type: 'loop', icon: '🔁', label: '循环', desc: 'for-each / while 循环' },
        ];

        const nodeIcon = (type) => ({ start: '▶', end: '⏹', llm: '🤖', tool: '🔧', agent: '🤝', template: '📝', http: '🌐', condition: '🔀', parallel: '⚡', code: '💻', loop: '🔁' }[type] || '❓');
        const nodeLabel = (type) => ({ start: '开始', end: '结束', llm: 'LLM 对话', tool: '工具调用', agent: '子 Agent', template: '模板转换', http: 'HTTP 请求', condition: '条件分支', parallel: '并行汇聚', code: '代码执行', loop: '循环' }[type] || type);
        const paletteColor = (type) => ({ llm: '#667eea', tool: '#faad14', agent: '#722ed1', template: '#1890ff', http: '#f5222d', condition: '#faad14', parallel: '#13c2c2', code: '#52c41a', loop: '#eb2f96' }[type] || '#667eea');
        const nodeColor = (type) => ({
            start: 'url(#grad-start)', end: 'url(#grad-end)',
            llm: 'url(#grad-llm)', tool: 'url(#grad-tool)', agent: 'url(#grad-agent)',
            template: 'url(#grad-template)', http: 'url(#grad-http)',
            condition: 'url(#grad-condition)', parallel: 'url(#grad-parallel)',
            code: 'url(#grad-code)', loop: 'url(#grad-loop)'
        }[type] || '#999');
        const nodeWidth = (node) => {
            const name = node.name || nodeLabel(node.type);
            if (node.type === 'condition') return Math.max(140, name.length * 12 + 60);
            return Math.max(130, name.length * 12 + 44);
        };

        // 节点高度：条件节点根据分支数量动态计算
        const nodeHeight = (node) => {
            if (node.type === 'condition' && node.config && node.config.conditions && node.config.conditions.length > 0) {
                return Math.max(70, 24 + node.config.conditions.length * 22 + 14);
            }
            if (node.type === 'start' || node.type === 'end') return 48;
            return 64;
        };

        // 条件节点第 i 个分支的输出端口 Y 坐标
        const conditionPortY = (index, node) => {
            const h = nodeHeight(node);
            const count = (node.config && node.config.conditions) ? node.config.conditions.length : 1;
            const startY = (h - (count - 1) * 22) / 2;
            return startY + index * 22;
        };

        // 菱形节点的顶点坐标（条件分支）
        const diamondPoints = (node) => {
            const w = nodeWidth(node), h = nodeHeight(node);
            return `${w/2},0 ${w},${h/2} ${w/2},${h} 0,${h/2}`;
        };

        // 六边形节点的顶点坐标（并行汇聚）
        const hexagonPoints = (node) => {
            const w = nodeWidth(node), h = nodeHeight(node);
            const inset = h * 0.3;
            return `${inset},0 ${w - inset},0 ${w},${h/2} ${w - inset},${h} ${inset},${h} 0,${h/2}`;
        };

        // 获取端口位置（支持不同形状节点）
        const getPortPos = (node, dir) => {
            const w = nodeWidth(node), h = nodeHeight(node);
            if (dir === 'in') {
                // 输入端口在左侧
                if (node.type === 'condition') return { x: 0, y: h / 2 }; // 菱形左顶点
                if (node.type === 'parallel') return { x: 0, y: h / 2 }; // 六边形左顶点
                return { x: 0, y: h / 2 };
            } else {
                // 输出端口在右侧
                if (node.type === 'condition') return { x: w, y: h / 2 }; // 菱形右顶点
                if (node.type === 'parallel') return { x: w, y: h / 2 }; // 六边形右顶点
                return { x: w, y: h / 2 };
            }
        };

        // 选中的节点
        const selectedNode = computed(() => {
            if (!selectedNodeId.value) return null;
            return editorNodes.value.find(n => n.id === selectedNodeId.value);
        });

        // 自动检测节点配置中引用的变量（如 {{query}}），合并手动定义的全局变量
        const execRequiredVars = computed(() => {
            const varMap = {};
            // 1. 先加入手动定义的全局变量
            editorVariables.value.forEach(v => {
                if (v.name) {
                    varMap[v.name] = { name: v.name, required: v.required, description: v.description || '', default_value: v.default_value || '' };
                }
            });
            // 2. 扫描所有节点配置中的 ${变量名} 引用（排除 ${node_id.output} 格式）
            const simpleVarRe = /\$\{(\w+)\}/g;
            const builtinVars = new Set(['current_time', 'current_date']);
            editorNodes.value.forEach(node => {
                if (!node.config) return;
                const fields = [
                    node.config.system_prompt,
                    node.config.user_prompt,
                    node.config.agent_message,
                    node.config.template,
                    node.config.url,
                    node.config.body,
                ];
                // 也扫描 tool_args 中的值
                if (node.config.tool_args && typeof node.config.tool_args === 'object') {
                    Object.values(node.config.tool_args).forEach(v => {
                        if (typeof v === 'string') fields.push(v);
                    });
                }
                fields.forEach(field => {
                    if (!field || typeof field !== 'string') return;
                    let m;
                    while ((m = simpleVarRe.exec(field)) !== null) {
                        const varName = m[1];
                        // 排除内置变量和已存在的节点 ID
                        if (builtinVars.has(varName)) continue;
                        if (editorNodes.value.some(n => n.id === varName)) continue;
                        if (!varMap[varName]) {
                            varMap[varName] = { name: varName, required: true, description: '节点配置中引用的变量', default_value: '' };
                        }
                    }
                });
            });
            return Object.values(varMap);
        });

        // Tool 参数 JSON 编辑
        const toolArgsJson = computed({
            get() {
                if (!selectedNode.value || !selectedNode.value.config || !selectedNode.value.config.tool_args) return '';
                return JSON.stringify(selectedNode.value.config.tool_args, null, 2);
            },
            set() {}
        });
        const updateToolArgs = (val) => {
            if (!selectedNode.value) return;
            try {
                selectedNode.value.config.tool_args = JSON.parse(val);
            } catch (e) {}
        };

        // Code 节点输入变量 JSON 编辑
        const codeInputsJson = computed({
            get() {
                if (!selectedNode.value || !selectedNode.value.config || !selectedNode.value.config.code_inputs) return '';
                return JSON.stringify(selectedNode.value.config.code_inputs, null, 2);
            },
            set() {}
        });
        const updateCodeInputs = (val) => {
            if (!selectedNode.value) return;
            try {
                selectedNode.value.config.code_inputs = JSON.parse(val);
            } catch (e) {}
        };

        // 打开编辑器
        const openEditor = (wf) => {
            currentWorkflow.value = wf;
            editorNodes.value = JSON.parse(JSON.stringify(wf.nodes || []));
            editorEdges.value = JSON.parse(JSON.stringify(wf.edges || []));
            editorVariables.value = JSON.parse(JSON.stringify(wf.variables || []));
            // 确保每个节点都有 config
            editorNodes.value.forEach(n => {
                if (!n.config) n.config = {};
                if (!n.position) n.position = { x: 100, y: 100 };
                // 确保 config 中的嵌套对象存在
                if (n.type === 'tool' && !n.config.tool_args) n.config.tool_args = {};
                if (n.type === 'http' && !n.config.headers) n.config.headers = {};
            });
            selectedNodeId.value = null;
            selectedEdgeId.value = null;
            nodeExecStatus.value = {};
            // 清除上一次执行面板的状态
            showEditorExec.value = false;
            editorExecEvents.value = [];
            editorExecOutput.value = '';
            editorExecuting.value = false;
            Object.keys(editorExecInputs).forEach(k => delete editorExecInputs[k]);
            // 加载可用工具和 Agent 列表
            loadAvailableTools();
            loadAvailableAgents();
            view.value = 'editor';
        };

        const backToList = () => {
            view.value = 'list';
            currentWorkflow.value = null;
            loadWorkflows();
        };

        // ===== 导入/导出（Phase 3）=====
        const exportWorkflow = async (wf) => {
            try {
                const r = await fetch(`/api/workflows/${wf.id}/export`, { headers: getAuthHeaders() });
                if (!r.ok) {
                    const err = await r.json();
                    throw new Error(err.error || '导出失败');
                }
                const data = await r.json();
                const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' });
                const url = URL.createObjectURL(blob);
                const a = document.createElement('a');
                a.href = url;
                a.download = `workflow_${wf.name || wf.id}.json`;
                document.body.appendChild(a);
                a.click();
                document.body.removeChild(a);
                URL.revokeObjectURL(url);
                ElementPlus.ElMessage.success('工作流导出成功');
            } catch (e) {
                ElementPlus.ElMessage.error(e.message);
            }
        };

        const importFileRef = ref(null);
        const importWorkflow = () => {
            // 触发文件选择
            const input = document.createElement('input');
            input.type = 'file';
            input.accept = '.json';
            input.onchange = async (e) => {
                const file = e.target.files[0];
                if (!file) return;
                try {
                    const text = await file.text();
                    const r = await fetch('/api/workflows/import', {
                        method: 'POST',
                        headers: getAuthHeaders(),
                        body: text
                    });
                    if (!r.ok) {
                        const err = await r.json();
                        throw new Error(err.error || '导入失败');
                    }
                    ElementPlus.ElMessage.success('工作流导入成功');
                    await loadWorkflows();
                } catch (err) {
                    ElementPlus.ElMessage.error(err.message);
                }
            };
            input.click();
        };

        // 保存工作流
        const saveWorkflow = async () => {
            if (!currentWorkflow.value) return;
            saving.value = true;
            try {
                const r = await fetch(`/api/workflows/${currentWorkflow.value.id}`, {
                    method: 'PUT',
                    headers: getAuthHeaders(),
                    body: JSON.stringify({
                        name: currentWorkflow.value.name,
                        description: currentWorkflow.value.description,
                        nodes: editorNodes.value,
                        edges: editorEdges.value,
                        variables: editorVariables.value,
                        status: 'draft'
                    })
                });
                if (!r.ok) {
                    const err = await r.json();
                    throw new Error(err.error || '保存失败');
                }
                ElementPlus.ElMessage.success('工作流已保存');
            } catch (e) {
                ElementPlus.ElMessage.error(e.message);
            } finally {
                saving.value = false;
            }
        };

        // ===== 画布交互 =====

        // 拖拽添加节点
        let dragNodeType = null;
        const onDragStart = (e, type) => {
            dragNodeType = type;
            e.dataTransfer.effectAllowed = 'copy';
        };

        const onCanvasDrop = (e) => {
            if (!dragNodeType) return;
            saveSnapshot(); // 保存快照以支持撤销
            const rect = canvasRef.value.getBoundingClientRect();
            const x = (e.clientX - rect.left) / zoom.value - panX.value / zoom.value;
            const y = (e.clientY - rect.top) / zoom.value - panY.value / zoom.value;
            const id = dragNodeType + '_' + Date.now().toString(36);
            // 为不同节点类型初始化默认配置
            let config = {};
            if (dragNodeType === 'condition') {
                config = {
                    conditions: [
                        { id: 'branch_1', operator: '==', field: '', value: '', label: '分支 1', is_default: false },
                        { id: 'default', operator: '', field: '', value: '', label: '默认', is_default: true },
                    ]
                };
            } else if (dragNodeType === 'parallel') {
                config = { wait_all: true };
            } else if (dragNodeType === 'code') {
                config = { code_language: 'javascript', code: '// 通过 inputs 对象读取上游节点输出\n// 通过 return 返回结果\nconst data = inputs;\nreturn JSON.stringify(data);', code_inputs: {} };
            } else if (dragNodeType === 'loop') {
                config = { loop_type: 'foreach', loop_list: '', loop_item_var: 'item', loop_index_var: 'index', loop_max_iter: 100, loop_body: '// inputs.item = 当前元素\n// inputs.index = 当前索引\n// inputs.results = 之前的迭代结果\nreturn inputs.item;' };
            }
            editorNodes.value.push({
                id,
                type: dragNodeType,
                name: nodeLabel(dragNodeType),
                description: '',
                config,
                position: { x: Math.round(x), y: Math.round(y) }
            });
            dragNodeType = null;
            selectedNodeId.value = id;
        };

        // 添加条件分支
        const addConditionBranch = () => {
            if (!selectedNode.value || selectedNode.value.type !== 'condition') return;
            if (!selectedNode.value.config.conditions) selectedNode.value.config.conditions = [];
            const idx = selectedNode.value.config.conditions.length + 1;
            selectedNode.value.config.conditions.push({
                id: 'branch_' + Date.now().toString(36),
                operator: '==',
                field: '',
                value: '',
                label: '分支 ' + idx,
                is_default: false,
            });
        };

        // 节点拖拽移动
        const onNodeMouseDown = (e, node) => {
            if (e.target.classList.contains('port')) return;
            saveSnapshot(); // 保存快照以支持撤销
            draggingNode.value = node.id;
            const rect = canvasRef.value.getBoundingClientRect();
            dragOffset.x = (e.clientX - rect.left) / zoom.value - panX.value / zoom.value - node.position.x;
            dragOffset.y = (e.clientY - rect.top) / zoom.value - panY.value / zoom.value - node.position.y;
        };

        const onCanvasMouseMove = (e) => {
            if (draggingNode.value) {
                const rect = canvasRef.value.getBoundingClientRect();
                const node = editorNodes.value.find(n => n.id === draggingNode.value);
                if (node) {
                    let newX = Math.round((e.clientX - rect.left) / zoom.value - panX.value / zoom.value - dragOffset.x);
                    let newY = Math.round((e.clientY - rect.top) / zoom.value - panY.value / zoom.value - dragOffset.y);

                    // 对齐辅助线计算
                    const guides = [];
                    const nw = nodeWidth(node), nh = nodeHeight(node);
                    const ncx = newX + nw / 2, ncy = newY + nh / 2;
                    editorNodes.value.forEach(other => {
                        if (other.id === node.id) return;
                        const ow = nodeWidth(other), oh = nodeHeight(other);
                        const ocx = other.position.x + ow / 2, ocy = other.position.y + oh / 2;
                        // 水平中心对齐
                        if (Math.abs(ncx - ocx) < SNAP_THRESHOLD) {
                            newX = ocx - nw / 2;
                            guides.push({ x1: ocx, y1: Math.min(newY, other.position.y) - 20, x2: ocx, y2: Math.max(newY + nh, other.position.y + oh) + 20 });
                        }
                        // 垂直中心对齐
                        if (Math.abs(ncy - ocy) < SNAP_THRESHOLD) {
                            newY = ocy - nh / 2;
                            guides.push({ x1: Math.min(newX, other.position.x) - 20, y1: ocy, x2: Math.max(newX + nw, other.position.x + ow) + 20, y2: ocy });
                        }
                        // 顶部对齐
                        if (Math.abs(newY - other.position.y) < SNAP_THRESHOLD) {
                            newY = other.position.y;
                            guides.push({ x1: Math.min(newX, other.position.x) - 20, y1: newY, x2: Math.max(newX + nw, other.position.x + ow) + 20, y2: newY });
                        }
                        // 左侧对齐
                        if (Math.abs(newX - other.position.x) < SNAP_THRESHOLD) {
                            newX = other.position.x;
                            guides.push({ x1: newX, y1: Math.min(newY, other.position.y) - 20, x2: newX, y2: Math.max(newY + nh, other.position.y + oh) + 20 });
                        }
                    });
                    alignGuides.value = guides;
                    node.position.x = newX;
                    node.position.y = newY;

                    // 多选拖拽：同时移动其他选中的节点
                    if (selectedNodeIds.value.length > 1 && selectedNodeIds.value.includes(node.id)) {
                        const dx = newX - node.position.x;
                        const dy = newY - node.position.y;
                        // dx/dy 已经应用到 node 了，这里需要用增量
                    }
                }
            }
            if (draggingEdge.value) {
                const rect = canvasRef.value.getBoundingClientRect();
                edgeMouse.x = (e.clientX - rect.left) / zoom.value - panX.value / zoom.value;
                edgeMouse.y = (e.clientY - rect.top) / zoom.value - panY.value / zoom.value;
            }
        };

        const onCanvasMouseUp = () => {
            draggingNode.value = null;
            alignGuides.value = []; // 清除辅助线
            if (draggingEdge.value) {
                draggingEdge.value = false;
                edgeFrom.value = null;
            }
        };

        const onCanvasMouseDown = (e) => {
            // 点击空白区域取消选中
        };

        const onCanvasClick = () => {
            selectedNodeId.value = null;
            selectedEdgeId.value = null;
            selectedNodeIds.value = [];
        };

        // ===== SVG 鼠标滚轮缩放 =====
        const onWheel = (e) => {
            const delta = e.deltaY > 0 ? -0.05 : 0.05;
            const newZoom = Math.max(0.2, Math.min(2.5, zoom.value + delta));
            // 以鼠标位置为中心缩放
            const rect = canvasRef.value.getBoundingClientRect();
            const mx = (e.clientX - rect.left);
            const my = (e.clientY - rect.top);
            const scale = newZoom / zoom.value;
            panX.value = mx - scale * (mx - panX.value);
            panY.value = my - scale * (my - panY.value);
            zoom.value = newZoom;
        };

        // ===== SVG 画布拖拽平移 =====
        let isPanning = false;
        let panStartX = 0, panStartY = 0;
        let panStartPanX = 0, panStartPanY = 0;

        const onSvgMouseDown = (e) => {
            // 右键或中键拖拽平移，左键框选
            if (e.button === 1 || e.button === 2 || e.altKey) {
                isPanning = true;
                panStartX = e.clientX;
                panStartY = e.clientY;
                panStartPanX = panX.value;
                panStartPanY = panY.value;
                e.preventDefault();
                return;
            }
            // 左键框选
            if (e.button === 0 && !e.altKey) {
                const rect = canvasRef.value.getBoundingClientRect();
                const x = (e.clientX - rect.left) / zoom.value - panX.value / zoom.value;
                const y = (e.clientY - rect.top) / zoom.value - panY.value / zoom.value;
                selectionBox.value = { x1: x, y1: y, x2: x, y2: y };
                isSelecting = true;
            }
        };

        const onSvgMouseMove = (e) => {
            if (isPanning) {
                panX.value = panStartPanX + (e.clientX - panStartX);
                panY.value = panStartPanY + (e.clientY - panStartY);
                return;
            }
            if (isSelecting && selectionBox.value) {
                const rect = canvasRef.value.getBoundingClientRect();
                selectionBox.value.x2 = (e.clientX - rect.left) / zoom.value - panX.value / zoom.value;
                selectionBox.value.y2 = (e.clientY - rect.top) / zoom.value - panY.value / zoom.value;
            }
        };

        const onSvgMouseUp = (e) => {
            if (isPanning) {
                isPanning = false;
                return;
            }
            if (isSelecting && selectionBox.value) {
                // 计算框选区域内的节点
                const box = selectionBox.value;
                const minX = Math.min(box.x1, box.x2);
                const maxX = Math.max(box.x1, box.x2);
                const minY = Math.min(box.y1, box.y2);
                const maxY = Math.max(box.y1, box.y2);
                // 只有框选面积大于一定阈值才触发多选
                if (Math.abs(maxX - minX) > 10 && Math.abs(maxY - minY) > 10) {
                    const ids = editorNodes.value.filter(n => {
                        const nx = n.position.x;
                        const ny = n.position.y;
                        const nw = nodeWidth(n);
                        const nh = nodeHeight(n);
                        return nx + nw > minX && nx < maxX && ny + nh > minY && ny < maxY;
                    }).map(n => n.id);
                    selectedNodeIds.value = ids;
                    if (ids.length > 0) selectedNodeId.value = ids[0];
                } else {
                    // 点击空白
                    selectedNodeId.value = null;
                    selectedEdgeId.value = null;
                    selectedNodeIds.value = [];
                }
                selectionBox.value = null;
                isSelecting = false;
            }
        };

        // 节点选中（支持 Shift 多选）
        const selectNode = (id, event) => {
            if (event && (event.shiftKey || event.ctrlKey || event.metaKey)) {
                // 多选模式
                const idx = selectedNodeIds.value.indexOf(id);
                if (idx >= 0) {
                    selectedNodeIds.value.splice(idx, 1);
                } else {
                    selectedNodeIds.value.push(id);
                }
                selectedNodeId.value = id;
                selectedEdgeId.value = null;
                return;
            }
            selectedNodeId.value = id;
            selectedEdgeId.value = null;
            selectedNodeIds.value = [id];
        };

        const clearSelection = () => {
            selectedNodeIds.value = [];
            selectedNodeId.value = null;
        };

        const selectEdge = (id) => {
            selectedEdgeId.value = id;
            selectedNodeId.value = null;
        };

        // 连线端口交互
        let edgeFromHandle = null; // 条件节点的 sourceHandle
        const onPortMouseDown = (e, nodeId, portType, handle) => {
            e.preventDefault();
            e.stopPropagation();
            draggingEdge.value = true;
            edgeFrom.value = nodeId;
            edgeFromHandle = handle || null;
            const rect = canvasRef.value.getBoundingClientRect();
            edgeMouse.x = (e.clientX - rect.left) / zoom.value - panX.value / zoom.value;
            edgeMouse.y = (e.clientY - rect.top) / zoom.value - panY.value / zoom.value;
        };

        const onPortMouseUp = (nodeId, portType) => {
            if (draggingEdge.value && edgeFrom.value && edgeFrom.value !== nodeId) {
                // 检查是否已存在相同连线（同源+同目标+同 handle）
                const exists = editorEdges.value.some(e => e.source === edgeFrom.value && e.target === nodeId && (e.source_handle || '') === (edgeFromHandle || ''));
                if (!exists) {
                    saveSnapshot(); // 保存快照以支持撤销
                    const newEdge = {
                        id: 'e_' + Date.now().toString(36),
                        source: edgeFrom.value,
                        target: nodeId
                    };
                    if (edgeFromHandle) {
                        newEdge.source_handle = edgeFromHandle;
                        // 自动设置边标签为分支名称
                        const fromNode = editorNodes.value.find(n => n.id === edgeFrom.value);
                        if (fromNode && fromNode.config && fromNode.config.conditions) {
                            const cond = fromNode.config.conditions.find(c => c.id === edgeFromHandle);
                            if (cond) newEdge.label = cond.label || '';
                        }
                    }
                    editorEdges.value.push(newEdge);
                }
            }
            draggingEdge.value = false;
            edgeFrom.value = null;
            edgeFromHandle = null;
        };

        // 正在拖拽的连线路径（智能方向）
        const draggingEdgePath = computed(() => {
            if (!draggingEdge.value || !edgeFrom.value) return '';
            const fromNode = editorNodes.value.find(n => n.id === edgeFrom.value);
            if (!fromNode) return '';
            const fw = nodeWidth(fromNode), fh = nodeHeight(fromNode);

            // 条件节点的分支端口
            if (edgeFromHandle && fromNode.type === 'condition' && fromNode.config && fromNode.config.conditions) {
                const ci = fromNode.config.conditions.findIndex(c => c.id === edgeFromHandle);
                const sx = fromNode.position.x + fw;
                const sy = fromNode.position.y + (ci >= 0 ? conditionPortY(ci, fromNode) : fh / 2);
                const ex = edgeMouse.x, ey = edgeMouse.y;
                const cx = (sx + ex) / 2;
                return `M ${sx} ${sy} C ${cx} ${sy}, ${cx} ${ey}, ${ex} ${ey}`;
            }

            // 智能方向：根据鼠标位置选择最佳出发端口
            const fromCx = fromNode.position.x + fw / 2;
            const fromCy = fromNode.position.y + fh / 2;
            const dx = edgeMouse.x - fromCx;
            const dy = edgeMouse.y - fromCy;

            let sx, sy;
            if (Math.abs(dx) >= Math.abs(dy)) {
                sx = dx >= 0 ? fromNode.position.x + fw : fromNode.position.x;
                sy = fromNode.position.y + fh / 2;
                const ex = edgeMouse.x, ey = edgeMouse.y;
                const cx = (sx + ex) / 2;
                return `M ${sx} ${sy} C ${cx} ${sy}, ${cx} ${ey}, ${ex} ${ey}`;
            } else {
                sx = fromNode.position.x + fw / 2;
                sy = dy >= 0 ? fromNode.position.y + fh : fromNode.position.y;
                const ex = edgeMouse.x, ey = edgeMouse.y;
                const cy = (sy + ey) / 2;
                return `M ${sx} ${sy} C ${sx} ${cy}, ${ex} ${cy}, ${ex} ${ey}`;
            }
        });

        // 计算连线路径（智能贝塞尔曲线，自动选择最佳连接方向）
        const getEdgePath = (edge) => {
            const fromNode = editorNodes.value.find(n => n.id === edge.source);
            const toNode = editorNodes.value.find(n => n.id === edge.target);
            if (!fromNode || !toNode) return '';

            const fw = nodeWidth(fromNode), fh = nodeHeight(fromNode);
            const tw = nodeWidth(toNode), th = nodeHeight(toNode);

            // 条件节点的连线从对应分支端口出发
            if (edge.source_handle && fromNode.type === 'condition' && fromNode.config && fromNode.config.conditions) {
                const ci = fromNode.config.conditions.findIndex(c => c.id === edge.source_handle);
                const sx = fromNode.position.x + fw;
                const sy = fromNode.position.y + (ci >= 0 ? conditionPortY(ci, fromNode) : fh / 2);
                const ex = toNode.position.x;
                const ey = toNode.position.y + th / 2;
                const cx = (sx + ex) / 2;
                return `M ${sx} ${sy} C ${cx} ${sy}, ${cx} ${ey}, ${ex} ${ey}`;
            }

            // 计算源节点和目标节点的中心点
            const fromCx = fromNode.position.x + fw / 2;
            const fromCy = fromNode.position.y + fh / 2;
            const toCx = toNode.position.x + tw / 2;
            const toCy = toNode.position.y + th / 2;

            // 判断最佳连接方向
            const dx = toCx - fromCx;
            const dy = toCy - fromCy;

            let sx, sy, ex, ey;

            if (Math.abs(dx) >= Math.abs(dy)) {
                // 水平方向为主：左右连接
                if (dx >= 0) {
                    // 目标在右边：从右端口到左端口
                    sx = fromNode.position.x + fw;
                    sy = fromNode.position.y + fh / 2;
                    ex = toNode.position.x;
                    ey = toNode.position.y + th / 2;
                } else {
                    // 目标在左边：从左端口到右端口
                    sx = fromNode.position.x;
                    sy = fromNode.position.y + fh / 2;
                    ex = toNode.position.x + tw;
                    ey = toNode.position.y + th / 2;
                }
                const cx = (sx + ex) / 2;
                return `M ${sx} ${sy} C ${cx} ${sy}, ${cx} ${ey}, ${ex} ${ey}`;
            } else {
                // 垂直方向为主：上下连接
                if (dy >= 0) {
                    // 目标在下方：从底部端口到顶部端口
                    sx = fromNode.position.x + fw / 2;
                    sy = fromNode.position.y + fh;
                    ex = toNode.position.x + tw / 2;
                    ey = toNode.position.y;
                } else {
                    // 目标在上方：从顶部端口到底部端口
                    sx = fromNode.position.x + fw / 2;
                    sy = fromNode.position.y;
                    ex = toNode.position.x + tw / 2;
                    ey = toNode.position.y + th;
                }
                const cy = (sy + ey) / 2;
                return `M ${sx} ${sy} C ${sx} ${cy}, ${ex} ${cy}, ${ex} ${ey}`;
            }
        };

        // 计算边的中点坐标（用于显示标签）
        const getEdgeMidpoint = (edge) => {
            const fromNode = editorNodes.value.find(n => n.id === edge.source);
            const toNode = editorNodes.value.find(n => n.id === edge.target);
            if (!fromNode || !toNode) return { x: 0, y: 0 };

            const fw = nodeWidth(fromNode), fh = nodeHeight(fromNode);
            const tw = nodeWidth(toNode), th = nodeHeight(toNode);

            // 条件节点的连线
            if (edge.source_handle && fromNode.type === 'condition' && fromNode.config && fromNode.config.conditions) {
                const ci = fromNode.config.conditions.findIndex(c => c.id === edge.source_handle);
                const sx = fromNode.position.x + fw;
                const sy = fromNode.position.y + (ci >= 0 ? conditionPortY(ci, fromNode) : fh / 2);
                const ex = toNode.position.x;
                const ey = toNode.position.y + th / 2;
                return { x: (sx + ex) / 2, y: (sy + ey) / 2 };
            }

            const fromCx = fromNode.position.x + fw / 2;
            const fromCy = fromNode.position.y + fh / 2;
            const toCx = toNode.position.x + tw / 2;
            const toCy = toNode.position.y + th / 2;
            return { x: (fromCx + toCx) / 2, y: (fromCy + toCy) / 2 };
        };

        // 删除选中（支持多选删除）
        const deleteSelected = () => {
            saveSnapshot(); // 保存快照以支持撤销
            // 多选删除
            if (selectedNodeIds.value.length > 1) {
                const ids = selectedNodeIds.value.filter(id => {
                    const n = editorNodes.value.find(n => n.id === id);
                    return n && n.type !== 'start';
                });
                editorNodes.value = editorNodes.value.filter(n => !ids.includes(n.id));
                editorEdges.value = editorEdges.value.filter(e => !ids.includes(e.source) && !ids.includes(e.target));
                selectedNodeIds.value = [];
                selectedNodeId.value = null;
                return;
            }
            if (selectedNodeId.value) {
                const id = selectedNodeId.value;
                if (id === 'start' || editorNodes.value.find(n => n.id === id)?.type === 'start') {
                    ElementPlus.ElMessage.warning('不能删除开始节点');
                    return;
                }
                editorNodes.value = editorNodes.value.filter(n => n.id !== id);
                editorEdges.value = editorEdges.value.filter(e => e.source !== id && e.target !== id);
                selectedNodeId.value = null;
                selectedNodeIds.value = [];
            }
            if (selectedEdgeId.value) {
                editorEdges.value = editorEdges.value.filter(e => e.id !== selectedEdgeId.value);
                selectedEdgeId.value = null;
            }
        };

        // 缩放
        const zoomIn = () => { zoom.value = Math.min(zoom.value + 0.1, 2.5); };
        const zoomOut = () => { zoom.value = Math.max(zoom.value - 0.1, 0.2); };
        const resetZoom = () => { zoom.value = 1; panX.value = 0; panY.value = 0; };

        // 适应画布（自动缩放和平移使所有节点可见）
        const fitView = () => {
            if (editorNodes.value.length === 0) return;
            let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
            editorNodes.value.forEach(n => {
                minX = Math.min(minX, n.position.x);
                minY = Math.min(minY, n.position.y);
                maxX = Math.max(maxX, n.position.x + nodeWidth(n));
                maxY = Math.max(maxY, n.position.y + nodeHeight(n));
            });
            const padding = 60;
            const contentW = maxX - minX + padding * 2;
            const contentH = maxY - minY + padding * 2;
            const canvasEl = canvasRef.value;
            if (!canvasEl) return;
            const canvasW = canvasEl.clientWidth;
            const canvasH = canvasEl.clientHeight;
            const scaleX = canvasW / contentW;
            const scaleY = canvasH / contentH;
            const newZoom = Math.max(0.2, Math.min(1.5, Math.min(scaleX, scaleY)));
            zoom.value = newZoom;
            panX.value = (canvasW / 2 - (minX + maxX) / 2 * newZoom);
            panY.value = (canvasH / 2 - (minY + maxY) / 2 * newZoom);
        };

        // ===== dagre 自动布局 =====
        const autoLayout = () => {
            if (typeof dagre === 'undefined') {
                ElementPlus.ElMessage.warning('dagre 库未加载');
                return;
            }
            saveSnapshot(); // 保存快照以支持撤销

            const g = new dagre.graphlib.Graph();
            g.setGraph({
                rankdir: 'LR', // 从左到右布局
                nodesep: 60,   // 节点间距
                ranksep: 100,  // 层级间距
                marginx: 40,
                marginy: 40
            });
            g.setDefaultEdgeLabel(() => ({}));

            // 添加节点
            editorNodes.value.forEach(n => {
                g.setNode(n.id, {
                    width: nodeWidth(n),
                    height: nodeHeight(n)
                });
            });

            // 添加边
            editorEdges.value.forEach(e => {
                g.setEdge(e.source, e.target);
            });

            // 执行布局
            dagre.layout(g);

            // 应用布局结果
            editorNodes.value.forEach(n => {
                const layoutNode = g.node(n.id);
                if (layoutNode) {
                    n.position.x = Math.round(layoutNode.x - nodeWidth(n) / 2);
                    n.position.y = Math.round(layoutNode.y - nodeHeight(n) / 2);
                }
            });

            // 触发连线动画
            editorEdges.value.forEach(e => {
                edgeAnimations.value[e.id] = true;
            });
            setTimeout(() => {
                edgeAnimations.value = {};
            }, 1500);

            // 自动适应视图
            nextTick(() => fitView());
            ElementPlus.ElMessage.success('自动布局完成');
        };

        // ===== MiniMap =====
        const minimapViewBox = computed(() => {
            if (editorNodes.value.length === 0) return '0 0 800 600';
            let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
            editorNodes.value.forEach(n => {
                minX = Math.min(minX, n.position.x);
                minY = Math.min(minY, n.position.y);
                maxX = Math.max(maxX, n.position.x + nodeWidth(n));
                maxY = Math.max(maxY, n.position.y + nodeHeight(n));
            });
            const pad = 50;
            return `${minX - pad} ${minY - pad} ${maxX - minX + pad * 2} ${maxY - minY + pad * 2}`;
        });

        const minimapViewport = computed(() => {
            const canvasEl = canvasRef.value;
            if (!canvasEl) return { x: 0, y: 0, w: 200, h: 150 };
            const w = canvasEl.clientWidth / zoom.value;
            const h = canvasEl.clientHeight / zoom.value;
            const x = -panX.value / zoom.value;
            const y = -panY.value / zoom.value;
            return { x, y, w, h };
        });

        const minimapNodeColor = (type) => ({
            start: '#52c41a', end: '#8c8c8c', llm: '#667eea', tool: '#faad14',
            agent: '#722ed1', template: '#1890ff', http: '#f5222d',
            condition: '#faad14', parallel: '#13c2c2', code: '#52c41a', loop: '#eb2f96'
        }[type] || '#999');

        const onMinimapClick = (e) => {
            // 点击小地图跳转到对应位置
            const svg = e.currentTarget;
            const rect = svg.getBoundingClientRect();
            const vb = minimapViewBox.value.split(' ').map(Number);
            const clickX = vb[0] + (e.clientX - rect.left) / rect.width * vb[2];
            const clickY = vb[1] + (e.clientY - rect.top) / rect.height * vb[3];
            const canvasEl = canvasRef.value;
            if (!canvasEl) return;
            panX.value = canvasEl.clientWidth / 2 - clickX * zoom.value;
            panY.value = canvasEl.clientHeight / 2 - clickY * zoom.value;
        };

        // 添加变量
        const addVariable = () => {
            editorVariables.value.push({ name: '', type: 'string', default_value: '', description: '', required: false });
        };

        // ===== 编辑器内执行 =====
        const openExecPanel = () => {
            showEditorExec.value = true;
            editorExecEvents.value = [];
            editorExecOutput.value = '';
            nodeExecStatus.value = {};
            Object.keys(editorExecInputs).forEach(k => delete editorExecInputs[k]);
            // 使用自动检测的变量列表初始化输入
            execRequiredVars.value.forEach(v => {
                editorExecInputs[v.name] = v.default_value || '';
            });
        };

        const doEditorExecute = async () => {
            if (!currentWorkflow.value) return;
            // 先保存
            await saveWorkflow();
            editorExecuting.value = true;
            editorExecEvents.value = [];
            editorExecOutput.value = '';
            nodeExecStatus.value = {};

            try {
                const r = await fetch(`/api/workflows/${currentWorkflow.value.id}/execute`, {
                    method: 'POST',
                    headers: getAuthHeaders(),
                    body: JSON.stringify({ inputs: { ...editorExecInputs } })
                });

                const reader = r.body.getReader();
                const decoder = new TextDecoder();
                let buffer = '';

                while (true) {
                    const { done, value } = await reader.read();
                    if (done) break;
                    buffer += decoder.decode(value, { stream: true });
                    const lines = buffer.split('\n');
                    buffer = lines.pop();
                    for (const line of lines) {
                        if (line.startsWith('data: ')) {
                            try {
                                const evt = JSON.parse(line.slice(6));
                                editorExecEvents.value.push(evt);
                                handleEditorExecEvent(evt);
                            } catch (e) {}
                        }
                    }
                }
            } catch (e) {
                ElementPlus.ElMessage.error('执行失败: ' + e.message);
            } finally {
                editorExecuting.value = false;
            }
        };

        // 处理执行事件（列表视图）
        const handleExecEvent = (evt) => {
            if (evt.node_id) {
                if (evt.type === 'node_start') nodeExecStatus.value[evt.node_id] = 'running';
                else if (evt.type === 'node_output' || evt.type === 'node_done') nodeExecStatus.value[evt.node_id] = 'done';
                else if (evt.type === 'node_error') nodeExecStatus.value[evt.node_id] = 'error';
            }
            if (evt.type === 'workflow_done' && evt.output != null) {
                // 提取最终输出
                const outputs = evt.output;
                if (typeof outputs === 'string') {
                    execFinalOutput.value = outputs;
                } else if (outputs && typeof outputs === 'object') {
                    // 取最后一个非空输出
                    const vals = Object.values(outputs).filter(v => v != null);
                    if (vals.length > 0) {
                        const last = vals[vals.length - 1];
                        execFinalOutput.value = typeof last === 'string' ? last : JSON.stringify(last, null, 2);
                    } else {
                        execFinalOutput.value = JSON.stringify(outputs, null, 2);
                    }
                }
            }
        };

        // 处理执行事件（编辑器视图）
        const handleEditorExecEvent = (evt) => {
            if (evt.node_id) {
                if (evt.type === 'node_start') nodeExecStatus.value[evt.node_id] = 'running';
                else if (evt.type === 'node_output' || evt.type === 'node_done') nodeExecStatus.value[evt.node_id] = 'done';
                else if (evt.type === 'node_error') nodeExecStatus.value[evt.node_id] = 'error';
            }
            if (evt.type === 'workflow_done' && evt.output != null) {
                const outputs = evt.output;
                if (typeof outputs === 'string') {
                    editorExecOutput.value = outputs;
                } else if (outputs && typeof outputs === 'object') {
                    const vals = Object.values(outputs).filter(v => v != null);
                    if (vals.length > 0) {
                        const last = vals[vals.length - 1];
                        editorExecOutput.value = typeof last === 'string' ? last : JSON.stringify(last, null, 2);
                    } else {
                        editorExecOutput.value = JSON.stringify(outputs, null, 2);
                    }
                }
            }
            if (evt.type === 'workflow_error') {
                editorExecOutput.value = '❌ 执行失败: ' + (evt.error || '未知错误');
            }
        };

        const getNodeExecStatus = (nodeId) => nodeExecStatus.value[nodeId] || '';

        // 事件显示
        const eventIcon = (type) => ({
            'node_start': '⏳',
            'node_output': '✅',
            'node_done': '✅',
            'node_error': '❌',
            'workflow_done': '🎉',
            'workflow_error': '💥'
        }[type] || '📌');

        const eventText = (evt) => {
            switch (evt.type) {
                case 'node_start': return `开始执行：${evt.node_name || evt.node_id}`;
                case 'node_output': {
                    const raw = evt.output;
                    const output = raw == null ? '' : (typeof raw === 'string' ? raw : JSON.stringify(raw, null, 2));
                    return `${evt.node_name || evt.node_id} 输出：${output}`;
                }
                case 'node_done': return `${evt.node_name || evt.node_id} 完成`;
                case 'node_error': return `${evt.node_name || evt.node_id} 失败：${evt.error}`;
                case 'workflow_done': return `🎉 工作流执行完成（${evt.duration_ms}ms，${evt.total_tokens || 0} tokens）`;
                case 'workflow_error': return `💥 工作流执行失败：${evt.error}`;
                default: return JSON.stringify(evt);
            }
        };

        // 格式化节点输出内容（完整展示）
        const formatNodeOutput = (raw) => {
            if (raw == null) return '';
            return typeof raw === 'string' ? raw : JSON.stringify(raw, null, 2);
        };

        // ===== 键盘快捷键 =====
        const handleKeydown = (e) => {
            // 只在编辑器视图中生效
            if (view.value !== 'editor') return;
            // 忽略输入框中的按键
            if (['INPUT', 'TEXTAREA', 'SELECT'].includes(e.target.tagName)) return;

            // Ctrl+Z / Cmd+Z 撤销
            if ((e.ctrlKey || e.metaKey) && e.key === 'z' && !e.shiftKey) {
                e.preventDefault();
                undo();
                return;
            }
            // Ctrl+Y / Cmd+Shift+Z 重做
            if ((e.ctrlKey || e.metaKey) && (e.key === 'y' || (e.key === 'z' && e.shiftKey))) {
                e.preventDefault();
                redo();
                return;
            }
            // Delete / Backspace 删除选中
            if (e.key === 'Delete' || e.key === 'Backspace') {
                if (selectedNodeId.value || selectedEdgeId.value || selectedNodeIds.value.length > 0) {
                    e.preventDefault();
                    deleteSelected();
                }
                return;
            }
            // Ctrl+A 全选
            if ((e.ctrlKey || e.metaKey) && e.key === 'a') {
                e.preventDefault();
                selectedNodeIds.value = editorNodes.value.map(n => n.id);
                if (selectedNodeIds.value.length > 0) selectedNodeId.value = selectedNodeIds.value[0];
                return;
            }
        };

        // ===== 生命周期 =====
        onMounted(() => {
            loadWorkflows();
            // 预加载工具和 Agent 列表
            loadAvailableTools();
            loadAvailableAgents();
            // 注册键盘快捷键
            document.addEventListener('keydown', handleKeydown);
        });

        return {
            // 视图
            view,
            // 列表
            workflows, listLoading, showCreateDialog, creating, createForm,
            statusLabel, loadWorkflows, createWorkflow, deleteWorkflow, publishWorkflow,
            exportWorkflow, importWorkflow, importFileRef,
            // 快速执行
            showExecDialog, execWorkflow, execInputs, execEvents, executing, execFinalOutput,
            quickExecute, doExecute,
            // 编辑器
            currentWorkflow, editorNodes, editorEdges, editorVariables,
            selectedNodeId, selectedEdgeId, selectedNode, saving,
            canvasRef, svgRef, zoom, svgViewBox,
            nodeTypes, nodeIcon, nodeLabel, paletteColor, nodeColor, nodeWidth, nodeHeight, conditionPortY,
            diamondPoints, hexagonPoints, getPortPos,
            toolArgsJson, updateToolArgs,
            codeInputsJson, updateCodeInputs,
            openEditor, backToList, saveWorkflow,
            // 画布交互
            onDragStart, onCanvasDrop, addConditionBranch,
            onNodeMouseDown, onCanvasMouseDown, onCanvasMouseMove, onCanvasMouseUp, onCanvasClick,
            selectNode, selectEdge, deleteSelected, clearSelection,
            onPortMouseDown, onPortMouseUp,
            draggingEdge, draggingEdgePath, getEdgePath, getEdgeMidpoint,
            zoomIn, zoomOut, resetZoom, fitView, autoLayout,
            addVariable,
            // 多选
            selectedNodeIds,
            // 框选
            selectionBox, onWheel,
            onSvgMouseDown, onSvgMouseMove, onSvgMouseUp,
            // 对齐辅助线
            alignGuides,
            // 撤销/重做
            undo, redo, canUndo, canRedo,
            // 连线动画
            edgeAnimations,
            // MiniMap
            minimapViewBox, minimapViewport, minimapNodeColor, onMinimapClick,
            // 节点执行状态
            nodeExecStatus, getNodeExecStatus,
            // 编辑器执行
            showEditorExec, editorExecInputs, editorExecEvents, editorExecuting, editorExecOutput,
            openExecPanel, doEditorExecute, execRequiredVars,
            // 工具和 Agent 列表
            availableTools, availableAgents, toolListLoading, agentListLoading,
            // 事件
            eventIcon, eventText, formatNodeOutput, handleExecEvent,
            // Element Plus 图标
            Plus: ElementPlusIconsVue.Plus,
        };
    }
}).use(ElementPlus).mount('#wf-app');
