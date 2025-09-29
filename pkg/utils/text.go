package utils

import (
	"strings"
	"unicode/utf8"
)

type QueryType string

const (
	QueryTypeHowTo      QueryType = "how_to"
	QueryTypeWhatIs     QueryType = "what_is"
	QueryTypeWhy        QueryType = "why"
	QueryTypeComparison QueryType = "comparison"
	QueryTypeList       QueryType = "list"
	QueryTypeExperience QueryType = "experience"
	QueryTypeTechnical  QueryType = "technical"
	QueryTypeProject    QueryType = "project"
	QueryTypeGeneral    QueryType = "general"
)

func AnalyzeQueryType(query string) QueryType {
	queryLower := strings.ToLower(query)
	patterns := map[QueryType][]string{
		QueryTypeHowTo:      {"怎么", "如何", "怎样", "怎么办", "how to", "how do"},
		QueryTypeWhatIs:     {"什么是", "是什么", "what is", "define", "定义"},
		QueryTypeWhy:        {"为什么", "为啥", "原因", "why", "because"},
		QueryTypeComparison: {"比较", "对比", "区别", "差异", "vs", "versus", "compare"},
		QueryTypeList:       {"有哪些", "包括", "种类", "类型", "list", "types"},
		QueryTypeExperience: {"经验", "心得", "体会", "感受", "experience"},
		QueryTypeTechnical:  {"技术", "算法", "架构", "实现", "技术栈", "technical"},
		QueryTypeProject:    {"项目", "工程", "系统", "应用", "project"},
	}
	for queryType, keywords := range patterns {
		for _, keyword := range keywords {
			if strings.Contains(queryLower, keyword) {
				return queryType
			}
		}
	}
	return QueryTypeGeneral
}

func GetQueryTypeGuidance(queryType QueryType) string {
	guidanceMap := map[QueryType]string{
		QueryTypeHowTo:      "以下信息将帮助您了解具体的操作方法和步骤：\n\n",
		QueryTypeWhatIs:     "以下信息将帮助您理解相关概念和定义：\n\n",
		QueryTypeWhy:        "以下信息将帮助您了解相关的原因和背景：\n\n",
		QueryTypeComparison: "以下信息将帮助您进行比较和分析：\n\n",
		QueryTypeList:       "以下信息列出了相关的项目和分类：\n\n",
		QueryTypeExperience: "以下是相关的经验分享和实践心得：\n\n",
		QueryTypeTechnical:  "以下是相关的技术信息和实现细节：\n\n",
		QueryTypeProject:    "以下是相关的项目信息和实践案例：\n\n",
		QueryTypeGeneral:    "以下是与您查询相关的信息：\n\n",
	}
	return guidanceMap[queryType]
}

func GetQuerySpecificSummary(queryType QueryType) string {
	summaryMap := map[QueryType]string{
		QueryTypeHowTo:      "从操作方法的角度来看，文档中提到的步骤和建议可以为您提供实用的指导。",
		QueryTypeWhatIs:     "从概念定义的角度分析，相关的解释和说明在文档中有详细描述。",
		QueryTypeWhy:        "从原因分析的角度来看，文档中提供了相关的背景信息和解释。",
		QueryTypeComparison: "从比较分析的角度来看，不同方案的特点和差异在文档中有所体现。",
		QueryTypeList:       "从分类整理的角度来看，相关项目的列举和说明在文档中比较全面。",
		QueryTypeExperience: "从实践经验的角度来看，文档中分享的经验和心得具有参考价值。",
		QueryTypeTechnical:  "从技术角度分析，相关的技术栈、架构和实现方案在文档中有详细说明。",
		QueryTypeProject:    "从项目实施的角度来看，相关的项目经验和实践案例为您提供了有价值的参考。",
		QueryTypeGeneral:    "这些信息从多个角度为您的查询提供了相关的背景知识。",
	}
	return summaryMap[queryType]
}

func SafeUTF8Truncate(str string, maxBytes int) string {
	if len(str) <= maxBytes {
		return str
	}
	for i := maxBytes; i >= 0 && i > maxBytes-4; i-- {
		if utf8.ValidString(str[:i]) {
			return str[:i]
		}
	}
	runes := []rune(str)
	result := ""
	for _, r := range runes {
		test := result + string(r)
		if len(test) > maxBytes {
			break
		}
		result = test
	}
	return result
}

func SanitizeUTF8(str string) string {
	if utf8.ValidString(str) {
		return str
	}
	var buf strings.Builder
	buf.Grow(len(str))
	for len(str) > 0 {
		r, size := utf8.DecodeRuneInString(str)
		if r == utf8.RuneError && size == 1 {
			str = str[1:]
		} else {
			buf.WriteRune(r)
			str = str[size:]
		}
	}
	return buf.String()
}

func CleanAndFormatContent(content string, maxLength int) string {
	content = strings.TrimSpace(content)
	lines := strings.Split(content, "\n")
	var cleanedLines []string
	lastWasEmpty := false
	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "" {
			if !lastWasEmpty {
				cleanedLines = append(cleanedLines, "")
			}
			lastWasEmpty = true
		} else {
			cleanedLines = append(cleanedLines, trimmedLine)
			lastWasEmpty = false
		}
	}
	result := strings.Join(cleanedLines, "\n")
	if len(result) > maxLength {
		result = SafeUTF8Truncate(result, maxLength) + "..."
	}
	return SanitizeUTF8(result)
}

// ExtractBasicKeywords provides a basic keyword extraction fallback.
func ExtractBasicKeywords(query string) []string {
	stopWords := map[string]bool{
		"的": true, "了": true, "在": true, "是": true, "我": true, "有": true, "和": true,
		"就": true, "不": true, "人": true, "都": true, "一": true, "一个": true, "上": true,
		"也": true, "很": true, "到": true, "说": true, "要": true, "去": true, "你": true,
		"会": true, "着": true, "没有": true, "看": true, "好": true, "自己": true, "这": true,
		"想": true,
	}

	var keywords []string
	runes := []rune(query)
	var currentWord []rune

	for _, r := range runes {
		if (r >= 0x4e00 && r <= 0x9fff) || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			currentWord = append(currentWord, r)
		} else {
			if len(currentWord) > 0 {
				word := string(currentWord)
				if len(word) > 1 && !stopWords[word] {
					keywords = append(keywords, word)
				}
				currentWord = nil
			}
		}
	}

	if len(currentWord) > 0 {
		word := string(currentWord)
		if len(word) > 1 && !stopWords[word] {
			keywords = append(keywords, word)
		}
	}

	if len(keywords) == 0 && len(query) > 0 {
		keywords = append(keywords, query)
	}
	return keywords
}
