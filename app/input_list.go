package main

import (
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/samber/lo"
)

type dialogItem struct {
	textinput.Model
	value string
}

func (i dialogItem) FilterValue() string { return "" }

func (i dialogItem) Update(msg tea.Msg) (dialogItem, tea.Cmd) {
	n, c := i.Model.Update(msg)
	return dialogItem{Model: n, value: n.Value()}, c
}

type dialogItemDelegate struct{}

func (d dialogItemDelegate) Height() int  { return 1 }
func (d dialogItemDelegate) Spacing() int { return 0 }
func (d dialogItemDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd {
	var cmds []tea.Cmd
	index := m.Index()
	if index >= 0 && index < len(m.Items()) {
		item, ok := m.SelectedItem().(dialogItem)
		if !ok || !item.Focused() {
			return nil
		}
		var cmd tea.Cmd
		item, cmd = item.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		m.SetItem(index, item)
	}
	return tea.Batch(cmds...)
}

func (d dialogItemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(dialogItem)
	if !ok {
		return
	}

	if i.Focused() {
		i.PlaceholderStyle = baseTextInputTheme.Focused.Placeholder
		i.PromptStyle = baseTextInputTheme.Focused.Prompt
		i.Cursor.Style = baseTextInputTheme.Focused.Cursor
		i.Cursor.TextStyle = baseTextInputTheme.Focused.CursorText
		i.TextStyle = baseTextInputTheme.Focused.Text
		i.Prompt = baseTextInputTheme.Focused.PromptText

		fmt.Fprint(w, i.Model.View())
	} else {
		str := fmt.Sprintf("%d. %s", index+1, i.value)
		if index == m.Index() {
			str = dialogListSelectedItemStyle.Render("> " + str)
		} else {
			str = dialogListItemStyle.Render(str)
		}
		fmt.Fprint(w, str)
	}
}

type InputListSelectModel struct {
	list.Model
	mode InputValueType
}

func (m InputListSelectModel) ListItems() []string {
	items := lo.Map(m.Items(), func(s list.Item, _ int) string {
		li, ok := s.(dialogItem)
		if ok {
			return li.value
		}
		return ""
	})
	return items
}

func NewInputListSelectModel(value inputValue, width, height int) InputListSelectModel {
	sitems := []string{}
	selectedIdx := 0
	title := ""
	var mode InputValueType
	if ss, ok := value.(sliceSelectValue); ok {
		mode = InputValueTypeSingleSelect
		title = "Select one and press Enter"
		sitems = ss.Options()
		selectedIdx = ss.index
	} else if ss, ok := value.(sliceValue); ok {
		mode = InputValueTypeSlice
		title = "Press <Enter> to edit or <Esc> to cancel"
		sitems = ss.Slice()
	}

	items := lo.Map(sitems, func(s string, _ int) list.Item {
		model := textinput.New()
		model.SetValue(s)

		return dialogItem{Model: model, value: s}
	})
	l := list.New(items, dialogItemDelegate{}, width, height)
	l.Title = title
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetFilteringEnabled(false)
	l.Styles.Title = dialogListTitleStyle
	l.Styles.PaginationStyle = dialogListPaginationStyle
	if selectedIdx > 0 {
		l.Select(selectedIdx)
	}
	return InputListSelectModel{
		Model: l,
		mode:  mode,
	}
}

func (m InputListSelectModel) Init() tea.Cmd {
	return nil
}

func (m InputListSelectModel) CanEscape(msg tea.Msg) bool {
	if m.mode == InputValueTypeSlice {
		item, ok := m.Model.SelectedItem().(dialogItem)
		if !ok || !item.Focused() {
			return true
		}
		return false
	} else {
		return true
	}
}

func (m InputListSelectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.mode == InputValueTypeSlice {
		return m.UpdateSlice(msg)
	} else {
		return m.UpdateSelect(msg)
	}
}

func (m InputListSelectModel) UpdateSlice(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			if item, ok := m.Model.SelectedItem().(dialogItem); ok {
				if item.Focused() {
					item.value = item.Model.Value()
					item.Blur()
					cmd := m.SetItem(m.Index(), item)
					return m, cmd
				} else {
					cmd := item.Focus()
					m.SetItem(m.Index(), item)
					return m, cmd
				}
			}
		case "a", "A":
			item, ok := m.Model.SelectedItem().(dialogItem)
			if !ok || !item.Focused() {
				s := fmt.Sprintf("New item %d", len(m.Model.Items())+1)
				model := textinput.New()
				model.SetValue(s)
				item := dialogItem{Model: model, value: s}
				cmd := m.InsertItem(m.Index(), item)
				return m, cmd
			}
		case "d", "D":
			item, ok := m.Model.SelectedItem().(dialogItem)
			if ok && !item.Focused() {
				m.RemoveItem(m.Index())
				return m, nil
			}
		case "esc":
			item, ok := m.Model.SelectedItem().(dialogItem)
			if !ok || !item.Focused() {
				return m, func() tea.Msg {
					return closeModalMsg{}
				}
			}
			// Cancel edit
			item.SetValue(item.value)
			item.Blur()
			cmd := m.SetItem(m.Index(), item)
			return m, cmd
		}
	}

	item, ok := m.Model.SelectedItem().(dialogItem)
	if ok && item.Focused() {
		item.Model, cmd = item.Model.Update(msg)
		m.SetItem(m.Index(), item)
		return m, cmd
	} else {
		m.Model, cmd = m.Model.Update(msg)
		return m, cmd
	}
}

func (m InputListSelectModel) UpdateSelect(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg {
				return closeModalMsg{}
			}
		}
	}
	m.Model, cmd = m.Model.Update(msg)
	return m, cmd
}
