package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
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
	AddPersonalTask(ctx context.Context, name, dueDate, content string) (domain.TaskItem, error)
	FetchTaskBody(ctx context.Context, pageID notionapi.ObjectID) (string, error)
	EditTaskBody(ctx context.Context, pageID notionapi.ObjectID, newMarkdown string) error
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
		if _, err := m.client.AddPersonalTask(context.Background(), name, due, ""); err != nil {
			return taskErrMsg{err}
		}
		return taskActionDoneMsg{}
	}
}

type taskBodyFetchedMsg struct {
	pageID  notionapi.ObjectID
	content string
}

func (m *TaskModel) fetchBodyCmd(pageID notionapi.ObjectID) tea.Cmd {
	return func() tea.Msg {
		content, err := m.client.FetchTaskBody(context.Background(), pageID)
		if err != nil {
			return taskErrMsg{err}
		}
		return taskBodyFetchedMsg{pageID: pageID, content: content}
	}
}

type taskUpdateBodyMsg struct {
	pageID  notionapi.ObjectID
	content string
}

type taskBodyUpdatedMsg struct {
	pageID notionapi.ObjectID
}

func (m *TaskModel) updateBodyCmd(pageID notionapi.ObjectID, content string) tea.Cmd {
	return func() tea.Msg {
		err := m.client.EditTaskBody(context.Background(), pageID, content)
		if err != nil {
			return taskErrMsg{err}
		}
		return taskBodyUpdatedMsg{pageID: pageID}
	}
}

func (m *TaskModel) openEditorCmd(pageID notionapi.ObjectID, content string) tea.Cmd {
	tmpFile, err := os.CreateTemp("", "ntui-edit-*.md")
	if err != nil {
		return func() tea.Msg { return taskErrMsg{err} }
	}

	tmpFile.WriteString(content)
	tmpFile.Close()

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vim"
	}

	c := exec.Command(editor, tmpFile.Name())
	return tea.ExecProcess(c, func(err error) tea.Msg {
		if err != nil {
			os.Remove(tmpFile.Name())
			return taskErrMsg{err}
		}

		newContentBytes, err := os.ReadFile(tmpFile.Name())
		os.Remove(tmpFile.Name())

		if err != nil {
			return taskErrMsg{err}
		}

		newContent := strings.TrimSpace(string(newContentBytes))
		if newContent == strings.TrimSpace(content) {
			return nil // No change, do nothing
		}

		return taskUpdateBodyMsg{pageID: pageID, content: newContent}
	})
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

	case taskBodyUpdatedMsg:
		m.loading = false
		return m, nil
	case taskBodyFetchedMsg:
		m.loading = false
		return m, m.openEditorCmd(msg.pageID, msg.content)
	case taskUpdateBodyMsg:
		m.loading = true
		return m, m.updateBodyCmd(msg.pageID, msg.content)

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
			case "s": // Toggle Status
				if len(m.tasks) > 0 {
					t := m.tasks[m.cursor]
					nextStatus := getNextStatus(t.Status)
					return m, m.updateStatusCmd(t.ID, nextStatus)
				}
			case "enter": // Open Editor
				if len(m.tasks) > 0 {
					t := m.tasks[m.cursor]
					m.loading = true
					return m, m.fetchBodyCmd(t.ID)
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
		sb.WriteString(helpStyle.Render("up/down: 移動 | s: ステータス切替 | enter: 詳細確認・編集 | d: 期限設定 | a: 新規追加 | q: 終了"))
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
