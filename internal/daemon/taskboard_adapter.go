package daemon

import (
	"github.com/BlakeLiAFK/kele/internal/taskboard"
)

// TaskSessionAdapter implements taskboard.TaskSessionManager using the daemon's SessionManager.
type TaskSessionAdapter struct {
	sm *SessionManager
}

// NewTaskSessionAdapter wraps a SessionManager to implement taskboard.TaskSessionManager.
func NewTaskSessionAdapter(sm *SessionManager) *TaskSessionAdapter {
	return &TaskSessionAdapter{sm: sm}
}

// CreateTaskSession creates a session and returns a taskboard.TaskSession wrapper.
func (a *TaskSessionAdapter) CreateTaskSession(name string) taskboard.TaskSession {
	sess := a.sm.Create(name)
	return &sessionWrapper{sess: sess}
}

// DeleteTaskSession deletes a session by ID.
func (a *TaskSessionAdapter) DeleteTaskSession(id string) {
	a.sm.Delete(id)
}

// sessionWrapper wraps daemon.Session to implement taskboard.TaskSession.
type sessionWrapper struct {
	sess *Session
}

func (w *sessionWrapper) GetID() string {
	return w.sess.ID
}

func (w *sessionWrapper) InjectContext(ctx string) {
	w.sess.InjectContext(ctx)
}

func (w *sessionWrapper) ChatStream(input string) (<-chan taskboard.SessionEvent, error) {
	events, err := w.sess.ChatStreamForTask(input)
	if err != nil {
		return nil, err
	}
	outCh := make(chan taskboard.SessionEvent, 100)
	go func() {
		defer close(outCh)
		for ev := range events {
			outCh <- taskboard.SessionEvent{
				Type:       ev.Type,
				Content:    ev.Content,
				ToolName:   ev.ToolName,
				ToolResult: ev.ToolResult,
				Error:      ev.Error,
			}
		}
	}()
	return outCh, nil
}
