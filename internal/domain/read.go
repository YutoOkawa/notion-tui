package domain

import "github.com/jomei/notionapi"

type Book struct {
	ID         notionapi.ObjectID
	Title      string
	Status     string
	ReadPages  int
	TotalPages int
}
