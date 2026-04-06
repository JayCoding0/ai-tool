package http

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"aiProject/internal/domain/knowledge"
)

// knowledgeServiceRequired 检查知识库服务是否可用
func (h *ChatHandler) knowledgeServiceRequired(w http.ResponseWriter) bool {
	if h.knowledgeSvc == nil {
		writeJSONError(w, "知识库功能未启用（RAG 服务未初始化）", http.StatusServiceUnavailable)
		return false
	}
	return true
}

// HandleListKnowledgeBases 列出知识库
func (h *ChatHandler) HandleListKnowledgeBases(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.knowledgeServiceRequired(w) {
		return
	}
	userID, _, _ := h.getCurrentUser(r)
	list, err := h.knowledgeSvc.ListKnowledgeBases(r.Context(), userID)
	if err != nil {
		writeJSONError(w, "获取知识库列表失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if list == nil {
		list = make([]*knowledge.KnowledgeBase, 0)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"knowledge_bases": list, "count": len(list)}) //nolint:errcheck
}

// HandleCreateKnowledgeBase 创建知识库
func (h *ChatHandler) HandleCreateKnowledgeBase(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.knowledgeServiceRequired(w) {
		return
	}
	userID, _, _ := h.getCurrentUser(r)
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	kb, err := h.knowledgeSvc.CreateKnowledgeBase(r.Context(), userID, req.Name, req.Description)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(kb) //nolint:errcheck
}

// HandleDeleteKnowledgeBase 删除知识库
func (h *ChatHandler) HandleDeleteKnowledgeBase(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.knowledgeServiceRequired(w) {
		return
	}
	idStr := r.URL.Query().Get("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		writeJSONError(w, "无效的知识库 ID", http.StatusBadRequest)
		return
	}
	if err := h.knowledgeSvc.DeleteKnowledgeBase(r.Context(), id); err != nil {
		writeJSONError(w, "删除知识库失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "知识库已删除"}) //nolint:errcheck
}

// HandleListDocuments 列出知识库下的文档
func (h *ChatHandler) HandleListDocuments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.knowledgeServiceRequired(w) {
		return
	}
	kbIDStr := r.URL.Query().Get("kb_id")
	kbID, err := strconv.ParseInt(kbIDStr, 10, 64)
	if err != nil || kbID <= 0 {
		writeJSONError(w, "无效的知识库 ID", http.StatusBadRequest)
		return
	}
	docs, err := h.knowledgeSvc.ListDocuments(r.Context(), kbID)
	if err != nil {
		writeJSONError(w, "获取文档列表失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if docs == nil {
		docs = make([]*knowledge.Document, 0)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"documents": docs, "count": len(docs)}) //nolint:errcheck
}

// HandleUploadDocument 上传文档到知识库（multipart/form-data 或 JSON）
func (h *ChatHandler) HandleUploadDocument(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.knowledgeServiceRequired(w) {
		return
	}

	kbID, docName, docContent, docType, err := parseDocumentUpload(r)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if kbID <= 0 {
		writeJSONError(w, "无效的知识库 ID", http.StatusBadRequest)
		return
	}
	if utf8.RuneCountInString(docContent) == 0 {
		writeJSONError(w, "文档内容不能为空", http.StatusBadRequest)
		return
	}

	doc, err := h.knowledgeSvc.AddDocument(r.Context(), kbID, docName, docType, docContent)
	if err != nil {
		writeJSONError(w, "添加文档失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(doc) //nolint:errcheck
}

// parseDocumentUpload 解析文档上传请求（支持 multipart/form-data 和 JSON 两种模式）
func parseDocumentUpload(r *http.Request) (kbID int64, docName, docContent, docType string, err error) {
	contentType := r.Header.Get("Content-Type")

	if strings.Contains(contentType, "multipart/form-data") {
		// 文件上传模式
		if err = r.ParseMultipartForm(32 << 20); err != nil { // 最大 32MB
			return 0, "", "", "", fmt.Errorf("解析表单失败: %s", err)
		}
		kbID, err = strconv.ParseInt(r.FormValue("kb_id"), 10, 64)
		if err != nil || kbID <= 0 {
			return 0, "", "", "", fmt.Errorf("无效的知识库 ID")
		}
		file, header, fileErr := r.FormFile("file")
		if fileErr != nil {
			return 0, "", "", "", fmt.Errorf("获取上传文件失败: %s", fileErr)
		}
		defer file.Close()

		data, readErr := io.ReadAll(file)
		if readErr != nil {
			return 0, "", "", "", fmt.Errorf("读取文件失败: %s", readErr)
		}
		return kbID, header.Filename, string(data), detectContentType(header.Filename), nil
	}

	// JSON 模式（直接传文本内容）
	var req struct {
		KbID        int64  `json:"kb_id"`
		Name        string `json:"name"`
		Content     string `json:"content"`
		ContentType string `json:"content_type"`
	}
	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		return 0, "", "", "", fmt.Errorf("Invalid JSON")
	}
	docType = req.ContentType
	if docType == "" {
		docType = "text"
	}
	return req.KbID, req.Name, req.Content, docType, nil
}

// HandleUploadDirectory 从目录上传多个文档到知识库
// 前端通过 webkitdirectory 选择目录后，将目录中所有文件以 multipart/form-data 方式上传
func (h *ChatHandler) HandleUploadDirectory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.knowledgeServiceRequired(w) {
		return
	}

	if err := r.ParseMultipartForm(128 << 20); err != nil { // 最大 128MB
		writeJSONError(w, "解析表单失败: "+err.Error(), http.StatusBadRequest)
		return
	}

	kbIDStr := r.FormValue("kb_id")
	kbID, err := strconv.ParseInt(kbIDStr, 10, 64)
	if err != nil || kbID <= 0 {
		writeJSONError(w, "无效的知识库 ID", http.StatusBadRequest)
		return
	}

	// 支持的文件扩展名
	supportedExts := map[string]bool{
		".txt": true, ".md": true, ".markdown": true,
	}

	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		writeJSONError(w, "未选择任何文件", http.StatusBadRequest)
		return
	}

	type uploadResult struct {
		Name    string `json:"name"`
		Status  string `json:"status"` // "ok" 或 "skipped" 或 "error"
		Message string `json:"message,omitempty"`
	}

	var results []uploadResult
	successCount := 0
	skippedCount := 0

	for _, fh := range files {
		// 获取文件的相对路径（前端通过 webkitRelativePath 传递）
		fileName := fh.Filename

		// 检查文件扩展名
		ext := strings.ToLower(fileName)
		hasValidExt := false
		for e := range supportedExts {
			if strings.HasSuffix(ext, e) {
				hasValidExt = true
				break
			}
		}
		if !hasValidExt {
			skippedCount++
			results = append(results, uploadResult{Name: fileName, Status: "skipped", Message: "不支持的文件类型"})
			continue
		}

		// 读取文件内容
		file, openErr := fh.Open()
		if openErr != nil {
			results = append(results, uploadResult{Name: fileName, Status: "error", Message: "打开文件失败: " + openErr.Error()})
			continue
		}
		data, readErr := io.ReadAll(file)
		file.Close()
		if readErr != nil {
			results = append(results, uploadResult{Name: fileName, Status: "error", Message: "读取文件失败: " + readErr.Error()})
			continue
		}

		content := string(data)
		if utf8.RuneCountInString(content) == 0 {
			skippedCount++
			results = append(results, uploadResult{Name: fileName, Status: "skipped", Message: "文件内容为空"})
			continue
		}

		// 添加文档
		_, addErr := h.knowledgeSvc.AddDocument(r.Context(), kbID, fileName, detectContentType(fileName), content)
		if addErr != nil {
			results = append(results, uploadResult{Name: fileName, Status: "error", Message: addErr.Error()})
			continue
		}

		successCount++
		results = append(results, uploadResult{Name: fileName, Status: "ok"})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck
		"total":   len(files),
		"success": successCount,
		"skipped": skippedCount,
		"failed":  len(files) - successCount - skippedCount,
		"results": results,
	})
}

// HandleDeleteDocument 删除文档
func (h *ChatHandler) HandleDeleteDocument(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.knowledgeServiceRequired(w) {
		return
	}
	idStr := r.URL.Query().Get("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		writeJSONError(w, "无效的文档 ID", http.StatusBadRequest)
		return
	}
	if err := h.knowledgeSvc.DeleteDocument(r.Context(), id); err != nil {
		writeJSONError(w, "删除文档失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "文档已删除"}) //nolint:errcheck
}

// HandleKnowledgeSearch 手动测试知识库检索
func (h *ChatHandler) HandleKnowledgeSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.knowledgeServiceRequired(w) {
		return
	}
	var req struct {
		KbID  int64  `json:"kb_id"`
		Query string `json:"query"`
		TopK  int    `json:"top_k"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if req.TopK <= 0 {
		req.TopK = 5
	}
	chunks, err := h.knowledgeSvc.Search(r.Context(), req.KbID, req.Query, req.TopK)
	if err != nil {
		writeJSONError(w, "检索失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	type resultItem struct {
		Content string  `json:"content"`
		Score   float32 `json:"score"`
		DocName string  `json:"doc_name"`
	}
	results := make([]resultItem, 0, len(chunks))
	for _, sc := range chunks {
		results = append(results, resultItem{
			Content: sc.Chunk.Content,
			Score:   sc.Score,
			DocName: sc.DocName,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"results": results, "count": len(results)}) //nolint:errcheck
}

// detectContentType 根据文件名推断内容类型
func detectContentType(filename string) string {
	// 取最后一段路径作为文件名（兼容目录上传时带路径的情况）
	name := filename
	if idx := strings.LastIndex(filename, "/"); idx >= 0 {
		name = filename[idx+1:]
	}
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".markdown"):
		return "markdown"
	case strings.HasSuffix(lower, ".pdf"):
		return "pdf"
	default:
		return "text"
	}
}
