package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/antonmedv/clipboard"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"zed-http-response-viewer/internal/engine"
	"zed-http-response-viewer/internal/fuzzy"
	"zed-http-response-viewer/internal/jsonpath"
	. "zed-http-response-viewer/internal/jsonx"
	"zed-http-response-viewer/internal/theme"
)

func main() {
	var args []string
	for _, arg := range os.Args[1:] {
		switch arg {
		case "-h", "--help":
			fmt.Println(usage(keyMap))
			return
		case "-v", "-V", "--version":
			fmt.Println(version)
			return
		default:
			args = append(args, arg)
		}
	}

	src, err := io.ReadAll(os.Stdin)

	if err != nil {
		fmt.Println(err.Error())
		return
	}

	println(string(src))
	req, err := parseRawRequest(string(src))

	if err != nil {
		fmt.Println(err.Error())
		return
	}

	resp, err := http.DefaultClient.Do(req)

	if err != nil {
		fmt.Println(err.Error())
		return
	}

	defer resp.Body.Close()

	var metadata strings.Builder

	for k, v := range resp.Header {
		for _, v := range v {
			metadata.Write([]byte("HTTP: "))
			metadata.Write([]byte(k))
			metadata.Write([]byte(": "))
			metadata.Write([]byte(v))
			metadata.WriteByte('\n')
		}
	}

	metadataNode, err := collectNodes([]byte(metadata.String()))

	if err != nil {
		fmt.Println(err.Error())
		return
	}

	src, err = io.ReadAll(resp.Body)

	if err != nil {
		fmt.Println(err.Error())
		return
	}

	bodyNode, err := collectNodes(src)

	if err != nil {
		fmt.Println(err.Error())
		return
	}

	digInput := textinput.New()
	digInput.Prompt = ""
	digInput.TextStyle = lipgloss.NewStyle().
		Background(lipgloss.Color("7")).
		Foreground(lipgloss.Color("0"))
	digInput.Cursor.Style = lipgloss.NewStyle().
		Background(lipgloss.Color("15")).
		Foreground(lipgloss.Color("0"))

	commandInput := textinput.New()
	commandInput.Prompt = ":"

	searchInput := textinput.New()
	searchInput.Prompt = "/"

	gotoSymbolInput := textinput.New()
	gotoSymbolInput.Prompt = "@"

	collapsed := false
	if _, ok := os.LookupEnv("FX_COLLAPSED"); ok {
		collapsed = true
	}

	showLineNumbers := false
	if _, ok := os.LookupEnv("FX_LINE_NUMBERS"); ok {
		showLineNumbers = true
	}

	showSizes := false
	showSizesValue, ok := os.LookupEnv("FX_SHOW_SIZE")
	if ok {
		showSizesValue := strings.ToLower(showSizesValue)
		showSizes = showSizesValue == "true" || showSizesValue == "yes" || showSizesValue == "on" || showSizesValue == "1"
	}

	m := &model{
		bodyNode:        bodyNode,
		bottom:          bodyNode,
		collapsed:       collapsed,
		commandInput:    commandInput,
		digInput:        digInput,
		gotoSymbolInput: gotoSymbolInput,
		head:            bodyNode,
		metadataNode:    metadataNode,
		search:          newSearch(),
		searchCache:     newSearchCache(50), // Cache up to 50 search queries
		searchInput:     searchInput,
		showCursor:      true,
		showLineNumbers: showLineNumbers,
		showSizes:       showSizes,
		suspending:      false,
		top:             bodyNode,
		totalLines:      bodyNode.End.LineNumber,
		wrap:            true,
	}

	lipgloss.SetColorProfile(theme.TermOutput.ColorProfile())

	withMouse := tea.WithMouseCellMotion()

	p := tea.NewProgram(m,
		tea.WithAltScreen(),
		withMouse,
		tea.WithOutput(os.Stderr),
	)

	_, err = p.Run()
	if err != nil {
		panic(err)
	}

	if m.printErrorOnExit != nil {
		fmt.Println(m.printErrorOnExit.Error())
	} else if m.printOnExit {
		fmt.Println(m.cursorValue())
	}
}

func parseRawRequest(raw string) (req *http.Request, err error) {
	reader := bufio.NewReader(strings.NewReader(raw))

	firstLine, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	firstLine = strings.TrimSpace(firstLine)
	parts := strings.Fields(firstLine)
	if len(parts) < 2 {
		return nil, fmt.Errorf("Invalid HTTP request")
	}
	method := parts[0]
	url := parts[1]

	// Read headers
	headers := http.Header{}
	for {
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return nil, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		kv := strings.SplitN(line, ":", 2)
		if len(kv) == 2 {
			headers.Add(strings.TrimSpace(kv[0]), strings.TrimSpace(kv[1]))
		}
		if err == io.EOF {
			break
		}
	}

	// Collect body
	bodyBytes, _ := io.ReadAll(reader)

	fmt.Println(string(bodyBytes))

	req, err = http.NewRequest(method, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header = headers

	return
}

func collectNodes(src []byte) (head *Node, err error) {
	parser := NewJsonParser(src)

	firstOk := false
	var node *Node
	var prevNode *Node
	for {
		node, err = parser.Parse()
		if err != nil {
			if err == io.EOF {
				if head != nil {
					err = nil
				}
				break
			}
			textNode := parser.Recover()
			if !firstOk && !strings.HasPrefix(textNode.Value, "HTTP") {
				break
			}

			node = textNode
		} else {
			firstOk = true
		}

		if head == nil {
			head = node
			prevNode = head
		} else {
			prevNode.Adjacent(node)
			prevNode = node
		}
	}

	return
}

type model struct {
	bodyNode              *Node
	collapsed             bool
	commandInput          textinput.Model
	cursor                int // cursor position [0, termHeight)
	deletePending         bool
	digInput              textinput.Model
	fuzzyMatch            *fuzzy.Match
	gotoSymbolInput       textinput.Model
	head, top, bottom     *Node
	help                  viewport.Model
	keysIndex             []string
	keysIndexNodes        []*Node
	locationHistory       []location
	locationIndex         int // position in locationHistory
	metadataNode          *Node
	preview               viewport.Model
	printErrorOnExit      error
	printOnExit           bool
	search                *search
	searchCache           *searchCache
	searchInput           textinput.Model
	showCursor            bool
	showHelp              bool
	showLineNumbers       bool
	showMetadata          bool
	showPreview           bool
	showShowSelector      bool
	showSizes             bool
	suspending            bool
	termWidth, termHeight int
	totalLines            int
	wrap                  bool
	yank                  bool
}

type location struct {
	head *Node
	node *Node
}

type errorMsg struct {
	err error
}

func (m *model) Init() tea.Cmd {
	return func() tea.Msg {
		return nil
	}
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if msg, ok := msg.(tea.WindowSizeMsg); ok {
		oldTermWidth := m.termWidth
		m.termWidth = msg.Width
		m.termHeight = msg.Height
		m.help.Width = m.termWidth
		m.help.Height = m.termHeight - 1
		m.preview.Width = m.termWidth - 4
		m.preview.Height = m.termHeight - 4
		Wrap(m.top, m.viewWidth())
		// Only invalidate cache if terminal width changed and wrapping is enabled
		if oldTermWidth != m.termWidth && m.wrap {
			m.searchCache.invalidate()
		}
		m.redoSearch()
	}

	if m.showHelp {
		return m.handleHelpKey(msg)
	}

	if m.showPreview {
		return m.handlePreviewKey(msg)
	}

	switch msg := msg.(type) {
	case errorMsg:
		m.printErrorOnExit = msg.err
		return m, tea.Quit

	case tea.MouseMsg:
		m.handlePendingDelete(msg)

		switch {
		case msg.Button == tea.MouseButtonWheelUp:
			m.up()

		case msg.Button == tea.MouseButtonWheelDown:
			m.down()

		case msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress:
			m.digInput.Blur()
			m.showCursor = true
			if msg.Y < m.viewHeight() {
				if m.cursor == msg.Y {
					to, ok := m.cursorPointsTo()
					if ok {
						if to.IsCollapsed() {
							to.Expand()
						} else {
							to.Collapse()
						}

						value, isRef := isRefNode(to)
						if isRef {
							refPath, ok := jsonpath.ParseSchemaRef(value)
							if ok {
								m.selectNode(m.findByPath(refPath))
								m.recordHistory()
							}
						}
					}
				} else {
					to := m.at(msg.Y)
					if to != nil {
						m.cursor = msg.Y
						if to.IsCollapsed() {
							to.Expand()
						}
					}
				}
				m.recordHistory()
			}
		}

	case tea.ResumeMsg:
		m.suspending = false
		return m, nil

	case tea.KeyMsg:
		if m.digInput.Focused() {
			return m.handleDigKey(msg)
		}
		if m.commandInput.Focused() {
			return m.handleGotoLineKey(msg)
		}
		if m.searchInput.Focused() {
			return m.handleSearchKey(msg)
		}
		if m.gotoSymbolInput.Focused() {
			return m.handleGotoSymbolKey(msg)
		}
		if m.yank {
			return m.handleYankKey(msg)
		}
		if m.showShowSelector {
			return m.handleShowSelectorKey(msg)
		}
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *model) handleDigKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch {
	case key.Matches(msg, arrowUp):
		m.up()
		m.digInput.SetValue(m.cursorPath())
		m.digInput.CursorEnd()

	case key.Matches(msg, arrowDown):
		m.down()
		m.digInput.SetValue(m.cursorPath())
		m.digInput.CursorEnd()

	case msg.Type == tea.KeyEscape:
		m.digInput.Blur()

	case msg.Type == tea.KeyTab:
		m.digInput.SetValue(m.cursorPath())
		m.digInput.CursorEnd()

	case msg.Type == tea.KeyEnter:
		m.digInput.Blur()
		digPath, ok := jsonpath.Split(m.digInput.Value())
		if ok {
			n := m.findByPath(digPath)
			if n != nil {
				m.selectNode(n)
			}
		}

	case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+w"))):
		digPath, ok := jsonpath.Split(m.digInput.Value())
		if ok {
			if len(digPath) > 0 {
				digPath = digPath[:len(digPath)-1]
			}
			n := m.findByPath(digPath)
			if n != nil {
				m.selectNode(n)
				m.digInput.SetValue(m.cursorPath())
				m.digInput.CursorEnd()
			}
		}

	case key.Matches(msg, textinput.DefaultKeyMap.WordBackward):
		value := m.digInput.Value()
		pth, ok := jsonpath.Split(value[0:m.digInput.Position()])
		if ok {
			if len(pth) > 0 {
				pth = pth[:len(pth)-1]
				m.digInput.SetCursor(len(jsonpath.Join(pth)))
			} else {
				m.digInput.CursorStart()
			}
		}

	case key.Matches(msg, textinput.DefaultKeyMap.WordForward):
		value := m.digInput.Value()
		fullPath, ok1 := jsonpath.Split(value)
		pth, ok2 := jsonpath.Split(value[0:m.digInput.Position()])
		if ok1 && ok2 {
			if len(pth) < len(fullPath) {
				pth = append(pth, fullPath[len(pth)])
				m.digInput.SetCursor(len(jsonpath.Join(pth)))
			} else {
				m.digInput.CursorEnd()
			}
		}

	default:
		if key.Matches(msg, key.NewBinding(key.WithKeys("."))) {
			if m.digInput.Position() == len(m.digInput.Value()) {
				m.digInput.SetValue(m.cursorPath())
				m.digInput.CursorEnd()
			}
		}

		m.digInput, cmd = m.digInput.Update(msg)
		n := m.dig(m.digInput.Value())
		if n != nil {
			m.selectNode(n)
		}
	}
	return m, cmd
}

func (m *model) handleHelpKey(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	if msg, ok := msg.(tea.KeyMsg); ok {
		switch {
		case key.Matches(msg, keyMap.Quit), key.Matches(msg, keyMap.Help):
			m.showHelp = false
		}
	}
	m.help, cmd = m.help.Update(msg)
	return m, cmd
}

func (m *model) handlePreviewKey(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	if msg, ok := msg.(tea.KeyMsg); ok {
		switch {
		case key.Matches(msg, keyMap.Quit),
			key.Matches(msg, keyMap.Preview):
			m.showPreview = false

		case key.Matches(msg, keyMap.Print):
			return m, m.print()
		}
	}
	return m, cmd
}

func (m *model) handleGotoLineKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg.Type {
	case tea.KeyEscape:
		m.commandInput.Blur()
		m.commandInput.SetValue("")
		m.showCursor = true

	case tea.KeyEnter:
		m.commandInput.Blur()
		command := m.commandInput.Value()
		m.commandInput.SetValue("")
		return m.runCommand(command)

	default:
		m.commandInput, cmd = m.commandInput.Update(msg)
	}
	return m, cmd
}

func (m *model) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg.Type {
	case tea.KeyEscape:
		m.searchInput.Blur()
		m.searchInput.SetValue("")
		m.doSearch("")
		m.showCursor = true

	case tea.KeyEnter:
		m.searchInput.Blur()
		m.doSearch(m.searchInput.Value())
		m.recordHistory()

	default:
		m.searchInput, cmd = m.searchInput.Update(msg)
	}
	return m, cmd
}

func (m *model) handleGotoSymbolKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg.Type {
	case tea.KeyEscape, tea.KeyEnter, tea.KeyUp, tea.KeyDown:
		m.gotoSymbolInput.Blur()
		m.gotoSymbolInput.SetValue("")
		m.recordHistory()

	default:
		m.gotoSymbolInput, cmd = m.gotoSymbolInput.Update(msg)
		pattern := []rune(m.gotoSymbolInput.Value())
		found := fuzzy.Find(pattern, m.keysIndex)
		if found != nil {
			m.fuzzyMatch = found
			m.selectNode(m.keysIndexNodes[found.Index])
		}
	}

	switch msg.Type {
	case tea.KeyUp:
		m.up()

	case tea.KeyDown:
		m.down()
	}

	return m, cmd
}

func (m *model) handleYankKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, yankPath):
		_ = clipboard.WriteAll(m.cursorPath())
	case key.Matches(msg, yankKey):
		_ = clipboard.WriteAll(m.cursorKey())
	case key.Matches(msg, yankValueY, yankValueV):
		_ = clipboard.WriteAll(m.cursorValue())
	case key.Matches(msg, yankKeyValue):
		k := m.cursorKey()
		v := m.cursorValue()
		keyValue := k + ": " + v
		_ = clipboard.WriteAll(keyValue)
	}
	m.yank = false
	return m, nil
}

func (m *model) handleShowSelectorKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, showSizes):
		m.showSizes = !m.showSizes
	case key.Matches(msg, showLineNumbers):
		m.showLineNumbers = !m.showLineNumbers
		Wrap(m.top, m.viewWidth())
	}
	m.showShowSelector = false
	return m, nil
}

func (m *model) handlePendingDelete(msg tea.Msg) {
	// Handle potential 'dd' sequence for delete
	if m.deletePending {
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			if key.Matches(keyMsg, keyMap.Delete) {
				m.deleteAtCursor()
				m.deletePending = true
				return
			}
		}
		m.deletePending = false
	}
}

func (m *model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.handlePendingDelete(msg)

	switch {
	case key.Matches(msg, keyMap.Suspend):
		m.suspending = true
		return m, tea.Suspend

	case key.Matches(msg, keyMap.Quit):
		return m, tea.Quit

	case key.Matches(msg, keyMap.Help):
		m.help.SetContent(help(keyMap))
		m.showHelp = true

	case key.Matches(msg, keyMap.Up):
		m.up()

	case key.Matches(msg, keyMap.Down):
		m.down()

	case key.Matches(msg, keyMap.PageUp):
		m.cursor = m.viewHeight() - 1
		m.showCursor = true
		m.scrollBackward(max(0, m.viewHeight()-2))
		m.scrollIntoView() // As the cursor is at the bottom, and it may be empty.
		m.recordHistory()

	case key.Matches(msg, keyMap.PageDown):
		m.cursor = 0
		m.showCursor = true
		m.scrollForward(max(0, m.viewHeight()-2))
		m.recordHistory()

	case key.Matches(msg, keyMap.HalfPageUp):
		m.showCursor = true
		m.scrollBackward(m.viewHeight() / 2)
		m.scrollIntoView() // As the cursor stays at the same position, and it may be empty.
		m.recordHistory()

	case key.Matches(msg, keyMap.HalfPageDown):
		m.showCursor = true
		m.scrollForward(m.viewHeight() / 2)
		m.scrollIntoView() // As the cursor stays at the same position, and it may be empty.
		m.recordHistory()

	case key.Matches(msg, keyMap.GotoTop):
		m.head = m.top
		m.cursor = 0
		m.showCursor = true
		m.scrollIntoView()
		m.recordHistory()

	case key.Matches(msg, keyMap.GotoBottom):
		m.scrollToBottom()
		m.recordHistory()

	case key.Matches(msg, keyMap.NextSibling):
		pointsTo, ok := m.cursorPointsTo()
		if !ok {
			return m, nil
		}
		var nextSibling *Node
		if pointsTo.End != nil && pointsTo.End.Next != nil {
			nextSibling = pointsTo.End.Next
		} else if pointsTo.ChunkEnd != nil && pointsTo.ChunkEnd.Next != nil {
			nextSibling = pointsTo.ChunkEnd.Next
		} else {
			nextSibling = pointsTo.Next
		}
		if nextSibling != nil {
			m.selectNode(nextSibling)
		}
		m.recordHistory()

	case key.Matches(msg, keyMap.PrevSibling):
		pointsTo, ok := m.cursorPointsTo()
		if !ok {
			return m, nil
		}
		var prevSibling *Node
		parent := pointsTo.Parent
		if parent != nil && parent.End == pointsTo {
			prevSibling = parent
		} else if pointsTo.Prev != nil {
			prevSibling = pointsTo.Prev
			parent := prevSibling.Parent
			if parent != nil && parent.End == prevSibling {
				prevSibling = parent
			} else if prevSibling.Chunk != "" {
				prevSibling = parent
			}
		}
		if prevSibling != nil {
			m.selectNode(prevSibling)
		}
		m.recordHistory()

	case key.Matches(msg, keyMap.Collapse):
		n, ok := m.cursorPointsTo()
		if !ok {
			return m, nil
		}
		if n.HasChildren() && !n.IsCollapsed() {
			n.Collapse()
		} else {
			if n.Parent != nil {
				n = n.Parent
			}
		}
		m.selectNode(n)
		m.recordHistory()

	case key.Matches(msg, keyMap.Expand):
		n, ok := m.cursorPointsTo()
		if !ok {
			return m, nil
		}
		n.Expand()
		m.showCursor = true

	case key.Matches(msg, keyMap.CollapseRecursively):
		n, ok := m.cursorPointsTo()
		if !ok {
			return m, nil
		}
		if n.HasChildren() {
			n.CollapseRecursively()
		}
		m.showCursor = true

	case key.Matches(msg, keyMap.ExpandRecursively):
		n, ok := m.cursorPointsTo()
		if !ok {
			return m, nil
		}
		if n.HasChildren() {
			n.ExpandRecursively(0, math.MaxInt)
		}
		m.showCursor = true

	case key.Matches(msg, keyMap.CollapseAll):
		at, ok := m.cursorPointsTo()
		if ok {
			m.collapsed = true
			n := m.top
			for n != nil {
				if n.Kind != Err {
					n.CollapseRecursively()
				}
				if n.End == nil {
					n = nil
				} else {
					n = n.End.Next
				}
			}
			m.selectNode(at.Root())
			m.recordHistory()
		}

	case key.Matches(msg, keyMap.ExpandAll):
		at, ok := m.cursorPointsTo()
		if !ok {
			return m, nil
		}
		m.collapsed = false
		n := m.top
		for n != nil {
			n.ExpandRecursively(0, math.MaxInt)
			if n.End == nil {
				n = nil
			} else {
				n = n.End.Next
			}
		}
		m.selectNode(at)

	case key.Matches(msg, keyMap.CollapseLevel):
		at, ok := m.cursorPointsTo()
		if ok && at.HasChildren() {
			toLevel, _ := strconv.Atoi(msg.String())
			at.CollapseRecursively()
			at.ExpandRecursively(0, toLevel)
			m.showCursor = true
		}

	case key.Matches(msg, keyMap.ToggleWrap):
		at, ok := m.cursorPointsTo()
		if !ok {
			return m, nil
		}
		m.wrap = !m.wrap
		if m.wrap {
			Wrap(m.top, m.viewWidth())
		} else {
			DropWrapAll(m.top)
		}
		m.searchCache.invalidate()
		if at.Chunk != "" && at.Value == "" {
			at = at.Parent
		}
		m.redoSearch()
		m.selectNode(at)

	case key.Matches(msg, keyMap.ShowSelector):
		m.showShowSelector = true

	case key.Matches(msg, keyMap.ShowMetadata):
		m.showMetadata = !m.showMetadata

		m.cursor = 0
		if m.showMetadata {
			m.bottom = m.metadataNode
			m.head = m.metadataNode
			m.top = m.metadataNode
		} else {
			m.bottom = m.bodyNode
			m.head = m.bodyNode
			m.top = m.bodyNode
		}

		m.searchCache.invalidate()
		m.redoSearch()
		m.selectNode(m.top)

	case key.Matches(msg, keyMap.Yank):
		m.yank = true

	case key.Matches(msg, keyMap.Print):
		return m, m.print()

	case key.Matches(msg, keyMap.Open):
		return m, m.open()

	case key.Matches(msg, keyMap.Dig):
		at, ok := m.cursorPointsTo()
		if !ok {
			return m, nil
		}
		if at.Kind == Err {
			nextJson := at.FindNextNonErr()
			if nextJson != nil {
				m.selectNode(nextJson)
			}
		}
		m.digInput.SetValue(m.cursorPath() + ".")
		m.digInput.CursorEnd()
		m.digInput.Width = m.termWidth - 1
		m.digInput.Focus()

	case key.Matches(msg, keyMap.GotoSymbol):
		m.gotoSymbolInput.CursorEnd()
		m.gotoSymbolInput.Width = m.termWidth - 2 // -1 for the prompt, -1 for the cursor
		m.gotoSymbolInput.Focus()
		m.createKeysIndex()

	case key.Matches(msg, keyMap.GotoRef):
		at, ok := m.cursorPointsTo()
		if !ok {
			return m, nil
		}
		value, isRef := isRefNode(at)
		if isRef {
			refPath, ok := jsonpath.ParseSchemaRef(value)
			if ok {
				m.selectNode(m.findByPath(refPath))
				m.recordHistory()
			}
		}

	case key.Matches(msg, keyMap.CommandLine):
		m.commandInput.CursorEnd()
		m.commandInput.Width = m.termWidth - 2 // -1 for the prompt, -1 for the cursor
		m.commandInput.Focus()

	case key.Matches(msg, keyMap.Search):
		m.searchInput.CursorEnd()
		m.searchInput.Width = m.termWidth - 2 // -1 for the prompt, -1 for the cursor
		m.searchInput.Focus()

	case key.Matches(msg, keyMap.SearchNext):
		m.selectSearchResult(m.search.cursor + 1)
		m.recordHistory()

	case key.Matches(msg, keyMap.SearchPrev):
		m.selectSearchResult(m.search.cursor - 1)
		m.recordHistory()

	case key.Matches(msg, keyMap.GoBack):
		if m.locationIndex > 0 {
			at, ok := m.cursorPointsTo()
			if !ok {
				return m, nil
			}
			m.locationIndex--

			loc := m.locationHistory[m.locationIndex]
			for loc.node == at && m.locationIndex > 0 {
				m.locationIndex--
				loc = m.locationHistory[m.locationIndex]
			}
			m.selectNode(loc.head)
			m.selectNode(loc.node)
		}

	case key.Matches(msg, keyMap.GoForward):
		if m.locationIndex < len(m.locationHistory)-1 {
			m.locationIndex++
			loc := m.locationHistory[m.locationIndex]
			m.selectNode(loc.head)
			m.selectNode(loc.node)
		}

	case key.Matches(msg, keyMap.Delete):
		m.deletePending = true
	}
	return m, nil
}

func (m *model) up() {
	if m.head == nil {
		return
	}
	m.showCursor = true
	m.cursor--
	if m.cursor < 0 {
		m.cursor = 0
		if m.head.Prev != nil {
			m.head = m.head.Prev
		}
	}
}

func (m *model) down() {
	if m.head == nil {
		return
	}
	m.showCursor = true
	m.cursor++
	_, ok := m.cursorPointsTo()
	if !ok {
		m.cursor--
		return
	}
	if m.cursor >= m.viewHeight() {
		m.cursor = m.viewHeight() - 1
		if m.head.Next != nil {
			m.head = m.head.Next
		}
	}
}

func (m *model) recordHistory() {
	at, ok := m.cursorPointsTo()
	if !ok {
		return
	}
	if at.Chunk != "" && at.Value == "" {
		// We at the wrapped string, save the location of the original string node.
		at = at.Parent
	}
	if len(m.locationHistory) > 0 && m.locationHistory[len(m.locationHistory)-1].node == at {
		return
	}
	if m.locationIndex < len(m.locationHistory) {
		m.locationHistory = m.locationHistory[:m.locationIndex+1]
	}
	m.locationHistory = append(m.locationHistory, location{
		head: m.head,
		node: at,
	})
	m.locationIndex = len(m.locationHistory)
}

func (m *model) scrollToBottom() {
	if m.bottom == nil {
		return
	}
	m.head = m.bottom.Bottom()
	m.cursor = 0
	m.showCursor = true
	m.scrollIntoView()
}

func (m *model) visibleLines() int {
	visibleLines := 0
	n := m.head
	for n != nil && visibleLines < m.viewHeight() {
		visibleLines++
		n = n.Next
	}
	return visibleLines
}

func (m *model) scrollIntoView() {
	if m.head == nil {
		return
	}

	visibleLines := m.visibleLines()
	if m.cursor >= visibleLines {
		m.cursor = visibleLines - 1
	}
	for visibleLines < m.viewHeight() && m.head.Prev != nil {
		visibleLines++
		m.cursor++
		m.head = m.head.Prev
	}
}

func (m *model) scrollBackward(lines int) {
	it := m.head
	for it.Prev != nil {
		it = it.Prev
		if lines--; lines == 0 {
			break
		}
	}
	m.head = it
}

func (m *model) scrollForward(lines int) {
	if m.head == nil {
		return
	}
	it := m.head
	for it.Next != nil {
		it = it.Next
		if lines--; lines == 0 {
			break
		}
	}
	m.head = it
}

func (m *model) prettyKey(node *Node, selected bool) []byte {
	b := node.Key

	style := theme.CurrentTheme.Key
	if selected {
		style = theme.CurrentTheme.Cursor
	}

	if indexes, ok := m.search.keys[node]; ok {
		var out []byte
		for i, p := range splitByIndexes(b, indexes) {
			if i%2 == 0 {
				out = append(out, style(p.b)...)
			} else if p.index == m.search.cursor {
				out = append(out, theme.CurrentTheme.Cursor(p.b)...)
			} else {
				out = append(out, theme.CurrentTheme.Search(p.b)...)
			}
		}
		return out
	} else {
		return []byte(style(b))
	}
}

func (m *model) prettyPrint(node *Node, isSelected, isRef bool) string {
	var s string
	if node.Chunk != "" {
		s = node.Chunk
	} else {
		s = node.Value
	}

	if len(s) == 0 {
		if isSelected {
			return theme.CurrentTheme.Cursor(" ")
		} else {
			return s
		}
	}

	var style theme.Color

	if isSelected {
		style = theme.CurrentTheme.Cursor
	} else {
		style = theme.Value(node.Kind)
	}

	if isRef {
		style = theme.CurrentTheme.Ref
	}

	if indexes, ok := m.search.values[node]; ok {
		var out strings.Builder
		for i, p := range splitByIndexes(s, indexes) {
			if i%2 == 0 {
				out.WriteString(style(p.b))
			} else if p.index == m.search.cursor {
				out.WriteString(theme.CurrentTheme.Cursor(p.b))
			} else {
				out.WriteString(theme.CurrentTheme.Search(p.b))
			}
		}
		return out.String()
	} else {
		return style(s)
	}
}

func (m *model) viewWidth() int {
	width := m.termWidth
	if m.showLineNumbers {
		width -= len(strconv.Itoa(m.totalLines))
		width -= 2 // For margin between line numbers and JSON.
	}
	return width
}

func (m *model) viewHeight() int {
	if m.gotoSymbolInput.Focused() {
		return m.termHeight - 5
	}
	if m.commandInput.Focused() {
		return m.termHeight - 5
	}
	if m.searchInput.Focused() || m.searchInput.Value() != "" {
		return m.termHeight - 5
	}
	if m.yank {
		return m.termHeight - 5
	}
	if m.showShowSelector {
		return m.termHeight - 5
	}
	return m.termHeight - 5
}

func (m *model) cursorPointsTo() (*Node, bool) {
	n := m.at(m.cursor)
	return n, n != nil
}

func (m *model) at(pos int) *Node {
	head := m.head
	for range pos {
		if head == nil {
			break
		}
		head = head.Next
	}
	return head
}

func (m *model) nodeInsideView(n *Node) bool {
	if n == nil {
		return false
	}
	head := m.head
	for i := 0; i < m.viewHeight(); i++ {
		if head == nil {
			break
		}
		if head == n {
			return true
		}
		head = head.Next
	}
	return false
}

func (m *model) selectNodeInView(n *Node) {
	head := m.head
	for i := 0; i < m.viewHeight(); i++ {
		if head == nil {
			break
		}
		if head == n {
			m.cursor = i
			return
		}
		head = head.Next
	}
}

func (m *model) selectNode(n *Node) {
	if n == nil {
		return
	}
	m.showCursor = true
	if m.nodeInsideView(n) {
		m.selectNodeInView(n)
		m.scrollIntoView()
	} else {
		m.cursor = 0
		m.head = n
		{
			parent := n.Parent
			for parent != nil {
				parent.Expand()
				parent = parent.Parent
			}
		}
		m.centerLine(n)
		m.scrollIntoView()
	}
}

func (m *model) cursorPath() string {
	at, ok := m.cursorPointsTo()
	if !ok {
		return ""
	}
	path := ""
	for at != nil {
		if at.Prev != nil {
			if at.Chunk != "" && at.Value == "" {
				at = at.Parent
			}
			if at.Key != "" {
				quoted := at.Key
				unquoted, err := strconv.Unquote(quoted)
				if err == nil && jsonpath.Identifier.MatchString(unquoted) {
					path = "." + unquoted + path
				} else {
					path = "[" + quoted + "]" + path
				}
			} else if at.Index >= 0 {
				path = "[" + strconv.Itoa(at.Index) + "]" + path
			}
		}
		at = at.Parent
	}
	return path
}

func (m *model) cursorValue() string {
	at, ok := m.cursorPointsTo()
	if !ok {
		return ""
	}
	parent := at.Parent
	if parent != nil {
		// wrapped string part
		if at.Chunk != "" && at.Value == "" {
			at = parent
		}
		if len(at.Value) >= 1 && at.Value[0] == '}' || at.Value[0] == ']' {
			at = parent
		}
	}

	if at.Kind == String {
		str, err := strconv.Unquote(at.Value)
		if err == nil {
			return str
		}
		return at.Value
	}

	var out strings.Builder
	out.WriteString(at.Value)
	out.WriteString("\n")
	if at.HasChildren() {
		it := at.Next
		if at.IsCollapsed() {
			it = at.Collapsed
		}
		for it != nil {
			out.WriteString(strings.Repeat("  ", int(it.Depth-at.Depth)))
			if it.Key != "" {
				out.WriteString(it.Key)
				out.WriteString(": ")
			}
			if it.Value != "" {
				out.WriteString(it.Value)
			}
			if it == at.End {
				break
			}
			if it.Comma {
				out.WriteString(",")
			}
			out.WriteString("\n")
			if it.ChunkEnd != nil {
				it = it.ChunkEnd.Next
			} else if it.IsCollapsed() {
				it = it.Collapsed
			} else {
				it = it.Next
			}
		}
	}
	return out.String()
}

func (m *model) cursorKey() string {
	at, ok := m.cursorPointsTo()
	if !ok {
		return ""
	}
	if at.IsWrap() {
		at = at.Parent
	}
	if at.Key != "" {
		var v string
		_ = json.Unmarshal([]byte(at.Key), &v)
		return v
	}
	return strconv.Itoa(at.Index)
}

func (m *model) findByPath(path []any) *Node {
	n := m.currentTopNode()
	return n.FindByPath(path)
}

func (m *model) currentTopNode() *Node {
	at, ok := m.cursorPointsTo()
	if !ok {
		return nil
	}
	for at.Parent != nil {
		at = at.Parent
	}
	return at
}

func (m *model) doSearch(s string) {
	if s == "" {
		return
	}

	// Check cache first
	if cachedSearch, _, found := m.searchCache.get(s); found {
		m.search = cachedSearch
		return
	}

	m.search = newSearch()

	code, ci := regexCase(s)
	if ci {
		code = "(?i)" + code
	}

	re, err := regexp.Compile(code)
	if err != nil {
		m.search.err = err
		return
	}

	n := m.top
	searchIndex := 0
	for n != nil {
		if n.Key != "" {
			indexes := re.FindAllStringIndex(n.Key, -1)
			if len(indexes) > 0 {
				for i, pair := range indexes {
					m.search.results = append(m.search.results, n)
					m.search.keys[n] = append(m.search.keys[n], match{start: pair[0], end: pair[1], index: searchIndex + i})
				}
				searchIndex += len(indexes)
			}
		}
		indexes := re.FindAllStringIndex(n.Value, -1)
		if len(indexes) > 0 {
			for range indexes {
				m.search.results = append(m.search.results, n)
			}
			if n.Chunk != "" {
				// String can be split into chunks, so we need to map the indexes to the chunks.
				chunks := []string{n.Chunk}
				chunkNodes := []*Node{n}

				it := n.Next
				for it != nil {
					chunkNodes = append(chunkNodes, it)
					chunks = append(chunks, it.Chunk)
					if it == n.ChunkEnd {
						break
					}
					it = it.Next
				}

				chunkMatches := splitIndexesToChunks(chunks, indexes, searchIndex)
				for i, matches := range chunkMatches {
					m.search.values[chunkNodes[i]] = matches
				}
			} else {
				for i, pair := range indexes {
					m.search.values[n] = append(m.search.values[n], match{start: pair[0], end: pair[1], index: searchIndex + i})
				}
			}
			searchIndex += len(indexes)
		}

		if n.IsCollapsed() {
			n = n.Collapsed
		} else {
			n = n.Next
		}
	}

	m.searchCache.put(s, re, m.search)

	m.selectSearchResult(0)
}

func (m *model) selectSearchResult(i int) {
	if len(m.search.results) == 0 {
		return
	}
	if i < 0 {
		i = len(m.search.results) - 1
	}
	if i >= len(m.search.results) {
		i = 0
	}
	m.search.cursor = i
	result := m.search.results[i]
	m.selectNode(result)
	m.showCursor = false
}

func (m *model) redoSearch() {
	if m.searchInput.Value() != "" && len(m.search.results) > 0 {
		cursor := m.search.cursor
		m.doSearch(m.searchInput.Value())
		m.selectSearchResult(cursor)
	}
}

func (m *model) createKeysIndex() {
	at, ok := m.cursorPointsTo()
	if !ok {
		return
	}
	root := at.Root()
	if root == nil {
		return
	}
	paths := make([]string, 0, 100_000)
	nodes := make([]*Node, 0, 100_000)

	root.Paths(&paths, &nodes)

	m.keysIndex = paths
	m.keysIndexNodes = nodes
	m.fuzzyMatch = nil
}

func (m *model) dig(v string) *Node {
	p, ok := jsonpath.Split(v)
	if !ok {
		return nil
	}
	at := m.findByPath(p)
	if at != nil {
		return at
	}

	lastPart := p[len(p)-1]
	searchTerm, ok := lastPart.(string)
	if !ok {
		return nil
	}
	p = p[:len(p)-1]

	at = m.findByPath(p)
	if at == nil {
		return nil
	}

	keys, nodes := at.Children()

	found := fuzzy.Find([]rune(searchTerm), keys)
	if found == nil {
		return nil
	}

	return nodes[found.Index]
}

func (m *model) print() tea.Cmd {
	m.printOnExit = true
	return tea.Quit
}

func (m *model) open() tea.Cmd {
	if engine.FilePath == "" {
		return nil
	}
	command := append(
		strings.Split(lookup([]string{"FX_EDITOR", "EDITOR"}, "vim"), " "),
		engine.FilePath,
	)
	if command[0] == "vi" || command[0] == "vim" {
		at, ok := m.cursorPointsTo()
		if ok {
			tail := command[1:]
			command = append([]string{command[0]}, fmt.Sprintf("+%d", at.LineNumber))
			command = append(command, tail...)
		}
	}
	execCmd := exec.Command(command[0], command[1:]...)
	return tea.ExecProcess(execCmd, func(err error) tea.Msg {
		return nil
	})
}

// deleteAtCursor deletes the current key/value (node) from the view structure.
func (m *model) deleteAtCursor() {
	at, ok := m.cursorPointsTo()
	if !ok || at == nil {
		return
	}
	if next, ok := DeleteNode(at); ok {
		m.selectNode(next)
		m.recordHistory()
	}
}
