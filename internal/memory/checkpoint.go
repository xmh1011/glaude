// Package memory — checkpoint.go implements stack-based file snapshots.
//
// Every file write (edit or create) should capture the previous state before
// mutation. Snapshots are grouped into transactions identified by a UUID.
// Undo pops the most recent transaction and restores all affected files.
package memory

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"
)

// Snapshot records the previous state of a single file.
type Snapshot struct {
	Path    string // absolute path
	Content []byte // original content (nil means file did not exist)
	Mode    os.FileMode
}

// Transaction groups snapshots that belong to one logical operation.
type Transaction struct {
	ID        string // unique identifier (typically a tool_use ID)
	Snapshots []Snapshot
}

// Checkpoint manages a stack of file snapshot transactions.
// It is concurrency-safe.
type Checkpoint struct {
	mu      sync.Mutex
	stack   []Transaction
	counter atomic.Int64
}

// NewCheckpoint creates an empty checkpoint stack.
func NewCheckpoint() *Checkpoint {
	return &Checkpoint{}
}

// NextTxID returns a unique transaction ID for grouping snapshots.
func (c *Checkpoint) NextTxID() string {
	n := c.counter.Add(1)
	return fmt.Sprintf("tx-%d", n)
}

// Save captures the current state of a file before mutation.
// txID groups related saves into one transaction. If the file does not exist,
// a "create" snapshot is recorded so that Undo can remove it.
func (c *Checkpoint) Save(txID, path string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	snap := Snapshot{Path: path}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist yet — record nil content so Undo removes it.
			snap.Content = nil
			snap.Mode = 0644
		} else {
			return fmt.Errorf("checkpoint save %s: %w", path, err)
		}
	} else {
		snap.Content = data
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("checkpoint stat %s: %w", path, err)
		}
		snap.Mode = info.Mode()
	}

	// Append to the current transaction or create a new one.
	if len(c.stack) > 0 && c.stack[len(c.stack)-1].ID == txID {
		c.stack[len(c.stack)-1].Snapshots = append(c.stack[len(c.stack)-1].Snapshots, snap)
	} else {
		c.stack = append(c.stack, Transaction{
			ID:        txID,
			Snapshots: []Snapshot{snap},
		})
	}

	return nil
}

// Undo pops the most recent transaction and restores all affected files.
// It returns the transaction ID that was undone, or an error if the stack is empty.
func (c *Checkpoint) Undo() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.stack) == 0 {
		return "", fmt.Errorf("nothing to undo")
	}

	tx := c.stack[len(c.stack)-1]
	c.stack = c.stack[:len(c.stack)-1]

	// Restore in reverse order to handle multiple edits to the same file correctly.
	for i := len(tx.Snapshots) - 1; i >= 0; i-- {
		snap := tx.Snapshots[i]
		if snap.Content == nil {
			// File did not exist before — remove it.
			if err := os.Remove(snap.Path); err != nil && !os.IsNotExist(err) {
				return tx.ID, fmt.Errorf("undo remove %s: %w", snap.Path, err)
			}
		} else {
			if err := os.WriteFile(snap.Path, snap.Content, snap.Mode); err != nil {
				return tx.ID, fmt.Errorf("undo restore %s: %w", snap.Path, err)
			}
		}
	}

	return tx.ID, nil
}

// Len returns the number of transactions in the stack.
func (c *Checkpoint) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.stack)
}

// Peek returns the most recent transaction ID without popping.
// Returns empty string if the stack is empty.
func (c *Checkpoint) Peek() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.stack) == 0 {
		return ""
	}
	return c.stack[len(c.stack)-1].ID
}
