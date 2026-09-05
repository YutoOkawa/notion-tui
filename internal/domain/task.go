package domain

import "github.com/jomei/notionapi"

type TaskItem struct {
	ID        notionapi.ObjectID
	Name      string
	Status    string
	DueDate   string
	TaskType  string
}
