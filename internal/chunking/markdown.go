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
	extensionast "github.com/yuin/goldmark/extension/ast"
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
	DefaultMaxChunkSize  = 2000
	DefaultMinChunkSize  = 500
	DefaultOverlapSize   = 200
	DefaultMaxEmptyLines = 2
	DefaultMinWordLen    = 2
	DefaultTokenRatio    = 1.3 // English word to token ratio
)

// Common errors.
var (
	ErrEmptyContent     = errors.New("content cannot be empty")
	ErrInvalidChunkSize = errors.New("invalid chunk size configuration")
	ErrContextCanceled  = errors.New("operation was canceled")
)

// Chunk represents a semantic unit of content with enhanced metadata and relationships.
type Chunk struct {
	Content       string            `json:"content"`
	Type          string            `json:"type"`
	Level         int               `json:"level"`
	Title         string            `json:"title"`
	StartIndex    int               `json:"start_index"`
	EndIndex      int               `json:"end_index"`
	Metadata      map[string]string `json:"metadata"`
	ID            string            `json:"id,omitempty"`
	TokenCount    int               `json:"token_count,omitempty"`
	Relationships []string          `json:"relationships,omitempty"`
}

// ChunkerConfig defines configuration parameters for the markdown chunker.
type ChunkerConfig struct {
	MaxChunkSize       int
	MinChunkSize       int
	OverlapSize        int
	PreserveStructure  bool
	EnableSemantic     bool
	MergeSparseParents bool // [NEW] Option to merge sparse parent sections.
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

// OptimizedMarkdownChunker provides advanced, non-recursive markdown chunking.
type OptimizedMarkdownChunker struct {
	maxChunkSize       int
	minChunkSize       int
	overlapSize        int
	preserveStructure  bool
	enableSemantic     bool
	mergeSparseParents bool // [NEW] Stores the config option.

	wordRegex     *regexp.Regexp
	chineseRegex  *regexp.Regexp
	sentenceRegex *regexp.Regexp
	markdownRegex *regexp.Regexp
}

// NewOptimizedMarkdownChunker creates a new optimized markdown chunker.
func NewOptimizedMarkdownChunker(config ChunkerConfig) (*OptimizedMarkdownChunker, error) {
	if err := config.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	chunker := &OptimizedMarkdownChunker{
		maxChunkSize:       config.MaxChunkSize,
		minChunkSize:       config.MinChunkSize,
		overlapSize:        config.OverlapSize,
		preserveStructure:  config.PreserveStructure,
		enableSemantic:     config.EnableSemantic,
		mergeSparseParents: config.MergeSparseParents, // [MODIFIED] Assign from config.
	}

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
// [MODIFIED] This now enables the MergeSparseParents feature by default for best results.
func NewMarkdownChunker(maxChunkSize, overlapSize int, preserveStructure bool) (*OptimizedMarkdownChunker, error) {
	config := ChunkerConfig{
		MaxChunkSize:       maxChunkSize,
		MinChunkSize:       maxChunkSize / 4, // 25% of max
		OverlapSize:        overlapSize,
		PreserveStructure:  preserveStructure,
		EnableSemantic:     true,
		MergeSparseParents: true, // [MODIFIED] Enabled by default.
	}
	return NewOptimizedMarkdownChunker(config)
}

// ChunkMarkdown orchestrates the entire chunking process.
// [MODIFIED] Added the parent merging step.
func (omc *OptimizedMarkdownChunker) ChunkMarkdown(content string) ([]Chunk, error) {
	if content == "" {
		return nil, ErrEmptyContent
	}

	cleanContent := omc.preprocessContent(content)

	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM, extension.Table),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
	)

	source := []byte(cleanContent)
	reader := text.NewReader(source)
	doc := md.Parser().Parse(reader)

	documentTree, err := omc.buildDocumentTree(doc, source)
	if err != nil {
		return nil, fmt.Errorf("failed to build document tree: %w", err)
	}

	// Step 4: Initial structural chunking.
	chunks := omc.intelligentChunking(documentTree, source)

	// [NEW] Step 4.5: If enabled, merge sparse parent chunks into their children.
	if omc.mergeSparseParents {
		chunks = omc.mergeSparseParentChunks(chunks)
	}

	// Step 5: Post-process the final list of chunks to add relationships.
	optimizedChunks := omc.postProcessChunks(chunks)

	return optimizedChunks, nil
}

// [NEW] mergeSparseParentChunks identifies and merges content-sparse parent sections
// into their first child chunk to create more semantically complete units.
func (omc *OptimizedMarkdownChunker) mergeSparseParentChunks(chunks []Chunk) []Chunk {
	if len(chunks) < 2 {
		return chunks
	}

	// Define a threshold to identify a "sparse" chunk.
	const sparseTokenThreshold = 20

	var optimizedChunks []Chunk
	i := 0
	for i < len(chunks) {
		currentChunk := chunks[i]

		// Check if the chunk is a sparse parent section with a following chunk to merge into.
		isSparseParent := currentChunk.Type == string(ChunkTypeSection) &&
			currentChunk.TokenCount < sparseTokenThreshold &&
			(i+1) < len(chunks)

		if isSparseParent {
			// Merge the sparse parent (currentChunk) down into its child (nextChunk).
			nextChunk := chunks[i+1]

			var builder strings.Builder
			builder.WriteString(strings.TrimSpace(currentChunk.Content))
			builder.WriteString("\n\n")
			builder.WriteString(nextChunk.Content)
			nextChunk.Content = builder.String()

			// Update fields to reflect the merge.
			nextChunk.StartIndex = currentChunk.StartIndex
			nextChunk.TokenCount = omc.estimateTokenCount(nextChunk.Content) // Recalculate token count.

			// Preserve the parent's title in metadata for context.
			if nextChunk.Metadata == nil {
				nextChunk.Metadata = make(map[string]string)
			}
			nextChunk.Metadata["parent_section_title"] = currentChunk.Title

			optimizedChunks = append(optimizedChunks, nextChunk)

			// Advance the index by 2 to skip both the original parent and child.
			i += 2
		} else {
			// This chunk is not a sparse parent, so add it as is.
			optimizedChunks = append(optimizedChunks, currentChunk)
			i++
		}
	}
	return optimizedChunks
}

// DocumentNode represents a node in the document's AST structure.
type DocumentNode struct {
	Type       ast.NodeKind
	Level      int
	Title      string
	Content    string
	Children   []*DocumentNode
	StartIndex int
	EndIndex   int
	ASTNode    ast.Node
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
	stack := []walkFrame{{node: doc, entering: true}}
	for len(stack) > 0 {
		frame := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if !frame.entering {
			continue
		}
		if omc.isInlineNode(frame.node) {
			continue
		}
		nodeInfo := omc.extractNodeInfo(frame.node, source)
		switch n := frame.node.(type) {
		case *ast.Heading:
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
	if omc.isInlineNode(node) {
		return info
	}
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
	if heading, ok := node.(*ast.Heading); ok {
		info.Title = omc.extractTextFromNode(heading, source)
	}
	return info
}

// intelligentChunking performs intelligent chunking based on document structure.
func (omc *OptimizedMarkdownChunker) intelligentChunking(root *DocumentNode, source []byte) []Chunk {
	var chunks []Chunk
	chunkID := 0
	omc.processNodeForChunking(root, source, &chunks, &chunkID)
	return chunks
}

// processNodeForChunking processes document nodes for chunking using non-recursive approach.
func (omc *OptimizedMarkdownChunker) processNodeForChunking(node *DocumentNode, source []byte, chunks *[]Chunk, chunkID *int) {
	stack := []*DocumentNode{node}
	for len(stack) > 0 {
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
			for i := len(currentNode.Children) - 1; i >= 0; i-- {
				stack = append(stack, currentNode.Children[i])
			}
		}
	}
}

// collectSectionContent collects section content using non-recursive approach.
func (omc *OptimizedMarkdownChunker) collectSectionContent(section *DocumentNode) string {
	var contentParts []string
	if section.Level > 0 && section.Title != "" {
		headerPrefix := strings.Repeat("#", section.Level)
		contentParts = append(contentParts, fmt.Sprintf("%s %s", headerPrefix, section.Title))
	}
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
	if omc.enableSemantic {
		chunk.TokenCount = omc.estimateTokenCount(content)
	}
	return chunk
}

// preprocessContent cleans and normalizes content before processing.
func (omc *OptimizedMarkdownChunker) preprocessContent(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
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
		ast.KindCodeSpan, ast.KindAutoLink, ast.KindRawHTML, ast.KindString:
		return true
	case extensionast.KindStrikethrough:
		return true
	default:
		if node.Parent() != nil {
			switch node.Parent().Kind() {
			case ast.KindEmphasis, ast.KindLink:
				return true
			}
		}
		return false
	}
}

// extractTextFromNode extracts plain text from AST node using non-recursive traversal.
func (omc *OptimizedMarkdownChunker) extractTextFromNode(node ast.Node, source []byte) string {
	var buf bytes.Buffer
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
			stack = append(stack, textWalkFrame{node: frame.node, entering: false})
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
		if strings.HasPrefix(trimmed, "```") {
			if inCodeBlock {
				currentPart.WriteString("\n\n")
				currentPart.WriteString(part)
				result = append(result, currentPart.String())
				currentPart.Reset()
				inCodeBlock = false
			} else {
				if currentPart.Len() > 0 {
					result = append(result, currentPart.String())
					currentPart.Reset()
				}
				currentPart.WriteString(part)
				inCodeBlock = true
			}
		} else if inCodeBlock {
			currentPart.WriteString("\n\n")
			currentPart.WriteString(part)
		} else {
			if currentPart.Len() > 0 {
				result = append(result, currentPart.String())
				currentPart.Reset()
			}
			result = append(result, part)
		}
	}
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
	words := omc.wordRegex.FindAllString(content, -1)
	englishWords := 0
	for _, word := range words {
		if !omc.chineseRegex.MatchString(word) {
			englishWords++
		}
	}
	return chineseCount + int(float64(englishWords)*DefaultTokenRatio)
}

// getSmartOverlap creates intelligent overlap between chunks at sentence boundaries.
func (omc *OptimizedMarkdownChunker) getSmartOverlap(content string) string {
	if omc.overlapSize <= 0 {
		return ""
	}
	sentences := omc.sentenceRegex.Split(content, -1)
	if len(sentences) < 2 {
		return omc.getSimpleOverlap(content)
	}
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
	for i := range chunks {
		chunks[i].Relationships = make([]string, 0, 3) // Typical: prev, next, parent
		if i > 0 {
			chunks[i].Relationships = append(chunks[i].Relationships, chunks[i-1].ID)
		}
		if i < len(chunks)-1 {
			chunks[i].Relationships = append(chunks[i].Relationships, chunks[i+1].ID)
		}
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
