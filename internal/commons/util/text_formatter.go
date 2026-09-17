package util

import (
	"regexp"
	"strings"
)

// TextFormatter handles beautification of AI chat responses
type TextFormatter struct {
	// Configuration options for formatting
	EnableMarkdown     bool
	EnableEmojis       bool
	EnableLineBreaks   bool
	MaxLineLength      int
	EnableBulletPoints bool
}

// NewTextFormatter creates a new text formatter with default settings
func NewTextFormatter() *TextFormatter {
	return &TextFormatter{
		EnableMarkdown:     true,
		EnableEmojis:       true,
		EnableLineBreaks:   true,
		MaxLineLength:      80,
		EnableBulletPoints: true,
	}
}

// BeautifyText processes and beautifies AI response text
func (tf *TextFormatter) BeautifyText(text string) string {
	if text == "" {
		return text
	}

	// Start with the original text
	formatted := text

	// Apply various formatting rules
	if tf.EnableMarkdown {
		formatted = tf.processMarkdown(formatted)
	}

	if tf.EnableLineBreaks {
		formatted = tf.processLineBreaks(formatted)
	}

	if tf.EnableBulletPoints {
		formatted = tf.processBulletPoints(formatted)
	}

	if tf.EnableEmojis {
		formatted = tf.addContextualEmojis(formatted)
	}

	// Clean up extra whitespace
	formatted = tf.cleanupWhitespace(formatted)

	return formatted
}

// processMarkdown converts basic markdown-like patterns to proper formatting
func (tf *TextFormatter) processMarkdown(text string) string {
	// Remove **bold** markdown syntax and replace with emphasis
	text = regexp.MustCompile(`\*\*([^*]+)\*\*`).ReplaceAllString(text, "$1")

	// Remove *italic* markdown syntax
	text = regexp.MustCompile(`\*([^*]+)\*`).ReplaceAllString(text, "$1")

	// Convert numbered lists with better formatting (no bold)
	text = regexp.MustCompile(`(?m)^(\d+\.\s)`).ReplaceAllString(text, "$1")

	// Convert bullet points with consistent formatting
	text = regexp.MustCompile(`(?m)^(\-\s)`).ReplaceAllString(text, "• ")

	// Convert headers (simple pattern) - remove markdown syntax
	text = regexp.MustCompile(`(?m)^(#{1,3}\s)`).ReplaceAllString(text, "$1")

	// Convert inline code blocks
	text = regexp.MustCompile("`([^`]+)`").ReplaceAllString(text, "`$1`")

	// Convert code blocks
	text = regexp.MustCompile("(?s)```([^`]+)```").ReplaceAllString(text, "```\n$1\n```")

	// Convert links
	text = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`).ReplaceAllString(text, "[$1]($2)")

	// Convert tables (basic support)
	text = tf.processTables(text)

	return text
}

// processLineBreaks ensures proper line breaks for readability
func (tf *TextFormatter) processLineBreaks(text string) string {
	// Add line breaks after sentences that are too long
	lines := strings.Split(text, "\n")
	var formattedLines []string

	for _, line := range lines {
		if len(line) > tf.MaxLineLength {
			// Break long lines at natural break points
			words := strings.Fields(line)
			var currentLine strings.Builder

			for _, word := range words {
				if currentLine.Len()+len(word)+1 > tf.MaxLineLength {
					if currentLine.Len() > 0 {
						formattedLines = append(formattedLines, currentLine.String())
						currentLine.Reset()
					}
				}
				if currentLine.Len() > 0 {
					currentLine.WriteString(" ")
				}
				currentLine.WriteString(word)
			}

			if currentLine.Len() > 0 {
				formattedLines = append(formattedLines, currentLine.String())
			}
		} else {
			formattedLines = append(formattedLines, line)
		}
	}

	return strings.Join(formattedLines, "\n")
}

// processBulletPoints formats bullet points consistently
func (tf *TextFormatter) processBulletPoints(text string) string {
	// Standardize bullet point formats
	text = regexp.MustCompile(`(?m)^(\*\s)`).ReplaceAllString(text, "• ")
	text = regexp.MustCompile(`(?m)^(\+\s)`).ReplaceAllString(text, "• ")
	text = regexp.MustCompile(`(?m)^(\-\s)`).ReplaceAllString(text, "• ")

	return text
}

// processTables formats basic table structures
func (tf *TextFormatter) processTables(text string) string {
	// Simple table detection and formatting
	lines := strings.Split(text, "\n")
	var formattedLines []string

	for i, line := range lines {
		// Check if line looks like a table row (contains |)
		if strings.Contains(line, "|") && !strings.Contains(line, "```") {
			// Format table rows
			parts := strings.Split(line, "|")
			var formattedParts []string

			for _, part := range parts {
				trimmed := strings.TrimSpace(part)
				if trimmed != "" {
					formattedParts = append(formattedParts, " "+trimmed+" ")
				}
			}

			if len(formattedParts) > 0 {
				formattedLine := "|" + strings.Join(formattedParts, "|") + "|"
				formattedLines = append(formattedLines, formattedLine)

				// Add separator line after header (first table row)
				if i == 0 || (i > 0 && !strings.Contains(lines[i-1], "|")) {
					separator := "|" + strings.Repeat("---|", len(formattedParts))
					formattedLines = append(formattedLines, separator)
				}
			}
		} else {
			formattedLines = append(formattedLines, line)
		}
	}

	return strings.Join(formattedLines, "\n")
}

// addContextualEmojis adds relevant emojis based on content
func (tf *TextFormatter) addContextualEmojis(text string) string {
	// Financial context emojis with more comprehensive mapping
	emojiMap := map[string]string{
		"budget":         "💰",
		"expense":        "💸",
		"income":         "💵",
		"saving":         "🏦",
		"investment":     "📈",
		"debt":           "📉",
		"goal":           "🎯",
		"analysis":       "📊",
		"trend":          "📈",
		"alert":          "⚠️",
		"success":        "✅",
		"warning":        "⚠️",
		"tip":            "💡",
		"recommendation": "💡",
		"transaction":    "💳",
		"payment":        "💳",
		"transfer":       "🔄",
		"balance":        "⚖️",
		"profit":         "📈",
		"loss":           "📉",
		"cash":           "💵",
		"credit":         "💳",
		"loan":           "🏦",
		"interest":       "📊",
		"tax":            "📋",
		"receipt":        "🧾",
		"statement":      "📄",
		"report":         "📊",
		"summary":        "📋",
		"insight":        "🔍",
		"advice":         "💡",
		"plan":           "📋",
		"strategy":       "🎯",
	}

	// Add emojis based on content (case insensitive)
	lowerText := strings.ToLower(text)
	emojiAdded := false

	for keyword, emoji := range emojiMap {
		if strings.Contains(lowerText, keyword) && !strings.Contains(text, emoji) {
			// Add emoji at the beginning if not already present
			text = emoji + " " + text
			emojiAdded = true
			break // Only add one emoji to avoid clutter
		}
	}

	// If no specific emoji was added, add a general financial emoji for financial content
	if !emojiAdded && tf.isFinancialContent(lowerText) {
		text = "💰 " + text
	}

	return text
}

// isFinancialContent checks if the text contains financial-related content
func (tf *TextFormatter) isFinancialContent(text string) bool {
	financialKeywords := []string{
		"money", "finance", "financial", "budget", "expense", "income",
		"transaction", "payment", "balance", "account", "bank", "credit",
		"debit", "saving", "investment", "loan", "debt", "interest",
		"tax", "receipt", "statement", "report", "analysis", "trend",
	}

	for _, keyword := range financialKeywords {
		if strings.Contains(text, keyword) {
			return true
		}
	}

	return false
}

// cleanupWhitespace removes excessive whitespace
func (tf *TextFormatter) cleanupWhitespace(text string) string {
	// Remove multiple consecutive newlines (keep max 2)
	text = regexp.MustCompile(`\n{3,}`).ReplaceAllString(text, "\n\n")

	// Remove trailing whitespace from lines
	text = regexp.MustCompile(`(?m)[ \t]+$`).ReplaceAllString(text, "")

	// Remove leading whitespace from lines (except indented content)
	text = regexp.MustCompile(`(?m)^[ \t]+`).ReplaceAllString(text, "")

	// Collapse duplicate punctuation marks (excluding periods, which could be ellipses)
	text = regexp.MustCompile(`([,;:|\\_=+#%^&\-])\1+`).ReplaceAllString(text, "$1")

	return strings.TrimSpace(text)
}

// FormatFinancialData formats financial data for better readability
func (tf *TextFormatter) FormatFinancialData(text string) string {
	// Format currency amounts
	text = regexp.MustCompile(`(\d+\.?\d*)\s*(rupees?|rs|inr)`).ReplaceAllString(text, "₹$1")
	text = regexp.MustCompile(`(\d+\.?\d*)\s*(dollars?|usd|\$)`).ReplaceAllString(text, "$$1")

	// Format percentages
	text = regexp.MustCompile(`(\d+\.?\d*)\s*percent`).ReplaceAllString(text, "$1%")

	return text
}

// FormatTransactionData formats transaction-related text
func (tf *TextFormatter) FormatTransactionData(text string) string {
	// Format dates
	text = regexp.MustCompile(`(\d{4}-\d{2}-\d{2})`).ReplaceAllString(text, "📅 $1")

	// Format transaction IDs (no bold)
	text = regexp.MustCompile(`(transaction\s+id|tx\s+id):\s*([a-zA-Z0-9-]+)`).ReplaceAllString(text, "🆔 Transaction ID: $2")

	// Format categories (no bold)
	text = regexp.MustCompile(`(category):\s*([a-zA-Z\s]+)`).ReplaceAllString(text, "📂 Category: $2")

	return text
}

// FormatWithContext applies context-specific formatting
func (tf *TextFormatter) FormatWithContext(text string, context string) string {
	formatted := tf.BeautifyText(text)

	switch strings.ToLower(context) {
	case "financial", "budget", "transaction":
		formatted = tf.FormatFinancialData(formatted)
		formatted = tf.FormatTransactionData(formatted)
	case "analysis", "report":
		formatted = tf.FormatFinancialData(formatted)
		formatted = tf.FormatAnalysisData(formatted)
	case "goal", "planning":
		formatted = tf.FormatGoalData(formatted)
	}

	return formatted
}

// FormatAnalysisData formats analysis-related text
func (tf *TextFormatter) FormatAnalysisData(text string) string {
	// Format percentages with better visibility (no bold)
	text = regexp.MustCompile(`(\d+\.?\d*)\s*%`).ReplaceAllString(text, "$1%")

	// Format comparison indicators (no bold)
	text = regexp.MustCompile(`(increased|decreased|up|down|higher|lower)\s+by\s+(\d+\.?\d*%)`).ReplaceAllString(text, "$1 by $2")

	// Format trend indicators (no bold)
	text = regexp.MustCompile(`(trend|pattern|change):\s*([a-zA-Z\s]+)`).ReplaceAllString(text, "📈 $1: $2")

	return text
}

// FormatGoalData formats goal-related text
func (tf *TextFormatter) FormatGoalData(text string) string {
	// Format goal progress (no bold)
	text = regexp.MustCompile(`(progress|achievement):\s*(\d+\.?\d*%)`).ReplaceAllString(text, "🎯 $1: $2")

	// Format deadlines (no bold)
	text = regexp.MustCompile(`(deadline|target date):\s*(\d{4}-\d{2}-\d{2})`).ReplaceAllString(text, "📅 $1: $2")

	// Format milestones (no bold)
	text = regexp.MustCompile(`(milestone|checkpoint):\s*([a-zA-Z\s]+)`).ReplaceAllString(text, "🏁 $1: $2")

	return text
}

// FormatSummaryData formats summary-related text
func (tf *TextFormatter) FormatSummaryData(text string) string {
	// Format key metrics (no bold)
	text = regexp.MustCompile(`(total|sum|average|median):\s*([₹$]?\d+\.?\d*)`).ReplaceAllString(text, "📊 $1: $2")

	// Format time periods (no bold)
	text = regexp.MustCompile(`(monthly|weekly|daily|yearly|annual):\s*([a-zA-Z\s]+)`).ReplaceAllString(text, "📅 $1: $2")

	return text
}
