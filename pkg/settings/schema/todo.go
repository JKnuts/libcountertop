package schema

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/ygelfand/libcountertop/pkg/settings/storage"
)

type Todo struct {
	Entities  []string `json:"entities"`
	Completed bool     `json:"completed"`
}

func DefaultTodo() Todo { return Todo{Entities: []string{}} }

func (t Todo) Clone() Todo {
	t.Entities = slices.Clone(t.Entities)
	return t
}

func MigrateTodo(data []byte, todo *Todo) error {
	var old struct {
		Todo struct {
			Entity string `json:"entity"`
		} `json:"todo"`
	}
	if err := json.Unmarshal(data, &old); err != nil {
		return err
	}
	if e := strings.TrimSpace(old.Todo.Entity); e != "" && len(todo.Entities) == 0 {
		todo.Entities = []string{e}
	}
	return nil
}

type TodoWriter struct{ s storage.Section[Todo] }

func NewTodoWriter(s storage.Section[Todo]) TodoWriter { return TodoWriter{s: s} }

func (w TodoWriter) Show(entity string, on bool) error {
	return w.s.Update(func(c *Todo) { c.Entities = Toggled(c.Entities, entity, on) })
}

func (w TodoWriter) Completed(v bool) error {
	return w.s.Update(func(c *Todo) { c.Completed = v })
}
