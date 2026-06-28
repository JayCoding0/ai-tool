# 📋 AI Agent 平台 - TODO List

> 对标 Dify、Coze（扣子）、FastGPT、LangChain/LangGraph、AutoGen、CrewAI 等主流平台
> 最后更新：2026-06-28

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
  - [x] 进一步升级（dagre 自动布局 + MiniMap 小地图 + 框选多选 + 鼠标滚轮缩放 + 对齐辅助线 + 撤销重做 + 连线动画 + 键盘快捷键）

- **Phase 3（4 周）— 高级特性**:
  - [x] Code 节点（嵌入式 JavaScript 代码执行，goja 沙箱隔离，支持读取上游节点输出，超时控制）
  - [x] Loop 循环节点（支持 for-each 遍历列表、while 条件循环，可配置最大迭代次数，循环体 JS 代码执行）
  - [ ] Workflow 版本管理增强（草稿 → 发布 → 归档，版本对比 diff，回滚到历史版本）
  - [ ] 执行历史 + 回放（可视化回放每个节点的执行过程，查看中间数据）
  - [x] Workflow 导入/导出（JSON 格式，支持跨实例迁移，前端一键导入/导出）

### 3. 长期记忆 / Memory 系统
- **现状**: 仅有会话级滑动窗口（`maxPromptMessages = 20`），无跨会话记忆
- **主流做法**: Mem0（提取→更新双阶段记忆管理）、Coze（变量持久化+用户画像）、Dify（会话摘要+向量记忆库）、ChatGPT（跨会话记忆+用户偏好提取）
- **参考架构**: Mem0 论文 — 从对话中提取候选记忆 → 与现有记忆库比对 → 决定 ADD/UPDATE/DELETE/NOOP 操作，维护一致性知识库

- **Phase 1（3 周）— 会话摘要记忆 + Token 窗口管理**:
  - [x] **Token 计数器**：基于 rune 计数的混合估算策略（中英文混合场景经验公式），实现 `EstimateTokens()` / `EstimateMessagesTokens()`
  - [x] **动态上下文窗口**：替代当前 `maxPromptMessages = 20` 的固定消息数滑动窗口，改为基于模型 `max_context_tokens` 的动态 token 预算管理（预留 System Prompt + RAG + 记忆 + 回复空间），未配置时自动回退到固定消息数模式
  - [x] **会话摘要生成**：当会话消息超过 token 预算时，自动调用 LLM 将旧消息压缩为摘要（异步执行，不阻塞用户对话）
  - [x] **摘要持久化**：`chat_sessions` 表增加 `summary TEXT` 字段，存储当前会话的累积摘要
  - [x] **摘要注入**：`buildMessagesWithRAG()` 升级为 `buildMessagesWithContext()`，在 System Prompt 后注入会话摘要（`## 对话历史摘要\n{summary}`），再拼接最近的消息窗口
  - [x] **增量摘要策略**：每次摘要不重新处理全部历史，而是将"旧摘要 + 被淘汰的消息"合并生成新摘要（参考 LangChain ConversationSummaryBufferMemory）
  - **相关文件**: `application/chat_service.go`（`buildMessagesWithContext` 改造）, `application/token_counter.go`（Token 计数与预算管理）, `application/summary_service.go`（摘要服务）, `domain/session/session.go`（增加 Summary 字段）, `infrastructure/session/mysql/`（DDL + 持久化）, `database/migrations/001_add_session_summary.sql`（迁移脚本）

- **Phase 2（4 周）— 向量记忆（跨会话语义检索）**:
  - [x] **记忆领域模型**：新建 `domain/memory/` 包，定义 `Memory` 实体（ID/UserID/Content/Embedding/MemoryType/Source/Importance/AccessCount/CreatedAt/UpdatedAt/ExpiredAt）
  - [x] **记忆类型枚举**：`fact`（事实性记忆，如"用户是Go开发者"）、`preference`（偏好，如"喜欢简洁回答"）、`episode`（情景记忆，重要对话片段）、`summary`（会话摘要归档）
  - [x] **记忆仓储接口**：`memory.Repository`（CreateMemory/UpdateMemory/DeleteMemory/ListByUser/SearchByEmbedding）
  - [x] **MySQL 持久化**：新建 `user_memories` 表（id/user_id/content/embedding MEDIUMBLOB/memory_type/source_session_id/importance FLOAT/access_count/created_at/updated_at/expired_at），复用现有 `knowledge_chunks` 的向量存储模式
  - [x] **记忆提取器**（参考 Mem0 Extraction Phase）：每轮对话结束后，异步调用 LLM 从对话中提取值得记忆的信息（Prompt: "从以下对话中提取用户的关键信息、偏好、事实，以 JSON 数组格式返回 [{content, type, importance}]"）
  - [x] **记忆更新器**（参考 Mem0 Update Phase）：将新提取的记忆与用户现有记忆库做向量相似度比对，决定操作：
    - 相似度 > 0.9 → **UPDATE**（合并/更新已有记忆）
    - 相似度 < 0.5 → **ADD**（新增记忆）
    - 新信息与旧记忆矛盾 → **DELETE** 旧记忆 + **ADD** 新记忆
    - 无新信息 → **NOOP**
  - [x] **记忆检索注入**：`buildMessagesWithContext()` 增加记忆检索步骤 — 将用户当前消息向量化，从 `user_memories` 中检索 Top-5 相关记忆，注入 System Prompt（`## 用户记忆\n以下是关于该用户的已知信息：\n{memories}`）
  - [x] **记忆衰减机制**：长期未被检索命中的记忆降低 importance 分数，低于阈值自动归档/删除（模拟人类遗忘曲线）
  - [x] **记忆管理 API**：CRUD 接口（GET/POST/PUT/DELETE `/api/memory`），支持用户查看和手动管理自己的记忆
  - **相关文件**: `domain/memory/memory.go`（领域模型）, `domain/memory/repository.go`（仓储接口）, `application/memory_service.go`（记忆服务）, `infrastructure/memory/mysql/memory_repository.go`（MySQL 实现）, `interfaces/http/memory_handler.go`（API）, `database/migrations/002_add_user_memories.sql`（迁移脚本）

- **Phase 3（3 周）— 用户画像 + 智能记忆策略**:
  - [ ] **用户画像实体**：`domain/memory/user_profile.go`，结构化 KV 存储（姓名/职业/技术栈/语言偏好/沟通风格/常用工具/关注领域等），JSON 格式存入 `user_profiles` 表
  - [ ] **画像自动提取**：每 N 轮对话后，LLM 从累积记忆中提炼/更新用户画像（Prompt: "根据以下用户记忆，生成/更新结构化用户画像 JSON"）
  - [ ] **画像注入 System Prompt**：在 `renderPromptTemplate()` 中增加 `{{user_profile}}` 内置变量，自动注入用户画像
  - [x] **记忆重要性评估**：引入独立 LLM 评分机制（Phase 1.5），对每条候选记忆评估重要性（0-1），参考用户已有记忆去重，仅存储 importance > 0.3 的记忆，避免记忆库膨胀
  - [ ] **记忆容量管理**：每用户设置记忆上限（如 500 条），超限时按 importance × recency 加权排序，淘汰最低分记忆
  - [ ] **记忆来源追溯**：每条记忆关联 `source_session_id`，支持追溯记忆来源对话
  - [ ] **前端记忆面板**：用户可查看/编辑/删除自己的记忆列表和用户画像，支持手动添加记忆
  - **相关文件**: `domain/memory/user_profile.go`, `application/memory_service.go`（画像提取逻辑）, `application/prompt_template.go`（`{{user_profile}}` 变量）

- **数据库设计**:
  ```sql
  -- 用户记忆表（向量记忆 + 结构化记忆统一存储）
  CREATE TABLE user_memories (
      id                BIGINT AUTO_INCREMENT PRIMARY KEY,
      user_id           BIGINT NOT NULL COMMENT '用户ID',
      content           TEXT NOT NULL COMMENT '记忆内容（自然语言描述）',
      embedding         MEDIUMBLOB COMMENT '向量（复用 knowledge_chunks 的存储方式）',
      memory_type       ENUM('fact','preference','episode','summary') NOT NULL DEFAULT 'fact',
      source_session_id VARCHAR(36) COMMENT '来源会话ID',
      importance        FLOAT NOT NULL DEFAULT 0.5 COMMENT '重要性分数 0-1',
      access_count      INT NOT NULL DEFAULT 0 COMMENT '被检索命中次数',
      created_at        TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
      updated_at        TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
      expired_at        TIMESTAMP NULL COMMENT '过期时间（NULL=永不过期）',
      INDEX idx_user_id (user_id),
      INDEX idx_memory_type (user_id, memory_type),
      INDEX idx_importance (user_id, importance)
  );

  -- 用户画像表
  CREATE TABLE user_profiles (
      id         BIGINT AUTO_INCREMENT PRIMARY KEY,
      user_id    BIGINT NOT NULL UNIQUE COMMENT '用户ID',
      profile    JSON NOT NULL COMMENT '结构化画像（姓名/职业/偏好等）',
      version    INT NOT NULL DEFAULT 1 COMMENT '画像版本号',
      created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
      updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
  );

  -- chat_sessions 表增加摘要字段
  ALTER TABLE chat_sessions ADD COLUMN summary TEXT DEFAULT NULL COMMENT '会话摘要（长对话自动压缩）';
  ```

- **架构流程图**:
  ```
  用户消息 → Token 预算计算 → 检索用户记忆(Top-5) → 检索用户画像
       ↓                              ↓                    ↓
  [System Prompt] + [用户画像] + [相关记忆] + [会话摘要] + [最近N条消息] + [RAG知识库]
       ↓
  发送给 LLM → 生成回复 → 异步提取记忆 → Mem0 式更新（ADD/UPDATE/DELETE）
  ```

- **与现有架构的集成点**:
  - 复用 `knowledge.Embedder` 接口进行记忆向量化（与 RAG 共享 embedding 模型）
  - 复用 `infrastructure/knowledge/similarity.go` 的余弦相似度计算
  - 参考 `PromptVarsService` 的服务模式，新建 `MemoryService` 注入 `ChatService`
  - 参考 `prompt_vars_user` 表的 user_id 隔离模式
  - `buildMessagesWithRAG()` 扩展为 `buildMessagesWithContext()`，统一管理 RAG + 记忆 + 摘要 + 画像的注入

- **预估工作量**: 2.5 个月（Phase 1: 3周 → Phase 2: 4周 → Phase 3: 3周）

### 4. 多模态支持
- **现状**: 仅支持纯文本输入输出
- **主流做法**: GPTs/Coze 支持图片理解、图片生成、语音、视频
- **改进方案**:
  - [ ] 扩展 `model.Message` 支持 `image_url` / `file` 类型（OpenAI Vision API 格式）
  - [ ] 前端支持图片上传（拖拽/粘贴）和图片展示
  - [ ] 集成图片生成工具（DALL-E / Stable Diffusion）
  - [ ] 支持文件上传解析（PDF/Word/Excel → 文本提取）
- **预估工作量**: 2 个月

### 24. ✅ 模型层能力补齐（结构化输出 + 推理模型 + 并行工具调用）已完成
- **现状**: ~~无 `response_format` / `json_schema` 支持（代码 0 处引用）~~；~~`model.StreamChunk.Thinking` 仅用于多 Agent 思考过程展示，无 reasoning 模型原生支持~~；~~ReAct 为串行工具调用循环~~
- **主流做法**: OpenAI Structured Output（JSON Schema 强约束）、`reasoning_effort` 参数、parallel tool calls；Dify/Coze 的 JSON mode
- **改进方案**:
  - [x] `model.GenerateOptions` 增加 `ResponseFormat`（text / json_object / json_schema）+ 可选 `StructuredGenerator` 接口，`openai_generator` 实现 `GenerateWithToolsOpts` 透传 `response_format`/温度
  - [x] Workflow LLM 节点支持 JSON Schema 约束输出（NodeConfig 新增 `response_format`/`json_schema`/`schema_strict`），返回内容做 JSON 合法性校验 + markdown 围栏剥离；不支持原生 response_format 的生成器自动降级为 Prompt 注入 JSON 指令；前端编辑器新增输出格式配置 UI
  - [x] 推理模型支持：`GenerateOptions` 增加 `ReasoningEffort`（low/medium/high）并透传 OpenAI `reasoning_effort`；流式 `GenerateStreamWithMessages` 从 delta 的 `reasoning_content` 扩展字段（DashScope/DeepSeek-R1）分离思考过程到 `Thinking`（聊天界面自动展示）；Workflow LLM 节点新增 `reasoning_effort` 配置 + 前端 UI
  - [x] 并行工具调用：ReAct 循环 `executeToolsConcurrently` 对单轮多 `tool_calls` 用 goroutine + `sync.WaitGroup` 并发执行，按索引聚合结果保序，再按序推送 `tool_result` 事件（OpenAI `parallel_tool_calls` 默认开启，无需额外透传）
- **相关文件**: `domain/model/model.go`（`ResponseFormat`/`GenerateOptions`/`StructuredGenerator`）, `infrastructure/model/openai_generator.go`（`GenerateWithToolsOpts` + `extractReasoningContent`）, `domain/workflow/workflow.go`（NodeConfig 字段）, `application/workflow_engine_nodes.go`（`executeLLMNode` + `buildLLMGenerateOptions`/`extractJSONContent`/`injectJSONInstruction`）, `application/agent_runner.go`（`executeToolsConcurrently` 并发工具执行）, `frontend/workflow.html`（配置 UI）
- **预估工作量**: ~~1 个月~~ → 已完成

### 25. ✅ Agent 评估体系（Evaluation）已完成 MVP + 增强
- **现状**: ~~无任何评测/数据集系统，Agent/Prompt/Workflow 改动无法量化好坏~~
- **主流做法**: LangSmith / Dify / Coze：测试数据集 → 批量运行 → LLM-as-judge 自动打分 → 版本回归对比
- **改进方案**:
  - [x] 评测数据集管理（数据集 CRUD + 用例单条/JSON 批量导入）
  - [x] 批量运行 Agent（复用 `agentRunner.runReActLoop` 跑真实 Agent 路径，含工具调用），记录每条结果、耗时、token；异步执行 + 前端轮询进度
  - [x] 评分器三选一：精确匹配（exact）/ 语义相似度（semantic，复用 `knowledge.Embedder` + 余弦相似度）/ LLM-as-judge（复用 #24 结构化输出严格返回 `{score,reason}`）
  - [x] 版本回归对比（`CompareRuns` 按 case_id 匹配两次运行，输出平均分变化 + 逐用例 delta + 变好/变差/持平计数）
  - [x] 前端评测报告页（通过率/平均分/逐条结果卡片：输入/期望/实际输出/通过标记/评分理由/耗时，运行中自动刷新）+ 运行对比页
  - [ ] Workflow 评测（当前仅支持 Agent，Workflow 批量评测待补）
  - [ ] CSV 导入 / 报告导出
- **相关文件**: `domain/eval/`（实体+仓储接口）, `infrastructure/eval/mysql/eval_repository.go`, `application/eval_service.go`（CRUD + 批量运行 + 三种评分器 + CompareRuns）, `interfaces/http/eval_handler.go`, `bootstrap.go`（initEvalService）, `frontend/eval.html`, `database/migrations/004_add_eval_tables.sql` + `005_add_eval_scorer.sql`
- **预估工作量**: ~~2 个月~~ → MVP + 增强已完成（Workflow 评测/CSV/导出待补）

### 26. 🆕 应用发布与开放生态（API / Widget / Webhook）
- **现状**: Agent/Workflow 仅能通过自带 Web UI 使用，无对外发布形态
- **主流做法**: Dify 一键发布为 API / 嵌入式聊天 Widget / 独立 Web App；Coze Bot Store 模板市场
- **改进方案**:
  - [ ] 发布为标准对话 API（独立 App API Key + 用量隔离 + 限流）
  - [ ] 嵌入式 Web Widget（一行 `<script>` 嵌入任意网站，iframe / 浮窗）
  - [ ] Webhook / 触发器（外部事件触发 Agent/Workflow 执行）
  - [ ] 应用模板市场（Agent/Workflow 模板一键复制创建）
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

### 27. 🆕 MCP Client（消费外部 MCP 工具）
- **现状**: 已实现 MCP Server（对外暴露自身工具），但无 MCP Client，无法接入第三方 MCP 服务器的工具
- **主流做法**: Claude Desktop / Cursor / Cline 作为 MCP Client 接入海量第三方 MCP Server
- **改进方案**:
  - [ ] 实现 MCP Client（stdio / SSE / streamable-http 传输）
  - [ ] 配置化接入外部 MCP Server，自动发现并注册其 tools 到工具注册表
  - [ ] 外部 MCP 工具纳入 ReAct 循环与 Workflow 工具节点
  - [ ] 前端 MCP Server 管理页（添加/启停/查看工具列表）
- **预估工作量**: 1 个月

### 28. 🆕 语义缓存 / Prompt 缓存
- **现状**: ~~无任何缓存层~~ → 已落地 Redis 缓存基础设施 + Embedding 缓存 + 命中率监控页
- **主流做法**: GPTCache（语义缓存）、OpenAI/Anthropic Prompt Caching
- **改进方案**:
  - [x] **缓存基础设施**：统一 `cache.Cache` 接口（Get/Set/Delete/Size/Clear）+ Redis 实现（命名空间隔离 `aiproject:cache:`，密码走环境变量 `REDIS_PASSWORD`）+ 不可用时自动降级 no-op，不阻断主流程
  - [x] **Embedding 缓存**：`CachedEmbedder` 装饰器（SHA256(model+text) 为 key），RAG 检索 / 跨会话记忆 / 评估语义评分共享，向量化结果确定性强可长期缓存
  - [x] **命中率统计**：进程内原子计数器按类别统计命中/未命中，`GET /api/cache/stats` + `POST /api/cache/clear` 接口
  - [x] **监控页面**：`frontend/cache.html` 总体命中率环形图 + 分类命中率 + 后端状态 + 条目数 + 自动刷新 + 一键清空
  - [ ] LLM 精确缓存：相同 messages 命中缓存直接返回（流式需模拟吐字，带工具调用不缓存）
  - [ ] 语义缓存：问题向量化，相似度超阈值复用历史回答（阈值可配置）
  - [ ] 缓存失效策略（TTL / 知识库更新时失效）
- **相关文件**: `domain/cache/cache.go`, `infrastructure/cache/{redis_cache,noop_cache,stats}.go`, `infrastructure/knowledge/cached_embedder.go`, `interfaces/http/cache_handler.go`, `bootstrap.go`（initCache/wrapEmbedderWithCache）, `frontend/cache.html`
- **预估工作量**: 0.5 个月（基础设施 + Embedding 缓存 + 监控已完成，LLM 语义缓存待补）

### 29. 🆕 成本与配额治理
- **现状**: 仅有事后 Token 统计，无预算/配额/告警机制
- **主流做法**: Dify/Coze 工作空间级配额、超额告警、按部门计费
- **改进方案**:
  - [ ] 用户 / 工作空间级 Token & 费用配额上限
  - [ ] 配额限流（超额拒绝或降级）
  - [ ] 用量预警（接近阈值时告警通知）
  - [ ] 成本报表（按用户 / 模型 / Agent 维度，支持导出）
- **预估工作量**: 1 个月

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

### 30. 🆕 RAG 下一代能力（GraphRAG + 数据源连接器）
- **现状**: 仅支持文档手动上传 + 固定分块 + 向量检索（与 P1 #6 互补，本项聚焦数据接入与知识图谱）
- **主流做法**: 微软 GraphRAG 知识图谱检索；Dify 数据源连接器自动同步
- **改进方案**:
  - [ ] GraphRAG：实体/关系抽取构建知识图谱，图谱增强检索
  - [ ] 数据源连接器：Notion / 网页 / 数据库 / 对象存储自动同步
  - [ ] 增量更新与定时同步（数据源变更自动重新索引）
  - [ ] 多模态知识解析（表格、图表、扫描件）
- **预估工作量**: 2.5 个月

### 31. 🆕 国际化（i18n）
- **现状**: 仅中文界面，无多语言支持
- **改进方案**:
  - [ ] 前端 i18n 框架接入，文案抽取为语言包
  - [ ] 中 / 英文切换，记忆用户偏好
  - [ ] 后端错误信息 / 系统提示多语言
- **预估工作量**: 0.5 个月

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
- **现状**: 逐步补齐单元测试（缓存子系统已覆盖，`go test -race ./...` 全绿）
- **改进方案**:
  - [x] 缓存子系统单元测试：命中率统计收集器（含并发）、NoopCache、Redis 集成测试（不可用时跳过）、Embedding 缓存装饰器（命中/批量部分命中/模型隔离）、语义缓存服务（scope 哈希/命中/阈值/隔离/FIFO 淘汰）
  - [ ] 核心服务层单元测试（chat_service, agent_runner, knowledge_service）
  - [ ] 模板引擎单元测试（prompt_template）
  - [ ] HTTP 接口集成测试
  - [ ] RAG 端到端测试（上传 → 分块 → 检索 → 回答）
- **相关文件**: `infrastructure/cache/{stats,noop_cache,redis_cache}_test.go`, `infrastructure/knowledge/cached_embedder_test.go`, `application/semantic_cache_service_test.go`

### 32. 🆕 前端工程化 + 水平扩展能力
- **现状**: 前端 Vue 3 CDN 零构建（适合 Demo，缺组件化 / TS / 构建优化）；SSE + 部分内存态仓储（`infrastructure/session/memory_repository.go`）多实例部署时存在状态一致性风险
- **改进方案**:
  - [ ] 前端引入构建工具（Vite）+ TypeScript + 组件化拆分 + 按需加载
  - [ ] 会话 / 任务状态外置（Redis），支持多实例水平扩展
  - [ ] 引入任务队列（异步执行长任务，见 P3 #17）
  - [ ] 提供 K8s Helm Chart / 健康检查 / 优雅退出
- **预估工作量**: 1.5 个月

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
├── ✅ 模型层能力补齐：结构化输出 + 推理模型 + 并行工具调用（P0 #24，已完成）
├── ✅ Agent 评估体系（P0 #25，已完成 MVP + 三种评分器 + 回归对比）
├── 🆕🔴 应用发布：API / Widget / Webhook（P0 #26，平台价值出口）
├── 🆕🟡 MCP Client：消费外部 MCP 工具（P1 #27）
├── 🔴 Memory Phase 1：会话摘要 + Token 窗口管理（P0 #3，3周）
├── 🔴 Memory Phase 2：向量记忆 + Mem0 式提取更新（P0 #3，4周）
├── 🔴 Memory Phase 3：用户画像 + 智能记忆策略（P0 #3，3周）
├── 🔴 多模态支持 - 图片（P0 #4）
├── 🟡 RAG 高级策略（P1 #6）
├── 🟡 对话分叉/重新生成（P1 #9）
└── 🟢 对话导出（P2 #13）

2026 Q4 (10-12月)
├── 🆕🟡 语义缓存 / Prompt 缓存（P1 #28）
├── 🆕🟡 成本与配额治理（P1 #29）
├── 🆕🟢 国际化 i18n（P2 #31）
├── 🟡 API Key / Provider 管理（P1 #10）
├── 🟢 安全护栏（P2 #12）
├── 🟢 WebSocket（P2 #15）
└── 🔵 认证系统增强（P3 #18）

2027 Q1 (1-3月)
├── 🆕🟢 RAG 下一代：GraphRAG + 数据源连接器（P2 #30）
├── 🆕⚪ 前端工程化 + 水平扩展（P4 #32）
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
