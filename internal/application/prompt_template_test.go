package application

import (
	"strings"
	"testing"
	"time"
)

func TestRenderPromptTemplate_BuiltinVars(t *testing.T) {
	out := RenderPromptTemplate("你好 {{user_name}}，模型 {{model_name}}", PromptContext{
		UserName:  "张三",
		ModelName: "qwen-plus",
	})
	if out != "你好 张三，模型 qwen-plus" {
		t.Errorf("内置变量替换错误: %s", out)
	}
}

func TestRenderPromptTemplate_NoTemplateFastPath(t *testing.T) {
	in := "这是一段没有变量的纯文本"
	if out := RenderPromptTemplate(in, PromptContext{}); out != in {
		t.Errorf("无变量文本应原样返回，得到: %s", out)
	}
}

func TestRenderPromptTemplate_UnknownVarKept(t *testing.T) {
	out := RenderPromptTemplate("保留 {{unknown_var}}", PromptContext{})
	if out != "保留 {{unknown_var}}" {
		t.Errorf("未知变量应原样保留，得到: %s", out)
	}
}

func TestRenderPromptTemplate_EmptyValueKept(t *testing.T) {
	// 值为空时保留原样（user_name 未提供）
	out := RenderPromptTemplate("你好 {{user_name}}", PromptContext{})
	if out != "你好 {{user_name}}" {
		t.Errorf("空值变量应保留原样，得到: %s", out)
	}
}

func TestRenderPromptTemplate_CustomOverridesBuiltin(t *testing.T) {
	// 自定义变量可覆盖非时间类内置变量（model_name）
	out := RenderPromptTemplate("模型 {{model_name}}", PromptContext{
		ModelName:  "qwen-plus",
		CustomVars: map[string]string{"model_name": "custom-model"},
	})
	if out != "模型 custom-model" {
		t.Errorf("自定义变量应覆盖内置变量，得到: %s", out)
	}
}

func TestRenderPromptTemplate_CurrentDate(t *testing.T) {
	out := RenderPromptTemplate("今天 {{current_date}}", PromptContext{})
	today := time.Now().Format("2006-01-02")
	if !strings.Contains(out, today) {
		t.Errorf("current_date 应替换为今天日期 %s，得到: %s", today, out)
	}
}

func TestRenderPromptTemplate_KnowledgeContext(t *testing.T) {
	out := RenderPromptTemplate("参考：{{knowledge_context}}", PromptContext{
		KnowledgeContext: "[1] 文档片段",
	})
	if out != "参考：[1] 文档片段" {
		t.Errorf("knowledge_context 替换错误: %s", out)
	}
}

func TestMergePromptVars_Priority(t *testing.T) {
	user := map[string]string{"a": "u", "b": "u"}
	session := map[string]string{"b": "s", "c": "s"}
	request := map[string]string{"c": "r", "d": "r"}

	merged := MergePromptVars(user, session, request)

	// 优先级：请求 > 会话 > 用户
	cases := map[string]string{"a": "u", "b": "s", "c": "r", "d": "r"}
	for k, want := range cases {
		if merged[k] != want {
			t.Errorf("变量 %s 合并优先级错误: got=%s want=%s", k, merged[k], want)
		}
	}
}

func TestMergePromptVars_EmptyInputs(t *testing.T) {
	merged := MergePromptVars(nil, nil, nil)
	if len(merged) != 0 {
		t.Errorf("空输入应返回空 map，得到 %d 项", len(merged))
	}
}
