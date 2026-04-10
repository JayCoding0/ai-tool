# 📋 AI Agent 平台 - TODO List

> 对标 Dify、Coze（扣子）、FastGPT、LangChain/LangGraph、AutoGen、CrewAI 等主流平台
> 最后更新：2026-04-09

---

## 🔴 P0 - 核心缺失（影响平台竞争力）

### 1. ~~Prompt 模板变量系统~~ ✅ 已完成
- **现状**: ~~System Prompt 是纯文本硬编码，无变量插值~~
- **已实现**:
  - `{{变量名}}` 模板语法，内置变量（current_time/current_date/user_name/user_id/session_id/model_name/knowledge_context）
  - 三级变量优先级：用户级（DB）→ 会话级（DB）→ 请求级
  - MySQL 持久化（prompt_vars_user / prompt_vars_session 表）
  - 完整 CRUD API（5 个接口）
- **相关文件**: `application/prompt_template.go`, `application/prompt_vars_service.go`, `infrastructure/promptvars/mysql/`

### 2. ~~可视化 Workflow / DAG 编排~~ ✅ 已完成
- **现状**: ~~仅有硬编码的主/子 Agent 模式，Agent 定义写死在 `bootstrap.go` 的 `registerAgents()` 中~~
- **已实现**:
  - DAG 工作流引擎：基于 Kahn 拓扑排序的节点编排执行
  - 7 种节点类型：Start、End、LLM 对话、工具调用、子 Agent、模板转换、HTTP 请求
  - `${变量名}` / `${node_id.output}` 模板变量系统，支持节点间数据传递
  - 前端可视化画布编辑器（Vue 3 + Canvas 拖拽连线）
  - 工作流 CRUD + 版本管理 + 发布/草稿/归档状态
  - SSE 流式执行事件推送（node_start/node_output/node_done/workflow_done）
  - 执行记录持久化（workflow_runs 表），支持历史回溯
  - 全局变量定义 + 自动检测节点配置中的变量引用
  - 工具/Agent 下拉选择（从后端加载已注册列表）
- **相关文件**: `domain/workflow/workflow.go`, `application/workflow_engine.go`（核心定义）, `application/workflow_engine_dag.go`（DAG 调度器）, `application/workflow_engine_nodes.go`（节点执行器）, `application/workflow_service.go`, `interfaces/http/workflow_handler.go`, `infrastructure/workflow/mysql/`, `frontend/workflow.html`（HTML 模板）, `frontend/workflow-style.css`（样式）, `frontend/js/workflow.js`（JS 逻辑）
- **预估工作量**: ~~3 个月~~ → 实际 Phase 1 完成

- **Phase 2（4 周）— 条件分支 + 并行**:
  - [x] Condition 节点（条件表达式评估，支持 `==`/`!=`/`>`/`<`/`>=`/`<=`/`contains`/`not_contains`/`is_empty`/`is_not_empty`/`starts_with`/`ends_with` 等操作符，根据条件走不同分支，支持默认分支）
  - [x] Parallel 并行网关节点（基于入度的 DAG 并发调度引擎，多分支 goroutine 并行执行，汇聚节点自动合并所有上游分支结果）
  - [x] 执行引擎重构（从串行 for 循环改为基于入度的并发调度，sync.Mutex 保护并发安全，支持条件分支路径跳过传播）
  - [x] Agent 节点增强（复用 CallSubAgent，支持传入上下文和接收结构化输出）
  - [x] HTTP 请求节点增强（支持请求头模板变量、超时配置）
  - [x] 前端可视化画布升级（节点形状差异化、SVG 渐变色、四向连接端口、智能连线路径、点阵背景）
  - [ ] 进一步升级（Vue Flow / ReactFlow 替代原生 Canvas，支持条件分支连线、并行分支布局）

- **Phase 3（4 周）— 高级特性**:
  - [x] Code 节点（嵌入式 JavaScript 代码执行，goja 沙箱隔离，支持读取上游节点输出，超时控制）
  - [x] Loop 循环节点（支持 for-each 遍历列表、while 条件循环，可配置最大迭代次数，循环体 JS 代码执行）
  - [ ] Workflow 版本管理增强（草稿 → 发布 → 归档，版本对比 diff，回滚到历史版本）
  - [ ] 执行历史 + 回放（可视化回放每个节点的执行过程，查看中间数据）
  - [x] Workflow 导入/导出（JSON 格式，支持跨实例迁移，前端一键导入/导出）

### 3. 长期记忆 / Memory 系统
- **现状**: 仅有会话级滑动窗口（`maxPromptMessages = 20`），无跨会话记忆
- **主流做法**: Coze/Dify 支持变量持久化、用户画像记忆、向量记忆库
- **改进方案**:
  - [ ] 摘要记忆：长对话自动摘要压缩（LLM 生成摘要替代旧消息）
  - [ ] 向量记忆：跨会话语义检索（将重要对话片段向量化存储）
  - [ ] 用户画像：结构化 KV 存储（自动提取用户偏好、习惯）
  - [ ] 基于 token 计数的上下文窗口管理（替代当前的消息数滑动窗口）
- **预估工作量**: 2 个月

### 4. 多模态支持
- **现状**: 仅支持纯文本输入输出
- **主流做法**: GPTs/Coze 支持图片理解、图片生成、语音、视频
- **改进方案**:
  - [ ] 扩展 `model.Message` 支持 `image_url` / `file` 类型（OpenAI Vision API 格式）
  - [ ] 前端支持图片上传（拖拽/粘贴）和图片展示
  - [ ] 集成图片生成工具（DALL-E / Stable Diffusion）
  - [ ] 支持文件上传解析（PDF/Word/Excel → 文本提取）
- **预估工作量**: 2 个月

---

## 🟡 P1 - 重要改进（提升用户体验和可用性）

### 5. 向量数据库集成
- **现状**: RAG 检索是全量加载到内存做余弦相似度（`Search()` 中 `ListChunks` 加载所有分块），数据量大时性能堪忧
- **主流做法**: Dify/FastGPT 集成 Milvus、Qdrant、Weaviate、Pinecone 等专业向量库
- **改进方案**:
  - [ ] 抽象 `VectorStore` 接口（Insert/Search/Delete）
  - [ ] 实现 Milvus 适配器（推荐首选）
  - [ ] 实现 Qdrant 适配器
  - [ ] 实现 pgvector 适配器（PostgreSQL 扩展）
  - [ ] MySQL 全量检索仅作为 fallback / 开发模式
- **预估工作量**: 2 个月

### 6. RAG 高级策略
- **现状**: 仅有基础的 Top-K 余弦相似度检索，固定分块大小 500 字符
- **主流做法**: Dify 支持混合检索（向量+关键词）、重排序（Rerank）、父子分块、递归摘要
- **改进方案**:
  - [ ] 混合检索：BM25 关键词检索 + 向量语义检索，加权融合
  - [ ] Rerank 模型重排序（Cohere Rerank / BGE Reranker）
  - [ ] 自适应分块策略（按段落/语义边界分块，而非固定字符数）
  - [ ] 多知识库联合检索
  - [ ] 检索结果引用溯源（标注来源文档和页码）
- **预估工作量**: 2 个月

### 7. Agent 可观测性 / 调试面板
- **现状**: 仅有 zap 日志，无结构化 trace
- **主流做法**: LangSmith/Dify 提供完整的 LLM 调用链追踪、token 消耗分析、延迟分布
- **改进方案**:
  - [ ] 集成 OpenTelemetry，记录每次 LLM 调用的 input/output/latency/tokens
  - [ ] 记录 ReAct 循环的每一步（思考→工具调用→观察→回答）
  - [ ] 前端提供调试面板：查看完整的推理链路、token 消耗、耗时分布
  - [ ] 支持 Prompt 调试（查看实际发送给模型的完整 messages）
- **预估工作量**: 1.5 个月

### 8. 错误恢复 / 重试机制
- **现状**: ReAct 循环中工具执行失败直接返回错误文本给模型，无重试机制
- **主流做法**: AutoGen/CrewAI 支持工具调用失败重试、人工介入、降级策略
- **改进方案**:
  - [ ] 工具调用自动重试（指数退避，最多 3 次）
  - [ ] Human-in-the-loop：工具执行前可配置人工确认
  - [ ] 降级策略配置（工具不可用时的替代方案）
  - [ ] LLM 调用失败时自动切换备用模型
- **预估工作量**: 1 个月

### 9. 对话分叉 / 重新生成
- **现状**: 不支持对某条消息重新生成或从某个节点分叉
- **主流做法**: ChatGPT/Coze 支持 "Regenerate"、编辑历史消息重新生成
- **改进方案**:
  - [ ] 消息表增加 `parent_id` 字段支持树状对话结构
  - [ ] 前端支持 "重新生成" 按钮
  - [ ] 前端支持编辑历史消息并重新生成后续对话
  - [ ] 支持对话分支切换（左右箭头切换不同生成结果）
- **预估工作量**: 1 个月

### 10. API Key / Provider 管理
- **现状**: 全局共用一个 API Key，无用户级别的 Key 管理
- **主流做法**: Dify/Coze 支持用户自带 API Key、多 Provider 配置
- **改进方案**:
  - [ ] 增加 Provider 管理模块（OpenAI / Azure / 通义千问 / DeepSeek 等）
  - [ ] 支持多个 API Key 配置和负载均衡
  - [ ] 用户级 API Key 管理（用户自带 Key）
  - [ ] 用量配额和计费统计
- **预估工作量**: 1.5 个月

---

## 🟢 P2 - 锦上添花（提升平台完整度）

### 11. 插件 / 工具市场
- **现状**: 工具通过 `skills/` 目录静态加载，无在线安装机制
- **改进方案**:
  - [ ] 设计 Skill 包格式（zip：manifest.json + 脚本文件）
  - [ ] 支持在线上传/安装/卸载工具包
  - [ ] 工具版本管理和依赖检查
  - [ ] 工具市场前端页面（分类、搜索、评分）
- **预估工作量**: 2 个月

### 12. 安全护栏 / Guardrails
- **现状**: 仅有 IP 限流，无内容安全审核
- **改进方案**:
  - [ ] 输入过滤中间件：敏感词检测、Prompt 注入防护
  - [ ] 输出过滤中间件：PII（个人信息）脱敏、有害内容过滤
  - [ ] 输出格式校验（JSON Schema 约束）
  - [ ] 可配置的安全策略（按 Agent / 用户组）
- **预估工作量**: 1 个月

### 13. 对话导出
- **现状**: 不支持导出
- **改进方案**:
  - [ ] 导出为 Markdown 格式
  - [ ] 导出为 JSON 格式（含完整元数据）
  - [ ] 导出为 PDF（服务端渲染）
  - [ ] 支持批量导出
- **预估工作量**: 0.5 个月

### 14. Agent 配置版本管理
- **现状**: Agent 配置无版本概念，修改即生效
- **改进方案**:
  - [ ] Agent 配置版本化（草稿 → 发布 → 归档）
  - [ ] 支持回滚到历史版本
  - [ ] 灰度发布（按用户比例切流）
  - [ ] 配置变更审计日志
- **预估工作量**: 1 个月

### 15. WebSocket 替代 SSE
- **现状**: 使用 SSE 单向推送
- **改进方案**:
  - [ ] 引入 WebSocket 支持双向通信
  - [ ] 支持中断生成（用户主动停止）
  - [ ] 实时状态同步（工具执行进度、Agent 切换通知）
  - [ ] 心跳保活和断线重连
- **预估工作量**: 1 个月

---

## 🔵 P3 - 企业级特性

### 16. 多租户 / 工作空间
- **现状**: 用户数据通过 `user_id` 简单隔离
- **改进方案**:
  - [ ] 增加 Workspace（工作空间）概念
  - [ ] 团队协作：共享 Agent、知识库、工具
  - [ ] 资源隔离：按工作空间隔离数据和配额
  - [ ] 角色权限：工作空间管理员 / 成员 / 访客
- **预估工作量**: 2 个月

### 17. 批量 / 异步任务
- **现状**: A2A 支持异步任务，但无批量处理能力
- **改进方案**:
  - [ ] 批量任务接口（CSV 输入 → 批量处理 → 结果导出）
  - [ ] 数据集测试（用测试集评估 Agent 效果）
  - [ ] 定时任务（Cron 触发 Agent 执行）
  - [ ] 任务队列和进度追踪
- **预估工作量**: 1.5 个月

### 18. 认证系统增强
- **现状**: 自定义 Token（非 JWT），固定 24 小时有效期，无刷新机制
- **改进方案**:
  - [ ] 引入 JWT + Refresh Token 机制
  - [ ] 支持 OAuth2 / SSO 集成（企业微信、飞书、GitHub）
  - [ ] API Key 认证（面向开发者调用）
  - [ ] 登录设备管理和异地登录提醒
- **预估工作量**: 1.5 个月

---

## ⚪ P4 - 代码层优化

### 19. RAG 性能优化（紧急）
- **现状**: `knowledge_service.go` 的 `Search()` 全量加载所有分块到内存计算相似度
- **改进方案**:
  - [ ] 短期：增加分页加载 + 缓存热点分块
  - [ ] 中期：MySQL 端计算余弦相似度（存储向量为 JSON，用存储过程计算）
  - [ ] 长期：迁移到专业向量数据库（见 P1 #5）

### 20. 上下文窗口管理优化
- **现状**: 基于消息数的滑动窗口（`maxPromptMessages = 20`），未考虑 token 数量
- **改进方案**:
  - [ ] 引入 token 计数器（tiktoken-go）
  - [ ] 基于 token 数量的动态窗口管理
  - [ ] 超长对话自动摘要压缩（LLM 生成摘要替代旧消息）

### 21. Agent 定义数据库化
- **现状**: Agent 定义硬编码在 `bootstrap_agents.go` 的 `registerAgents()` 中
- **改进方案**:
  - [ ] Agent 定义存入数据库（agent 表）
  - [ ] 提供 Agent CRUD API
  - [ ] 前端提供 Agent 配置界面（System Prompt 编辑器、工具选择、模型选择）
  - [ ] 支持 Agent 导入/导出（JSON 格式）

### 22. ~~大文件 / 大方法拆分~~ ✅ 已完成
- **现状**: ~~多个文件超过 500 行，职责混杂，不利于维护~~
- **已完成**:
  - `workflow_engine.go`（31KB/1023行 → 8KB）拆分为 3 个文件：
    - `workflow_engine.go` — 引擎核心定义（WorkflowEvent / ExecutionContext / WorkflowEngine 结构体 + Execute 入口）
    - `workflow_engine_dag.go` — DAG 并发调度器（runDAG / activateDownstream / propagateSkip 等）
    - `workflow_engine_nodes.go` — 节点执行器（executeLLMNode / executeToolNode 等）+ 条件评估 + 模板解析
  - `bootstrap.go`（17KB → 12KB）拆分为 2 个文件：
    - `bootstrap.go` — 组件初始化核心（模型工厂 / AgentCard / InitComponents / A2A / MCP）
    - `bootstrap_agents.go` — 多 Agent 注册中心（InitAgentRegistry / registerAgents / restoreAgentToolsFromDB）
  - `builtin_tools.go`（16KB → 5KB）拆分为 2 个文件：
    - `builtin_tools.go` — 工具加载器核心（LoadToolsFromSkillsDir / registerScriptTool）
    - `builtin_tools_executors.go` — 工具执行器实现（天气 / IP / 目录 / 脚本执行器）
  - `workflow.html`（86KB/1614行 → 28KB/566行）拆分为 3 个文件：
    - `workflow.html` — 仅 HTML 模板结构
    - `workflow-style.css` — 工作流专用 CSS 样式
    - `js/workflow.js` — 所有 Vue.js 逻辑

### 23. 测试覆盖率提升
- **现状**: 缺少单元测试和集成测试
- **改进方案**:
  - [ ] 核心服务层单元测试（chat_service, agent_runner, knowledge_service）
  - [ ] 模板引擎单元测试（prompt_template）
  - [ ] HTTP 接口集成测试
  - [ ] RAG 端到端测试（上传 → 分块 → 检索 → 回答）

---

## 📊 实施路线图

```
2026 Q2 (4-6月)
├── ✅ Prompt 模板变量系统（已完成）
├── ✅ 可视化 Workflow / DAG 编排（Phase 1 已完成）
├── ✅ Workflow Phase 2：条件分支 + 并行（P0 #2，已完成）
├── ✅ Workflow Phase 3：Code 节点 + Loop 节点 + 导入导出（P0 #2，已完成）
├── 🔴 Workflow Phase 3 剩余：版本管理增强 + 执行历史回放（P0 #2）
├── 🔴 向量数据库集成（P1 #5）
├── 🔴 RAG 性能优化（P4 #19）
├── 🟡 Agent 可观测性（P1 #7）
└── 🟡 错误恢复/重试（P1 #8）

2026 Q3 (7-9月)
├── 🔴 长期记忆 / Memory 系统（P0 #3）
├── 🔴 多模态支持 - 图片（P0 #4）
├── 🟡 RAG 高级策略（P1 #6）
├── 🟡 对话分叉/重新生成（P1 #9）
└── 🟢 对话导出（P2 #13）

2026 Q4 (10-12月)
├── 🟡 API Key / Provider 管理（P1 #10）
├── 🟢 安全护栏（P2 #12）
├── 🟢 WebSocket（P2 #15）
└── 🔵 认证系统增强（P3 #18）

2027 Q1 (1-3月)
├── 🟢 插件/工具市场（P2 #11）
├── 🟢 Agent 配置版本管理（P2 #14）
├── 🔵 多租户/工作空间（P3 #16）
└── 🔵 批量/异步任务（P3 #17）
```

---

## 💡 当前优势（保持并强化）

- ✅ **协议支持**: A2A + MCP 双协议，领先大部分开源平台
- ✅ **工具系统**: ReAct 循环 + 脚本驱动工具，灵活度高
- ✅ **架构设计**: DDD 分层架构，代码组织清晰，大文件已按职责拆分
- ✅ **Prompt 模板变量**: 三级变量优先级 + 数据库持久化（已实现）
- ✅ **Workflow 编排**: DAG 可视化工作流引擎 + 11 种节点类型（含 Condition / Parallel / Code / Loop）+ SSE 流式执行 + 并发调度 + 导入导出（已实现）
