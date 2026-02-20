package daemon

import (
	"context"
	"fmt"
	"log"
	"time"

	pb "github.com/BlakeLiAFK/kele/internal/proto"
	"github.com/BlakeLiAFK/kele/internal/config"
)

// Service implements the KeleService gRPC server.
type Service struct {
	pb.UnimplementedKeleServiceServer
	daemon *Daemon
}

// NewService creates a new gRPC service.
func NewService(d *Daemon) *Service {
	return &Service{daemon: d}
}

// Chat handles streaming chat requests.
func (s *Service) Chat(req *pb.ChatRequest, stream pb.KeleService_ChatServer) error {
	sess := s.daemon.sessions.Get(req.SessionId)
	if sess == nil {
		return fmt.Errorf("session not found: %s", req.SessionId)
	}

	sess.mu.Lock()
	defer sess.mu.Unlock()

	eventChan, err := sess.brain.ChatStream(req.Input)
	if err != nil {
		return fmt.Errorf("chat stream: %w", err)
	}

	for ev := range eventChan {
		if err := stream.Send(&pb.ChatEvent{
			Type:       ev.Type,
			Content:    ev.Content,
			ToolName:   ev.ToolName,
			ToolResult: ev.ToolResult,
			Error:      ev.Error,
		}); err != nil {
			log.Printf("Stream send error: %v", err)
			return err
		}
	}

	return nil
}

// Complete handles AI completion requests.
func (s *Service) Complete(_ context.Context, req *pb.CompleteRequest) (*pb.CompleteResponse, error) {
	sess := s.daemon.sessions.Get(req.SessionId)
	if sess == nil {
		return nil, fmt.Errorf("session not found: %s", req.SessionId)
	}

	suggestion, err := sess.brain.Complete(req.Input)
	if err != nil {
		return &pb.CompleteResponse{Suggestion: ""}, nil
	}

	return &pb.CompleteResponse{Suggestion: suggestion}, nil
}

// RunCommand handles slash command execution.
func (s *Service) RunCommand(_ context.Context, req *pb.RunCommandRequest) (*pb.RunCommandResponse, error) {
	sess := s.daemon.sessions.Get(req.SessionId)
	if sess == nil {
		return nil, fmt.Errorf("session not found: %s", req.SessionId)
	}

	sess.mu.Lock()
	defer sess.mu.Unlock()

	output, quit := sess.brain.RunCommand(req.Command)
	return &pb.RunCommandResponse{
		Output: output,
		Quit:   quit,
	}, nil
}

// CreateSession creates a new daemon-side session.
func (s *Service) CreateSession(_ context.Context, req *pb.CreateSessionRequest) (*pb.SessionInfo, error) {
	sess := s.daemon.sessions.Create(req.Name)
	return &pb.SessionInfo{
		Id:           sess.ID,
		Name:         sess.Name,
		MessageCount: 0,
		HistoryCount: 0,
		Model:        s.daemon.provider.GetModel(),
		Provider:     s.daemon.provider.GetActiveProviderName(),
	}, nil
}

// DeleteSession removes a session.
func (s *Service) DeleteSession(_ context.Context, req *pb.DeleteSessionRequest) (*pb.Empty, error) {
	s.daemon.sessions.Delete(req.SessionId)
	return &pb.Empty{}, nil
}

// ListSessions returns all active sessions.
func (s *Service) ListSessions(_ context.Context, _ *pb.Empty) (*pb.ListSessionsResponse, error) {
	sessions := s.daemon.sessions.List()
	infos := make([]*pb.SessionInfo, len(sessions))
	for i, sess := range sessions {
		infos[i] = &pb.SessionInfo{
			Id:           sess.ID,
			Name:         sess.Name,
			HistoryCount: int32(len(sess.brain.history)),
			Model:        s.daemon.provider.GetModel(),
			Provider:     s.daemon.provider.GetActiveProviderName(),
		}
	}
	return &pb.ListSessionsResponse{Sessions: infos}, nil
}

// GetStatus returns daemon status information.
func (s *Service) GetStatus(_ context.Context, _ *pb.Empty) (*pb.StatusResponse, error) {
	uptime := int32(time.Since(s.daemon.startTime).Seconds())
	heartbeatActive := s.daemon.heartbeat != nil && s.daemon.heartbeat.IsActive()

	return &pb.StatusResponse{
		Version:         config.Version,
		Provider:        s.daemon.provider.GetActiveProviderName(),
		Providers:       s.daemon.provider.ListProviders(),
		Model:           s.daemon.provider.GetModel(),
		SmallModel:      s.daemon.provider.GetSmallModel(),
		ActiveSessions:  int32(s.daemon.sessions.Count()),
		UptimeSeconds:   uptime,
		HeartbeatActive: heartbeatActive,
	}, nil
}

// GetHeartbeatStatus returns heartbeat system status.
func (s *Service) GetHeartbeatStatus(_ context.Context, _ *pb.Empty) (*pb.HeartbeatStatusResponse, error) {
	hb := s.daemon.heartbeat
	if hb == nil {
		return &pb.HeartbeatStatusResponse{Active: false}, nil
	}

	lastRun := ""
	if !hb.LastRun().IsZero() {
		lastRun = hb.LastRun().Format("2006-01-02 15:04:05")
	}

	return &pb.HeartbeatStatusResponse{
		Active:          hb.IsActive(),
		IntervalMinutes: int32(hb.IntervalMinutes()),
		LastRun:         lastRun,
		LastDecision:    hb.LastDecision(),
		TotalHeartbeats: int32(hb.TotalHeartbeats()),
		ActionsTaken:    int32(hb.TotalActions()),
	}, nil
}
