// Copyright 2026, Pulumi Corporation.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package provider

import "github.com/pulumi/pulumi-go-provider/infer"

// The types in this file are the Pulumi inputs of the Agent resource. They also define the
// JSON bodies of the OpenAI Agents API: each API field is the snake_case form of the Pulumi
// property name (see wire.go). They deliberately carry no `json` tags, because Pulumi's
// property mapper would read those in preference to the `pulumi` tags.

// AgentArgs are the inputs of the Agent resource.
type AgentArgs struct {
	Model        string             `pulumi:"model"`
	Name         *string            `pulumi:"name,optional"`
	Instructions *string            `pulumi:"instructions,optional"`
	Metadata     map[string]string  `pulumi:"metadata,optional"`
	Reasoning    *AgentReasoning    `pulumi:"reasoning,optional"`
	ServiceTier  *ServiceTier       `pulumi:"serviceTier,optional"`
	Text         *AgentText         `pulumi:"text,optional"`
	Tools        []AgentTool        `pulumi:"tools,optional"`
	MultiAgent   *AgentMultiAgent   `pulumi:"multiAgent,optional"`
	Compaction   *AgentCompaction   `pulumi:"compaction,optional"`
	Experimental *AgentExperimental `pulumi:"experimental,optional"`
}

// Annotate describes the inputs of the Agent resource.
func (a *AgentArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.Model, "The model the agent uses for inference.")
	an.Describe(&a.Name, "A human-readable name for the agent, up to 128 characters.")
	an.Describe(&a.Instructions, "Custom instructions, appended to the default base instructions.")
	an.Describe(&a.Metadata, "Up to 16 custom key-value pairs. "+
		"Keys can be up to 64 characters and values up to 512 characters.")
	an.Describe(&a.Reasoning, "Reasoning configuration. When unset, the model's defaults apply.")
	an.Describe(&a.ServiceTier, "The service tier used to run the agent. Defaults to `auto`.")
	an.Describe(&a.Text, "Configuration of the agent's text output.")
	an.Describe(&a.Tools, "The tools available to the agent, up to 2000.")
	an.Describe(&a.MultiAgent, "Controls whether the agent can delegate work to subagents.")
	an.Describe(&a.Compaction, "Controls how the agent's context is compacted.")
	an.Describe(&a.Experimental, "Experimental settings, which may change without notice.")
}

// AgentReasoning configures model reasoning.
type AgentReasoning struct {
	Effort  *ReasoningEffort  `pulumi:"effort,optional"`
	Summary *ReasoningSummary `pulumi:"summary,optional"`
}

// Annotate describes AgentReasoning.
func (r *AgentReasoning) Annotate(an infer.Annotator) {
	an.Describe(&r.Effort, "How much reasoning effort the model spends. When unset, the model chooses its default.")
	an.Describe(&r.Summary, "The kind of reasoning summary to produce. When unset, summaries are disabled.")
}

// AgentText configures text output.
type AgentText struct {
	Format    *AgentTextFormat `pulumi:"format,optional"`
	Verbosity *TextVerbosity   `pulumi:"verbosity,optional"`
}

// Annotate describes AgentText.
func (t *AgentText) Annotate(an infer.Annotator) {
	an.Describe(&t.Format, "The output format. Defaults to plain text.")
	an.Describe(&t.Verbosity, "How verbose the agent's responses are. Defaults to `medium`.")
}

// AgentTextFormat is the output format of an agent.
type AgentTextFormat struct {
	Type   TextFormatType `pulumi:"type"`
	Schema map[string]any `pulumi:"schema,optional"`
}

// Annotate describes AgentTextFormat.
func (f *AgentTextFormat) Annotate(an infer.Annotator) {
	an.Describe(&f.Type, "The format type: `text` for ordinary text, "+
		"or `json_schema` to constrain output to a JSON Schema.")
	an.Describe(&f.Schema, "The JSON Schema the output must conform to. Required when `type` is `json_schema`.")
}

// AgentMultiAgent configures subagents.
type AgentMultiAgent struct {
	Enabled                bool `pulumi:"enabled"`
	MaxConcurrentSubagents *int `pulumi:"maxConcurrentSubagents,optional"`
}

// Annotate describes AgentMultiAgent.
func (m *AgentMultiAgent) Annotate(an infer.Annotator) {
	an.Describe(&m.Enabled, "Whether subagent tools are enabled.")
	an.Describe(&m.MaxConcurrentSubagents, "The maximum number of subagents that run at once. Defaults to 6.")
}

// AgentCompaction configures context compaction.
type AgentCompaction struct {
	Async *bool `pulumi:"async,optional"`
}

// Annotate describes AgentCompaction.
func (c *AgentCompaction) Annotate(an infer.Annotator) {
	an.Describe(&c.Async, "Whether compaction runs in the background. Defaults to true; "+
		"set to false for blocking compaction.")
}

// AgentExperimental holds experimental settings.
type AgentExperimental struct {
	TimeAwareness *bool `pulumi:"timeAwareness,optional"`
}

// Annotate describes AgentExperimental.
func (e *AgentExperimental) Annotate(an infer.Annotator) {
	an.Describe(&e.TimeAwareness, "Whether the model receives periodic reminders of the current time. "+
		"Defaults to false.")
}

// AgentTool is one tool available to an agent.
//
// The API models tools as a union discriminated by `type`. It is flattened here into a
// single object: `type` selects the kind of tool, and each other field applies only to
// the tool types named in its description.
type AgentTool struct {
	Type ToolType `pulumi:"type"`

	// Function tools.
	Name         *string        `pulumi:"name,optional"`
	Description  *string        `pulumi:"description,optional"`
	Parameters   map[string]any `pulumi:"parameters,optional"`
	Async        *bool          `pulumi:"async,optional"`
	DeferLoading *bool          `pulumi:"deferLoading,optional"`

	// Programmatic tool calling.
	Enabled *bool `pulumi:"enabled,optional"`

	// MCP tools.
	ServerLabel      *string            `pulumi:"serverLabel,optional"`
	Transport        *AgentMcpTransport `pulumi:"transport,optional"`
	AllowedTools     []string           `pulumi:"allowedTools,optional"`
	ConnectionOrigin *ConnectionOrigin  `pulumi:"connectionOrigin,optional"`
	CredentialID     *string            `pulumi:"credentialId,optional"`
	RequestMetadata  map[string]any     `pulumi:"requestMetadata,optional"`
	Required         *bool              `pulumi:"required,optional"`

	// Web search tools.
	AllowedDomains []string                `pulumi:"allowedDomains,optional"`
	ContextSize    *WebSearchContextSize   `pulumi:"contextSize,optional"`
	Location       *AgentWebSearchLocation `pulumi:"location,optional"`
	Mode           *WebSearchMode          `pulumi:"mode,optional"`

	// Computer use tools.
	IncludeScreenshots *bool `pulumi:"includeScreenshots,optional"`
}

// Annotate describes AgentTool.
func (t *AgentTool) Annotate(an infer.Annotator) {
	an.Describe(&t, "A tool available to an agent. `type` selects the kind of tool; "+
		"each other property applies only to the tool types named in its description.")
	an.Describe(&t.Type, "The kind of tool.")

	an.Describe(&t.Name, "`function` tools: the name of the function. Required.")
	an.Describe(&t.Description, "`function` tools: a description of what the function does.")
	an.Describe(&t.Parameters, "`function` tools: a JSON Schema describing the function's parameters.")
	an.Describe(&t.Async, "`function` tools: whether the function is called asynchronously.")
	an.Describe(&t.DeferLoading, "`function` tools: whether the function is only loaded "+
		"once discovered through a `tool_search` tool.")

	an.Describe(&t.Enabled, "`programmatic_tool_calling` tools: whether programmatic tool calling "+
		"is enabled. Defaults to true.")

	an.Describe(&t.ServerLabel, "`mcp` tools: a label identifying the MCP server. Required.")
	an.Describe(&t.Transport, "`mcp` tools: how to connect to the MCP server. Required.")
	an.Describe(&t.AllowedTools, "`mcp` tools: the names of the server's tools the agent may use. "+
		"When unset, all tools are allowed.")
	an.Describe(&t.ConnectionOrigin, "`mcp` tools: where the connection to the server is made from.")
	an.Describe(&t.CredentialID, "`mcp` tools: the ID of a stored credential used to authenticate to the server.")
	an.Describe(&t.RequestMetadata, "`mcp` tools: metadata sent to the server with each request.")
	an.Describe(&t.Required, "`mcp` tools: whether the agent requires the server to be available.")

	an.Describe(&t.AllowedDomains, "`web_search` tools: the domains searches are restricted to.")
	an.Describe(&t.ContextSize, "`web_search` tools: how much search context is given to the model.")
	an.Describe(&t.Location, "`web_search` tools: an approximate user location used to localize results.")
	an.Describe(&t.Mode, "`web_search` tools: whether results are fetched live, served from cache, or disabled.")

	an.Describe(&t.IncludeScreenshots, "`computer_use` tools: whether screenshots are included.")
}

// AgentMcpTransport describes how to reach an MCP server.
type AgentMcpTransport struct {
	Type McpTransportType `pulumi:"type"`

	// HTTP transport.
	ServerURL *string           `pulumi:"serverUrl,optional"`
	Headers   map[string]string `pulumi:"headers,optional"`

	// Stdio transport.
	Command *string  `pulumi:"command,optional"`
	Args    []string `pulumi:"args,optional"`
	Cwd     *string  `pulumi:"cwd,optional"`
	EnvVars []string `pulumi:"envVars,optional"`
}

// Annotate describes AgentMcpTransport.
func (t *AgentMcpTransport) Annotate(an infer.Annotator) {
	an.Describe(&t.Type, "The transport type: `http` or `stdio`.")
	an.Describe(&t.ServerURL, "`http` transport: the URL of the MCP server. Required.")
	an.Describe(&t.Headers, "`http` transport: non-secret headers sent with each request to the server.")
	an.Describe(&t.Command, "`stdio` transport: the command that starts the MCP server. Required.")
	an.Describe(&t.Args, "`stdio` transport: arguments passed to the command.")
	an.Describe(&t.Cwd, "`stdio` transport: the working directory the command runs in. Required.")
	an.Describe(&t.EnvVars, "`stdio` transport: names of environment variables the command inherits "+
		"from the execution environment.")
}

// AgentWebSearchLocation is an approximate user location for web search.
type AgentWebSearchLocation struct {
	City     *string `pulumi:"city,optional"`
	Country  *string `pulumi:"country,optional"`
	Region   *string `pulumi:"region,optional"`
	Timezone *string `pulumi:"timezone,optional"`
}

// ToolType is the kind of an agent tool.
type ToolType string

// Values lists the allowed tool types.
func (ToolType) Values() []infer.EnumValue[ToolType] {
	return []infer.EnumValue[ToolType]{
		{Name: "Function", Value: "function", Description: "A function the caller implements."},
		{Name: "ToolSearch", Value: "tool_search", Description: "Lets the agent discover deferred function tools."},
		{
			Name: "ProgrammaticToolCalling", Value: "programmatic_tool_calling",
			Description: "Lets the agent call tools from code.",
		},
		{Name: "Mcp", Value: "mcp", Description: "Tools served by a Model Context Protocol server."},
		{Name: "WebSearch", Value: "web_search", Description: "Web search."},
		{Name: "ComputerUse", Value: "computer_use", Description: "Computer use."},
		{Name: "Automations", Value: "automations", Description: "Lets the agent manage its own scheduled wakeups."},
	}
}

// ReasoningEffort is how much reasoning effort a model spends.
type ReasoningEffort string

// Values lists the allowed reasoning efforts.
func (ReasoningEffort) Values() []infer.EnumValue[ReasoningEffort] {
	return []infer.EnumValue[ReasoningEffort]{
		{Name: "None", Value: "none"},
		{Name: "Minimal", Value: "minimal"},
		{Name: "Low", Value: "low"},
		{Name: "Medium", Value: "medium"},
		{Name: "High", Value: "high"},
		{Name: "XHigh", Value: "xhigh"},
		{Name: "Max", Value: "max"},
	}
}

// ReasoningSummary is the kind of reasoning summary a model produces.
type ReasoningSummary string

// Values lists the allowed reasoning summaries.
func (ReasoningSummary) Values() []infer.EnumValue[ReasoningSummary] {
	return []infer.EnumValue[ReasoningSummary]{
		{Name: "Concise", Value: "concise"},
		{Name: "Detailed", Value: "detailed"},
		{Name: "Auto", Value: "auto"},
	}
}

// ServiceTier is the service tier an agent runs on.
type ServiceTier string

// Values lists the allowed service tiers.
func (ServiceTier) Values() []infer.EnumValue[ServiceTier] {
	return []infer.EnumValue[ServiceTier]{
		{Name: "Auto", Value: "auto"},
		{Name: "Default", Value: "default"},
		{Name: "Flex", Value: "flex"},
		{Name: "Priority", Value: "priority"},
		{Name: "Fast", Value: "fast"},
		{Name: "Ultrafast", Value: "ultrafast"},
	}
}

// TextVerbosity is how verbose an agent's responses are.
type TextVerbosity string

// Values lists the allowed verbosity levels.
func (TextVerbosity) Values() []infer.EnumValue[TextVerbosity] {
	return []infer.EnumValue[TextVerbosity]{
		{Name: "Low", Value: "low"},
		{Name: "Medium", Value: "medium"},
		{Name: "High", Value: "high"},
	}
}

// TextFormatType is the type of an agent's output format.
type TextFormatType string

// Values lists the allowed output format types.
func (TextFormatType) Values() []infer.EnumValue[TextFormatType] {
	return []infer.EnumValue[TextFormatType]{
		{Name: "Text", Value: "text"},
		{Name: "JsonSchema", Value: "json_schema"},
	}
}

// McpTransportType is the type of an MCP transport.
type McpTransportType string

// Values lists the allowed MCP transport types.
func (McpTransportType) Values() []infer.EnumValue[McpTransportType] {
	return []infer.EnumValue[McpTransportType]{
		{Name: "Http", Value: "http"},
		{Name: "Stdio", Value: "stdio"},
	}
}

// ConnectionOrigin is where an MCP connection is made from.
type ConnectionOrigin string

// Values lists the allowed connection origins.
func (ConnectionOrigin) Values() []infer.EnumValue[ConnectionOrigin] {
	return []infer.EnumValue[ConnectionOrigin]{
		{Name: "Service", Value: "service"},
		{Name: "Environment", Value: "environment"},
	}
}

// WebSearchContextSize is how much search context is given to the model.
type WebSearchContextSize string

// Values lists the allowed context sizes.
func (WebSearchContextSize) Values() []infer.EnumValue[WebSearchContextSize] {
	return []infer.EnumValue[WebSearchContextSize]{
		{Name: "Low", Value: "low"},
		{Name: "Medium", Value: "medium"},
		{Name: "High", Value: "high"},
	}
}

// WebSearchMode is how web search results are fetched.
type WebSearchMode string

// Values lists the allowed web search modes.
func (WebSearchMode) Values() []infer.EnumValue[WebSearchMode] {
	return []infer.EnumValue[WebSearchMode]{
		{Name: "Disabled", Value: "disabled"},
		{Name: "Cached", Value: "cached"},
		{Name: "Live", Value: "live"},
	}
}
