package main

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"zed-http-response-viewer/internal/ident"
	. "zed-http-response-viewer/internal/jsonx"
	"zed-http-response-viewer/internal/theme"
	"zed-http-response-viewer/internal/utils"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func renderTabs(m *model, screen *[]byte, tabs []string, selected int) {
	*screen = append(*screen, ' ')

	color := lipgloss.Color("4")
	fg := theme.ToColor(lipgloss.NewStyle().Foreground(color).Render)
	dimmed := theme.ToColor(lipgloss.NewStyle().Faint(true).Render)

	for _, tab := range tabs {
		*screen = append(*screen, []byte(fg("╭"))...)
		*screen = append(*screen, bytes.Repeat([]byte(fg("─")), len(tab)+2)...)
		*screen = append(*screen, []byte(fg("╮ "))...)
	}

	*screen = append(*screen, '\n', ' ')

	for i, tab := range tabs {
		var color theme.Color

		if i == selected {
			color = theme.ToColor(lipgloss.NewStyle().Render)
		} else {
			color = dimmed
		}

		*screen = append(*screen, []byte(fg("│"))...)
		*screen = append(*screen, []byte(color(" "+tab+" "))...)
		*screen = append(*screen, []byte(fg("│ "))...)
	}

	*screen = append(*screen, '\n')

	*screen = append(*screen, []byte(fg("╭"))...)

	screenOffset := 1

	for i, tab := range tabs {
		screenOffset += len(tab) + 5

		if i == selected {
			*screen = append(*screen, []byte(fg("╯"))...)
			*screen = append(*screen, bytes.Repeat([]byte(" "), len(tab)+2)...)
			*screen = append(*screen, []byte(fg("╰─"))...)
		} else {
			*screen = append(*screen, []byte(fg("┴"))...)
			*screen = append(*screen, bytes.Repeat([]byte(fg("─")), len(tab)+2)...)
			*screen = append(*screen, []byte(fg("┴─"))...)
		}
	}

	remainingSpace := m.termWidth - screenOffset - 1

	if remainingSpace > 0 {
		*screen = append(*screen, bytes.Repeat([]byte(fg("─")), remainingSpace)...)
	}

	*screen = append(*screen, []byte(fg("╮"))...)

	*screen = append(*screen, '\n')
}

func (m *model) View() string {
	if m.suspending || m.termWidth <= 5 {
		return ""
	}

	if m.showHelp {
		statusBar := flex(m.termWidth, ": press q or ? to close help", "")
		return m.help.View() + "\n" + theme.CurrentTheme.StatusBar(statusBar)
	}

	if m.showPreview {
		statusBar := flex(m.termWidth, m.cursorPath(), "")
		return m.preview.View() + "\n" + theme.CurrentTheme.StatusBar(statusBar)
	}

	var screen []byte
	printedLines := 0
	n := m.head

	var selected int

	if m.showMetadata {
		selected = 1
	} else {
		selected = 0
	}

	renderTabs(m, &screen, []string{"Body", "Metadata"}, selected)

	borderColor := theme.ToColor(lipgloss.NewStyle().Foreground(lipgloss.Color("4")).Render)

	var cursorLineNumber int

	for lineNumber := 0; lineNumber < m.viewHeight(); lineNumber++ {
		if n == nil {
			break
		}

		screenOffset := len(screen)

		screen = append(screen, borderColor("│ ")...)

		if m.showLineNumbers {
			lineNumbersWidth := len(strconv.Itoa(m.totalLines))
			if n.LineNumber == 0 {
				screen = append(screen, bytes.Repeat([]byte{' '}, lineNumbersWidth)...)
			} else {
				lineNumStr := fmt.Sprintf("%*d", lineNumbersWidth, n.LineNumber)
				screen = append(screen, theme.CurrentTheme.LineNumber(lineNumStr)...)
			}
			screen = append(screen, ' ', ' ')
		}

		for i := 0; i < int(n.Depth); i++ {
			screen = append(screen, ident.IdentBytes...)
		}

		isSelected := m.cursor == lineNumber
		if isSelected {
			if n.LineNumber == 0 {
				cursorLineNumber = n.Parent.LineNumber
			} else {
				cursorLineNumber = n.LineNumber
			}
		}
		if !m.showCursor {
			isSelected = false // don't highlight the cursor while iterating search results
		}

		isRef := false
		isRefSelected := false

		if n.Key != "" {
			screen = append(screen, m.prettyKey(n, isSelected)...)
			screen = append(screen, theme.Colon...)

			_, isRef = isRefNode(n)
			isRefSelected = isRef && isSelected
			isSelected = false // don't highlight the key's value
		}

		screen = append(screen, m.prettyPrint(n, isSelected, isRef)...)

		if n.IsCollapsed() {
			if n.Kind == Object {
				if n.Collapsed.Key != "" {
					screen = append(screen, theme.CurrentTheme.Preview(n.Collapsed.Key)...)
					screen = append(screen, theme.ColonPreview...)
					if len(n.Collapsed.Value) > 0 &&
						len(n.Collapsed.Value) < 42 &&
						n.Collapsed.Kind != Object &&
						n.Collapsed.Kind != Array {
						screen = append(screen, theme.CurrentTheme.Preview(n.Collapsed.Value)...)
						if n.Size > 1 {
							screen = append(screen, theme.CommaPreview...)
							screen = append(screen, theme.Dot3...)
						}
					} else {
						screen = append(screen, theme.Dot3...)
					}
				}
				screen = append(screen, theme.CloseCurlyBracket...)
			} else if n.Kind == Array {
				screen = append(screen, theme.Dot3...)
				screen = append(screen, theme.CloseSquareBracket...)
			}
			if n.End != nil && n.End.Comma {
				screen = append(screen, theme.Comma...)
			}
		}
		if n.Comma {
			screen = append(screen, theme.Comma...)
		}

		if m.showSizes && (n.Kind == Array || n.Kind == Object) {
			if n.IsCollapsed() || n.Size > 1 {
				screen = append(screen, theme.CurrentTheme.Size(fmt.Sprintf(" |%d|", n.Size))...)
			}
		}

		if isRefSelected {
			screen = append(screen, theme.CurrentTheme.Preview("  ctrl+g goto")...)
		}

		screenOffset = len(ansi.Strip(string(screen[screenOffset:])))

		remainingSpace := m.termWidth - screenOffset

		if remainingSpace > 0 {
			screen = append(screen, bytes.Repeat([]byte(" "), remainingSpace)...)
		}
		screen = append(screen, borderColor(" │")...)
		screen = append(screen, '\n')
		printedLines++
		n = n.Next
	}

	for i := printedLines; i < m.viewHeight(); i++ {
		screen = append(screen, borderColor("│")...)
		screen = append(screen, bytes.Repeat([]byte(" "), m.termWidth-2)...)
		screen = append(screen, borderColor("│")...)
		screen = append(screen, '\n')
	}

	screen = append(screen, borderColor("│ ")...)

	if m.yank {
		screen = append(screen, []byte("(y)value  (p)path  (k)key  (b)key+value")...)
	} else if m.showShowSelector {
		screen = append(screen, []byte("(s)sizes  (l)line numbers")...)
	} else if m.gotoSymbolInput.Focused() {
		screen = append(screen, m.gotoSymbolInput.View()...)
	} else if m.commandInput.Focused() {
		screen = append(screen, m.commandInput.View()...)
	} else if m.searchInput.Focused() {
		screen = append(screen, m.searchInput.View()...)
	} else if m.searchInput.Value() != "" {
		re, ci := regexCase(m.searchInput.Value())
		re = "/" + re + "/"
		if ci {
			re += "i"
		}
		if m.search.err != nil {
			screen = append(screen, flex(m.termWidth-4, re, m.search.err.Error())...)
		} else if len(m.search.results) == 0 {
			screen = append(screen, flex(m.termWidth-4, re, "not found")...)
		} else {
			cursor := fmt.Sprintf("found: [%v/%v]", m.search.cursor+1, len(m.search.results))
			screen = append(screen, flex(m.termWidth-4, re, cursor)...)
		}
	} else if m.gotoSymbolInput.Focused() && m.fuzzyMatch != nil {
		var matchedStr []byte
		str := m.fuzzyMatch.Str
		for i := 0; i < len(str); i++ {
			if utils.Contains(i, m.fuzzyMatch.Pos) {
				matchedStr = append(matchedStr, theme.CurrentTheme.Search(string(str[i]))...)
			} else {
				matchedStr = append(matchedStr, theme.CurrentTheme.StatusBar(string(str[i]))...)
			}
		}
		repeatCount := m.termWidth - len(str)
		if repeatCount > 0 {
			matchedStr = append(matchedStr, theme.CurrentTheme.StatusBar(strings.Repeat(" ", repeatCount))...)
		}
		screen = append(screen, matchedStr...)
	} else if m.digInput.Focused() {
		screen = append(screen, m.digInput.View()...)
	} else {
		statusBarWidth := m.termWidth - 4
		var indicator string
		percent := int(float64(cursorLineNumber) / float64(m.totalLines) * 100)
		if cursorLineNumber == 1 {
			percent = min(1, percent)
		}
		indicator = fmt.Sprintf("%d%%", percent)

		info := fmt.Sprintf("%s %s", indicator, "")
		statusBar := flex(statusBarWidth, m.cursorPath(), info)
		screen = append(screen, theme.CurrentTheme.StatusBar(statusBar)...)
	}

	screen = append(screen, borderColor(" │")...)

	screen = append(screen, '\n')
	screen = append(screen, borderColor("╰")...)
	screen = append(screen, bytes.Repeat([]byte(borderColor("─")), m.termWidth-2)...)
	screen = append(screen, borderColor("╯")...)

	return string(screen)
}

func (m *model) centerLine(n *Node) {
	middle := m.visibleLines() / 2

	for range middle {
		m.up()
	}

	m.selectNodeInView(n)
}
