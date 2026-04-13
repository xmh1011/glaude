package state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTodos(t *testing.T) {
	s := New()

	t.Run("empty by default", func(t *testing.T) {
		assert.Empty(t, s.Todos())
	})

	t.Run("set and get", func(t *testing.T) {
		s.SetTodos([]TodoItem{
			{Content: "task 1", Status: TodoPending},
			{Content: "task 2", Status: TodoInProgress},
		})
		todos := s.Todos()
		require.Len(t, todos, 2)
		assert.Equal(t, "task 1", todos[0].Content)
		assert.Equal(t, TodoInProgress, todos[1].Status)
	})

	t.Run("all completed clears list", func(t *testing.T) {
		s.SetTodos([]TodoItem{
			{Content: "done", Status: TodoCompleted},
		})
		assert.Empty(t, s.Todos())
	})

	t.Run("empty status defaults to pending", func(t *testing.T) {
		s.SetTodos([]TodoItem{{Content: "no status"}})
		todos := s.Todos()
		require.Len(t, todos, 1)
		assert.Equal(t, TodoPending, todos[0].Status)
	})
}

func TestTasks(t *testing.T) {
	s := New()

	t.Run("create and get", func(t *testing.T) {
		id := s.CreateTask("Fix bug", "Fix the login bug", "Fixing bug", nil)
		assert.Equal(t, "1", id)

		task := s.GetTask(id)
		require.NotNil(t, task)
		assert.Equal(t, "Fix bug", task.Subject)
		assert.Equal(t, TaskPending, task.Status)
	})

	t.Run("get nonexistent returns nil", func(t *testing.T) {
		assert.Nil(t, s.GetTask("999"))
	})

	t.Run("update task", func(t *testing.T) {
		id := s.CreateTask("Test", "Test desc", "", nil)
		err := s.UpdateTask(id, UpdateOpts{
			Subject: Ptr("Updated"),
			Status:  Ptr(TaskInProgress),
			Owner:   Ptr("agent-1"),
		})
		require.NoError(t, err)

		task := s.GetTask(id)
		assert.Equal(t, "Updated", task.Subject)
		assert.Equal(t, TaskInProgress, task.Status)
		assert.Equal(t, "agent-1", task.Owner)
	})

	t.Run("update nonexistent returns error", func(t *testing.T) {
		err := s.UpdateTask("999", UpdateOpts{Subject: Ptr("X")})
		assert.Error(t, err)
	})

	t.Run("delete removes from list", func(t *testing.T) {
		id := s.CreateTask("To delete", "Will be deleted", "", nil)
		err := s.DeleteTask(id)
		require.NoError(t, err)
		// GetTask still returns the task (with deleted status), but AllTasks skips it.
		task := s.GetTask(id)
		require.NotNil(t, task)
		assert.Equal(t, TaskDeleted, task.Status)
		// AllTasks should not include deleted.
		for _, task := range s.AllTasks() {
			assert.NotEqual(t, id, task.ID)
		}
	})

	t.Run("dependencies", func(t *testing.T) {
		s2 := New()
		id1 := s2.CreateTask("Task 1", "First", "", nil)
		id2 := s2.CreateTask("Task 2", "Second", "", nil)

		err := s2.UpdateTask(id2, UpdateOpts{AddBlockedBy: []string{id1}})
		require.NoError(t, err)

		t1 := s2.GetTask(id1)
		t2 := s2.GetTask(id2)
		assert.True(t, t1.Blocks[id2])
		assert.True(t, t2.BlockedBy[id1])
	})

	t.Run("metadata merge", func(t *testing.T) {
		id := s.CreateTask("Meta", "Meta test", "", map[string]any{"key1": "val1"})
		err := s.UpdateTask(id, UpdateOpts{
			Metadata: map[string]any{"key2": "val2", "key1": nil},
		})
		require.NoError(t, err)

		task := s.GetTask(id)
		assert.Nil(t, task.Metadata["key1"])
		assert.Equal(t, "val2", task.Metadata["key2"])
	})

	t.Run("all tasks ordered", func(t *testing.T) {
		s3 := New()
		s3.CreateTask("A", "", "", nil)
		s3.CreateTask("B", "", "", nil)
		s3.CreateTask("C", "", "", nil)

		tasks := s3.AllTasks()
		require.Len(t, tasks, 3)
		assert.Equal(t, "A", tasks[0].Subject)
		assert.Equal(t, "B", tasks[1].Subject)
		assert.Equal(t, "C", tasks[2].Subject)
	})
}

func TestPlanMode(t *testing.T) {
	s := New()
	assert.False(t, s.InPlanMode())

	s.SetPlanMode(true, "default")
	assert.True(t, s.InPlanMode())
	assert.Equal(t, "default", s.PrePlanMode())

	s.SetPlanMode(false, "")
	assert.False(t, s.InPlanMode())
}

func TestWorktree(t *testing.T) {
	s := New()
	assert.Nil(t, s.Worktree())

	ws := &WorktreeSession{Path: "/tmp/wt", Branch: "glaude-test", OriginalWD: "/home"}
	s.SetWorktree(ws)
	assert.Equal(t, "/tmp/wt", s.Worktree().Path)

	s.SetWorktree(nil)
	assert.Nil(t, s.Worktree())
}
