package domain

import "github.com/jomei/notionapi"

type KBItem struct {
	ID       notionapi.ObjectID
	Title    string
	Category string
}
