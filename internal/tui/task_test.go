package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jomei/notionapi"
	"github.com/ookawayuuto/notion-tui/internal/domain"
)

type mockTaskClient struct {
	tasks       []domain.TaskItem
	updatedID   notionapi.ObjectID
	nextStatus  string
	updatedDue  string
	addedName   string
	fetchErr    error
	actionErr   error
}

func (m *mockTaskClient) FetchPersonalTasks(ctx context.Context) ([]domain.TaskItem, error) {
	if m.fetchErr != nil {
		return nil, m.fetchErr
	}
	return m.tasks, nil
}

func (m *mockTaskClient) UpdateTaskStatus(ctx context.Context, pageID notionapi.ObjectID, newStatus string) error {
	m.updatedID = pageID
	m.nextStatus = newStatus
	return m.actionErr
}

func (m *mockTaskClient) UpdateTaskDueDate(ctx context.Context, pageID notionapi.ObjectID, dueDate string) error {
	m.updatedID = pageID
	m.updatedDue = dueDate
	return m.actionErr
}

func (m *mockTaskClient) AddPersonalTask(ctx context.Context, name, dueDate, content string) (domain.TaskItem, error) {
	m.addedName = name
	return domain.TaskItem{ID: "new-task-id", Name: name}, m.actionErr
}

func (m *mockTaskClient) FetchTaskBody(ctx context.Context, pageID notionapi.ObjectID) (string, error) {
	return "# Task Body", m.actionErr
}

func (m *mockTaskClient) EditTaskBody(ctx context.Context, pageID notionapi.ObjectID, newMarkdown string) error {
	return m.actionErr
}

func TestTaskModel_LoadTasks(t *testing.T) {
	mockClient := &mockTaskClient{
		tasks: []domain.TaskItem{
			{ID: "task-1", Name: "Buy milk", Status: "Not Started"},
			{ID: "task-2", Name: "Read book", Status: "In Progress"},
		},
	}

	model := NewTaskModel(mockClient)
	if !model.loading {
		t.Error("初期化状態では loading が true であるべき")
	}

	// タスクロード完了メッセージを受信
	updated, _ := model.Update(taskLoadedMsg{tasks: mockClient.tasks})
	m := updated.(*TaskModel)

	if m.loading {
		t.Error("タスクロード後は loading が false であるべき")
	}
	if len(m.tasks) != 2 {
		t.Fatalf("タスク数が一致しません: got %d, want 2", len(m.tasks))
	}

	// View のスモークテスト
	view := m.View()
	if !strings.Contains(view, "Buy milk") || !strings.Contains(view, "Read book") {
		t.Error("View にロードされたタスク名が含まれているべき")
	}
}

func TestTaskModel_StatusToggle(t *testing.T) {
	mockClient := &mockTaskClient{
		tasks: []domain.TaskItem{
			{ID: "task-1", Name: "Task 1", Status: "Not Started"},
			{ID: "task-2", Name: "Task 2", Status: "In Progress"},
			{ID: "task-3", Name: "Task 3", Status: "Done"},
		},
	}

	model := NewTaskModel(mockClient)
	updated, _ := model.Update(taskLoadedMsg{tasks: mockClient.tasks})
	m := updated.(*TaskModel)

	// 1. Not Started -> In Progress
	sKey := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}}
	updated, cmd := m.Update(sKey)
	if cmd == nil {
		t.Fatal("ステータス更新の tea.Cmd が発行されるべき")
	}
	msg := cmd()
	if _, ok := msg.(taskActionDoneMsg); !ok {
		t.Errorf("ステータス更新成功時は taskActionDoneMsg が返るべき: got %T", msg)
	}
	if mockClient.nextStatus != "In Progress" {
		t.Errorf("次のステータスは 'In Progress' であるべき: got %s", mockClient.nextStatus)
	}

	// 2. カーソル移動して In Progress -> Done
	downKey := tea.KeyMsg{Type: tea.KeyDown}
	updated, _ = m.Update(downKey)
	m = updated.(*TaskModel)

	updated, cmd = m.Update(sKey)
	if cmd == nil {
		t.Fatal("ステータス更新の tea.Cmd が発行されるべき")
	}
	cmd()
	if mockClient.nextStatus != "Done" {
		t.Errorf("次のステータスは 'Done' であるべき: got %s", mockClient.nextStatus)
	}
}

func TestTaskModel_DueDateUpdate(t *testing.T) {
	mockClient := &mockTaskClient{
		tasks: []domain.TaskItem{
			{ID: "task-1", Name: "Task 1", DueDate: "2026-09-15"},
		},
	}

	model := NewTaskModel(mockClient)
	updated, _ := model.Update(taskLoadedMsg{tasks: mockClient.tasks})
	m := updated.(*TaskModel)

	// 'd' キーで期限入力モードへ遷移
	dKey := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}}
	updated, _ = m.Update(dKey)
	m = updated.(*TaskModel)

	if m.state != taskStateInputDue {
		t.Fatalf("状態が taskStateInputDue になるべき: got %v", m.state)
	}

	// Enter キーで期限更新
	m.textInput.SetValue("2026-09-30")
	enterKey := tea.KeyMsg{Type: tea.KeyEnter}
	updated, cmd := m.Update(enterKey)
	m = updated.(*TaskModel)

	if m.state != taskStateBrowse {
		t.Errorf("決定後は taskStateBrowse に戻るべき: got %v", m.state)
	}
	if cmd == nil {
		t.Fatal("期限更新用の tea.Cmd が発行されるべき")
	}
	cmd()
	if mockClient.updatedDue != "2026-09-30" {
		t.Errorf("更新期限が一致しません: got %s, want 2026-09-30", mockClient.updatedDue)
	}

	// 再度 'd' を押して Esc でキャンセルしたときの挙動
	updated, _ = m.Update(dKey)
	m = updated.(*TaskModel)
	escKey := tea.KeyMsg{Type: tea.KeyEsc}
	updated, cmd = m.Update(escKey)
	m = updated.(*TaskModel)

	if m.state != taskStateBrowse {
		t.Errorf("Esc後は taskStateBrowse に戻るべき: got %v", m.state)
	}
	if cmd != nil {
		t.Error("Escキャンセル時は Cmd が発行されないべき")
	}
}

func TestTaskModel_AddTask(t *testing.T) {
	mockClient := &mockTaskClient{}
	model := NewTaskModel(mockClient)
	updated, _ := model.Update(taskLoadedMsg{tasks: []domain.TaskItem{}})
	m := updated.(*TaskModel)

	// 'a' キーで新規タスク入力モードへ
	aKey := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}
	updated, _ = m.Update(aKey)
	m = updated.(*TaskModel)

	if m.state != taskStateInputNew {
		t.Fatalf("状態が taskStateInputNew になるべき: got %v", m.state)
	}

	// 空文字列での決定時は何もしない
	m.textInput.SetValue("")
	enterKey := tea.KeyMsg{Type: tea.KeyEnter}
	updated, cmd := m.Update(enterKey)
	m = updated.(*TaskModel)
	if cmd != nil {
		t.Error("タスク名が空の場合は追加 Cmd が発行されないべき")
	}

	// 有効なタスク名での決定
	updated, _ = m.Update(aKey)
	m = updated.(*TaskModel)
	m.textInput.SetValue("New Task Item")
	updated, cmd = m.Update(enterKey)
	m = updated.(*TaskModel)

	if cmd == nil {
		t.Fatal("タスク追加用の tea.Cmd が発行されるべき")
	}
	cmd()
	if mockClient.addedName != "New Task Item" {
		t.Errorf("追加タスク名が一致しません: got %s, want 'New Task Item'", mockClient.addedName)
	}
}

func TestTaskModel_ErrorHandling(t *testing.T) {
	mockClient := &mockTaskClient{}
	model := NewTaskModel(mockClient)

	// エラーメッセージ受信時の挙動
	testErr := errors.New("notion api failure")
	updated, cmd := model.Update(taskErrMsg{err: testErr})
	m := updated.(*TaskModel)

	if cmd != nil {
		t.Error("エラー受信時は追加 Cmd が不要")
	}
	if m.err != testErr {
		t.Errorf("エラーが保持されていません: got %v", m.err)
	}

	// エラー表示 View のスモークテスト
	view := m.View()
	if !strings.Contains(view, "notion api failure") {
		t.Error("View にエラー内容が含まれるべき")
	}
}
