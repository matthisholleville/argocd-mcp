package gateway

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// RegisterMCPPrompts registers pre-packaged prompt templates on the MCP server.
// These guide the LLM through common ArgoCD workflows.
func RegisterMCPPrompts(srv *server.MCPServer) {
	srv.AddPrompt(
		mcp.NewPrompt("unhealthy-apps",
			mcp.WithPromptDescription("Find all applications with degraded health or out-of-sync status"),
		),
		handleUnhealthyApps(),
	)

	srv.AddPrompt(
		mcp.NewPrompt("app-diff",
			mcp.WithPromptDescription("Show what would change if an application is synced"),
			mcp.WithArgument("appName",
				mcp.ArgumentDescription("Application name"),
				mcp.RequiredArgument(),
			),
		),
		handleAppDiff(),
	)

	srv.AddPrompt(
		mcp.NewPrompt("rollback",
			mcp.WithPromptDescription("Show deployment history and rollback an application to a previous revision"),
			mcp.WithArgument("appName",
				mcp.ArgumentDescription("Application name"),
				mcp.RequiredArgument(),
			),
		),
		handleRollback(),
	)

	srv.AddPrompt(
		mcp.NewPrompt("sync-status",
			mcp.WithPromptDescription("Overview of all applications sync and health status"),
		),
		handleSyncStatus(),
	)

	srv.AddPrompt(
		mcp.NewPrompt("app-logs",
			mcp.WithPromptDescription("Fetch and analyze logs for an application's container"),
			mcp.WithArgument("appName",
				mcp.ArgumentDescription("Application name"),
				mcp.RequiredArgument(),
			),
			mcp.WithArgument("container",
				mcp.ArgumentDescription("Container name (optional, defaults to main container)"),
			),
		),
		handleAppLogs(),
	)
}

func handleUnhealthyApps() server.PromptHandlerFunc {
	return func(_ context.Context, _ mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return &mcp.GetPromptResult{
			Description: "Find unhealthy ArgoCD applications",
			Messages: []mcp.PromptMessage{
				mcp.NewPromptMessage(
					mcp.RoleUser,
					mcp.NewTextContent(
						"List all ArgoCD applications that need attention.\n\n"+
							"Steps:\n"+
							"1. Use execute_operation to call GET /api/v1/applications\n"+
							"2. Filter applications where health.status is not \"Healthy\" or sync.status is not \"Synced\"\n"+
							"3. For each problematic app, show: name, namespace, health status, sync status, and last sync time\n"+
							"4. Group results by severity: Degraded/Missing first, then Progressing, then OutOfSync\n"+
							"5. If all apps are healthy, confirm that everything looks good",
					),
				),
			},
		}, nil
	}
}

func handleAppDiff() server.PromptHandlerFunc {
	return func(_ context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		appName := req.Params.Arguments["appName"]
		if appName == "" {
			appName = "<appName>"
		}

		return &mcp.GetPromptResult{
			Description: fmt.Sprintf("Show diff for application %s", appName),
			Messages: []mcp.PromptMessage{
				mcp.NewPromptMessage(
					mcp.RoleUser,
					mcp.NewTextContent(fmt.Sprintf(
						"Show what would change if I sync the ArgoCD application \"%s\".\n\n"+
							"Steps:\n"+
							"1. Use execute_operation to call GET /api/v1/applications/%s with query param refresh=true\n"+
							"2. Compare the desired state (spec.source) with the live state\n"+
							"3. Show the managed resources that differ between desired and live\n"+
							"4. Present a clear summary of what resources would be created, updated, or deleted on sync\n"+
							"5. Highlight any potentially destructive changes (deletions, PVC changes, etc.)",
						appName, appName,
					)),
				),
			},
		}, nil
	}
}

func handleRollback() server.PromptHandlerFunc {
	return func(_ context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		appName := req.Params.Arguments["appName"]
		if appName == "" {
			appName = "<appName>"
		}

		return &mcp.GetPromptResult{
			Description: fmt.Sprintf("Rollback application %s", appName),
			Messages: []mcp.PromptMessage{
				mcp.NewPromptMessage(
					mcp.RoleUser,
					mcp.NewTextContent(fmt.Sprintf(
						"Help me rollback the ArgoCD application \"%s\" to a previous revision.\n\n"+
							"Steps:\n"+
							"1. Use execute_operation to call GET /api/v1/applications/%s to get current status and history\n"+
							"2. Show the deployment history: revision number, deployed at, commit message, and author\n"+
							"3. Highlight the currently deployed revision\n"+
							"4. Ask me which revision I want to rollback to\n"+
							"5. Once I confirm, use execute_operation to call PUT /api/v1/applications/%s/rollback with the chosen revision ID",
						appName, appName, appName,
					)),
				),
			},
		}, nil
	}
}

func handleSyncStatus() server.PromptHandlerFunc {
	return func(_ context.Context, _ mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return &mcp.GetPromptResult{
			Description: "Overview of all applications sync and health status",
			Messages: []mcp.PromptMessage{
				mcp.NewPromptMessage(
					mcp.RoleUser,
					mcp.NewTextContent(
						"Give me a dashboard-style overview of all ArgoCD applications.\n\n"+
							"Steps:\n"+
							"1. Use execute_operation to call GET /api/v1/applications\n"+
							"2. For each application, extract: name, project, namespace, health status, sync status, and target revision\n"+
							"3. Present a summary table with all apps\n"+
							"4. Show counts: total apps, healthy, degraded, progressing, missing, unknown\n"+
							"5. Show sync counts: synced, OutOfSync, unknown\n"+
							"6. Highlight any apps that need immediate attention",
					),
				),
			},
		}, nil
	}
}

func handleAppLogs() server.PromptHandlerFunc {
	return func(_ context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		appName := req.Params.Arguments["appName"]
		if appName == "" {
			appName = "<appName>"
		}
		container := req.Params.Arguments["container"]

		prompt := fmt.Sprintf(
			"Fetch and analyze the logs for ArgoCD application \"%s\".\n\n"+
				"Steps:\n"+
				"1. Use execute_operation to call GET /api/v1/applications/%s to find the pod and container names\n"+
				"2. Use execute_operation to call GET /api/v1/applications/%s/logs",
			appName, appName, appName,
		)

		if container != "" {
			prompt += fmt.Sprintf(" with query param container=%s", container)
		}

		prompt += "\n3. Analyze the log output for errors, warnings, or unusual patterns\n" +
			"4. Summarize the key findings: error count, most frequent errors, any stack traces\n" +
			"5. Suggest potential causes and next steps if issues are found"

		return &mcp.GetPromptResult{
			Description: fmt.Sprintf("Logs for application %s", appName),
			Messages: []mcp.PromptMessage{
				mcp.NewPromptMessage(
					mcp.RoleUser,
					mcp.NewTextContent(prompt),
				),
			},
		}, nil
	}
}
