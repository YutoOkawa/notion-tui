package domain

import "github.com/jomei/notionapi"

type Wakuwaku struct {
	ID   notionapi.ObjectID
	Name string
}

type Monster struct {
	ID        notionapi.ObjectID
	Name      string
	Attribute string
	Priority  string
	Has       bool
	AccountA  []notionapi.ObjectID
	AccountB  []notionapi.ObjectID
	AccountC  []notionapi.ObjectID
	AccountD  []notionapi.ObjectID
	AccountA2 []notionapi.ObjectID
	AccountB2 []notionapi.ObjectID
}
