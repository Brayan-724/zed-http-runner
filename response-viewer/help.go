package main

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
)

func usage(keyMap KeyMap) string {
	title := lipgloss.NewStyle().Bold(true)
	pad := lipgloss.NewStyle().PaddingLeft(4)
	return fmt.Sprintf(`
  %v
    Terminal JSON viewer

  %v
    response-viewer <url>

  %v
    -h, --help            print help
    -v, --version         print version

  %v
%v
`,
		title.Render(version),
		title.Render("Usage"),
		title.Render("Flags"),
		title.Render("Key Bindings"),
		strings.Join(keyMapInfo(keyMap, pad), "\n"),
	)
}

func help(keyMap KeyMap) string {
	title := lipgloss.NewStyle().Bold(true)
	pad := lipgloss.NewStyle().PaddingLeft(4)
	return fmt.Sprintf(`
  %v
%v
`,
		title.Render("Key Bindings"),
		strings.Join(keyMapInfo(keyMap, pad), "\n"),
	)
}

func keyMapInfo(keyMap KeyMap, style lipgloss.Style) []string {
	v := reflect.ValueOf(keyMap)
	fields := reflect.VisibleFields(v.Type())

	keys := make([]string, 0)
	for i := range fields {
		k := v.Field(i).Interface().(key.Binding)
		str := k.Help().Key
		if len(str) == 0 {
			if len(k.Keys()) > 5 {
				str = fmt.Sprintf("%v-%v", k.Keys()[0], k.Keys()[len(k.Keys())-1])
			} else {
				str = strings.Join(k.Keys(), ", ")
			}
		}
		keys = append(keys, fmt.Sprintf("%v    ", str))
	}

	desc := make([]string, 0)
	for i := range fields {
		k := v.Field(i).Interface().(key.Binding)
		desc = append(desc, fmt.Sprintf("%v", k.Help().Desc))
	}

	content := lipgloss.JoinHorizontal(
		lipgloss.Top,
		strings.Join(keys, "\n"),
		strings.Join(desc, "\n"),
	)

	return strings.Split(style.Render(content), "\n")
}
