package daemon

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/BlakeLiAFK/kele/internal/config"
	pb "github.com/BlakeLiAFK/kele/internal/proto"
	"github.com/BlakeLiAFK/kele/internal/taskboard"
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

// ============================================================
// TaskBoard RPC Handlers
// ============================================================

func (s *Service) boardOrErr() (*taskboard.Board, error) {
	if s.daemon.board == nil {
		return nil, fmt.Errorf("taskboard not initialized")
	}
	return s.daemon.board, nil
}

// --- Workspace ---

func (s *Service) CreateWorkspace(_ context.Context, req *pb.CreateWorkspaceRequest) (*pb.WorkspaceInfo, error) {
	board, err := s.boardOrErr()
	if err != nil {
		return nil, err
	}
	ws := &taskboard.Workspace{
		Name:          req.Name,
		Description:   req.Description,
		Goal:          req.Goal,
		MaxConcurrent: int(req.MaxConcurrent),
		Context:       req.Context,
		WorkDir:       req.WorkDir,
	}
	if err := board.CreateWorkspace(ws); err != nil {
		return nil, err
	}
	return wsToProto(ws, board)
}

func (s *Service) GetWorkspace(_ context.Context, req *pb.GetWorkspaceRequest) (*pb.WorkspaceInfo, error) {
	board, err := s.boardOrErr()
	if err != nil {
		return nil, err
	}
	ws, err := board.GetWorkspace(req.Id)
	if err != nil {
		return nil, err
	}
	return wsToProto(ws, board)
}

func (s *Service) UpdateWorkspace(_ context.Context, req *pb.UpdateWorkspaceRequest) (*pb.WorkspaceInfo, error) {
	board, err := s.boardOrErr()
	if err != nil {
		return nil, err
	}
	ws, err := board.GetWorkspace(req.Id)
	if err != nil {
		return nil, err
	}
	if req.Name != "" {
		ws.Name = req.Name
	}
	if req.Description != "" {
		ws.Description = req.Description
	}
	if req.MaxConcurrent > 0 {
		ws.MaxConcurrent = int(req.MaxConcurrent)
	}
	if req.Context != "" {
		ws.Context = req.Context
	}
	if req.Status != "" {
		ws.Status = taskboard.WorkspaceStatus(req.Status)
	}
	if err := board.UpdateWorkspace(ws); err != nil {
		return nil, err
	}
	return wsToProto(ws, board)
}

func (s *Service) DeleteWorkspace(_ context.Context, req *pb.DeleteWorkspaceRequest) (*pb.Empty, error) {
	board, err := s.boardOrErr()
	if err != nil {
		return nil, err
	}
	return &pb.Empty{}, board.DeleteWorkspace(req.Id)
}

func (s *Service) ListWorkspaces(_ context.Context, _ *pb.Empty) (*pb.ListWorkspacesResponse, error) {
	board, err := s.boardOrErr()
	if err != nil {
		return nil, err
	}
	wsList, err := board.ListWorkspaces()
	if err != nil {
		return nil, err
	}
	resp := &pb.ListWorkspacesResponse{}
	for _, ws := range wsList {
		info, err := wsToProto(ws, board)
		if err != nil {
			continue
		}
		resp.Workspaces = append(resp.Workspaces, info)
	}
	return resp, nil
}

// --- Task ---

func (s *Service) CreateTask(_ context.Context, req *pb.CreateTaskRequest) (*pb.TaskInfo, error) {
	board, err := s.boardOrErr()
	if err != nil {
		return nil, err
	}
	status := taskboard.StatusBacklog
	if req.AutoReady {
		status = taskboard.StatusReady
	}
	t := &taskboard.Task{
		WorkspaceID: req.WorkspaceId,
		Title:       req.Title,
		Description: req.Description,
		Prompt:      req.Prompt,
		Priority:    int(req.Priority),
		DependsOn:   req.DependsOn,
		MaxRetries:  int(req.MaxRetries),
		Tags:        req.Tags,
		Status:      status,
	}
	if err := board.CreateTask(t); err != nil {
		return nil, err
	}
	return taskToProto(t), nil
}

func (s *Service) GetTask(_ context.Context, req *pb.GetTaskRequest) (*pb.TaskInfo, error) {
	board, err := s.boardOrErr()
	if err != nil {
		return nil, err
	}
	t, err := board.GetTask(req.Id)
	if err != nil {
		return nil, err
	}
	return taskToProto(t), nil
}

func (s *Service) UpdateTaskRPC(_ context.Context, req *pb.UpdateTaskRequest) (*pb.TaskInfo, error) {
	board, err := s.boardOrErr()
	if err != nil {
		return nil, err
	}
	t, err := board.GetTask(req.Id)
	if err != nil {
		return nil, err
	}
	if req.Title != "" {
		t.Title = req.Title
	}
	if req.Description != "" {
		t.Description = req.Description
	}
	if req.Prompt != "" {
		t.Prompt = req.Prompt
	}
	if req.Priority > 0 {
		t.Priority = int(req.Priority)
	}
	if len(req.Tags) > 0 {
		t.Tags = req.Tags
	}
	if err := board.UpdateTask(t); err != nil {
		return nil, err
	}
	return taskToProto(t), nil
}

func (s *Service) DeleteTask(_ context.Context, req *pb.DeleteTaskRequest) (*pb.Empty, error) {
	board, err := s.boardOrErr()
	if err != nil {
		return nil, err
	}
	return &pb.Empty{}, board.DeleteTask(req.Id)
}

func (s *Service) ListTasks(_ context.Context, req *pb.ListTasksRequest) (*pb.ListTasksResponse, error) {
	board, err := s.boardOrErr()
	if err != nil {
		return nil, err
	}
	tasks, err := board.ListTasks(req.WorkspaceId, req.StatusFilter)
	if err != nil {
		return nil, err
	}
	resp := &pb.ListTasksResponse{}
	for _, t := range tasks {
		resp.Tasks = append(resp.Tasks, taskToProto(t))
	}
	return resp, nil
}

// --- Task Execution ---

func (s *Service) StartTask(_ context.Context, req *pb.StartTaskRequest) (*pb.TaskInfo, error) {
	board, err := s.boardOrErr()
	if err != nil {
		return nil, err
	}
	t, err := board.StartTask(req.Id)
	if err != nil {
		return nil, err
	}
	return taskToProto(t), nil
}

func (s *Service) CancelTask(_ context.Context, req *pb.CancelTaskRequest) (*pb.TaskInfo, error) {
	board, err := s.boardOrErr()
	if err != nil {
		return nil, err
	}
	t, err := board.CancelTask(req.Id)
	if err != nil {
		return nil, err
	}
	return taskToProto(t), nil
}

func (s *Service) RetryTask(_ context.Context, req *pb.RetryTaskRequest) (*pb.TaskInfo, error) {
	board, err := s.boardOrErr()
	if err != nil {
		return nil, err
	}
	t, err := board.RetryTask(req.Id)
	if err != nil {
		return nil, err
	}
	return taskToProto(t), nil
}

// --- Planner ---

func (s *Service) PlanWorkspace(req *pb.PlanWorkspaceRequest, stream pb.KeleService_PlanWorkspaceServer) error {
	if s.daemon.planner == nil {
		return fmt.Errorf("planner not initialized")
	}
	events, err := s.daemon.planner.Plan(req.Goal)
	if err != nil {
		return err
	}
	for ev := range events {
		if err := stream.Send(&pb.PlanEventMsg{
			Type:     ev.Type,
			Content:  ev.Content,
			PlanJson: ev.PlanJSON,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) ApprovePlan(_ context.Context, req *pb.ApprovePlanRequest) (*pb.ApprovePlanResponse, error) {
	board, err := s.boardOrErr()
	if err != nil {
		return nil, err
	}
	if s.daemon.planner == nil {
		return nil, fmt.Errorf("planner not initialized")
	}
	plan, err := taskboard.ParsePlan(req.PlanJson)
	if err != nil {
		return nil, err
	}
	ws, tasks, err := s.daemon.planner.ApproveAndCreate(board, plan, req.Goal, req.WorkDir, req.AutoStart)
	if err != nil {
		return nil, err
	}
	wsInfo, err := wsToProto(ws, board)
	if err != nil {
		return nil, err
	}
	resp := &pb.ApprovePlanResponse{Workspace: wsInfo}
	for _, t := range tasks {
		resp.Tasks = append(resp.Tasks, taskToProto(t))
	}
	return resp, nil
}

// --- Board Overview ---

func (s *Service) GetBoardOverview(_ context.Context, _ *pb.Empty) (*pb.BoardOverviewMsg, error) {
	board, err := s.boardOrErr()
	if err != nil {
		return nil, err
	}
	overview, err := board.GetOverview()
	if err != nil {
		return nil, err
	}
	resp := &pb.BoardOverviewMsg{
		TotalTasks:     int32(overview.TotalTasks),
		RunningTasks:   int32(overview.RunningTasks),
		PendingTasks:   int32(overview.PendingTasks),
		CompletedTasks: int32(overview.CompletedTasks),
	}
	for _, wo := range overview.Workspaces {
		resp.Workspaces = append(resp.Workspaces, &pb.WorkspaceOverviewMsg{
			Id:            wo.ID,
			Name:          wo.Name,
			Status:        string(wo.Status),
			Backlog:       int32(wo.Backlog),
			Ready:         int32(wo.Ready),
			Running:       int32(wo.Running),
			Done:          int32(wo.Done),
			Failed:        int32(wo.Failed),
			MaxConcurrent: int32(wo.MaxConcurrent),
		})
	}
	return resp, nil
}

func (s *Service) WatchBoard(req *pb.WatchBoardRequest, stream pb.KeleService_WatchBoardServer) error {
	board, err := s.boardOrErr()
	if err != nil {
		return err
	}
	subID, eventCh := board.Subscribe()
	defer board.Unsubscribe(subID)

	for {
		select {
		case ev, ok := <-eventCh:
			if !ok {
				return nil
			}
			// Filter by workspace if specified
			if req.WorkspaceId != "" && ev.WorkspaceID != req.WorkspaceId {
				continue
			}
			if err := stream.Send(&pb.BoardEventMsg{
				Type:        ev.Type,
				WorkspaceId: ev.WorkspaceID,
				TaskId:      ev.TaskID,
				Detail:      ev.Detail,
				Timestamp:   ev.Timestamp.Format("2006-01-02 15:04:05"),
			}); err != nil {
				return err
			}
		case <-stream.Context().Done():
			return nil
		}
	}
}

// --- Task Log ---

func (s *Service) GetTaskLog(_ context.Context, req *pb.GetTaskLogRequest) (*pb.TaskLogResponse, error) {
	board, err := s.boardOrErr()
	if err != nil {
		return nil, err
	}
	logs, err := board.Store().GetTaskLog(req.TaskId, int(req.Limit))
	if err != nil {
		return nil, err
	}
	resp := &pb.TaskLogResponse{}
	for _, l := range logs {
		resp.Entries = append(resp.Entries, &pb.TaskLogEntry{
			EventType: l.EventType,
			Content:   l.Content,
			ToolName:  l.ToolName,
			Timestamp: l.Timestamp.Format("2006-01-02 15:04:05"),
		})
	}
	return resp, nil
}

// --- Proto conversion helpers ---

func wsToProto(ws *taskboard.Workspace, board *taskboard.Board) (*pb.WorkspaceInfo, error) {
	counts, _ := board.Store().CountByStatus(ws.ID)
	taskCount := 0
	runningCount := 0
	if counts != nil {
		taskCount = counts.Total()
		runningCount = counts.Running
	}
	return &pb.WorkspaceInfo{
		Id:            ws.ID,
		Name:          ws.Name,
		Description:   ws.Description,
		Goal:          ws.Goal,
		Status:        string(ws.Status),
		MaxConcurrent: int32(ws.MaxConcurrent),
		Context:       ws.Context,
		WorkDir:       ws.WorkDir,
		Summary:       ws.Summary,
		TaskCount:     int32(taskCount),
		RunningCount:  int32(runningCount),
		CreatedAt:     ws.CreatedAt.Format("2006-01-02 15:04:05"),
	}, nil
}

func taskToProto(t *taskboard.Task) *pb.TaskInfo {
	startedAt := ""
	if !t.StartedAt.IsZero() {
		startedAt = t.StartedAt.Format("2006-01-02 15:04:05")
	}
	completedAt := ""
	if !t.CompletedAt.IsZero() {
		completedAt = t.CompletedAt.Format("2006-01-02 15:04:05")
	}
	return &pb.TaskInfo{
		Id:              t.ID,
		WorkspaceId:     t.WorkspaceID,
		Title:           t.Title,
		Description:     t.Description,
		Prompt:          t.Prompt,
		Status:          string(t.Status),
		Priority:        int32(t.Priority),
		DependsOn:       t.DependsOn,
		AssignedSession: t.AssignedSession,
		Result:          t.Result,
		Error:           t.Error,
		RetryCount:      int32(t.RetryCount),
		MaxRetries:      int32(t.MaxRetries),
		Tags:            t.Tags,
		CreatedAt:       t.CreatedAt.Format("2006-01-02 15:04:05"),
		StartedAt:       startedAt,
		CompletedAt:     completedAt,
	}
}
