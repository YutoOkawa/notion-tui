package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jomei/notionapi"
	"github.com/ookawayuuto/notion-tui/internal/domain"
)

type TaskClientInterface interface {
	FetchPersonalTasks(ctx context.Context) ([]domain.TaskItem, error)
	UpdateTaskStatus(ctx context.Context, pageID notionapi.ObjectID, newStatus string) error
	UpdateTaskDueDate(ctx context.Context, pageID notionapi.ObjectID, dueDate string) error
	AddPersonalTask(ctx context.Context, name, dueDate string) (domain.TaskItem, error)
}

type TaskState int

const (
	taskStateBrowse TaskState = iota
	taskStateInputDue
	taskStateInputNew
)

type TaskModel struct {
	client TaskClientInterface

	tasks  []domain.TaskItem
	cursor int
	state  TaskState

	textInput textinput.Model

	loading bool
	err     error
}

func NewTaskModel(client TaskClientInterface) *TaskModel {
	ti := textinput.New()
	ti.CharLimit = 156
	ti.Width = 30

	return &TaskModel{
		client:    client,
		state:     taskStateBrowse,
		textInput: ti,
		loading:   true,
	}
}

type taskLoadedMsg struct {
	tasks []domain.TaskItem
}
type taskActionDoneMsg struct{}
type taskErrMsg struct{ err error }

func (m *TaskModel) Init() tea.Cmd {
	return tea.Batch(m.fetchTasksCmd(), textinput.Blink)
}

func (m *TaskModel) fetchTasksCmd() tea.Cmd {
	return func() tea.Msg {
		tasks, err := m.client.FetchPersonalTasks(context.Background())
		if err != nil {
			return taskErrMsg{err}
		}
		return taskLoadedMsg{tasks: tasks}
	}
}

func (m *TaskModel) updateStatusCmd(id notionapi.ObjectID, status string) tea.Cmd {
	return func() tea.Msg {
		if err := m.client.UpdateTaskStatus(context.Background(), id, status); err != nil {
			return taskErrMsg{err}
		}
		return taskActionDoneMsg{}
	}
}

func (m *TaskModel) updateDueCmd(id notionapi.ObjectID, due string) tea.Cmd {
	return func() tea.Msg {
		if err := m.client.UpdateTaskDueDate(context.Background(), id, due); err != nil {
			return taskErrMsg{err}
		}
		return taskActionDoneMsg{}
	}
}

func (m *TaskModel) addTaskCmd(name, due string) tea.Cmd {
	return func() tea.Msg {
		if _, err := m.client.AddPersonalTask(context.Background(), name, due); err != nil {
			return taskErrMsg{err}
		}
		return taskActionDoneMsg{}
	}
}

func (m *TaskModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case taskLoadedMsg:
		m.tasks = msg.tasks
		m.loading = false
		if m.cursor >= len(m.tasks) {
			m.cursor = len(m.tasks) - 1
		}
		if m.cursor < 0 {
			m.cursor = 0
		}
		return m, nil

	case taskActionDoneMsg:
		// Refresh tasks after action
		m.loading = true
		return m, m.fetchTasksCmd()

	case taskErrMsg:
		m.err = msg.err
		m.state = taskStateBrowse // reset state
		return m, nil

	case tea.KeyMsg:
		if m.state == taskStateBrowse {
			switch msg.String() {
			case "ctrl+c", "q":
				return m, tea.Quit
			case "up", "k":
				if m.cursor > 0 {
					m.cursor--
				}
			case "down", "j":
				if m.cursor < len(m.tasks)-1 {
					m.cursor++
				}
			case "s", "enter": // Toggle Status
				if len(m.tasks) > 0 {
					t := m.tasks[m.cursor]
					nextStatus := getNextStatus(t.Status)
					return m, m.updateStatusCmd(t.ID, nextStatus)
				}
			case "d": // Set Due Date
				if len(m.tasks) > 0 {
					m.state = taskStateInputDue
					m.textInput.Placeholder = "YYYY-MM-DD"
					m.textInput.SetValue(m.tasks[m.cursor].DueDate)
					m.textInput.Focus()
				}
			case "a": // Add new task
				m.state = taskStateInputNew
				m.textInput.Placeholder = "タスク名を入力..."
				m.textInput.SetValue("")
				m.textInput.Focus()
			}
		} else if m.state == taskStateInputDue {
			switch msg.Type {
			case tea.KeyEsc:
				m.state = taskStateBrowse
			case tea.KeyEnter:
				due := strings.TrimSpace(m.textInput.Value())
				t := m.tasks[m.cursor]
				m.state = taskStateBrowse
				return m, m.updateDueCmd(t.ID, due)
			default:
				m.textInput, cmd = m.textInput.Update(msg)
				return m, cmd
			}
		} else if m.state == taskStateInputNew {
			switch msg.Type {
			case tea.KeyEsc:
				m.state = taskStateBrowse
			case tea.KeyEnter:
				name := strings.TrimSpace(m.textInput.Value())
				m.state = taskStateBrowse
				if name != "" {
					return m, m.addTaskCmd(name, "") // Initially no due date when adding from here
				}
			default:
				m.textInput, cmd = m.textInput.Update(msg)
				return m, cmd
			}
		}
	}
	return m, nil
}

func getNextStatus(current string) string {
	switch current {
	case "Not Started":
		return "In Progress"
	case "In Progress":
		return "Done"
	case "Done":
		return "Not Started"
	default:
		return "Not Started"
	}
}

func (m *TaskModel) View() string {
	if m.err != nil {
		return fmt.Sprintf("エラー: %v\n\n(q: 終了)", m.err)
	}
	if m.loading {
		return "Now Loading... (タスクを取得中)\n"
	}

	var sb strings.Builder

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	sb.WriteString(titleStyle.Render("=== Personal Tasks ===") + "\n\n")

	for i, t := range m.tasks {
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}

		statusColor := "240" // default/gray
		switch t.Status {
		case "Not Started":
			statusColor = "240"
		case "In Progress":
			statusColor = "39" // blue
		case "Done":
			statusColor = "40" // green
		}
		statusStr := lipgloss.NewStyle().Foreground(lipgloss.Color(statusColor)).Render(fmt.Sprintf("[%11s]", t.Status))
		
		dueStr := ""
		if t.DueDate != "" {
			dueStr = lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Render(fmt.Sprintf("(Due: %s)", t.DueDate))
		}

		nameStr := t.Name
		if i == m.cursor {
			nameStr = lipgloss.NewStyle().Bold(true).Render(nameStr)
			cursor = lipgloss.NewStyle().Bold(true).Render("> ")
		}

		line := fmt.Sprintf("%s %s %s %s", cursor, statusStr, nameStr, dueStr)
		sb.WriteString(line + "\n")
	}

	sb.WriteString("\n")

	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	if m.state == taskStateBrowse {
		sb.WriteString(helpStyle.Render("up/down: 移動 | s/enter: ステータス切替 | d: 期限設定 | a: 新規追加 | q: 終了"))
	} else if m.state == taskStateInputDue {
		sb.WriteString("期限を入力 (YYYY-MM-DD または空でクリア):\n")
		sb.WriteString(m.textInput.View() + "\n\n")
		sb.WriteString(helpStyle.Render("enter: 決定 | esc: キャンセル"))
	} else if m.state == taskStateInputNew {
		sb.WriteString("新規タスク名を入力:\n")
		sb.WriteString(m.textInput.View() + "\n\n")
		sb.WriteString(helpStyle.Render("enter: 決定 | esc: キャンセル"))
	}

	return sb.String()
}
