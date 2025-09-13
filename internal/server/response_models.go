package server

import (
	"time"
)

// ContextResponse represents the structured response format for context queries.
type ContextResponse struct {
	Query     string           `json:"query"`
	Timestamp time.Time        `json:"timestamp"`
	Status    ResponseStatus   `json:"status"`
	Data      ResponseData     `json:"data"`
	Metadata  ResponseMetadata `json:"metadata"`
}

// ResponseStatus indicates the quality and completeness of the response.
type ResponseStatus struct {
	Success      bool     `json:"success"`
	Confidence   float64  `json:"confidence"`   // 0.0 to 1.0
	Completeness float64  `json:"completeness"` // 0.0 to 1.0
	Limitations  []string `json:"limitations,omitempty"`
}

// ResponseData contains the main content organized by sections.
type ResponseData struct {
	Summary    string              `json:"summary"`
	Sections   []ContentSection    `json:"sections"`
	Highlights []string            `json:"highlights,omitempty"`
	Actions    []RecommendedAction `json:"recommended_actions,omitempty"`
}

// ContentSection represents a logical section of the response.
type ContentSection struct {
	ID       string         `json:"id"`
	Title    string         `json:"title"`
	Type     SectionType    `json:"type"`
	Priority int            `json:"priority"` // 1 (highest) to 5 (lowest)
	Content  SectionContent `json:"content"`
}

// SectionType defines the type of content section.
type SectionType string

const (
	SectionTypeMain       SectionType = "main"
	SectionTypeSupporting SectionType = "supporting"
	SectionTypeWarning    SectionType = "warning"
	SectionTypeSuggestion SectionType = "suggestion"
	SectionTypeReference  SectionType = "reference"
)

// SectionContent holds the actual content in various formats.
type SectionContent struct {
	Text      string            `json:"text,omitempty"`
	List      []ListItem        `json:"list,omitempty"`
	Table     *TableData        `json:"table,omitempty"`
	Code      *CodeSnippet      `json:"code,omitempty"`
	KeyValues map[string]string `json:"key_values,omitempty"`
}

// ListItem represents an item in a list.
type ListItem struct {
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	SubItems    []ListItem `json:"sub_items,omitempty"`
	Tags        []string   `json:"tags,omitempty"`
}

// TableData represents tabular data.
type TableData struct {
	Headers []string   `json:"headers"`
	Rows    [][]string `json:"rows"`
}

// CodeSnippet represents code content.
type CodeSnippet struct {
	Language string `json:"language"`
	Code     string `json:"code"`
	Purpose  string `json:"purpose,omitempty"`
}

// RecommendedAction suggests next steps for the user.
type RecommendedAction struct {
	Action      string `json:"action"`
	Description string `json:"description"`
	Priority    string `json:"priority"` // high, medium, low
	Type        string `json:"type"`     // query, data, system
}

// ResponseMetadata contains information about the response generation.
type ResponseMetadata struct {
	Sources         []SourceInfo `json:"sources"`
	ProcessingTime  float64      `json:"processing_time_ms"`
	ChunksRetrieved int          `json:"chunks_retrieved"`
	ChunksUsed      int          `json:"chunks_used"`
	Model           string       `json:"model_used"`
	SearchStrategy  string       `json:"search_strategy"`
	Tags            []string     `json:"tags,omitempty"`
}

// SourceInfo describes a source used in generating the response.
type SourceInfo struct {
	DocumentID   string           `json:"document_id"`
	DocumentName string           `json:"document_name"`
	ChunkIDs     []string         `json:"chunk_ids"`
	Relevance    float64          `json:"relevance_score"`
	Citations    []CitationDetail `json:"citations,omitempty"`
	LastModified time.Time        `json:"last_modified,omitempty"`
}

// CitationDetail contains specific cited content from a source.
type CitationDetail struct {
	ChunkID     string  `json:"chunk_id"`
	Content     string  `json:"content"`
	Summary     string  `json:"summary"`
	Relevance   float64 `json:"relevance"`
	StartOffset int     `json:"start_offset,omitempty"`
	EndOffset   int     `json:"end_offset,omitempty"`
}
