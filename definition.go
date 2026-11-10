package sse

import "webtyp.com/model"

var SSEMessageModel = model.Definition{
	Name: "ssemessage",
	Fields: []model.Field{
		{Name: "id", Type: model.Text()},
		{Name: "event", Type: model.Text()},
		{Name: "data", Type: model.Blob()},
	},
}
