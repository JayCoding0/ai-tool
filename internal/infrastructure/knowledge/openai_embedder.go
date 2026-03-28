package knowledge

import (
	"context"
	"fmt"

	"aiProject/internal/domain/knowledge"
	openai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

const (
	// DefaultEmbedModel 默认 embedding 模型（OpenAI text-embedding-3-small，1536维）
	DefaultEmbedModel = "text-embedding-3-small"
	// DefaultEmbedDimensions text-embedding-3-small 的向量维度
	DefaultEmbedDimensions = 1536
)

// OpenAIEmbedder 基于 OpenAI 兼容接口的向量化实现
type OpenAIEmbedder struct {
	client    openai.Client
	modelName string
	dims      int
}

// NewOpenAIEmbedder 创建 OpenAI Embedder
// baseURL 和 apiKey 与 chat 接口共用同一套配置（如阿里云 DashScope）
func NewOpenAIEmbedder(baseURL, apiKey, modelName string) *OpenAIEmbedder {
	if modelName == "" {
		modelName = DefaultEmbedModel
	}
	client := openai.NewClient(
		option.WithBaseURL(baseURL),
		option.WithAPIKey(apiKey),
	)
	return &OpenAIEmbedder{
		client:    client,
		modelName: modelName,
		dims:      DefaultEmbedDimensions,
	}
}

// Embed 将单条文本转为向量
func (e *OpenAIEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	vecs, err := e.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vecs) == 0 {
		return nil, fmt.Errorf("embedding 返回空结果")
	}
	return vecs[0], nil
}

// EmbedBatch 批量向量化
func (e *OpenAIEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	resp, err := e.client.Embeddings.New(ctx, openai.EmbeddingNewParams{
		Model: openai.EmbeddingModel(e.modelName),
		Input: openai.EmbeddingNewParamsInputUnion{
			OfArrayOfStrings: texts,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("embedding 请求失败: %w", err)
	}

	result := make([][]float32, len(resp.Data))
	for i, item := range resp.Data {
		vec := make([]float32, len(item.Embedding))
		for j, v := range item.Embedding {
			vec[j] = float32(v)
		}
		result[i] = vec
	}
	return result, nil
}

// Dimensions 返回向量维度
func (e *OpenAIEmbedder) Dimensions() int {
	return e.dims
}

// ModelName 返回模型名称
func (e *OpenAIEmbedder) ModelName() string {
	return e.modelName
}

// 确保 OpenAIEmbedder 实现了 knowledge.Embedder 接口
var _ knowledge.Embedder = (*OpenAIEmbedder)(nil)
