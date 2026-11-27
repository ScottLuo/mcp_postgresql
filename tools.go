package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// targetInput is the input for pg_list_target.
type targetInput struct{}

// targetOutput is the output for pg_list_target.
type targetOutput struct {
	Targets []TargetInfo `json:"targets"`
}

// TargetInfo describes a single target.
type TargetInfo struct {
	Name        string `json:"name" jsonschema:"The target name"`
	Description string `json:"description" jsonschema:"The target description"`
}

// executeSQLInput is the input for pg_execute_sql.
type executeSQLInput struct {
	TargetName string `json:"target_name" jsonschema:"The name of the PostgreSQL target to use"`
	SQL        string `json:"sql" jsonschema:"The SQL statement to execute"`
	LimitRows  int    `json:"limit_rows,omitempty" jsonschema:"Maximum number of rows to return (default 1000)"`
}

// executeSQLOutput is the output for pg_execute_sql.
type executeSQLOutput struct {
	Result QueryResult `json:"result" jsonschema:"The query result"`
}

// executeSQLFileInput is the input for pg_execute_sql_file.
type executeSQLFileInput struct {
	TargetName string `json:"target_name" jsonschema:"The name of the PostgreSQL target to use"`
	FileName   string `json:"file_name" jsonschema:"The path to the SQL file to execute"`
	DryRun     bool   `json:"dry_run,omitempty" jsonschema:"If true, execute in a transaction and roll back"`
}

// executeSQLFileOutput is the output for pg_execute_sql_file.
type executeSQLFileOutput struct {
	Result QueryResult `json:"result" jsonschema:"The execution result"`
}

// pgListTarget lists all configured PostgreSQL targets.
func pgListTarget(ctx context.Context, req *mcp.CallToolRequest, input targetInput, pm *PoolManager, cfg *Config) (*mcp.CallToolResult, targetOutput, error) {
	var targets []TargetInfo
	for name, tc := range cfg.Targets {
		targets = append(targets, TargetInfo{Name: name, Description: tc.Description})
	}
	return nil, targetOutput{Targets: targets}, nil
}

// pgExecuteSQL executes a single SQL statement.
func pgExecuteSQL(ctx context.Context, req *mcp.CallToolRequest, input executeSQLInput, pm *PoolManager, cfg *Config) (*mcp.CallToolResult, executeSQLOutput, error) {
	target, err := cfg.GetTarget(input.TargetName)
	if err != nil {
		return nil, executeSQLOutput{}, err
	}

	result, err := pm.ExecSQL(ctx, target.Name, input.SQL, input.LimitRows)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: formatError(err)},
			},
			IsError: true,
		}, executeSQLOutput{}, nil
	}

	return nil, executeSQLOutput{Result: *result}, nil
}

// pgExecuteSQLFile executes a SQL file in a transaction.
func pgExecuteSQLFile(ctx context.Context, req *mcp.CallToolRequest, input executeSQLFileInput, pm *PoolManager, cfg *Config) (*mcp.CallToolResult, executeSQLFileOutput, error) {
	target, err := cfg.GetTarget(input.TargetName)
	if err != nil {
		return nil, executeSQLFileOutput{}, err
	}

	content, err := os.ReadFile(input.FileName)
	if err != nil {
		return nil, executeSQLFileOutput{}, fmt.Errorf("read file: %w", err)
	}

	result, err := pm.ExecSQLFile(ctx, target.Name, string(content), input.DryRun)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: formatError(err)},
			},
			IsError: true,
		}, executeSQLFileOutput{}, nil
	}

	return nil, executeSQLFileOutput{Result: *result}, nil
}

// formatError formats an error into a useful JSON string for the LLM.
func formatError(err error) string {
	data, _ := json.Marshal(map[string]string{
		"error": err.Error(),
	})
	return string(data)
}
