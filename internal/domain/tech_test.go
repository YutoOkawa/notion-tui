package domain_test

import (
	"testing"

	"github.com/ookawayuuto/notion-tui/internal/domain"
)

func TestTechItem_IsUnlabeled(t *testing.T) {
	tests := []struct {
		name     string
		item     domain.TechItem
		expected bool
	}{
		{
			name: "Genre and Tags are empty",
			item: domain.TechItem{
				Title: "Test Blog",
				Genre: "",
				Tags:  nil,
			},
			expected: true,
		},
		{
			name: "Genre is set but Tags is empty",
			item: domain.TechItem{
				Title: "Test Blog",
				Genre: "Backend",
				Tags:  []string{},
			},
			expected: true,
		},
		{
			name: "Genre is empty but Tags is set",
			item: domain.TechItem{
				Title: "Test Blog",
				Genre: "",
				Tags:  []string{"Go"},
			},
			expected: true,
		},
		{
			name: "Both Genre and Tags are set",
			item: domain.TechItem{
				Title: "Test Blog",
				Genre: "Backend",
				Tags:  []string{"Go"},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := tt.item.IsUnlabeled()
			if actual != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, actual)
			}
		})
	}
}

func TestTechItem_StatusHelpers(t *testing.T) {
	tests := []struct {
		status       string
		expectUnread bool
		expectRead   bool
	}{
		{status: "未読", expectUnread: true, expectRead: false},
		{status: "", expectUnread: true, expectRead: false},
		{status: "読書中", expectUnread: false, expectRead: false},
		{status: "読了", expectUnread: false, expectRead: true},
		{status: "要復習", expectUnread: false, expectRead: false},
	}

	for _, tt := range tests {
		t.Run("Status="+tt.status, func(t *testing.T) {
			item := domain.TechItem{Status: tt.status}
			if item.IsUnread() != tt.expectUnread {
				t.Errorf("IsUnread() expected %v, got %v", tt.expectUnread, item.IsUnread())
			}
			if item.IsRead() != tt.expectRead {
				t.Errorf("IsRead() expected %v, got %v", tt.expectRead, item.IsRead())
			}
		})
	}
}
