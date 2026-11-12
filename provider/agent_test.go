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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/blang/semver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/integration"
	presource "github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
)

const agentToken = "openai:index:Agent"

// recordedRequest is one request received by the fake API.
type recordedRequest struct {
	Method string
	Path   string
	Header http.Header
	Body   map[string]any
}

// fakeAPI is an in-memory stand-in for the OpenAI Agents API. Like the real API, it
// returns agents with every default filled in and leaves omitted fields unchanged on update.
type fakeAPI struct {
	t      *testing.T
	server *httptest.Server

	mu          sync.Mutex
	agents      map[string]map[string]any
	automations map[string]map[string]any
	requests    []recordedRequest
	nextID      int
	clock       int
}

// agentDefaults are the resolved values the API reports for unset fields.
func agentDefaults() map[string]any {
	return map[string]any{
		"object":       "agent",
		"name":         nil,
		"instructions": nil,
		"metadata":     map[string]any{},
		"reasoning":    map[string]any{"effort": nil, "summary": nil},
		"service_tier": "auto",
		"text":         map[string]any{"format": map[string]any{"type": "text"}, "verbosity": "medium"},
		"tools":        []any{},
		"multi_agent":  map[string]any{"enabled": false, "max_concurrent_subagents": nil},
		"compaction":   map[string]any{"async": true},
		"experimental": map[string]any{"time_awareness": false},
	}
}

// resolve fills in the per-tool defaults the API adds to its responses.
func resolve(agent map[string]any) {
	tools, _ := agent["tools"].([]any)
	for _, tool := range tools {
		tool := tool.(map[string]any)
		switch tool["type"] {
		case "function":
			if _, ok := tool["defer_loading"]; !ok {
				tool["defer_loading"] = false
			}
		case "mcp":
			if _, ok := tool["connection_origin"]; !ok {
				tool["connection_origin"] = "service"
			}
			if _, ok := tool["required"]; !ok {
				tool["required"] = false
			}
			if transport, ok := tool["transport"].(map[string]any); ok && transport["headers"] == nil {
				transport["headers"] = map[string]any{}
			}
		case "web_search":
			if _, ok := tool["mode"]; !ok {
				tool["mode"] = "live"
			}
		}
	}
}

func newFakeAPI(t *testing.T) *fakeAPI {
	api := &fakeAPI{
		t: t, agents: map[string]map[string]any{}, automations: map[string]map[string]any{}, clock: 1700000000,
	}
	api.server = httptest.NewServer(http.HandlerFunc(api.handle))
	t.Cleanup(api.server.Close)
	return api
}

func (api *fakeAPI) handle(w http.ResponseWriter, r *http.Request) {
	api.mu.Lock()
	defer api.mu.Unlock()

	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &body); err != nil {
			api.fail(w, http.StatusBadRequest, "invalid JSON")
			return
		}
	}
	// Record a separate copy of the body: the handlers below keep and modify `body`.
	var recorded map[string]any
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &recorded)
	}
	api.requests = append(api.requests, recordedRequest{
		Method: r.Method, Path: r.URL.Path, Header: r.Header.Clone(), Body: recorded,
	})

	if r.Header.Get("Authorization") != "Bearer test-token" {
		api.fail(w, http.StatusUnauthorized, "Incorrect API key provided")
		return
	}
	if r.Header.Get("OpenAI-Beta") != "agents=v1" {
		api.fail(w, http.StatusBadRequest, "missing OpenAI-Beta header")
		return
	}

	api.clock++
	if strings.HasPrefix(r.URL.Path, "/v1/agents/automations") {
		api.handleAutomation(w, r, body)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/v1/agents/")
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/v1/agents":
		if model, _ := body["model"].(string); model == "" {
			api.fail(w, http.StatusBadRequest, "model is required")
			return
		}
		api.nextID++
		agent := agentDefaults()
		for k, v := range body {
			agent[k] = v
		}
		agent["id"] = fmt.Sprintf("agent_%d", api.nextID)
		agent["created_at"], agent["updated_at"] = api.clock, api.clock
		resolve(agent)
		api.agents[agent["id"].(string)] = agent
		api.respond(w, http.StatusCreated, agent)

	case api.agents[id] == nil:
		api.fail(w, http.StatusNotFound, "No agent found with id '"+id+"'")

	case r.Method == http.MethodGet:
		api.respond(w, http.StatusOK, api.agents[id])

	case r.Method == http.MethodPost:
		agent := api.agents[id]
		for k, v := range body {
			if v == nil {
				v = agentDefaults()[k]
			}
			agent[k] = v
		}
		agent["updated_at"] = api.clock
		resolve(agent)
		api.respond(w, http.StatusOK, agent)

	case r.Method == http.MethodDelete:
		delete(api.agents, id)
		api.respond(w, http.StatusOK, map[string]any{"id": id, "deleted": true, "object": "agent.deleted"})

	default:
		api.fail(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (api *fakeAPI) respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	assert.NoError(api.t, json.NewEncoder(w).Encode(v))
}

func (api *fakeAPI) fail(w http.ResponseWriter, status int, message string) {
	api.respond(w, status, map[string]any{
		"error": map[string]any{"message": message, "type": "invalid_request_error", "code": nil},
	})
}

// last returns the most recent request with the given method.
func (api *fakeAPI) last(method string) recordedRequest {
	api.mu.Lock()
	defer api.mu.Unlock()
	for i := len(api.requests) - 1; i >= 0; i-- {
		if api.requests[i].Method == method {
			return api.requests[i]
		}
	}
	api.t.Fatalf("no %s request was recorded", method)
	return recordedRequest{}
}

// patch changes an agent behind the provider's back.
func (api *fakeAPI) patch(id string, fields map[string]any) {
	api.mu.Lock()
	defer api.mu.Unlock()
	for k, v := range fields {
		api.agents[id][k] = v
	}
}

// newServer starts a provider configured to talk to the fake API.
func newServer(t *testing.T, api *fakeAPI) integration.Server {
	server, err := integration.NewServer(t.Context(), Name, semver.Version{Minor: 1},
		integration.WithProvider(Provider()))
	require.NoError(t, err)
	require.NoError(t, server.Configure(p.ConfigureRequest{
		Args: props(map[string]any{
			"apiToken": "test-token",
			"baseUrl":  api.server.URL + "/v1",
		}),
	}))
	return server
}

func props(m map[string]any) property.Map {
	return presource.FromResourcePropertyMap(presource.NewPropertyMapFromMap(m))
}

func agentURN() presource.URN {
	return presource.NewURN("test", "provider", "", agentToken, "test")
}

// fullInputs exercises every kind of nested input.
func fullInputs() map[string]any {
	return map[string]any{
		"model":        "gpt-test",
		"name":         "support-agent",
		"instructions": "Be helpful.",
		"metadata":     map[string]any{"team": "support"},
		"reasoning":    map[string]any{"effort": "high"},
		"text":         map[string]any{"verbosity": "low"},
		"multiAgent":   map[string]any{"enabled": true, "maxConcurrentSubagents": 3},
		"tools": []any{
			map[string]any{
				"type":        "function",
				"name":        "lookup_order",
				"description": "Look up an order.",
				"parameters": map[string]any{
					"type":       "object",
					"properties": map[string]any{"orderId": map[string]any{"type": "string"}},
					"required":   []any{"orderId"},
				},
			},
			map[string]any{"type": "web_search", "allowedDomains": []any{"example.com"}},
			map[string]any{
				"type":        "mcp",
				"serverLabel": "docs",
				"transport": map[string]any{
					"type":      "http",
					"serverUrl": "https://mcp.example.com",
					"headers":   map[string]any{"X-Team": "support"},
				},
			},
			map[string]any{"type": "automations"},
		},
	}
}

func TestAgentLifecycle(t *testing.T) {
	t.Parallel()
	api := newFakeAPI(t)
	server := newServer(t, api)

	integration.LifeCycleTest{
		Resource: agentToken,
		Create: integration.Operation{
			Inputs: props(fullInputs()),
			Hook: func(_, output property.Map) {
				assert.Equal(t, "support-agent", output.Get("name").AsString())
				assert.Equal(t, 1700000001.0, output.Get("createdAt").AsNumber())
				assert.Equal(t, 1700000001.0, output.Get("updatedAt").AsNumber())
				// Inputs are stored as written: defaults resolved by the API stay out of state.
				assert.True(t, output.Get("serviceTier").IsNull())

				// The create request carries exactly what the user set, in API naming.
				create := api.last(http.MethodPost)
				assert.Equal(t, "/v1/agents", create.Path)
				assert.Equal(t, map[string]any{
					"model":        "gpt-test",
					"name":         "support-agent",
					"instructions": "Be helpful.",
					"metadata":     map[string]any{"team": "support"},
					"reasoning":    map[string]any{"effort": "high"},
					"text":         map[string]any{"verbosity": "low"},
					"multi_agent":  map[string]any{"enabled": true, "max_concurrent_subagents": 3.0},
					"tools": []any{
						map[string]any{
							"type":        "function",
							"name":        "lookup_order",
							"description": "Look up an order.",
							"parameters": map[string]any{
								"type":       "object",
								"properties": map[string]any{"orderId": map[string]any{"type": "string"}},
								"required":   []any{"orderId"},
							},
						},
						map[string]any{"type": "web_search", "allowed_domains": []any{"example.com"}},
						map[string]any{
							"type":         "mcp",
							"server_label": "docs",
							"transport": map[string]any{
								"type":       "http",
								"server_url": "https://mcp.example.com",
								"headers":    map[string]any{"X-Team": "support"},
							},
						},
						map[string]any{"type": "automations"},
					},
				}, create.Body)
			},
		},
		Updates: []integration.Operation{
			{
				// Change the model, and drop every optional input except metadata.
				Inputs: props(map[string]any{
					"model":    "gpt-test-2",
					"metadata": map[string]any{"team": "sales"},
				}),
				Hook: func(_, output property.Map) {
					assert.Equal(t, "gpt-test-2", output.Get("model").AsString())
					assert.True(t, output.Get("name").IsNull())
					assert.Equal(t, 1700000001.0, output.Get("createdAt").AsNumber())
					assert.Greater(t, output.Get("updatedAt").AsNumber(), 1700000001.0)

					// The update happens in place, and resets what was removed.
					update := api.last(http.MethodPost)
					assert.Equal(t, "/v1/agents/agent_1", update.Path)
					assert.Equal(t, map[string]any{
						"model":        "gpt-test-2",
						"metadata":     map[string]any{"team": "sales"},
						"name":         nil,
						"instructions": nil,
						"reasoning":    nil,
						"service_tier": nil,
						"text":         nil,
						"tools":        nil,
						"multi_agent":  nil,
						"compaction":   nil,
						"experimental": map[string]any{"time_awareness": false},
					}, update.Body)
				},
			},
		},
	}.Run(t, server)

	deleted := api.last(http.MethodDelete)
	assert.Equal(t, "/v1/agents/agent_1", deleted.Path)
	assert.Empty(t, api.agents)
}

func TestAgentPreviewDoesNotCallAPI(t *testing.T) {
	t.Parallel()
	api := newFakeAPI(t)
	server := newServer(t, api)

	_, err := server.Create(p.CreateRequest{Urn: agentURN(), Properties: props(fullInputs()), DryRun: true})
	require.NoError(t, err)
	assert.Empty(t, api.requests)
}

func TestAgentRead(t *testing.T) {
	t.Parallel()
	api := newFakeAPI(t)
	server := newServer(t, api)

	inputs := props(fullInputs())
	created, err := server.Create(p.CreateRequest{Urn: agentURN(), Properties: inputs})
	require.NoError(t, err)

	t.Run("refresh without drift leaves inputs unchanged", func(t *testing.T) {
		read, err := server.Read(p.ReadRequest{
			ID: created.ID, Urn: agentURN(), Properties: created.Properties, Inputs: inputs,
		})
		require.NoError(t, err)
		assert.Equal(t, created.ID, read.ID)

		// None of the defaults resolved by the API leak into the inputs, so there is no diff.
		diff, err := server.Diff(p.DiffRequest{
			ID: created.ID, Urn: agentURN(), State: read.Properties, Inputs: inputs,
		})
		require.NoError(t, err)
		assert.False(t, diff.HasChanges, "unexpected diff: %v", diff.DetailedDiff)
		assert.True(t, read.Inputs.Get("serviceTier").IsNull())
	})

	t.Run("refresh picks up drift", func(t *testing.T) {
		api.patch(created.ID, map[string]any{
			"name":     "renamed-elsewhere",
			"metadata": map[string]any{"team": "support", "owner": "someone"},
		})
		read, err := server.Read(p.ReadRequest{
			ID: created.ID, Urn: agentURN(), Properties: created.Properties, Inputs: inputs,
		})
		require.NoError(t, err)
		assert.Equal(t, "renamed-elsewhere", read.Inputs.Get("name").AsString())
		assert.Equal(t, "someone", read.Inputs.Get("metadata").AsMap().Get("owner").AsString())

		diff, err := server.Diff(p.DiffRequest{
			ID: created.ID, Urn: agentURN(), State: read.Properties, Inputs: inputs,
		})
		require.NoError(t, err)
		assert.True(t, diff.HasChanges)
		assert.Contains(t, diff.DetailedDiff, "name")
	})

	t.Run("import adopts the whole remote configuration", func(t *testing.T) {
		read, err := server.Read(p.ReadRequest{ID: created.ID, Urn: agentURN()})
		require.NoError(t, err)
		assert.Equal(t, created.ID, read.ID)
		assert.Equal(t, "gpt-test", read.Inputs.Get("model").AsString())
		assert.Equal(t, "auto", read.Inputs.Get("serviceTier").AsString())
		assert.Equal(t, "high", read.Inputs.Get("reasoning").AsMap().Get("effort").AsString())
		assert.Equal(t, 4, read.Inputs.Get("tools").AsArray().Len())
	})

	t.Run("an agent deleted elsewhere reads as missing", func(t *testing.T) {
		api.mu.Lock()
		delete(api.agents, created.ID)
		api.mu.Unlock()

		read, err := server.Read(p.ReadRequest{
			ID: created.ID, Urn: agentURN(), Properties: created.Properties, Inputs: inputs,
		})
		require.NoError(t, err)
		assert.Empty(t, read.ID)

		// Deleting it is then a no-op rather than an error.
		require.NoError(t, server.Delete(p.DeleteRequest{
			ID: created.ID, Urn: agentURN(), Properties: created.Properties,
		}))
	})
}

func TestAgentAPIErrorIsReported(t *testing.T) {
	t.Parallel()
	api := newFakeAPI(t)
	server, err := integration.NewServer(t.Context(), Name, semver.Version{Minor: 1},
		integration.WithProvider(Provider()))
	require.NoError(t, err)
	require.NoError(t, server.Configure(p.ConfigureRequest{
		Args: props(map[string]any{"apiToken": "wrong-token", "baseUrl": api.server.URL + "/v1"}),
	}))

	_, err = server.Create(p.CreateRequest{Urn: agentURN(), Properties: props(map[string]any{"model": "gpt-test"})})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "401")
	assert.Contains(t, err.Error(), "Incorrect API key provided")
}

func TestMissingAPITokenIsReported(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	server, err := integration.NewServer(t.Context(), Name, semver.Version{Minor: 1},
		integration.WithProvider(Provider()))
	require.NoError(t, err)
	require.NoError(t, server.Configure(p.ConfigureRequest{}))

	_, err = server.Create(p.CreateRequest{Urn: agentURN(), Properties: props(map[string]any{"model": "gpt-test"})})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "OPENAI_API_KEY")
}

func TestUpdatePayloadKeepsExperimentalSettings(t *testing.T) {
	t.Parallel()
	enabled := true
	payload := updatePayload(AgentArgs{
		Model:        "gpt-test",
		Experimental: &AgentExperimental{TimeAwareness: &enabled},
	})
	assert.Equal(t, map[string]any{"time_awareness": true}, payload["experimental"])
}
