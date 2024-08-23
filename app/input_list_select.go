package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/samber/lo"
	"golang.org/x/exp/slices"
)

type dialogItem string

func (i dialogItem) FilterValue() string { return "" }

type dialogItemDelegate struct{}

func (d dialogItemDelegate) Height() int                             { return 1 }
func (d dialogItemDelegate) Spacing() int                            { return 0 }
func (d dialogItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d dialogItemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(dialogItem)
	if !ok {
		return
	}

	str := fmt.Sprintf("%d. %s", index+1, i)

	fn := dialogListItemStyle.Render
	if index == m.Index() {
		fn = func(s ...string) string {
			return dialogListSelectedItemStyle.Render("> " + strings.Join(s, " "))
		}
	}

	fmt.Fprint(w, fn(str))
}

type InputListSelectModel struct {
	list.Model
}

func NewInputListSelectModel(value inputValue, width, height int) InputListSelectModel {
	items := lo.Map(value.Slice(), func(s string, _ int) list.Item {
		return dialogItem(s)
	})
	l := list.New(items, dialogItemDelegate{}, width, height)
	l.Title = "Select one and press Enter"
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetFilteringEnabled(false)
	l.Styles.Title = dialogListTitleStyle
	l.Styles.PaginationStyle = dialogListPaginationStyle
	l.Select(slices.Index(value.Slice(), value.String()))
	return InputListSelectModel{
		Model: l,
	}
}

func (m InputListSelectModel) Init() tea.Cmd {
	return nil
}

func (m InputListSelectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.Model, cmd = m.Model.Update(msg)
	return m, cmd
}
