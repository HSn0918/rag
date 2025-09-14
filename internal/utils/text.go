package utils

import (
	"strings"
	"unicode/utf8"
)

// SafeUTF8Truncate truncates a UTF-8 string to a maximum number of bytes
// without breaking multi-byte character boundaries.
//
// This function ensures that the truncation point doesn't fall in the middle
// of a multi-byte UTF-8 character, which would result in invalid UTF-8.
// If the string is already within the limit, it returns unchanged.
//
// Example:
//
//	result := SafeUTF8Truncate("你好世界", 6) // Returns "你好" (6 bytes)
func SafeUTF8Truncate(str string, maxBytes int) string {
	if len(str) <= maxBytes {
		return str
	}

	// Ensure we don't truncate in the middle of a multi-byte character
	for i := maxBytes; i >= 0 && i > maxBytes-4; i-- {
		if utf8.ValidString(str[:i]) {
			return str[:i]
		}
	}

	// If no suitable truncation point found, use rune-level truncation
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

// SanitizeUTF8 validates and cleans a string to ensure it contains only
// valid UTF-8 characters.
//
// Invalid UTF-8 byte sequences are removed from the string. This is useful
// when dealing with data from untrusted sources or when encoding issues
// might have corrupted the text.
//
// The function returns a clean UTF-8 string safe for storage and display.
func SanitizeUTF8(str string) string {
	if utf8.ValidString(str) {
		return str
	}

	// Remove or replace invalid UTF-8 characters
	var buf strings.Builder
	buf.Grow(len(str))

	for len(str) > 0 {
		r, size := utf8.DecodeRuneInString(str)
		if r == utf8.RuneError && size == 1 {
			// Skip invalid byte
			str = str[1:]
		} else {
			buf.WriteRune(r)
			str = str[size:]
		}
	}

	return buf.String()
}

// CleanAndFormatContent normalizes and formats text content for consistent
// presentation.
//
// This function performs several cleaning operations:
//   - Trims leading and trailing whitespace
//   - Removes excessive consecutive newlines (max 2)
//   - Truncates content to maxLength with ellipsis if needed
//   - Ensures the result is valid UTF-8
//
// The maxLength parameter specifies the maximum allowed length in bytes.
// Content exceeding this limit will be truncated with "..." appended.
func CleanAndFormatContent(content string, maxLength int) string {
	// Basic cleaning
	content = strings.TrimSpace(content)

	// Remove excessive newlines
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

	// Ensure content isn't too long
	result := strings.Join(cleanedLines, "\n")
	if len(result) > maxLength {
		result = SafeUTF8Truncate(result, maxLength) + "..."
	}

	// Ensure result is valid UTF-8
	result = SanitizeUTF8(result)

	return result
}
