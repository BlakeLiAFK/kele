package daemon

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"google.golang.org/grpc"

	"github.com/BlakeLiAFK/kele/internal/config"
	"github.com/BlakeLiAFK/kele/internal/cron"
	"github.com/BlakeLiAFK/kele/internal/heartbeat"
	"github.com/BlakeLiAFK/kele/internal/llm"
	"github.com/BlakeLiAFK/kele/internal/memory"
	pb "github.com/BlakeLiAFK/kele/internal/proto"
	"github.com/BlakeLiAFK/kele/internal/tools"
)

// Daemon is the background service process that owns all shared resources.
type Daemon struct {
	cfg       *config.Config
	provider  *llm.ProviderManager
	executor  *tools.Executor
	store     *memory.Store
	scheduler *cron.Scheduler
	sessions  *SessionManager
	heartbeat *heartbeat.Runner
	server    *grpc.Server
	startTime time.Time
	socketPath string
	pidPath    string
	logPath    string
}

// New creates a new daemon instance.
func New(cfg *config.Config) *Daemon {
	homeDir, _ := os.UserHomeDir()
	keleDir := filepath.Join(homeDir, ".kele")
	os.MkdirAll(keleDir, 0755)

	return &Daemon{
		cfg:        cfg,
		socketPath: filepath.Join(keleDir, "kele.sock"),
		pidPath:    filepath.Join(keleDir, "kele.pid"),
		logPath:    filepath.Join(keleDir, "daemon.log"),
	}
}

// SocketPath returns the Unix socket path.
func SocketPath() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".kele", "kele.sock")
}

// PIDPath returns the PID file path.
func PIDPath() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".kele", "kele.pid")
}

// Run starts the daemon in foreground mode (blocking).
func (d *Daemon) Run() error {
	// Setup logging
	logFile, err := os.OpenFile(d.logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err == nil {
		log.SetOutput(logFile)
		defer logFile.Close()
	}

	log.Println("=== Kele daemon starting ===")
	d.startTime = time.Now()

	// Initialize shared resources
	if err := d.initResources(); err != nil {
		return fmt.Errorf("init resources: %w", err)
	}
	defer d.cleanup()

	// Write PID file
	if err := os.WriteFile(d.pidPath, []byte(strconv.Itoa(os.Getpid())), 0644); err != nil {
		return fmt.Errorf("write pid: %w", err)
	}

	// Remove stale socket
	os.Remove(d.socketPath)

	// Listen on Unix socket
	lis, err := net.Listen("unix", d.socketPath)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	// Ensure socket is accessible
	os.Chmod(d.socketPath, 0660)

	// Create gRPC server
	d.server = grpc.NewServer()
	svc := NewService(d)
	pb.RegisterKeleServiceServer(d.server, svc)

	// Signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		sig := <-sigCh
		log.Printf("Received %s, shutting down gracefully...", sig)
		d.server.GracefulStop()
	}()

	log.Printf("Daemon ready on unix://%s (PID: %d)", d.socketPath, os.Getpid())
	fmt.Printf("Daemon started (PID: %d)\n", os.Getpid())

	// Serve (blocks until stopped)
	if err := d.server.Serve(lis); err != nil {
		return fmt.Errorf("serve: %w", err)
	}

	log.Println("=== Kele daemon stopped ===")
	return nil
}

// initResources creates all shared resources.
func (d *Daemon) initResources() error {
	// Memory store
	store, err := memory.NewStore(d.cfg)
	if err != nil {
		log.Printf("Warning: memory store init failed: %v", err)
	}
	d.store = store

	// LLM provider
	d.provider = llm.NewProviderManager(d.cfg)

	// Cron scheduler
	wd, _ := os.Getwd()
	d.scheduler = cron.NewScheduler(d.cfg.Memory.DBPath, wd)
	d.scheduler.Start()

	// Tool executor
	d.executor = tools.NewExecutor(d.scheduler, d.cfg)

	// Session manager
	d.sessions = NewSessionManager(d.provider, d.executor, d.store, d.cfg)

	// Create default session
	d.sessions.Create("default")

	// Heartbeat runner
	d.heartbeat = heartbeat.NewRunner(d.provider, d.executor, d.sessions.Count)
	d.heartbeat.Start()

	log.Println("All resources initialized")
	return nil
}

// cleanup releases all resources.
func (d *Daemon) cleanup() {
	if d.heartbeat != nil {
		d.heartbeat.Stop()
	}
	if d.scheduler != nil {
		d.scheduler.Stop()
	}
	os.Remove(d.socketPath)
	os.Remove(d.pidPath)
	log.Println("Resources cleaned up")
}

// IsRunning checks if a daemon is already running.
func IsRunning() (int, bool) {
	pidPath := PIDPath()
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(string(data))
	if err != nil {
		return 0, false
	}
	// Check if process exists
	process, err := os.FindProcess(pid)
	if err != nil {
		return 0, false
	}
	// Signal 0 checks if process exists without actually sending a signal
	err = process.Signal(syscall.Signal(0))
	return pid, err == nil
}

// Stop sends SIGTERM to the running daemon.
func Stop() error {
	pid, running := IsRunning()
	if !running {
		return fmt.Errorf("daemon is not running")
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process: %w", err)
	}
	return process.Signal(syscall.SIGTERM)
}
