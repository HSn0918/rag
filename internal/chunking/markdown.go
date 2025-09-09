// Package chunking provides advanced text chunking functionality for RAG systems.
package chunking

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// Chunker defines the interface for text chunking operations.
type Chunker interface {
	ChunkMarkdown(content string) ([]Chunk, error)
	ChunkMarkdownWithContext(ctx context.Context, content string) ([]Chunk, error)
}

// ChunkType defines the enumeration of chunk types for semantic categorization.
type ChunkType string

// Chunk type constants define different semantic categories for chunks.
const (
	ChunkTypeHeader    ChunkType = "header"
	ChunkTypeParagraph ChunkType = "paragraph"
	ChunkTypeCode      ChunkType = "code"
	ChunkTypeList      ChunkType = "list"
	ChunkTypeSection   ChunkType = "section"
	ChunkTypeText      ChunkType = "text"
)

// Default configuration values.
const (
	DefaultMaxChunkSize   = 2000
	DefaultMinChunkSize   = 500
	DefaultOverlapSize    = 200
	DefaultMaxKeywords    = 10
	DefaultMaxSummaryLen  = 100
	DefaultMaxEmptyLines  = 2
	DefaultMinWordLen     = 2
	DefaultTokenRatio     = 1.3 // English word to token ratio
)

// Common errors.
var (
	ErrEmptyContent     = errors.New("content cannot be empty")
	ErrInvalidChunkSize = errors.New("invalid chunk size configuration")
	ErrContextCanceled  = errors.New("operation was canceled")
)

// Chunk represents a semantic unit of content with enhanced metadata and relationships.
// This structure maintains backward compatibility while adding advanced features.
type Chunk struct {
	// Core fields (backward compatible) - grouped by logical purpose
	Content    string `json:"content"`
	Type       string `json:"type"`
	Level      int    `json:"level"`
	Title      string `json:"title"`
	StartIndex int    `json:"start_index"`
	EndIndex   int    `json:"end_index"`

	// Metadata
	Metadata map[string]string `json:"metadata"`

	// Enhanced fields (optional for backward compatibility)
	ID            string   `json:"id,omitempty"`
	Summary       string   `json:"summary,omitempty"`
	Keywords      []string `json:"keywords,omitempty"`
	TokenCount    int      `json:"token_count,omitempty"`
	Relationships []string `json:"relationships,omitempty"`
}

// ChunkerConfig defines configuration parameters for the markdown chunker.
// Zero values will be replaced with sensible defaults.
type ChunkerConfig struct {
	MaxChunkSize      int
	MinChunkSize      int
	OverlapSize       int
	PreserveStructure bool
	EnableSemantic    bool
}

// validate validates the configuration and sets defaults.
func (c *ChunkerConfig) validate() error {
	if c.MaxChunkSize <= 0 {
		c.MaxChunkSize = DefaultMaxChunkSize
	}
	if c.MinChunkSize <= 0 {
		c.MinChunkSize = DefaultMinChunkSize
	}
	if c.OverlapSize < 0 {
		c.OverlapSize = DefaultOverlapSize
	}

	if c.MinChunkSize >= c.MaxChunkSize {
		return ErrInvalidChunkSize
	}
	if c.OverlapSize >= c.MaxChunkSize {
		return ErrInvalidChunkSize
	}

	return nil
}

// OptimizedMarkdownChunker provides advanced, non-recursive markdown chunking
// with semantic analysis and intelligent overlap handling.
type OptimizedMarkdownChunker struct {
	// Configuration fields
	maxChunkSize      int
	minChunkSize      int
	overlapSize       int
	preserveStructure bool
	enableSemantic    bool

	// Dependencies
	keywordExtractor *KeywordExtractor

	// Compiled regex patterns (cached for performance)
	wordRegex     *regexp.Regexp
	chineseRegex  *regexp.Regexp
	sentenceRegex *regexp.Regexp
	markdownRegex *regexp.Regexp
}

// KeywordExtractor handles intelligent keyword extraction with stop-word filtering.
type KeywordExtractor struct {
	stopWords map[string]bool
	minLen    int
}

// NewOptimizedMarkdownChunker creates a new optimized markdown chunker.
func NewOptimizedMarkdownChunker(config ChunkerConfig) (*OptimizedMarkdownChunker, error) {
	if err := config.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	extractor := NewKeywordExtractor()

	chunker := &OptimizedMarkdownChunker{
		maxChunkSize:      config.MaxChunkSize,
		minChunkSize:      config.MinChunkSize,
		overlapSize:       config.OverlapSize,
		preserveStructure: config.PreserveStructure,
		enableSemantic:    config.EnableSemantic,
		keywordExtractor:  extractor,
	}

	// Pre-compile regex patterns for better performance
	var err error
	if chunker.wordRegex, err = regexp.Compile(`\b\w+\b`); err != nil {
		return nil, fmt.Errorf("failed to compile word regex: %w", err)
	}
	if chunker.chineseRegex, err = regexp.Compile(`\p{Han}`); err != nil {
		return nil, fmt.Errorf("failed to compile chinese regex: %w", err)
	}
	if chunker.sentenceRegex, err = regexp.Compile(`[.!?。！？]\s*`); err != nil {
		return nil, fmt.Errorf("failed to compile sentence regex: %w", err)
	}
	if chunker.markdownRegex, err = regexp.Compile(`[#*\[\]()]`); err != nil {
		return nil, fmt.Errorf("failed to compile markdown regex: %w", err)
	}

	return chunker, nil
}

// NewMarkdownChunker creates a backward-compatible chunker with simplified parameters.
func NewMarkdownChunker(maxChunkSize, overlapSize int, preserveStructure bool) (*OptimizedMarkdownChunker, error) {
	config := ChunkerConfig{
		MaxChunkSize:      maxChunkSize,
		MinChunkSize:      maxChunkSize / 4, // 25% of max
		OverlapSize:       overlapSize,
		PreserveStructure: preserveStructure,
		EnableSemantic:    true,
	}
	return NewOptimizedMarkdownChunker(config)
}

// NewKeywordExtractor creates a new keyword extractor with predefined stop words.
func NewKeywordExtractor() *KeywordExtractor {
	stopWords := map[string]bool{
		// Chinese stop words
		"的": true, "了": true, "在": true, "是": true, "我": true,
		"有": true, "和": true, "就": true, "不": true, "人": true,
		"都": true, "一": true, "个": true, "上": true, "也": true,
		"很": true, "到": true, "说": true, "要": true, "去": true, "你": true,
		
		// English stop words
		"the": true, "a": true, "an": true, "and": true, "or": true,
		"but": true, "in": true, "on": true, "at": true, "to": true,
		"for": true, "of": true, "with": true, "by": true, "is": true,
		"are": true, "was": true, "were": true, "be": true, "been": true,
	}

	return &KeywordExtractor{
		stopWords: stopWords,
		minLen:    DefaultMinWordLen,
	}
}

// ChunkMarkdown provides backward-compatible chunking interface.
func (omc *OptimizedMarkdownChunker) ChunkMarkdown(content string) ([]Chunk, error) {
	return omc.ChunkMarkdownWithContext(context.Background(), content)
}

// ChunkMarkdownWithContext performs advanced markdown chunking with context support.
func (omc *OptimizedMarkdownChunker) ChunkMarkdownWithContext(ctx context.Context, content string) ([]Chunk, error) {
	if content == "" {
		return nil, ErrEmptyContent
	}

	// Check for context cancellation
	select {
	case <-ctx.Done():
		return nil, ErrContextCanceled
	default:
	}

	// 1. Preprocessing - clean and normalize content
	cleanContent := omc.preprocessContent(content)

	// 2. Create enhanced goldmark parser
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			extension.Table,
			extension.Strikethrough,
			extension.Linkify,
			extension.TaskList,
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
	)

	source := []byte(cleanContent)
	reader := text.NewReader(source)
	doc := md.Parser().Parse(reader)

	// Check for context cancellation
	select {
	case <-ctx.Done():
		return nil, ErrContextCanceled
	default:
	}

	// 3. Build document structure tree
	documentTree, err := omc.buildDocumentTree(doc, source)
	if err != nil {
		return nil, fmt.Errorf("failed to build document tree: %w", err)
	}

	// 4. Intelligent chunking
	chunks := omc.intelligentChunking(ctx, documentTree, source)
	if ctx.Err() != nil {
		return nil, ErrContextCanceled
	}

	// 5. Post-processing - add metadata and relationships
	optimizedChunks := omc.postProcessChunks(chunks)

	return optimizedChunks, nil
}

// DocumentNode represents a node in the document's AST structure.
type DocumentNode struct {
	Type     ast.NodeKind
	Level    int
	Title    string
	Content  string
	Children []*DocumentNode

	// Position information
	StartIndex int
	EndIndex   int

	// AST reference
	ASTNode ast.Node
}

// NodeInfo contains extracted information from an AST node.
type NodeInfo struct {
	Title      string
	Content    string
	StartIndex int
	EndIndex   int
}

// walkFrame represents a frame in the non-recursive traversal stack.
type walkFrame struct {
	node     ast.Node
	entering bool
}

// buildDocumentTree constructs a document structure tree using non-recursive traversal.
func (omc *OptimizedMarkdownChunker) buildDocumentTree(doc ast.Node, source []byte) (*DocumentNode, error) {
	if doc == nil {
		return nil, errors.New("document node cannot be nil")
	}

	root := &DocumentNode{
		Type:     doc.Kind(),
		Level:    0,
		Title:    "Document Root",
		Children: make([]*DocumentNode, 0),
	}

	var currentSection *DocumentNode
	var headingStack []*DocumentNode

	// Non-recursive traversal using stack
	stack := []walkFrame{{node: doc, entering: true}}

	for len(stack) > 0 {
		frame := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if !frame.entering {
			continue
		}

		// Skip inline elements
		if omc.isInlineNode(frame.node) {
			continue
		}

		nodeInfo := omc.extractNodeInfo(frame.node, source)

		switch n := frame.node.(type) {
		case *ast.Heading:
			// Handle heading hierarchy
			for len(headingStack) > 0 && headingStack[len(headingStack)-1].Level >= n.Level {
				headingStack = headingStack[:len(headingStack)-1]
			}

			section := &DocumentNode{
				Type:       frame.node.Kind(),
				Level:      n.Level,
				Title:      nodeInfo.Title,
				Content:    nodeInfo.Content,
				StartIndex: nodeInfo.StartIndex,
				EndIndex:   nodeInfo.EndIndex,
				Children:   make([]*DocumentNode, 0),
				ASTNode:    frame.node,
			}

			// Add to appropriate parent node
			if len(headingStack) == 0 {
				root.Children = append(root.Children, section)
			} else {
				parent := headingStack[len(headingStack)-1]
				parent.Children = append(parent.Children, section)
			}

			headingStack = append(headingStack, section)
			currentSection = section

		case *ast.Paragraph, *ast.CodeBlock, *ast.FencedCodeBlock, *ast.List:
			contentNode := &DocumentNode{
				Type:       frame.node.Kind(),
				Level:      0,
				Content:    nodeInfo.Content,
				StartIndex: nodeInfo.StartIndex,
				EndIndex:   nodeInfo.EndIndex,
				ASTNode:    frame.node,
			}

			if currentSection != nil {
				currentSection.Children = append(currentSection.Children, contentNode)
			} else {
				root.Children = append(root.Children, contentNode)
			}
		}

		// Add child nodes to stack (reverse order to maintain traversal order)
		if frame.node.HasChildren() {
			child := frame.node.LastChild()
			for child != nil {
				stack = append(stack, walkFrame{node: child, entering: true})
				child = child.PreviousSibling()
			}
		}
	}

	return root, nil
}

// extractNodeInfo extracts relevant information from an AST node.
func (omc *OptimizedMarkdownChunker) extractNodeInfo(node ast.Node, source []byte) NodeInfo {
	info := NodeInfo{}

	// Extract position information
	if hasLines, ok := node.(interface{ Lines() *text.Segments }); ok {
		lines := hasLines.Lines()
		if lines.Len() > 0 {
			info.StartIndex = lines.At(0).Start
			info.EndIndex = lines.At(lines.Len() - 1).Stop
			if info.EndIndex <= len(source) {
				info.Content = string(source[info.StartIndex:info.EndIndex])
			}
		}
	}

	// Extract heading text
	if heading, ok := node.(*ast.Heading); ok {
		info.Title = omc.extractTextFromNode(heading, source)
	}

	return info
}

// intelligentChunking performs intelligent chunking based on document structure.
func (omc *OptimizedMarkdownChunker) intelligentChunking(ctx context.Context, root *DocumentNode, source []byte) []Chunk {
	var chunks []Chunk
	chunkID := 0

	// Use context-aware processing
	omc.processNodeForChunking(ctx, root, source, &chunks, &chunkID)

	return chunks
}

// processNodeForChunking processes document nodes for chunking using non-recursive approach.
func (omc *OptimizedMarkdownChunker) processNodeForChunking(ctx context.Context, node *DocumentNode, source []byte, chunks *[]Chunk, chunkID *int) {
	stack := []*DocumentNode{node}

	for len(stack) > 0 {
		// Check for context cancellation periodically
		select {
		case <-ctx.Done():
			return
		default:
		}

		currentNode := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if currentNode.Type == ast.KindHeading {
			sectionContent := omc.collectSectionContent(currentNode)

			if len(sectionContent) <= omc.maxChunkSize {
				chunk := omc.createChunk(*chunkID, sectionContent, currentNode)
				*chunks = append(*chunks, chunk)
				*chunkID++
			} else {
				subChunks := omc.splitLargeContent(sectionContent, currentNode, *chunkID)
				*chunks = append(*chunks, subChunks...)
				*chunkID += len(subChunks)
			}
		} else {
			// Add child nodes to stack (reverse order to maintain order)
			for i := len(currentNode.Children) - 1; i >= 0; i-- {
				stack = append(stack, currentNode.Children[i])
			}
		}
	}
}

// collectSectionContent collects section content using non-recursive approach.
func (omc *OptimizedMarkdownChunker) collectSectionContent(section *DocumentNode) string {
	var contentParts []string

	// Add section heading
	if section.Level > 0 && section.Title != "" {
		headerPrefix := strings.Repeat("#", section.Level)
		contentParts = append(contentParts, fmt.Sprintf("%s %s", headerPrefix, section.Title))
	}

	// Non-recursive child content collection
	omc.collectChildContent(section, &contentParts)

	return strings.Join(contentParts, "\n\n")
}

// collectChildContent collects content from child nodes using stack-based approach.
func (omc *OptimizedMarkdownChunker) collectChildContent(node *DocumentNode, contentParts *[]string) {
	stack := []*DocumentNode{node}

	for len(stack) > 0 {
		currentNode := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if currentNode.Content != "" {
			*contentParts = append(*contentParts, strings.TrimSpace(currentNode.Content))
		}

		// Add child nodes to stack (reverse order to maintain order)
		for i := len(currentNode.Children) - 1; i >= 0; i-- {
			stack = append(stack, currentNode.Children[i])
		}
	}
}

// splitLargeContent intelligently splits large content while preserving structure.
func (omc *OptimizedMarkdownChunker) splitLargeContent(content string, section *DocumentNode, startID int) []Chunk {
	var chunks []Chunk

	paragraphs := omc.smartSplitByParagraphs(content)

	var currentChunk strings.Builder
	chunkIndex := 0

	for _, para := range paragraphs {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}

		potentialContent := currentChunk.String()
		if potentialContent != "" {
			potentialContent += "\n\n"
		}
		potentialContent += para

		if len(potentialContent) > omc.maxChunkSize && currentChunk.Len() > omc.minChunkSize {
			chunk := omc.createChunk(
				startID+chunkIndex,
				strings.TrimSpace(currentChunk.String()),
				section,
			)
			omc.addPartialChunkMetadata(&chunk, section.Title, chunkIndex)
			chunks = append(chunks, chunk)

			// Start new chunk with overlap
			currentChunk.Reset()
			if omc.overlapSize > 0 && len(chunks) > 0 {
				overlap := omc.getSmartOverlap(chunks[len(chunks)-1].Content)
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
		chunk := omc.createChunk(
			startID+chunkIndex,
			strings.TrimSpace(currentChunk.String()),
			section,
		)
		if chunkIndex > 0 {
			omc.addPartialChunkMetadata(&chunk, section.Title, chunkIndex)
		}
		chunks = append(chunks, chunk)
	}

	return chunks
}

// addPartialChunkMetadata adds metadata for partial chunks.
func (omc *OptimizedMarkdownChunker) addPartialChunkMetadata(chunk *Chunk, originalTitle string, partIndex int) {
	chunk.Title = fmt.Sprintf("%s (Part %d)", originalTitle, partIndex+1)
	chunk.Metadata["is_partial"] = "true"
	chunk.Metadata["part_index"] = strconv.Itoa(partIndex)
}

// createChunk creates a new chunk with full metadata and semantic analysis.
func (omc *OptimizedMarkdownChunker) createChunk(id int, content string, section *DocumentNode) Chunk {
	chunkType := omc.determineChunkType(section)

	chunk := Chunk{
		ID:         fmt.Sprintf("chunk_%d", id),
		Content:    content,
		Type:       string(chunkType),
		Level:      section.Level,
		Title:      section.Title,
		StartIndex: section.StartIndex,
		EndIndex:   section.EndIndex,
		Metadata: map[string]string{
			"section_title": section.Title,
			"section_level": strconv.Itoa(section.Level),
			"chunk_type":    string(chunkType),
		},
		Relationships: make([]string, 0),
	}

	// Add semantic features if enabled
	if omc.enableSemantic {
		chunk.Keywords = omc.keywordExtractor.ExtractKeywords(content)
		chunk.Summary = omc.generateSummary(content)
		chunk.TokenCount = omc.estimateTokenCount(content)
	}

	return chunk
}

// Helper methods for content processing and analysis

// preprocessContent cleans and normalizes content before processing.
func (omc *OptimizedMarkdownChunker) preprocessContent(content string) string {
	// Normalize line endings
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")

	// Clean excessive empty lines
	lines := strings.Split(content, "\n")
	cleanedLines := make([]string, 0, len(lines))
	emptyLineCount := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			emptyLineCount++
			if emptyLineCount <= DefaultMaxEmptyLines {
				cleanedLines = append(cleanedLines, "")
			}
		} else {
			emptyLineCount = 0
			cleanedLines = append(cleanedLines, line)
		}
	}

	return strings.Join(cleanedLines, "\n")
}

// isInlineNode determines if a node is an inline element that should be skipped.
func (omc *OptimizedMarkdownChunker) isInlineNode(node ast.Node) bool {
	switch node.Kind() {
	case ast.KindText, ast.KindEmphasis, ast.KindLink, ast.KindImage,
		ast.KindCodeSpan, ast.KindAutoLink:
		return true
	default:
		return false
	}
}

// extractTextFromNode extracts plain text from AST node using non-recursive traversal.
func (omc *OptimizedMarkdownChunker) extractTextFromNode(node ast.Node, source []byte) string {
	var buf bytes.Buffer

	// Use stack for non-recursive AST traversal
	type textWalkFrame struct {
		node     ast.Node
		entering bool
	}

	stack := []textWalkFrame{{node: node, entering: true}}

	for len(stack) > 0 {
		frame := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if frame.entering {
			if textNode, ok := frame.node.(*ast.Text); ok {
				segment := textNode.Segment
				if segment.Stop <= len(source) {
					buf.Write(segment.Value(source))
				}
			}

			// Add exit frame
			stack = append(stack, textWalkFrame{node: frame.node, entering: false})

			// Add child nodes (reverse order to maintain order)
			if frame.node.HasChildren() {
				child := frame.node.LastChild()
				for child != nil {
					stack = append(stack, textWalkFrame{node: child, entering: true})
					child = child.PreviousSibling()
				}
			}
		}
	}

	return strings.TrimSpace(buf.String())
}

// smartSplitByParagraphs intelligently splits content by paragraphs while preserving code blocks.
func (omc *OptimizedMarkdownChunker) smartSplitByParagraphs(content string) []string {
	parts := strings.Split(content, "\n\n")
	result := make([]string, 0, len(parts))
	var inCodeBlock bool
	var currentPart strings.Builder

	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}

		// Check code block boundaries
		if strings.HasPrefix(trimmed, "```") {
			if inCodeBlock {
				// End of code block
				currentPart.WriteString("\n\n")
				currentPart.WriteString(part)
				result = append(result, currentPart.String())
				currentPart.Reset()
				inCodeBlock = false
			} else {
				// Start of code block
				if currentPart.Len() > 0 {
					result = append(result, currentPart.String())
					currentPart.Reset()
				}
				currentPart.WriteString(part)
				inCodeBlock = true
			}
		} else if inCodeBlock {
			// Inside code block
			currentPart.WriteString("\n\n")
			currentPart.WriteString(part)
		} else {
			// Normal paragraph
			if currentPart.Len() > 0 {
				result = append(result, currentPart.String())
				currentPart.Reset()
			}
			result = append(result, part)
		}
	}

	// Add remaining content
	if currentPart.Len() > 0 {
		result = append(result, currentPart.String())
	}

	return result
}

// determineChunkType determines the semantic type of a chunk based on its content.
func (omc *OptimizedMarkdownChunker) determineChunkType(section *DocumentNode) ChunkType {
	if section.Level > 0 {
		return ChunkTypeSection
	}

	switch section.Type {
	case ast.KindCodeBlock, ast.KindFencedCodeBlock:
		return ChunkTypeCode
	case ast.KindList:
		return ChunkTypeList
	case ast.KindParagraph:
		return ChunkTypeParagraph
	default:
		return ChunkTypeText
	}
}

// estimateTokenCount provides a rough estimate of token count for mixed Chinese/English content.
func (omc *OptimizedMarkdownChunker) estimateTokenCount(content string) int {
	chineseCount := 0
	for _, r := range content {
		if unicode.Is(unicode.Han, r) {
			chineseCount++
		}
	}

	// Use pre-compiled regex for better performance
	words := omc.wordRegex.FindAllString(content, -1)
	englishWords := 0
	for _, word := range words {
		if !omc.chineseRegex.MatchString(word) {
			englishWords++
		}
	}

	// Token estimation: Chinese 1 char ≈ 1 token, English 1 word ≈ 1.3 tokens
	return chineseCount + int(float64(englishWords)*DefaultTokenRatio)
}

// generateSummary creates a concise summary of the content for quick reference.
func (omc *OptimizedMarkdownChunker) generateSummary(content string) string {
	// Remove markdown markers using pre-compiled regex
	cleaned := omc.markdownRegex.ReplaceAllString(content, "")
	cleaned = strings.TrimSpace(cleaned)

	if len(cleaned) <= DefaultMaxSummaryLen {
		return cleaned
	}

	// Truncate at word boundary
	summary := cleaned[:DefaultMaxSummaryLen]
	if lastSpace := strings.LastIndex(summary, " "); lastSpace > DefaultMaxSummaryLen/2 {
		summary = summary[:lastSpace]
	}

	return summary + "..."
}

// getSmartOverlap creates intelligent overlap between chunks at sentence boundaries.
func (omc *OptimizedMarkdownChunker) getSmartOverlap(content string) string {
	if omc.overlapSize <= 0 {
		return ""
	}

	// Use pre-compiled regex for better performance
	sentences := omc.sentenceRegex.Split(content, -1)
	if len(sentences) < 2 {
		return omc.getSimpleOverlap(content)
	}

	// Start from the last few sentences
	var overlap strings.Builder
	for i := len(sentences) - 1; i >= 0 && overlap.Len() < omc.overlapSize; i-- {
		sentence := strings.TrimSpace(sentences[i])
		if sentence != "" {
			if overlap.Len() > 0 {
				overlap.WriteString(" ")
			}
			overlap.WriteString(sentence)
		}
	}

	return strings.TrimSpace(overlap.String())
}

// getSimpleOverlap provides fallback overlap at word boundaries when sentence splitting fails.
func (omc *OptimizedMarkdownChunker) getSimpleOverlap(content string) string {
	if len(content) <= omc.overlapSize {
		return content
	}

	overlapStart := len(content) - omc.overlapSize
	for overlapStart > 0 && overlapStart < len(content) {
		if content[overlapStart] == ' ' || content[overlapStart] == '\n' {
			break
		}
		overlapStart--
	}

	return strings.TrimSpace(content[overlapStart:])
}

// postProcessChunks establishes relationships between chunks and adds final metadata.
func (omc *OptimizedMarkdownChunker) postProcessChunks(chunks []Chunk) []Chunk {
	// Pre-allocate relationship slices for better performance
	for i := range chunks {
		chunks[i].Relationships = make([]string, 0, 3) // Typical: prev, next, parent

		// Sequential relationships
		if i > 0 {
			chunks[i].Relationships = append(chunks[i].Relationships, chunks[i-1].ID)
		}
		if i < len(chunks)-1 {
			chunks[i].Relationships = append(chunks[i].Relationships, chunks[i+1].ID)
		}

		// Hierarchical relationships - find parent sections
		for j := i - 1; j >= 0; j-- {
			if chunks[j].Level < chunks[i].Level && chunks[j].Type == string(ChunkTypeSection) {
				chunks[i].Relationships = append(chunks[i].Relationships, chunks[j].ID)
				chunks[i].Metadata["parent_section"] = chunks[j].ID
				break
			}
		}
	}

	return chunks
}

// wordFreq represents word frequency for sorting.
type wordFreq struct {
	word  string
	count int
}

// ExtractKeywords extracts the most frequent meaningful words from content.
func (ke *KeywordExtractor) ExtractKeywords(content string) []string {
	// Use a word regex pattern for consistent extraction
	wordRegex := regexp.MustCompile(`\b\w+\b`)
	words := wordRegex.FindAllString(content, -1)
	
	wordCount := make(map[string]int, len(words)/2) // Estimate capacity

	for _, word := range words {
		cleaned := strings.ToLower(strings.TrimSpace(word))
		if len(cleaned) >= ke.minLen && !ke.stopWords[cleaned] {
			wordCount[cleaned]++
		}
	}

	if len(wordCount) == 0 {
		return nil
	}

	// Sort by frequency and return top keywords
	frequencies := make([]wordFreq, 0, len(wordCount))
	for word, count := range wordCount {
		frequencies = append(frequencies, wordFreq{word: word, count: count})
	}

	// Use a more efficient sorting approach for small datasets
	for i := 0; i < len(frequencies)-1; i++ {
		maxIdx := i
		for j := i + 1; j < len(frequencies); j++ {
			if frequencies[j].count > frequencies[maxIdx].count {
				maxIdx = j
			}
		}
		if maxIdx != i {
			frequencies[i], frequencies[maxIdx] = frequencies[maxIdx], frequencies[i]
		}
	}

	// Return top keywords
	maxKeywords := DefaultMaxKeywords
	if len(frequencies) < maxKeywords {
		maxKeywords = len(frequencies)
	}

	keywords := make([]string, maxKeywords)
	for i := 0; i < maxKeywords; i++ {
		keywords[i] = frequencies[i].word
	}

	return keywords
}