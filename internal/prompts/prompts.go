// Package prompts manages LLM prompts and templates for the RAG system.
//
// This package provides centralized prompt management with support for
// embedding prompts and dynamic template rendering.
package prompts

import (
	"fmt"
	"strings"
)

// PromptType represents different types of prompts used in the system.
type PromptType string

const (
	// PromptTypeKeywordExtraction is for extracting keywords from queries.
	PromptTypeKeywordExtraction PromptType = "keyword_extraction"
	// PromptTypeContextSummary is for generating context summaries.
	PromptTypeContextSummary PromptType = "context_summary"
	// PromptTypeRAGResponse is for generating RAG responses.
	PromptTypeRAGResponse PromptType = "rag_response"
)

// Prompt represents a reusable prompt template.
type Prompt struct {
	Type         PromptType
	Name         string
	System       string
	UserTemplate string
	// Embedding can store pre-computed embeddings for prompt similarity matching
	Embedding []float32
}

// PromptManager manages all prompts and their embeddings.
type PromptManager struct {
	prompts map[PromptType]*Prompt
}

// NewPromptManager creates a new prompt manager with default prompts.
func NewPromptManager() *PromptManager {
	pm := &PromptManager{
		prompts: make(map[PromptType]*Prompt),
	}
	pm.initializeDefaultPrompts()
	return pm
}

// initializeDefaultPrompts loads all default prompts.
// initializeDefaultPrompts loads all default prompts.
func (pm *PromptManager) initializeDefaultPrompts() {
	// Keyword extraction prompt
	pm.prompts[PromptTypeKeywordExtraction] = &Prompt{
		Type: PromptTypeKeywordExtraction,
		Name: "keyword_extraction_zh_v2",
		System: `你是一个精通信息检索和自然语言处理的中文关键词提取引擎。你的唯一任务是从用户查询中精准地抽取出核心关键词。

核心指令：
1.  **目标**：提取 3 到 7 个最能代表查询意图的名词性、实体性或主题性关键词。
2.  **内容**：优先提取专业术语、产品名称、人名、地名等实体名词。
3.  **过滤**：必须忽略所有通用停用词（如：“的”、“了”、“是”、“一个”、“怎么样”、“请问”等）和无实际意义的动词或形容词。
4.  **格式**：输出必须是结构良好 (well-formed) 的 XML。除了 XML 内容，不要包含任何其他字符、注释或解释。

示例 1：
输入："我想了解一下最近很火的 AI 模型“通用文字-图像生成器”的原理和应用场景"
输出：
<keywords>
    <keyword>AI模型</keyword>
    <keyword>通用文字-图像生成器</keyword>
    <keyword>原理</keyword>
    <keyword>应用场景</keyword>
</keywords>

示例 2：
输入："从上海到北京的高铁票价是多少？"
输出：
<keywords>
    <keyword>上海</keyword>
    <keyword>北京</keyword>
    <keyword>高铁</keyword>
    <keyword>票价</keyword>
</keywords>

你的输出必须严格遵循 <keywords> -> <keyword> 的嵌套格式。`,
		UserTemplate: `用户查询：
"{{query}}"

请根据系统指令，提取上述查询的核心关键词。严格以 XML 格式返回结果。`,
	}

	// Context summary prompt
	pm.prompts[PromptTypeContextSummary] = &Prompt{
		Type: PromptTypeContextSummary,
		Name: "context_summary_rag_v2",
		System: `你是一个严谨、中立的 RAG (Retrieval-Augmented Generation) 内容处理器。你的任务是根据提供的上下文信息，以结构化的 XML 格式进行重组和呈现，而不是直接回答用户问题。

**最高指令：**
1.  **中立呈现**：你是一个信息的“搬运工”和“组织者”，而不是“解答者”。严禁对检索到的信息进行任何形式的推断、综合或给出结论。
2.  **忠于原文**：所有输出内容都必须直接来源于提供的上下文（{{context}}），不允许添加任何外部知识或个人观点。
3.  **明确归属**：在适当的地方使用“根据检索资料显示”、“文档指出”等短语，以强调信息来源。
4.  **绝不回答**：严禁使用“答案是”、“因此”、“所以”等引导性或结论性词语。你的目标是为用户提供判断所需的信息，而非替用户判断。
5.  **格式纯净**：最终输出必须是且仅是一个结构良好的 XML 文档，不含任何解释性文字或代码块标记。

**XML 输出结构详解：**

<rag_response>
    <summary>
        <text>一句话概括所有检索内容的共同主题。</text>
    </summary>

    <main_content>
        <info_points>
            <point>
                <title>核心信息点1的标题</title>
                <content>直接从文档中提取的具体信息，应简明扼要。</content>
            </point>
        </info_points>
    </main_content>

    <detailed_content>
        <section>
            <title>相关主题1</title>
            <content>对该主题的详细、系统的描述，整合自一份或多份文档。</content>
        </section>
    </detailed_content>

    <key_points>
        <point>关键要点1</point>
        <point>关键要点2</point>
    </key_points>

    <completeness>
        <assessment>例如：信息基本覆盖了查询，但缺少关于[某方面]的细节。</assessment>
        <missing_info>明确指出上下文中没有提及或缺失的信息点。</missing_info>
    </completeness>

    <sources>
        <source>
            <id>文档ID</id>
            <similarity>相似度得分，如：0.89</similarity>
            <summary>该信息片段的摘要。</summary>
        </source>
    </sources>
</rag_response>`,
		UserTemplate: `用户查询: "{{query}}"

检索到的上下文信息:
---
{{context}}
---

任务：请严格遵循系统定义的核心指令和 XML 结构，对上述上下文信息进行处理。确保所有输出都基于提供的信息，并保持绝对中立。`,
	}
}

// GetPrompt returns a prompt by type.
func (pm *PromptManager) GetPrompt(promptType PromptType) (*Prompt, error) {
	prompt, exists := pm.prompts[promptType]
	if !exists {
		return nil, fmt.Errorf("prompt not found for type: %s", promptType)
	}
	return prompt, nil
}

// RenderUserPrompt renders the user prompt template with variables.
func (pm *PromptManager) RenderUserPrompt(promptType PromptType, variables map[string]string) (string, error) {
	prompt, err := pm.GetPrompt(promptType)
	if err != nil {
		return "", err
	}

	rendered := prompt.UserTemplate
	for key, value := range variables {
		placeholder := fmt.Sprintf("{{%s}}", key)
		rendered = strings.ReplaceAll(rendered, placeholder, value)
	}

	return rendered, nil
}

// SetPromptEmbedding sets the embedding for a specific prompt.
func (pm *PromptManager) SetPromptEmbedding(promptType PromptType, embedding []float32) error {
	prompt, exists := pm.prompts[promptType]
	if !exists {
		return fmt.Errorf("prompt not found for type: %s", promptType)
	}
	prompt.Embedding = embedding
	return nil
}

// GetPromptEmbedding returns the embedding for a specific prompt.
func (pm *PromptManager) GetPromptEmbedding(promptType PromptType) ([]float32, error) {
	prompt, exists := pm.prompts[promptType]
	if !exists {
		return nil, fmt.Errorf("prompt not found for type: %s", promptType)
	}
	return prompt.Embedding, nil
}

// AddCustomPrompt adds a custom prompt to the manager.
func (pm *PromptManager) AddCustomPrompt(prompt *Prompt) error {
	if prompt == nil || prompt.Type == "" {
		return fmt.Errorf("invalid prompt: type is required")
	}
	pm.prompts[prompt.Type] = prompt
	return nil
}

// ListPromptTypes returns all available prompt types.
func (pm *PromptManager) ListPromptTypes() []PromptType {
	types := make([]PromptType, 0, len(pm.prompts))
	for t := range pm.prompts {
		types = append(types, t)
	}
	return types
}
