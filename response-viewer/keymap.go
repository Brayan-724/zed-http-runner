package main

import "github.com/charmbracelet/bubbles/key"

type KeyMap struct {
	Collapse            key.Binding
	CollapseAll         key.Binding
	CollapseLevel       key.Binding
	CollapseRecursively key.Binding
	CommandLine         key.Binding
	Delete              key.Binding
	Dig                 key.Binding
	Down                key.Binding
	Expand              key.Binding
	ExpandAll           key.Binding
	ExpandRecursively   key.Binding
	GoBack              key.Binding
	GoForward           key.Binding
	GotoBottom          key.Binding
	GotoRef             key.Binding
	GotoSymbol          key.Binding
	GotoTop             key.Binding
	HalfPageDown        key.Binding
	HalfPageUp          key.Binding
	Help                key.Binding
	NextSibling         key.Binding
	Open                key.Binding
	PageDown            key.Binding
	PageUp              key.Binding
	PrevSibling         key.Binding
	Preview             key.Binding
	Print               key.Binding
	Quit                key.Binding
	Search              key.Binding
	SearchNext          key.Binding
	SearchPrev          key.Binding
	ShowSelector        key.Binding
	ShowMetadata        key.Binding
	Suspend             key.Binding
	ToggleWrap          key.Binding
	Up                  key.Binding
	Yank                key.Binding
}

var keyMap KeyMap

func binding(desc string, keys ...string) key.Binding {
	return key.NewBinding(key.WithKeys(keys...), key.WithHelp("", desc))
}

func simpleBinding(keys ...string) key.Binding {
	return key.NewBinding(key.WithKeys(keys...))
}

func init() {
	keyMap = KeyMap{
		Collapse:            binding("collapse", "left", "h", "backspace"),
		CollapseAll:         binding("collapse all", "E"),
		CollapseLevel:       binding("collapse to nth level", "1", "2", "3", "4", "5", "6", "7", "8", "9"),
		CollapseRecursively: binding("collapse recursively", "H", "shift+left"),
		CommandLine:         binding("open command line", ":"),
		Delete:              binding("delete node", "d"),
		Dig:                 binding("dig", "."),
		Down:                binding("down", "down", "j"),
		Expand:              binding("expand", "right", "l", "enter"),
		ExpandAll:           binding("expand all", "e"),
		ExpandRecursively:   binding("expand recursively", "L", "shift+right"),
		GoBack:              binding("go back", "["),
		GoForward:           binding("go forward", "]"),
		GotoBottom:          binding("goto bottom", "G", "end"),
		GotoRef:             binding("goto ref", "ctrl+g"),
		GotoSymbol:          binding("goto symbol", "@"),
		GotoTop:             binding("goto top", "g", "home"),
		HalfPageDown:        binding("half page down", "ctrl+d"),
		HalfPageUp:          binding("half page up", "ctrl+u"),
		Help:                binding("show help", "?"),
		NextSibling:         binding("next sibling", "J", "shift+down"),
		Open:                binding("open in editor", "v"),
		PageDown:            binding("page down", "pgdown", " ", "f"),
		PageUp:              binding("page up", "pgup", "b"),
		PrevSibling:         binding("previous sibling", "K", "shift+up"),
		Preview:             binding("preview", "p"),
		Print:               binding("print", "P"),
		Quit:                binding("exit program", "q", "ctrl+c", "esc"),
		Search:              binding("search regexp", "/"),
		SearchNext:          binding("next search result", "n"),
		SearchPrev:          binding("prev search result", "N"),
		ShowSelector:        binding("show sizes/line numbers", "s"),
		ShowMetadata:        binding("show response metadata", "tab"),
		Suspend:             binding("suspend program", "ctrl+z"),
		ToggleWrap:          binding("toggle strings wrap", "z"),
		Up:                  binding("up", "up", "k"),
		Yank:                binding("yank/copy", "y"),
	}
}

var (
	yankValueY      = simpleBinding("y")
	yankValueV      = simpleBinding("v")
	yankKey         = simpleBinding("k")
	yankPath        = simpleBinding("p")
	yankKeyValue    = simpleBinding("b")
	arrowUp         = simpleBinding("up")
	arrowDown       = simpleBinding("down")
	showSizes       = simpleBinding("s")
	showLineNumbers = simpleBinding("l")
)
