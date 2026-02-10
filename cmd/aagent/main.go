package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gratheon/aagent/internal/agent"
	"github.com/gratheon/aagent/internal/config"
	"github.com/gratheon/aagent/internal/llm/kimi"
	"github.com/gratheon/aagent/internal/session"
	"github.com/gratheon/aagent/internal/storage"
	"github.com/gratheon/aagent/internal/tools"
	"github.com/gratheon/aagent/internal/tui"
	"github.com/spf13/cobra"
)

var (
	modelFlag    string
	agentFlag    string
	continueFlag string
	verboseFlag  bool
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "aagent [task]",
		Short: "A2gent - Autonomous AI coding agent with TUI",
		Long: `A2gent is a Go-based autonomous AI coding agent that executes tasks in sessions.
It features a TUI interface with scrollable history, multi-line input, and real-time status.`,
		Args: cobra.ArbitraryArgs,
		RunE: runAgent,
	}

	rootCmd.Flags().StringVarP(&modelFlag, "model", "m", "", "Override default model")
	rootCmd.Flags().StringVarP(&agentFlag, "agent", "a", "build", "Select agent type (build, plan)")
	rootCmd.Flags().StringVarP(&continueFlag, "continue", "c", "", "Resume previous session by ID")
	rootCmd.Flags().BoolVarP(&verboseFlag, "verbose", "v", false, "Verbose output")

	// Session management subcommand
	sessionCmd := &cobra.Command{
		Use:   "session",
		Short: "Manage sessions",
	}

	sessionListCmd := &cobra.Command{
		Use:   "list",
		Short: "List all sessions",
		RunE:  listSessions,
	}

	sessionCmd.AddCommand(sessionListCmd)
	rootCmd.AddCommand(sessionCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runAgent(cmd *cobra.Command, args []string) error {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Override model if specified
	if modelFlag != "" {
		cfg.DefaultModel = modelFlag
	}

	// Get API key
	apiKey := os.Getenv("KIMI_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("KIMI_API_KEY environment variable is required")
	}

	// Initialize storage
	store, err := storage.NewSQLiteStore(cfg.DataPath)
	if err != nil {
		return fmt.Errorf("failed to initialize storage: %w", err)
	}
	defer store.Close()

	// Initialize LLM client
	llmClient := kimi.NewClient(apiKey, cfg.DefaultModel)

	// Initialize tool manager
	toolManager := tools.NewManager(cfg.WorkDir)

	// Initialize session manager
	sessionManager := session.NewManager(store)

	// Create or resume session
	var sess *session.Session
	if continueFlag != "" {
		sess, err = sessionManager.Get(continueFlag)
		if err != nil {
			return fmt.Errorf("failed to resume session: %w", err)
		}
		fmt.Printf("Resuming session %s\n", sess.ID)
	} else {
		sess, err = sessionManager.Create(agentFlag)
		if err != nil {
			return fmt.Errorf("failed to create session: %w", err)
		}
	}

	// Get initial task from args if provided
	var initialTask string
	if len(args) > 0 {
		initialTask = args[0]
		// Add the initial task to the session
		sess.AddUserMessage(initialTask)
	}

	// Create agent config
	agentConfig := agent.Config{
		Name:        agentFlag,
		Model:       cfg.DefaultModel,
		MaxSteps:    cfg.MaxSteps,
		Temperature: cfg.Temperature,
	}

	// Create TUI model
	model := tui.New(
		sess,
		sessionManager,
		agentConfig,
		llmClient,
		toolManager,
		initialTask,
	)

	// Run the TUI
	p := tea.NewProgram(
		model,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	return nil
}

func listSessions(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	store, err := storage.NewSQLiteStore(cfg.DataPath)
	if err != nil {
		return fmt.Errorf("failed to initialize storage: %w", err)
	}
	defer store.Close()

	sessions, err := store.ListSessions()
	if err != nil {
		return fmt.Errorf("failed to list sessions: %w", err)
	}

	if len(sessions) == 0 {
		fmt.Println("No sessions found")
		return nil
	}

	fmt.Printf("%-36s  %-10s  %-20s  %s\n", "ID", "Agent", "Created", "Status")
	fmt.Println("-------------------------------------------------------------------------------------")
	for _, s := range sessions {
		fmt.Printf("%-36s  %-10s  %-20s  %s\n", s.ID, s.AgentID, s.CreatedAt.Format("2006-01-02 15:04:05"), s.Status)
	}

	return nil
}
