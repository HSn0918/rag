package chunking

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
)

// Chunk represents a single chunk of content
type Chunk struct {
	Content    string            `json:"content"`
	Type       string            `json:"type"`  // "header", "paragraph", "code", "table", etc.
	Level      int               `json:"level"` // for headers: 1-6, for others: 0
	Title      string            `json:"title"` // section title if applicable
	Metadata   map[string]string `json:"metadata"`
	StartIndex int               `json:"start_index"`
	EndIndex   int               `json:"end_index"`
}

// MarkdownChunker handles intelligent chunking of markdown documents
type MarkdownChunker struct {
	maxChunkSize      int
	overlapSize       int
	preserveStructure bool
}

// NewMarkdownChunker creates a new markdown chunker
func NewMarkdownChunker(maxChunkSize, overlapSize int, preserveStructure bool) *MarkdownChunker {
	return &MarkdownChunker{
		maxChunkSize:      maxChunkSize,
		overlapSize:       overlapSize,
		preserveStructure: preserveStructure,
	}
}

// ChunkMarkdown chunks markdown content based on AST structure
func (mc *MarkdownChunker) ChunkMarkdown(content string) ([]Chunk, error) {
	md := goldmark.New(
		goldmark.WithExtensions(),
		goldmark.WithParserOptions(),
		goldmark.WithRendererOptions(
			html.WithUnsafe(),
		),
	)

	source := []byte(content)
	reader := text.NewReader(source)

	// Parse the markdown into AST
	doc := md.Parser().Parse(reader)

	// Extract semantic chunks
	var chunks []Chunk
	var currentSection *SectionInfo

	err := ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		// 只处理块级节点，跳过内联节点
		switch node.Kind() {
		case ast.KindText, ast.KindEmphasis, ast.KindLink, ast.KindImage, ast.KindCodeSpan:
			return ast.WalkContinue, nil
		}

		nodeStart := 0
		nodeEnd := 0

		// 安全地获取节点位置信息
		if hasLines, ok := node.(interface{ Lines() *text.Segments }); ok {
			segment := hasLines.Lines()
			if segment.Len() > 0 {
				nodeStart = segment.At(0).Start
				nodeEnd = segment.At(segment.Len() - 1).Stop
			}
		}

		switch n := node.(type) {
		case *ast.Heading:
			// 处理标题层级关系 - 只有遇到同级或更高级标题时才分割
			if currentSection != nil {
				// 如果新标题是子标题（级别更高），添加到当前section
				if n.Level > currentSection.Level {
					currentSection.Nodes = append(currentSection.Nodes, n)
					currentSection.EndIndex = nodeEnd
				} else {
					// 否则完成当前section并开始新的
					sectionChunks := mc.processSectionChunks(currentSection, source)
					chunks = append(chunks, sectionChunks...)

					// Start new section
					currentSection = &SectionInfo{
						Level:      n.Level,
						Title:      mc.extractTextFromNode(n, source),
						StartIndex: nodeStart,
						Nodes:      []ast.Node{n},
					}
				}
			} else {
				// Start new section
				currentSection = &SectionInfo{
					Level:      n.Level,
					Title:      mc.extractTextFromNode(n, source),
					StartIndex: nodeStart,
					Nodes:      []ast.Node{n},
				}
			}

		case *ast.Paragraph, *ast.CodeBlock, *ast.FencedCodeBlock, *ast.List:
			if currentSection == nil {
				// Content before first header
				currentSection = &SectionInfo{
					Level:      0,
					Title:      "Introduction",
					StartIndex: nodeStart,
					Nodes:      []ast.Node{},
				}
			}
			currentSection.Nodes = append(currentSection.Nodes, n)
			currentSection.EndIndex = nodeEnd

		case *ast.ThematicBreak:
			// Treat horizontal rules as section breaks
			if currentSection != nil {
				sectionChunks := mc.processSectionChunks(currentSection, source)
				chunks = append(chunks, sectionChunks...)
				currentSection = nil
			}
		}

		return ast.WalkContinue, nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk AST: %w", err)
	}

	// Process last section
	if currentSection != nil {
		sectionChunks := mc.processSectionChunks(currentSection, source)
		chunks = append(chunks, sectionChunks...)
	}

	return chunks, nil
}

// SectionInfo holds information about a document section
type SectionInfo struct {
	Level      int
	Title      string
	StartIndex int
	EndIndex   int
	Nodes      []ast.Node
}

// processSectionChunks processes a section and creates appropriate chunks
func (mc *MarkdownChunker) processSectionChunks(section *SectionInfo, source []byte) []Chunk {
	var chunks []Chunk

	// Calculate section content
	var contentBuilder strings.Builder

	for _, node := range section.Nodes {
		nodeContent := mc.extractContentFromNode(node, source)
		contentBuilder.WriteString(nodeContent)
		contentBuilder.WriteString("\n\n")
	}

	sectionContent := strings.TrimSpace(contentBuilder.String())

	if len(sectionContent) == 0 {
		return chunks
	}

	// Add section title if it's a header
	fullContent := sectionContent
	if section.Level > 0 {
		headerPrefix := strings.Repeat("#", section.Level)
		fullContent = fmt.Sprintf("%s %s\n\n%s", headerPrefix, section.Title, sectionContent)
	}

	// If content fits in one chunk, return as single chunk
	if len(fullContent) <= mc.maxChunkSize {
		chunk := Chunk{
			Content:    fullContent,
			Type:       mc.getSectionType(section),
			Level:      section.Level,
			Title:      section.Title,
			StartIndex: section.StartIndex,
			EndIndex:   section.EndIndex,
			Metadata: map[string]string{
				"section_title": section.Title,
				"section_level": fmt.Sprintf("%d", section.Level),
			},
		}
		return []Chunk{chunk}
	}

	// Split large sections while preserving structure
	return mc.splitLargeSection(section, fullContent, source)
}

// splitLargeSection splits large sections into smaller chunks
func (mc *MarkdownChunker) splitLargeSection(section *SectionInfo, content string, _ []byte) []Chunk {
	var chunks []Chunk

	// Try to split by paragraphs first
	paragraphs := strings.Split(content, "\n\n")

	var currentChunk strings.Builder
	chunkIndex := 0

	for _, para := range paragraphs {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}

		// Check if adding this paragraph would exceed chunk size
		potentialContent := currentChunk.String()
		if potentialContent != "" {
			potentialContent += "\n\n"
		}
		potentialContent += para

		if len(potentialContent) > mc.maxChunkSize && currentChunk.Len() > 0 {
			// Finalize current chunk
			chunk := Chunk{
				Content: strings.TrimSpace(currentChunk.String()),
				Type:    mc.getSectionType(section),
				Level:   section.Level,
				Title:   fmt.Sprintf("%s (Part %d)", section.Title, chunkIndex+1),
				Metadata: map[string]string{
					"section_title":      section.Title,
					"section_level":      fmt.Sprintf("%d", section.Level),
					"chunk_index":        fmt.Sprintf("%d", chunkIndex),
					"is_partial_section": "true",
				},
			}
			chunks = append(chunks, chunk)

			// Start new chunk with overlap
			currentChunk.Reset()
			if mc.overlapSize > 0 && len(chunks) > 0 {
				overlap := mc.getOverlapText(chunks[len(chunks)-1].Content, mc.overlapSize)
				if overlap != "" {
					currentChunk.WriteString(overlap)
					currentChunk.WriteString("\n\n")
				}
			}
			chunkIndex++
		}

		if currentChunk.Len() > 0 {
			currentChunk.WriteString("\n\n")
		}
		currentChunk.WriteString(para)
	}

	// Add remaining content
	if currentChunk.Len() > 0 {
		chunk := Chunk{
			Content: strings.TrimSpace(currentChunk.String()),
			Type:    mc.getSectionType(section),
			Level:   section.Level,
			Title:   fmt.Sprintf("%s (Part %d)", section.Title, chunkIndex+1),
			Metadata: map[string]string{
				"section_title":      section.Title,
				"section_level":      fmt.Sprintf("%d", section.Level),
				"chunk_index":        fmt.Sprintf("%d", chunkIndex),
				"is_partial_section": "true",
			},
		}
		chunks = append(chunks, chunk)
	}

	return chunks
}

// extractTextFromNode extracts plain text from AST node
func (mc *MarkdownChunker) extractTextFromNode(node ast.Node, source []byte) string {
	var buf bytes.Buffer

	ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			if textNode, ok := n.(*ast.Text); ok {
				segment := textNode.Segment
				buf.Write(segment.Value(source))
			}
		}
		return ast.WalkContinue, nil
	})

	return strings.TrimSpace(buf.String())
}

// extractContentFromNode extracts the raw content from a node
func (mc *MarkdownChunker) extractContentFromNode(node ast.Node, source []byte) string {
	// 安全地获取节点位置信息
	if hasLines, ok := node.(interface{ Lines() *text.Segments }); ok {
		lines := hasLines.Lines()
		if lines.Len() == 0 {
			return ""
		}

		start := lines.At(0).Start
		end := lines.At(lines.Len() - 1).Stop
		return string(source[start:end])
	}

	// 如果无法获取位置信息，尝试从段落中提取文本
	return mc.extractTextFromNode(node, source)
}

// getSectionType determines the type of section
func (mc *MarkdownChunker) getSectionType(section *SectionInfo) string {
	if section.Level > 0 {
		return "section"
	}

	// Analyze content to determine type
	if len(section.Nodes) == 0 {
		return "text"
	}

	// Check predominant node type
	nodeTypes := make(map[string]int)
	for _, node := range section.Nodes {
		switch node.(type) {
		case *ast.CodeBlock, *ast.FencedCodeBlock:
			nodeTypes["code"]++
		case *ast.List:
			nodeTypes["list"]++
		default:
			nodeTypes["text"]++
		}
	}

	// Return most common type
	maxCount := 0
	predominantType := "text"
	for nodeType, count := range nodeTypes {
		if count > maxCount {
			maxCount = count
			predominantType = nodeType
		}
	}

	return predominantType
}

// getOverlapText gets the last N characters for overlap
func (mc *MarkdownChunker) getOverlapText(content string, overlapSize int) string {
	if len(content) <= overlapSize {
		return content
	}

	// Try to break at word boundaries
	overlapStart := len(content) - overlapSize
	for overlapStart > 0 && overlapStart < len(content) {
		if content[overlapStart] == ' ' || content[overlapStart] == '\n' {
			break
		}
		overlapStart--
	}

	return strings.TrimSpace(content[overlapStart:])
}
