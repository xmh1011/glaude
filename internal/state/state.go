// Package state provides session-level shared state for tools.
//
// Multiple tools (TodoWrite, TaskCreate, TaskUpdate, etc.) need access to
// shared mutable state such as the todo list, task board, plan mode flag,
// and worktree session. This package provides a thread-safe container for
// that state.
package state

import (
	"fmt"
	"sync"
)

// TodoStatus represents the status of a todo item.
type TodoStatus string

const (
	TodoPending    TodoStatus = "pending"
	TodoInProgress TodoStatus = "in_progress"
	TodoCompleted  TodoStatus = "completed"
)

// TodoItem is a single entry in the todo list.
type TodoItem struct {
	Content    string     `json:"content"`
	Status     TodoStatus `json:"status"`
	ActiveForm string     `json:"activeForm,omitempty"`
}

// TaskStatus represents the status of a task.
type TaskStatus string

const (
	TaskPending    TaskStatus = "pending"
	TaskInProgress TaskStatus = "in_progress"
	TaskCompleted  TaskStatus = "completed"
	TaskDeleted    TaskStatus = "deleted"
)

// Task is a structured work item with dependencies.
type Task struct {
	ID          string            `json:"id"`
	Subject     string            `json:"subject"`
	Description string            `json:"description"`
	Status      TaskStatus        `json:"status"`
	ActiveForm  string            `json:"activeForm,omitempty"`
	Owner       string            `json:"owner,omitempty"`
	Metadata    map[string]any    `json:"metadata,omitempty"`
	Blocks      map[string]bool   `json:"-"` // task IDs this task blocks
	BlockedBy   map[string]bool   `json:"-"` // task IDs that block this task
}

// WorktreeSession tracks the active git worktree.
type WorktreeSession struct {
	Path       string // worktree directory path
	Branch     string // branch name
	OriginalWD string // original working directory to return to
}

// State holds session-level shared mutable state.
type State struct {
	mu           sync.RWMutex
	todos        []TodoItem
	tasks        map[string]*Task
	nextTaskID   int
	planMode     bool
	prePlanMode  string
	worktree     *WorktreeSession
}

// New creates a new empty State.
func New() *State {
	return &State{
		tasks:      make(map[string]*Task),
		nextTaskID: 1,
	}
}

// --- Todo operations ---

// SetTodos atomically replaces the todo list. If all items are completed,
// the list is cleared.
func (s *State) SetTodos(items []TodoItem) {
	s.mu.Lock()
	defer s.mu.Unlock()

	allDone := len(items) > 0
	for i := range items {
		if items[i].Status == "" {
			items[i].Status = TodoPending
		}
		if items[i].Status != TodoCompleted {
			allDone = false
		}
	}
	if allDone {
		s.todos = nil
		return
	}
	s.todos = make([]TodoItem, len(items))
	copy(s.todos, items)
}

// Todos returns a copy of the current todo list.
func (s *State) Todos() []TodoItem {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]TodoItem, len(s.todos))
	copy(out, s.todos)
	return out
}

// --- Task operations ---

// CreateTask adds a new task and returns its ID.
func (s *State) CreateTask(subject, description, activeForm string, metadata map[string]any) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := fmt.Sprintf("%d", s.nextTaskID)
	s.nextTaskID++

	s.tasks[id] = &Task{
		ID:          id,
		Subject:     subject,
		Description: description,
		Status:      TaskPending,
		ActiveForm:  activeForm,
		Metadata:    metadata,
		Blocks:      make(map[string]bool),
		BlockedBy:   make(map[string]bool),
	}
	return id
}

// GetTask returns a task by ID, or nil if not found.
func (s *State) GetTask(id string) *Task {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tasks[id]
	if !ok {
		return nil
	}
	// Return a shallow copy to prevent mutation outside the lock.
	cp := *t
	cp.Blocks = copySet(t.Blocks)
	cp.BlockedBy = copySet(t.BlockedBy)
	if t.Metadata != nil {
		cp.Metadata = make(map[string]any, len(t.Metadata))
		for k, v := range t.Metadata {
			cp.Metadata[k] = v
		}
	}
	return &cp
}

// AllTasks returns all non-deleted tasks ordered by ID.
func (s *State) AllTasks() []*Task {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []*Task
	// Iterate in ID order (1, 2, 3, ...).
	for i := 1; i < s.nextTaskID; i++ {
		id := fmt.Sprintf("%d", i)
		t, ok := s.tasks[id]
		if !ok || t.Status == TaskDeleted {
			continue
		}
		cp := *t
		cp.Blocks = copySet(t.Blocks)
		cp.BlockedBy = copySet(t.BlockedBy)
		out = append(out, &cp)
	}
	return out
}

// UpdateTask applies partial updates to a task. Returns an error if the task
// is not found.
func (s *State) UpdateTask(id string, opts UpdateOpts) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	t, ok := s.tasks[id]
	if !ok {
		return fmt.Errorf("task %q not found", id)
	}

	if opts.Subject != nil {
		t.Subject = *opts.Subject
	}
	if opts.Description != nil {
		t.Description = *opts.Description
	}
	if opts.ActiveForm != nil {
		t.ActiveForm = *opts.ActiveForm
	}
	if opts.Status != nil {
		t.Status = *opts.Status
		if *opts.Status == TaskDeleted {
			// Remove from other tasks' dependency sets.
			for _, other := range s.tasks {
				delete(other.Blocks, id)
				delete(other.BlockedBy, id)
			}
		}
	}
	if opts.Owner != nil {
		t.Owner = *opts.Owner
	}
	if opts.Metadata != nil {
		if t.Metadata == nil {
			t.Metadata = make(map[string]any)
		}
		for k, v := range opts.Metadata {
			if v == nil {
				delete(t.Metadata, k)
			} else {
				t.Metadata[k] = v
			}
		}
	}
	for _, bid := range opts.AddBlocks {
		if other, ok := s.tasks[bid]; ok {
			t.Blocks[bid] = true
			other.BlockedBy[id] = true
		}
	}
	for _, bid := range opts.AddBlockedBy {
		if other, ok := s.tasks[bid]; ok {
			t.BlockedBy[bid] = true
			other.Blocks[id] = true
		}
	}
	return nil
}

// DeleteTask removes a task by setting its status to deleted.
func (s *State) DeleteTask(id string) error {
	return s.UpdateTask(id, UpdateOpts{Status: Ptr(TaskDeleted)})
}

// UpdateOpts holds optional fields for UpdateTask.
type UpdateOpts struct {
	Subject     *string
	Description *string
	ActiveForm  *string
	Status      *TaskStatus
	Owner       *string
	Metadata    map[string]any
	AddBlocks   []string
	AddBlockedBy []string
}

// --- Plan mode operations ---

// SetPlanMode enters or exits plan mode.
func (s *State) SetPlanMode(on bool, prePlanMode string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.planMode = on
	if on {
		s.prePlanMode = prePlanMode
	}
}

// InPlanMode returns whether plan mode is active.
func (s *State) InPlanMode() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.planMode
}

// PrePlanMode returns the permission mode string saved before entering plan mode.
func (s *State) PrePlanMode() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.prePlanMode
}

// --- Worktree operations ---

// SetWorktree sets the active worktree session.
func (s *State) SetWorktree(ws *WorktreeSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.worktree = ws
}

// Worktree returns the active worktree session, or nil.
func (s *State) Worktree() *WorktreeSession {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.worktree
}

// --- helpers ---

func copySet(m map[string]bool) map[string]bool {
	if m == nil {
		return make(map[string]bool)
	}
	cp := make(map[string]bool, len(m))
	for k, v := range m {
		cp[k] = v
	}
	return cp
}

// Ptr returns a pointer to v. Exported for use in tests and tool code.
func Ptr[T any](v T) *T { return &v }
