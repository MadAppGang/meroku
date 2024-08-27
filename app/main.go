package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type masterView struct {
	list        list.Model
	env         env
	selected    item
	width       int
	height      int
	detailView  detailView
	listFocused bool
	modal       *modalModel
}

type itemDelegate struct{}

func (d itemDelegate) Height() int                               { return 1 }
func (d itemDelegate) Spacing() int                              { return 0 }
func (d itemDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd { return nil }
func (d itemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(item)
	if !ok {
		return
	}

	str := ""

	// Add selection indicator
	if index == m.Index() {
		str += "> "
	} else {
		str += "  "
	}

	// Add indentation and item representation
	if i.isParent {
		if i.isExpanded {
			str += "▼ " + i.Title()
		} else {
			str += "▶ " + i.Title()
		}
	} else if i.title == ADD_NEW_SCHEDULED_TASK || i.title == ADD_NEW_EVENT_TASK {
		str += "    + " + i.Title()
		if index == m.Index() {
			fmt.Fprint(w, selectedItemStyle.Foreground(brightGreen).Render(str))
		} else {
			fmt.Fprint(w, itemStyle.Foreground(lightGray).Render(str))
		}
		return
	} else if i.isChild {
		str += "      " + i.Title() // 6 spaces for child items (2 for selection, 4 for indentation)
	} else {
		str += "  " + i.Title() // 2 spaces indentation for regular items
	}

	// Apply final styling
	if index == m.Index() {
		fmt.Fprint(w, selectedItemStyle.Render(str))
	} else {
		fmt.Fprint(w, itemStyle.Render(str))
	}
}

func initialModel(e env) masterView {
	items := menuListFromEnv(e)
	l := list.New(items, itemDelegate{}, listWidth, 0)
	l.Title = "Items"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.SetShowHelp(false)
	l.Styles.Title = lipgloss.NewStyle().MarginLeft(2)

	return masterView{
		list:        l,
		env:         e,
		selected:    items[0].(item),
		detailView:  nil,
		listFocused: true,
		modal:       nil,
	}
}

func (m masterView) Init() tea.Cmd {
	return nil
}

func (m masterView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// handle modal input
		if m.modal != nil {
			newModal, cmd := m.modal.Update(msg)
			if newModal, ok := newModal.(modalModel); ok {
				m.modal = &newModal
			} else {
				m.modal = nil
			}
			return m, cmd
		}
		// handle detail view input
		if m.detailView != nil && m.detailView.focused() {
			newDetailModel, newCmd := m.detailView.Update(msg)
			detailModel, ok := newDetailModel.(detailView)
			if ok {
				m.detailView = detailModel
				return m, newCmd
			}
		}
		// handle master view input
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "esc":
			if m.detailView != nil && m.detailView.focused() {
				m.listFocused = true
				m.detailView.setFocused(false)
			}
			return m, nil
		case "right":
			if _, ok := m.list.SelectedItem().(item); ok {
				if m.detailView != nil {
					m.listFocused = false
					m.detailView.setFocused(true)
					return m, nil
				}
			}
		case "d", "D":
			if i, ok := m.list.SelectedItem().(item); ok && i.isChild && i.title != ADD_NEW_SCHEDULED_TASK && i.title != ADD_NEW_EVENT_TASK {
				items := m.list.Items()
				index := m.list.Index()
				items = append(items[:index], items[index+1:]...)
				m.list.SetItems(items)
			}

		case "enter":
			if i, ok := m.list.SelectedItem().(item); ok {
				if m.detailView != nil {
					m.listFocused = false
					m.detailView.setFocused(true)
					return m, nil
				} else if i.isParent {
					items := m.list.Items()
					index := m.list.Index()
					i.isExpanded = !i.isExpanded
					items[index] = i
					if i.isExpanded {
						// Insert children
						newItems := make([]list.Item, len(items)+len(i.children))
						copy(newItems, items[:index+1])
						for j, child := range i.children {
							newItems[index+1+j] = child
						}
						copy(newItems[index+1+len(i.children):], items[index+1:])
						m.list.SetItems(newItems)
					} else {
						// Remove children
						newItems := make([]list.Item, 0, len(items))
						newItems = append(newItems, items[:index+1]...)
						childrenCount := 0
						for j := index + 1; j < len(items); j++ {
							if child, ok := items[j].(item); ok && !child.isParent {
								childrenCount++
							} else {
								break
							}
						}
						newItems = append(newItems, items[index+1+childrenCount:]...)
						m.list.SetItems(newItems)
					}
				} else if i.title == ADD_NEW_SCHEDULED_TASK || i.title == ADD_NEW_EVENT_TASK {
					// Handle adding a new child item
					items := m.list.Items()
					index := m.list.Index()
					var parent item
					var parentIndex int
					for i := index; i >= 0; i-- {
						itm, ok := items[i].(item)
						if ok && itm.isParent {
							parent = itm
							parentIndex = i
							break
						}
					}
					var newChild item
					if i.title == ADD_NEW_SCHEDULED_TASK {
						task := scheduledTask{name: fmt.Sprintf("Task_%d", len(parent.children)+1), schedule: "cron(0 6 * * ? *)"}
						newChild = item{title: task.name, desc: "New scheduled task", isChild: true, detailView: newScheduledTaskView(task)}
					} else {
						name := fmt.Sprintf("Task_%d", len(parent.children)+1)
						task := eventProcessorTask{name: name, ruleName: name + "_rule", detailTypes: []string{""}, sources: []string{""}}
						newChild = item{title: name, desc: "New event processor task", isChild: true, detailView: NewEventProcessorTaskView(task)}
					}
					parent.children = insertAt(parent.children, newChild, len(parent.children)-1)
					items = replaceAt[list.Item](items, parent, parentIndex)
					newItems := insertAt[list.Item](items, newChild, index)
					m.list.SetItems(newItems)
				}
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetSize(listWidth, msg.Height-7)
		if m.detailView != nil {
			m.detailView.setSize(m.width-listWidth-5, m.height-7)
		}
	case openModalMsg:
		m.modal = newModalModel(msg.input, m.width, m.height, msg.onConfirm)
		return m, textinput.Blink
	case closeModalMsg:
		m.modal = nil
	case detailLeaveFocusMsg:
		m.detailView.setFocused(false)
		m.listFocused = true
	case updateFieldMsg:
		if m.detailView != nil {
			newDetailModel, newCmd := m.detailView.Update(msg)
			detailModel, ok := newDetailModel.(detailView)
			if ok {
				m.detailView = detailModel
				return m, newCmd
			}
		}
	}

	m.list, cmd = m.list.Update(msg)

	if i, ok := m.list.SelectedItem().(item); ok {
		m.selected = i
		if m.detailView != i.detailView {
			if i.detailView != nil {
				i.detailView.setSize(m.width-listWidth-5, m.height-7)
			}
			m.detailView = i.detailView
		}
	}

	return m, cmd
}

func (m masterView) View() string {
	if m.modal != nil {
		return m.modal.View()
	}

	title := titleStyle.Render("MadAppGang Infrastructure App, v0.1. http://madappgang.com")

	// Calculate available height for views
	viewHeight := m.height - lipgloss.Height(title) - 4

	var listView, detailView string
	if m.listFocused {
		listView = focusedListStyle.Width(listWidth).Height(viewHeight).Render(m.list.View())
	} else {
		listView = listStyle.Width(listWidth).Height(viewHeight).Render(m.list.View())
	}

	if m.detailView != nil {
		if m.detailView.focused() {
			detailView = focusedDetailStyle.Width(m.width - listWidth - 5).Height(viewHeight).Render(m.detailView.View())
		} else {
			detailView = detailStyle.Width(m.width - listWidth - 5).Height(viewHeight).Render(m.detailView.View())
		}
	} else {
		detailView = detailStyle.Width(m.width - listWidth - 5).Height(viewHeight).Render(fmt.Sprintf("Selected: %s\n\nDescription: %s", m.selected.title, m.selected.desc))
	}

	main := lipgloss.JoinHorizontal(lipgloss.Left, listView, detailView)

	help := m.helpView()

	return lipgloss.JoinVertical(lipgloss.Left,
		title,
		main,
		help,
	)
}

func (m masterView) helpView() string {
	if m.detailView != nil {
		if m.listFocused {
			return lipgloss.NewStyle().
				Foreground(lipgloss.Color("241")).
				Render("↑/↓: Navigate • TAB: Focus Detail View • ESC: Back to List • q: Quit")
		}
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Render(fmt.Sprintf("TAB: Focus List • SHIFT+TAB: Prev Field • ↑/↓: Navigate Fields • ESC: Back to List • %s", m.detailView.helpMessage()))
	}
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Render("↑/↓: Navigate • ENTER: Select/Expand • ESC: Collapse • q: Quit")
}

func main() {
	file, err := os.OpenFile("app.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o666)
	if err != nil {
		panic(err)
	}
	defer file.Close()
	jsonHandler := slog.NewJSONHandler(file, nil)
	logger := slog.New(jsonHandler)
	slog.SetDefault(logger)

	e := createEnv("test")
	p := tea.NewProgram(initialModel(e), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Println("Error running program:", err)
		os.Exit(1)
	}
}
