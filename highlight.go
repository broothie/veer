package main

import (
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/charmbracelet/lipgloss"
)

// highlightedLine holds pre-tokenized syntax data for a single line.
type highlightedLine struct {
	tokens []chroma.Token
}

// highlightFile tokenizes an entire file's diff lines at once, returning
// highlighted lines indexed by position. This is much faster than per-line
// tokenization since the lexer is created once and processes all content together.
func highlightFile(filename string, lines []DiffLine) []highlightedLine {
	result := make([]highlightedLine, len(lines))

	lexer := lexers.Match(filename)
	if lexer == nil {
		return result
	}
	lexer = chroma.Coalesce(lexer)

	// Build full text from all content lines, tracking which diff line each belongs to.
	var fullText strings.Builder
	for i, dl := range lines {
		if dl.Type == LineSeparator || dl.Type == LineHeader {
			continue
		}
		_ = i
		fullText.WriteString(dl.Content)
		fullText.WriteByte('\n')
	}

	iter, err := lexer.Tokenise(nil, fullText.String())
	if err != nil {
		return result
	}

	// Map tokens back to diff lines.
	lineIdx := 0
	// Skip to first content line.
	for lineIdx < len(lines) && (lines[lineIdx].Type == LineSeparator || lines[lineIdx].Type == LineHeader) {
		lineIdx++
	}

	for _, tok := range iter.Tokens() {
		if lineIdx >= len(lines) {
			break
		}

		// Split tokens that span newlines.
		parts := strings.Split(tok.Value, "\n")
		for pi, part := range parts {
			if pi > 0 {
				// Advance to next content line.
				lineIdx++
				for lineIdx < len(lines) && (lines[lineIdx].Type == LineSeparator || lines[lineIdx].Type == LineHeader) {
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

// tokenStyle maps chroma token types to lipgloss styles.
func tokenStyle(t chroma.TokenType) *lipgloss.Style {
	s, ok := tokenStyles[t]
	if ok {
		return &s
	}
	// Walk up the token type hierarchy.
	parent := t.Parent()
	if parent != t {
		return tokenStyle(parent)
	}
	return nil
}

var tokenStyles = map[chroma.TokenType]lipgloss.Style{
	// Keywords
	chroma.Keyword:            lipgloss.NewStyle().Foreground(lipgloss.Color("5")), // magenta
	chroma.KeywordConstant:    lipgloss.NewStyle().Foreground(lipgloss.Color("5")), // magenta
	chroma.KeywordDeclaration: lipgloss.NewStyle().Foreground(lipgloss.Color("5")), // magenta
	chroma.KeywordType:        lipgloss.NewStyle().Foreground(lipgloss.Color("6")), // cyan
	chroma.KeywordNamespace:   lipgloss.NewStyle().Foreground(lipgloss.Color("5")), // magenta
	chroma.KeywordReserved:    lipgloss.NewStyle().Foreground(lipgloss.Color("5")), // magenta
	chroma.KeywordPseudo:      lipgloss.NewStyle().Foreground(lipgloss.Color("5")), // magenta

	// Names
	chroma.NameBuiltin:   lipgloss.NewStyle().Foreground(lipgloss.Color("6")), // cyan
	chroma.NameFunction:  lipgloss.NewStyle().Foreground(lipgloss.Color("4")), // blue
	chroma.NameClass:     lipgloss.NewStyle().Foreground(lipgloss.Color("6")), // cyan
	chroma.NameDecorator: lipgloss.NewStyle().Foreground(lipgloss.Color("3")), // yellow
	chroma.NameException: lipgloss.NewStyle().Foreground(lipgloss.Color("1")), // red
	chroma.NameTag:       lipgloss.NewStyle().Foreground(lipgloss.Color("4")), // blue
	chroma.NameAttribute: lipgloss.NewStyle().Foreground(lipgloss.Color("6")), // cyan

	// Literals
	chroma.LiteralString:         lipgloss.NewStyle().Foreground(lipgloss.Color("2")), // green
	chroma.LiteralStringEscape:   lipgloss.NewStyle().Foreground(lipgloss.Color("6")), // cyan
	chroma.LiteralStringInterpol: lipgloss.NewStyle().Foreground(lipgloss.Color("6")), // cyan
	chroma.LiteralNumber:         lipgloss.NewStyle().Foreground(lipgloss.Color("3")), // yellow
	chroma.LiteralStringChar:     lipgloss.NewStyle().Foreground(lipgloss.Color("2")), // green

	// Comments
	chroma.Comment:          lipgloss.NewStyle().Foreground(lipgloss.Color("8")), // bright black (gray)
	chroma.CommentSingle:    lipgloss.NewStyle().Foreground(lipgloss.Color("8")),
	chroma.CommentMultiline: lipgloss.NewStyle().Foreground(lipgloss.Color("8")),

	// Operators
	chroma.Operator:     lipgloss.NewStyle().Foreground(lipgloss.Color("5")), // magenta
	chroma.OperatorWord: lipgloss.NewStyle().Foreground(lipgloss.Color("5")),
	chroma.Punctuation:  lipgloss.NewStyle().Foreground(lipgloss.Color("7")), // white
}
