// Package application 应用服务层
// workflow_engine_dag.go — DAG 并发调度器（从 workflow_engine.go 拆分）
package application

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"aiProject/internal/domain/workflow"
	"aiProject/internal/shared"
	"go.uber.org/zap"
)

// ─── DAG 并发调度器 ──────────────────────────────────────────────────────────

// runDAG 基于入度的 DAG 并发调度执行
// 核心思路：维护每个节点的"剩余入度"，当某节点的所有上游都执行完毕（入度降为 0）时，
// 立即启动该节点的执行（goroutine）。同一层级的多个节点可以并行执行。
func (e *WorkflowEngine) runDAG(ctx context.Context, wf *workflow.Workflow, execCtx *ExecutionContext, inputs map[string]interface{}, outCh chan<- WorkflowEvent) {
	defer close(outCh)
	logger := shared.GetLogger()
	startTime := time.Now()
	runID := execCtx.RunID

	// 构建入度表和邻接表
	inDegree := make(map[string]*int32)   // nodeID → 剩余入度（atomic 操作）
	adjacency := make(map[string][]string) // nodeID → 下游节点列表

	for _, node := range wf.Nodes {
		deg := int32(0)
		inDegree[node.ID] = &deg
		adjacency[node.ID] = nil
	}
	for _, edge := range wf.Edges {
		adjacency[edge.Source] = append(adjacency[edge.Source], edge.Target)
		atomic.AddInt32(inDegree[edge.Target], 1)
	}

	// 去重邻接表（一个节点可能有多条边指向同一个下游）
	for nodeID, targets := range adjacency {
		adjacency[nodeID] = uniqueStrings(targets)
	}

	// 错误通道：任一节点失败时通知调度器
	errCh := make(chan error, 1)
	var firstErr error

	// WaitGroup 追踪所有正在执行的节点
	var wg sync.WaitGroup

	// cancelCtx 用于在某个节点失败时取消其他正在执行的节点
	cancelCtx, cancelFunc := context.WithCancel(ctx)
	defer cancelFunc()

	// dispatchNode 尝试调度一个节点执行
	// 该函数本身在 goroutine 中被调用，需要并发安全
	var dispatchNode func(nodeID string)
	dispatchNode = func(nodeID string) {
		node, ok := wf.GetNodeByID(nodeID)
		if !ok {
			return
		}

		// 检查是否已取消
		select {
		case <-cancelCtx.Done():
			return
		default:
		}

		// 检查节点是否被条件分支跳过
		if execCtx.IsSkipped(nodeID) {
			logger.Info("[Workflow] 节点被条件分支跳过",
				zap.String("node_id", nodeID),
				zap.String("node_name", node.Name),
			)
			// 跳过的节点也需要递减下游入度，但同时标记下游也被跳过
			e.propagateSkip(wf, nodeID, execCtx, inDegree, &wg, dispatchNode)
			return
		}

		wg.Add(1)
		go func() {
			defer wg.Done()

			// ── 处理 Start 节点 ──
			if node.Type == workflow.NodeTypeStart {
				execCtx.SetNodeOutput(nodeID, inputs)
				sendWorkflowEvent(cancelCtx, outCh, WorkflowEvent{
					Type:     "node_done",
					NodeID:   nodeID,
					NodeName: node.Name,
					NodeType: string(node.Type),
					RunID:    runID,
				})
				e.activateDownstream(cancelCtx, wf, nodeID, execCtx, inDegree, &wg, dispatchNode, outCh)
				return
			}

			// ── 处理 End 节点 ──
			if node.Type == workflow.NodeTypeEnd {
				sendWorkflowEvent(cancelCtx, outCh, WorkflowEvent{
					Type:     "node_done",
					NodeID:   nodeID,
					NodeName: node.Name,
					NodeType: string(node.Type),
					RunID:    runID,
				})
				// End 节点没有下游，不需要激活
				return
			}

			// ── 推送节点开始事件 ──
			sendWorkflowEvent(cancelCtx, outCh, WorkflowEvent{
				Type:     "node_start",
				NodeID:   nodeID,
				NodeName: node.Name,
				NodeType: string(node.Type),
				RunID:    runID,
			})

			// ── 执行节点 ──
			nodeStart := time.Now()
			output, execErr := e.executeNode(cancelCtx, wf, *node, execCtx)
			duration := time.Since(nodeStart).Milliseconds()

			if execErr != nil {
				logger.Error("[Workflow] 节点执行失败",
					zap.String("node_id", nodeID),
					zap.String("node_name", node.Name),
					zap.Error(execErr),
				)
				sendWorkflowEvent(cancelCtx, outCh, WorkflowEvent{
					Type:       "node_error",
					NodeID:     nodeID,
					NodeName:   node.Name,
					NodeType:   string(node.Type),
					Error:      execErr.Error(),
					DurationMs: duration,
					RunID:      runID,
				})
				// 通知调度器有错误发生
				select {
				case errCh <- execErr:
				default:
				}
				cancelFunc() // 取消其他节点
				return
			}

			// ── 存储节点输出 ──
			execCtx.SetNodeOutput(nodeID, output)

			logger.Info("[Workflow] 节点执行完成",
				zap.String("node_id", nodeID),
				zap.String("node_name", node.Name),
				zap.String("node_type", string(node.Type)),
				zap.Int64("duration_ms", duration),
			)

			sendWorkflowEvent(cancelCtx, outCh, WorkflowEvent{
				Type:       "node_output",
				NodeID:     nodeID,
				NodeName:   node.Name,
				NodeType:   string(node.Type),
				Output:     output,
				DurationMs: duration,
				RunID:      runID,
			})

			// ── 激活下游节点 ──
			e.activateDownstream(cancelCtx, wf, nodeID, execCtx, inDegree, &wg, dispatchNode, outCh)
		}()
	}

	// 启动所有入度为 0 的节点（通常只有 Start 节点）
	for _, node := range wf.Nodes {
		if atomic.LoadInt32(inDegree[node.ID]) == 0 {
			dispatchNode(node.ID)
		}
	}

	// 等待所有节点执行完毕
	wg.Wait()

	// 检查是否有错误
	select {
	case firstErr = <-errCh:
	default:
	}

	totalDuration := time.Since(startTime).Milliseconds()
	nodeOutputs := execCtx.GetNodeOutputsSnapshot()
	totalTokens := execCtx.TotalTokens

	// 更新执行记录
	if firstErr != nil {
		_ = e.runRepo.UpdateStatus(context.Background(), runID, workflow.RunStatusFailed,
			nil, nodeOutputs, totalTokens, totalDuration, firstErr.Error())
		sendWorkflowEvent(ctx, outCh, WorkflowEvent{
			Type:        "workflow_error",
			Error:       firstErr.Error(),
			RunID:       runID,
			DurationMs:  totalDuration,
			TotalTokens: totalTokens,
		})
	} else {
		_ = e.runRepo.UpdateStatus(context.Background(), runID, workflow.RunStatusCompleted,
			nodeOutputs, nodeOutputs, totalTokens, totalDuration, "")
		sendWorkflowEvent(ctx, outCh, WorkflowEvent{
			Type:        "workflow_done",
			Output:      nodeOutputs,
			RunID:       runID,
			DurationMs:  totalDuration,
			TotalTokens: totalTokens,
		})
	}

	logger.Info("[Workflow] 执行完成",
		zap.String("run_id", runID),
		zap.Int64("duration_ms", totalDuration),
		zap.Int("total_tokens", totalTokens),
		zap.Bool("success", firstErr == nil),
	)
}

// activateDownstream 激活下游节点：递减入度，入度归零时调度执行
// 对于 Condition 节点，只激活命中分支的下游；对于普通节点，激活所有下游
func (e *WorkflowEngine) activateDownstream(
	ctx context.Context,
	wf *workflow.Workflow,
	nodeID string,
	execCtx *ExecutionContext,
	inDegree map[string]*int32,
	wg *sync.WaitGroup,
	dispatchNode func(string),
	outCh chan<- WorkflowEvent,
) {
	node, ok := wf.GetNodeByID(nodeID)
	if !ok {
		return
	}

	// Condition 节点：只激活命中分支的下游，其他分支标记为跳过
	if node.Type == workflow.NodeTypeCondition {
		e.activateConditionDownstream(ctx, wf, nodeID, execCtx, inDegree, wg, dispatchNode, outCh)
		return
	}

	// 普通节点 / Parallel 节点：激活所有下游
	downstreamEdges := wf.GetDownstreamEdges(nodeID)
	for _, edge := range downstreamEdges {
		targetDeg, ok := inDegree[edge.Target]
		if !ok {
			continue
		}
		newDeg := atomic.AddInt32(targetDeg, -1)
		if newDeg == 0 {
			dispatchNode(edge.Target)
		}
	}
}

// activateConditionDownstream 条件分支节点的下游激活逻辑
// 评估条件后，只激活命中分支的下游，未命中分支的下游标记为跳过
func (e *WorkflowEngine) activateConditionDownstream(
	ctx context.Context,
	wf *workflow.Workflow,
	nodeID string,
	execCtx *ExecutionContext,
	inDegree map[string]*int32,
	wg *sync.WaitGroup,
	dispatchNode func(string),
	outCh chan<- WorkflowEvent,
) {
	node, _ := wf.GetNodeByID(nodeID)
	logger := shared.GetLogger()

	// 获取所有出边
	allEdges := wf.GetDownstreamEdges(nodeID)

	// 找到命中的分支 handle
	matchedHandle := ""
	for _, cond := range node.Config.Conditions {
		if cond.IsDefault {
			continue
		}
		if e.evaluateCondition(cond, execCtx) {
			matchedHandle = cond.ID
			logger.Info("[Workflow] 条件分支命中",
				zap.String("node_id", nodeID),
				zap.String("branch_id", cond.ID),
				zap.String("label", cond.Label),
			)
			break
		}
	}

	// 如果没有命中任何条件，走默认分支
	if matchedHandle == "" {
		for _, cond := range node.Config.Conditions {
			if cond.IsDefault {
				matchedHandle = cond.ID
				logger.Info("[Workflow] 条件分支走默认路径",
					zap.String("node_id", nodeID),
					zap.String("branch_id", cond.ID),
				)
				break
			}
		}
	}

	// 激活命中分支的下游，跳过未命中分支的下游
	for _, edge := range allEdges {
		targetDeg, ok := inDegree[edge.Target]
		if !ok {
			continue
		}

		if edge.SourceHandle == matchedHandle {
			// 命中分支：正常递减入度
			newDeg := atomic.AddInt32(targetDeg, -1)
			if newDeg == 0 {
				dispatchNode(edge.Target)
			}
		} else {
			// 未命中分支：标记跳过并递减入度
			execCtx.MarkSkipped(edge.Target)
			newDeg := atomic.AddInt32(targetDeg, -1)
			if newDeg == 0 {
				dispatchNode(edge.Target) // dispatchNode 内部会检查 IsSkipped
			}
		}
	}
}

// propagateSkip 传播跳过状态：被跳过的节点的所有下游也应被跳过
func (e *WorkflowEngine) propagateSkip(
	wf *workflow.Workflow,
	nodeID string,
	execCtx *ExecutionContext,
	inDegree map[string]*int32,
	wg *sync.WaitGroup,
	dispatchNode func(string),
) {
	downstreamEdges := wf.GetDownstreamEdges(nodeID)
	for _, edge := range downstreamEdges {
		targetDeg, ok := inDegree[edge.Target]
		if !ok {
			continue
		}
		// 标记下游也被跳过（除非下游还有其他未跳过的上游）
		execCtx.MarkSkipped(edge.Target)
		newDeg := atomic.AddInt32(targetDeg, -1)
		if newDeg == 0 {
			dispatchNode(edge.Target) // 会在 dispatchNode 中检查 IsSkipped 并继续传播
		}
	}
}

// ─── 事件发送辅助 ──────────────────────────────────────────────────────────────

// sendWorkflowEvent 向 channel 发送工作流事件
func sendWorkflowEvent(ctx context.Context, ch chan<- WorkflowEvent, event WorkflowEvent) bool {
	select {
	case ch <- event:
		return true
	case <-ctx.Done():
		return false
	}
}

// uniqueStrings 字符串切片去重
func uniqueStrings(ss []string) []string {
	seen := make(map[string]bool, len(ss))
	result := make([]string, 0, len(ss))
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}
