package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	// Load and validate configuration
	cfg, err := LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Initialize connection pools
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pm, err := NewPoolManager(ctx, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer pm.Close()

	// Create MCP server
	server := mcp.NewServer(
		&mcp.Implementation{Name: "mcp-postgresql", Version: version},
		nil,
	)

	// Register tools using closures to inject pm and cfg
	mcp.AddTool(server, &mcp.Tool{
		Name:        "pg_list_target",
		Description: "List all configured PostgreSQL targets and their descriptions.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input targetInput) (*mcp.CallToolResult, targetOutput, error) {
		return pgListTarget(ctx, req, input, pm, cfg)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "pg_execute_sql",
		Description: "Execute a single SQL statement against a PostgreSQL target. Returns JSON results.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input executeSQLInput) (*mcp.CallToolResult, executeSQLOutput, error) {
		return pgExecuteSQL(ctx, req, input, pm, cfg)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "pg_execute_sql_file",
		Description: "Execute a SQL file (multiple statements) in a transaction against a PostgreSQL target. Supports dry_run mode.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input executeSQLFileInput) (*mcp.CallToolResult, executeSQLFileOutput, error) {
		return pgExecuteSQLFile(ctx, req, input, pm, cfg)
	})

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutting down...")
		cancel()
	}()

	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil && ctx.Err() == nil {
		log.Fatalf("Server error: %v", err)
	}

	log.Println("MCP PostgreSQL server stopped")
}
