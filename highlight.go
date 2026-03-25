package main

import (
	"fmt"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/lipgloss"
)

// theme holds the resolved chroma style for syntax highlighting.
var theme *chroma.Style

func initTheme(name string, enabled bool) {
	if !enabled {
		theme = nil
		return
	}
	theme = styles.Get(name)
}

// highlightedLine holds pre-tokenized syntax data for a single line.
type highlightedLine struct {
	tokens []chroma.Token
}

// highlightFile tokenizes an entire file's diff lines at once, returning
// highlighted lines indexed by position. This is much faster than per-line
// tokenization since the lexer is created once and processes all content together.
func highlightFile(filename string, lines []DiffLine) []highlightedLine {
	result := make([]highlightedLine, len(lines))
	if theme == nil {
		return result
	}

	lexer := lexers.Match(filename)
	if lexer == nil {
		return result
	}
	lexer = chroma.Coalesce(lexer)

	// Build full text from all content lines.
	var fullText strings.Builder
	for _, dl := range lines {
		if dl.Type == LineSeparator || dl.Type == LineHeader || dl.Type == LineBinary {
			continue
		}
		fullText.WriteString(normalizeLineContent(dl.Content))
		fullText.WriteByte('\n')
	}

	iter, err := lexer.Tokenise(nil, fullText.String())
	if err != nil {
		return result
	}

	// Map tokens back to diff lines.
	lineIdx := 0
	for lineIdx < len(lines) && (lines[lineIdx].Type == LineSeparator || lines[lineIdx].Type == LineHeader || lines[lineIdx].Type == LineBinary) {
		lineIdx++
	}

	for _, tok := range iter.Tokens() {
		if lineIdx >= len(lines) {
			break
		}

		parts := strings.Split(tok.Value, "\n")
		for pi, part := range parts {
			if pi > 0 {
				lineIdx++
				for lineIdx < len(lines) && (lines[lineIdx].Type == LineSeparator || lines[lineIdx].Type == LineHeader || lines[lineIdx].Type == LineBinary) {
					lineIdx++
				}
				if lineIdx >= len(lines) {
					break
				}
			}
			if part != "" {
				result[lineIdx].tokens = append(result[lineIdx].tokens, chroma.Token{
					Type:  tok.Type,
					Value: part,
				})
			}
		}
	}

	return result
}

// renderHighlighted renders a pre-tokenized line with syntax colors.
func renderHighlighted(hl highlightedLine, content string) string {
	if len(hl.tokens) == 0 {
		return content
	}
	var sb strings.Builder
	for _, tok := range hl.tokens {
		style := tokenStyle(tok.Type)
		if style == nil {
			sb.WriteString(tok.Value)
		} else {
			sb.WriteString(style.Render(tok.Value))
		}
	}
	return sb.String()
}

// renderHighlightedWithBG renders a pre-tokenized line with syntax colors
// merged with a background color.
func renderHighlightedWithBG(hl highlightedLine, content string, bg lipgloss.Color) string {
	bgStyle := lipgloss.NewStyle().Background(bg)
	if len(hl.tokens) == 0 {
		return bgStyle.Render(content)
	}
	var sb strings.Builder
	for _, tok := range hl.tokens {
		style := tokenStyle(tok.Type)
		if style == nil {
			sb.WriteString(bgStyle.Render(tok.Value))
		} else {
			sb.WriteString(style.Inherit(bgStyle).Render(tok.Value))
		}
	}
	return sb.String()
}

// tokenStyle resolves a chroma token type to a lipgloss style using the active theme.
func tokenStyle(t chroma.TokenType) *lipgloss.Style {
	if theme == nil {
		return nil
	}
	entry := theme.Get(t)
	if entry.IsZero() {
		return nil
	}
	s := lipgloss.NewStyle()
	hasStyle := false
	if entry.Colour.IsSet() {
		s = s.Foreground(lipgloss.Color(chromaToHex(entry.Colour)))
		hasStyle = true
	}
	if entry.Bold == chroma.Yes {
		s = s.Bold(true)
		hasStyle = true
	}
	if entry.Italic == chroma.Yes {
		s = s.Italic(true)
		hasStyle = true
	}
	if entry.Underline == chroma.Yes {
		s = s.Underline(true)
		hasStyle = true
	}
	if !hasStyle {
		return nil
	}
	return &s
}

// chromaToHex converts a chroma Colour to a hex string for lipgloss.
func chromaToHex(c chroma.Colour) string {
	return fmt.Sprintf("#%02x%02x%02x", c.Red(), c.Green(), c.Blue())
}
