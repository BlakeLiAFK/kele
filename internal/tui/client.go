package tui

import (
	"context"
	"io"

	pb "github.com/BlakeLiAFK/kele/internal/proto"
)

// DaemonClient wraps the gRPC client for TUI use.
type DaemonClient struct {
	client pb.KeleServiceClient
}

// NewDaemonClient creates a new daemon client wrapper.
func NewDaemonClient(client pb.KeleServiceClient) *DaemonClient {
	return &DaemonClient{client: client}
}

// ChatStream starts a streaming chat and returns a channel of streamEvents.
func (dc *DaemonClient) ChatStream(sessionID, input string) (<-chan streamEvent, error) {
	stream, err := dc.client.Chat(context.Background(), &pb.ChatRequest{
		SessionId: sessionID,
		Input:     input,
	})
	if err != nil {
		return nil, err
	}

	eventChan := make(chan streamEvent, 100)
	go func() {
		defer close(eventChan)
		for {
			ev, err := stream.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				eventChan <- streamEvent{Type: "error", Error: err.Error()}
				return
			}
			eventChan <- streamEvent{
				Type:       ev.Type,
				Content:    ev.Content,
				ToolName:   ev.ToolName,
				ToolResult: ev.ToolResult,
				Error:      ev.Error,
			}
		}
	}()

	return eventChan, nil
}

// Complete performs AI completion via daemon.
func (dc *DaemonClient) Complete(sessionID, input string) (string, error) {
	resp, err := dc.client.Complete(context.Background(), &pb.CompleteRequest{
		SessionId: sessionID,
		Input:     input,
	})
	if err != nil {
		return "", err
	}
	return resp.Suggestion, nil
}

// RunCommand executes a slash command via daemon.
func (dc *DaemonClient) RunCommand(sessionID, command string) (string, bool, error) {
	resp, err := dc.client.RunCommand(context.Background(), &pb.RunCommandRequest{
		SessionId: sessionID,
		Command:   command,
	})
	if err != nil {
		return "", false, err
	}
	return resp.Output, resp.Quit, nil
}

// CreateSession creates a new session on the daemon.
func (dc *DaemonClient) CreateSession(name string) (*pb.SessionInfo, error) {
	return dc.client.CreateSession(context.Background(), &pb.CreateSessionRequest{Name: name})
}

// DeleteSession removes a session from the daemon.
func (dc *DaemonClient) DeleteSession(sessionID string) error {
	_, err := dc.client.DeleteSession(context.Background(), &pb.DeleteSessionRequest{SessionId: sessionID})
	return err
}

// ListSessions returns all active daemon sessions.
func (dc *DaemonClient) ListSessions() ([]*pb.SessionInfo, error) {
	resp, err := dc.client.ListSessions(context.Background(), &pb.Empty{})
	if err != nil {
		return nil, err
	}
	return resp.Sessions, nil
}

// GetStatus returns daemon status.
func (dc *DaemonClient) GetStatus() (*pb.StatusResponse, error) {
	return dc.client.GetStatus(context.Background(), &pb.Empty{})
}
