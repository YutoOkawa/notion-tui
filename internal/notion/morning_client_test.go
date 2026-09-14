package notion

import (
	"testing"

	"github.com/jomei/notionapi"
)

func TestRichTextToMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    []notionapi.RichText
		expected string
	}{
		{
			name: "Plain text",
			input: []notionapi.RichText{
				{PlainText: "Hello world"},
			},
			expected: "Hello world",
		},
		{
			name: "Bold text",
			input: []notionapi.RichText{
				{
					PlainText: "Bold Title",
					Annotations: &notionapi.Annotations{
						Bold: true,
					},
				},
			},
			expected: "**Bold Title**",
		},
		{
			name: "Code text",
			input: []notionapi.RichText{
				{
					PlainText: "const x = 10",
					Annotations: &notionapi.Annotations{
						Code: true,
					},
				},
			},
			expected: "`const x = 10`",
		},
		{
			name: "Mixed text",
			input: []notionapi.RichText{
				{PlainText: "Check "},
				{
					PlainText: "this",
					Annotations: &notionapi.Annotations{
						Bold: true,
					},
				},
				{PlainText: " out!"},
			},
			expected: "Check **this** out!",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := richTextToMarkdown(tt.input)
			if got != tt.expected {
				t.Errorf("richTextToMarkdown() = %v, want %v", got, tt.expected)
			}
		})
	}
}
