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

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/pulumi/pulumi-go-provider/infer"
)

// Agent is the controller for the Agent resource, a reusable OpenAI agent.
type Agent struct{}

// Annotate describes the Agent resource.
func (a *Agent) Annotate(an infer.Annotator) {
	an.Describe(&a, "A reusable OpenAI agent: a model together with its instructions, tools and settings.\n\n"+
		"This resource uses the beta Agents API.")
}

// AgentState is the persisted state of an Agent.
//
// The embedded inputs are stored as the user wrote them, not as the API resolved them, so
// that defaults filled in by the API never show up as a difference.
type AgentState struct {
	AgentArgs

	CreatedAt int `pulumi:"createdAt"`
	UpdatedAt int `pulumi:"updatedAt"`
}

// Annotate describes the outputs of the Agent resource.
func (s *AgentState) Annotate(an infer.Annotator) {
	an.Describe(&s.CreatedAt, "When the agent was created, as a Unix timestamp in seconds.")
	an.Describe(&s.UpdatedAt, "When the agent was last updated, as a Unix timestamp in seconds.")
}

// agentObject holds the fields of an API agent object that are not inputs.
type agentObject struct {
	ID        string `json:"id"`
	CreatedAt int    `json:"created_at"`
	UpdatedAt int    `json:"updated_at"`
}

func agentPath(id string) string {
	return "/agents/" + url.PathEscape(id)
}

func parseAgent(body []byte) (agentObject, error) {
	var agent agentObject
	if err := json.Unmarshal(body, &agent); err != nil {
		return agentObject{}, fmt.Errorf("decoding agent: %w", err)
	}
	return agent, nil
}

// Create creates a new agent.
func (Agent) Create(
	ctx context.Context, req infer.CreateRequest[AgentArgs],
) (infer.CreateResponse[AgentState], error) {
	state := AgentState{AgentArgs: req.Inputs}
	if req.DryRun {
		return infer.CreateResponse[AgentState]{Output: state}, nil
	}

	c, err := newClient(ctx)
	if err != nil {
		return infer.CreateResponse[AgentState]{}, err
	}
	body, err := c.do(ctx, http.MethodPost, "/agents", toAPI(req.Inputs))
	if err != nil {
		return infer.CreateResponse[AgentState]{}, fmt.Errorf("creating agent: %w", err)
	}
	agent, err := parseAgent(body)
	if err != nil {
		return infer.CreateResponse[AgentState]{}, err
	}
	if agent.ID == "" {
		return infer.CreateResponse[AgentState]{}, fmt.Errorf("creating agent: the API returned no agent ID")
	}

	state.CreatedAt, state.UpdatedAt = agent.CreatedAt, agent.UpdatedAt
	return infer.CreateResponse[AgentState]{ID: agent.ID, Output: state}, nil
}

// Update updates an agent in place. Every input can be changed without replacement.
func (Agent) Update(
	ctx context.Context, req infer.UpdateRequest[AgentArgs, AgentState],
) (infer.UpdateResponse[AgentState], error) {
	state := AgentState{
		AgentArgs: req.Inputs,
		CreatedAt: req.State.CreatedAt,
		UpdatedAt: req.State.UpdatedAt,
	}
	if req.DryRun {
		return infer.UpdateResponse[AgentState]{Output: state}, nil
	}

	c, err := newClient(ctx)
	if err != nil {
		return infer.UpdateResponse[AgentState]{}, err
	}
	body, err := c.do(ctx, http.MethodPost, agentPath(req.ID), updatePayload(req.Inputs))
	if err != nil {
		return infer.UpdateResponse[AgentState]{}, fmt.Errorf("updating agent %q: %w", req.ID, err)
	}
	agent, err := parseAgent(body)
	if err != nil {
		return infer.UpdateResponse[AgentState]{}, err
	}

	state.CreatedAt, state.UpdatedAt = agent.CreatedAt, agent.UpdatedAt
	return infer.UpdateResponse[AgentState]{Output: state}, nil
}

// clearedByNull lists the update fields that an explicit null clears or resets.
var clearedByNull = []string{
	"name", "instructions", "metadata", "reasoning", "service_tier",
	"text", "tools", "multi_agent", "compaction",
}

// updatePayload builds the body of an update request.
//
// The update endpoint leaves omitted fields unchanged, so an input the user has removed
// from their program must be reset explicitly rather than left out.
func updatePayload(inputs AgentArgs) map[string]any {
	payload := toAPI(inputs)
	for _, field := range clearedByNull {
		if _, set := payload[field]; !set {
			payload[field] = nil
		}
	}
	// `experimental` does not accept null and merges with the stored value, so its
	// settings are reset to their defaults individually.
	experimental, _ := payload["experimental"].(map[string]any)
	if experimental == nil {
		experimental = map[string]any{}
	}
	if _, set := experimental["time_awareness"]; !set {
		experimental["time_awareness"] = false
	}
	payload["experimental"] = experimental
	return payload
}

// Read refreshes an agent from the API. It also backs `pulumi import`.
func (Agent) Read(
	ctx context.Context, req infer.ReadRequest[AgentArgs, AgentState],
) (infer.ReadResponse[AgentArgs, AgentState], error) {
	c, err := newClient(ctx)
	if err != nil {
		return infer.ReadResponse[AgentArgs, AgentState]{}, err
	}
	body, err := c.do(ctx, http.MethodGet, agentPath(req.ID), nil)
	if isNotFound(err) {
		// An empty ID tells the engine the resource no longer exists.
		return infer.ReadResponse[AgentArgs, AgentState]{}, nil
	}
	if err != nil {
		return infer.ReadResponse[AgentArgs, AgentState]{}, fmt.Errorf("reading agent %q: %w", req.ID, err)
	}
	agent, err := parseAgent(body)
	if err != nil {
		return infer.ReadResponse[AgentArgs, AgentState]{}, err
	}
	inputs, err := refreshInputs(agentRefresh, req.Inputs, req.Inputs.Model != "", body)
	if err != nil {
		return infer.ReadResponse[AgentArgs, AgentState]{}, fmt.Errorf("reading agent %q: %w", req.ID, err)
	}

	return infer.ReadResponse[AgentArgs, AgentState]{
		ID:     req.ID,
		Inputs: inputs,
		State: AgentState{
			AgentArgs: inputs,
			CreatedAt: agent.CreatedAt,
			UpdatedAt: agent.UpdatedAt,
		},
	}, nil
}

// Delete deletes an agent. An agent that is already gone counts as deleted.
func (Agent) Delete(
	ctx context.Context, req infer.DeleteRequest[AgentState],
) (infer.DeleteResponse, error) {
	c, err := newClient(ctx)
	if err != nil {
		return infer.DeleteResponse{}, err
	}
	if _, err := c.do(ctx, http.MethodDelete, agentPath(req.ID), nil); err != nil && !isNotFound(err) {
		return infer.DeleteResponse{}, fmt.Errorf("deleting agent %q: %w", req.ID, err)
	}
	return infer.DeleteResponse{}, nil
}

// agentRefresh describes how an agent's inputs are refreshed from the API.
var agentRefresh = refreshSpec{
	freeForm: [][]string{
		{"metadata"},
		{"text", "format", "schema"},
		{"tools", "*", "parameters"},
		{"tools", "*", "request_metadata"},
		{"tools", "*", "transport", "headers"},
	},
}
