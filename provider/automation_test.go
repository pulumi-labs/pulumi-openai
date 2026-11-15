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
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/integration"
	presource "github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
)

const (
	periodicToken = "openai:index:PeriodicAutomation"
	oneTimeToken  = "openai:index:OneTimeAutomation"
)

func automationURN(token string) presource.URN {
	return presource.NewURN("test", "provider", "", tokens.Type(token), "test")
}

// handleAutomation serves the automations endpoints of the fake API.
func (api *fakeAPI) handleAutomation(w http.ResponseWriter, r *http.Request, body map[string]any) {
	id := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/v1/agents/automations"), "/")
	switch {
	case r.Method == http.MethodPost && id == "":
		// The real API requires a new session to start with at least one message.
		if target, _ := body["target"].(map[string]any); target["type"] == "new_session" {
			if input, _ := target["input"].([]any); len(input) == 0 {
				api.fail(w, http.StatusBadRequest, "Missing required parameter: 'target.input'.")
				return
			}
		}
		api.nextID++
		automation := map[string]any{"metadata": map[string]any{}}
		for k, v := range body {
			automation[k] = v
		}
		automation["id"] = fmt.Sprintf("auto_%d", api.nextID)
		automation["object"] = "agent.automation"
		automation["status"] = "active"
		automation["created_at"] = api.clock
		automation["created_by"] = map[string]any{"type": "developer"}
		resolveTarget(automation)
		api.automations[automation["id"].(string)] = automation
		api.respond(w, http.StatusCreated, automation)

	case api.automations[id] == nil:
		api.fail(w, http.StatusNotFound, "No automation found with id '"+id+"'")

	case r.Method == http.MethodGet:
		api.respond(w, http.StatusOK, api.automations[id])

	case r.Method == http.MethodPost:
		automation := api.automations[id]
		for k, v := range body {
			automation[k] = v
		}
		resolveTarget(automation)
		api.respond(w, http.StatusOK, automation)

	case r.Method == http.MethodDelete:
		delete(api.automations, id)
		api.respond(w, http.StatusOK, map[string]any{
			"id": id, "deleted": true, "object": "agent.automation.deleted",
		})

	default:
		api.fail(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// resolveTarget turns a submitted target into the view the API returns: defaults are
// filled in, and confidential values and uploaded content are left out.
func resolveTarget(automation map[string]any) {
	target := automation["target"].(map[string]any)
	if target["type"] != "new_session" {
		return
	}
	for key, value := range map[string]any{"agent": nil, "agent_id": nil, "metadata": map[string]any{}, "vault_ids": []any{}} {
		if _, ok := target[key]; !ok {
			target[key] = value
		}
	}
	resolveEnvironment(target["environment"].(map[string]any))
}

// resolveEnvironment turns a submitted environment into the view the API returns.
func resolveEnvironment(environment map[string]any) {
	if environment["type"] != "openai_hosted" {
		return
	}
	delete(environment, "env")
	delete(environment, "setup_commands")
	if _, ok := environment["container_size"]; !ok {
		environment["container_size"] = "medium"
	}
	if network, ok := environment["network"].(map[string]any); ok {
		delete(network, "blocked_domains")
		if _, ok := network["allowed_domains"]; !ok {
			network["allowed_domains"] = []any{}
		}
	}
	files, _ := environment["files"].([]any)
	for _, file := range files {
		file := file.(map[string]any)
		if data, ok := file["data"].(string); ok {
			delete(file, "data")
			file["size_bytes"] = len(data)
		}
	}
	for _, key := range []string{"plugins", "skills"} {
		items, _ := environment[key].([]any)
		for _, item := range items {
			delete(item.(map[string]any), "source")
		}
	}
}

// periodicInputs targets a saved agent and exercises a hosted environment.
func periodicInputs() map[string]any {
	return map[string]any{
		"name":       "daily-report",
		"expression": "0 9 * * 1-5",
		"timezone":   "Europe/Amsterdam",
		"metadata":   map[string]any{"team": "support"},
		"target": map[string]any{
			"agentId": "agent_123",
			"input": []any{
				map[string]any{"content": []any{
					map[string]any{"type": "input_text", "text": "Write the daily report."},
				}},
			},
			"environment": map[string]any{
				"type":     "openai_hosted",
				"env":      map[string]any{"REPORT_TOKEN": "s3cret"},
				"network":  map[string]any{"access": "restricted", "blockedDomains": []any{"evil.example.com"}},
				"packages": map[string]any{"python": []any{"pandas"}},
				"files": []any{
					map[string]any{"type": "inline", "path": "/workspace/notes.txt", "data": "aGVsbG8="},
				},
				"setupCommands": []any{map[string]any{"command": "make setup"}},
				"plugins": []any{
					map[string]any{"name": "reports", "description": "Report helpers.", "source": "UEsDBA=="},
				},
				"skills": []any{
					map[string]any{"type": "skill_reference", "skillId": "skill_1"},
					map[string]any{"type": "inline", "name": "charts", "description": "Draws charts.", "source": "UEsDBB=="},
				},
			},
		},
	}
}

// oneTimeInputs runs a saved agent once, without an execution environment.
func oneTimeInputs() map[string]any {
	return map[string]any{
		"name":  "one-off",
		"runAt": 1900000000,
		"target": map[string]any{
			"agentId":     "agent_123",
			"environment": map[string]any{"type": "none"},
			"input": []any{
				map[string]any{"content": []any{
					map[string]any{"type": "input_text", "text": "Say hello."},
				}},
			},
		},
	}
}

func TestAutomationLifecycle(t *testing.T) {
	t.Parallel()
	api := newFakeAPI(t)
	server := newServer(t, api)

	renamed := periodicInputs()
	renamed["name"] = "weekday-report"
	renamed["expression"] = "30 8 * * 1-5"
	delete(renamed, "metadata")

	integration.LifeCycleTest{
		Resource: periodicToken,
		Create: integration.Operation{
			Inputs: props(periodicInputs()),
			Hook: func(_, output property.Map) {
				assert.Equal(t, "active", output.Get("status").AsString())
				assert.Equal(t, "developer", output.Get("createdBy").AsMap().Get("type").AsString())
				assert.Equal(t, 1700000001.0, output.Get("createdAt").AsNumber())

				create := api.last(http.MethodPost)
				assert.Equal(t, "/v1/agents/automations", create.Path)
				assert.Equal(t, map[string]any{
					"type":       "periodic",
					"name":       "daily-report",
					"expression": "0 9 * * 1-5",
					"timezone":   "Europe/Amsterdam",
					"metadata":   map[string]any{"team": "support"},
					"target": map[string]any{
						"type":     "new_session",
						"agent_id": "agent_123",
						"input": []any{
							map[string]any{
								"role": "user",
								"type": "message",
								"content": []any{
									map[string]any{"type": "input_text", "text": "Write the daily report."},
								},
							},
						},
						"environment": map[string]any{
							"type": "openai_hosted",
							"env":  map[string]any{"REPORT_TOKEN": "s3cret"},
							"network": map[string]any{
								"access": "restricted", "blocked_domains": []any{"evil.example.com"},
							},
							"packages": map[string]any{"python": []any{"pandas"}},
							"files": []any{
								map[string]any{"type": "inline", "path": "/workspace/notes.txt", "data": "aGVsbG8="},
							},
							"setup_commands": []any{map[string]any{"command": "make setup"}},
							"plugins": []any{
								map[string]any{
									"type": "inline", "name": "reports", "description": "Report helpers.",
									"source": map[string]any{
										"type": "base64", "media_type": "application/zip", "data": "UEsDBA==",
									},
								},
							},
							"skills": []any{
								map[string]any{"type": "skill_reference", "skill_id": "skill_1"},
								map[string]any{
									"type": "inline", "name": "charts", "description": "Draws charts.",
									"source": map[string]any{
										"type": "base64", "media_type": "application/zip", "data": "UEsDBB==",
									},
								},
							},
						},
					},
				}, create.Body)
			},
		},
		Updates: []integration.Operation{
			{
				Inputs: props(renamed),
				Hook: func(_, output property.Map) {
					assert.Equal(t, "weekday-report", output.Get("name").AsString())

					// Only what changed is sent, and removed metadata is cleared.
					update := api.last(http.MethodPost)
					assert.Equal(t, "/v1/agents/automations/auto_1", update.Path)
					assert.Equal(t, map[string]any{
						"name":       "weekday-report",
						"expression": "30 8 * * 1-5",
						"metadata":   map[string]any{},
					}, update.Body)
				},
			},
		},
	}.Run(t, server)

	assert.Equal(t, "/v1/agents/automations/auto_1", api.last(http.MethodDelete).Path)
	assert.Empty(t, api.automations)
}

func TestAutomationRead(t *testing.T) {
	t.Parallel()

	for name, test := range map[string]struct {
		token string
		raw   map[string]any
	}{
		"periodic with a hosted environment": {periodicToken, periodicInputs()},
		"one-time without an environment":    {oneTimeToken, oneTimeInputs()},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			api := newFakeAPI(t)
			server := newServer(t, api)
			urn := automationURN(test.token)
			inputs := props(test.raw)
			created, err := server.Create(p.CreateRequest{Urn: urn, Properties: inputs})
			require.NoError(t, err)

			read := func(inputs property.Map) p.ReadResponse {
				response, err := server.Read(p.ReadRequest{
					ID: created.ID, Urn: urn, Properties: created.Properties, Inputs: inputs,
				})
				require.NoError(t, err)
				return response
			}
			diff := func(state property.Map) p.DiffResponse {
				response, err := server.Diff(p.DiffRequest{
					ID: created.ID, Urn: urn, State: state, Inputs: inputs,
				})
				require.NoError(t, err)
				return response
			}

			// Without drift there is no diff: resolved defaults stay out of the inputs, and
			// values the API does not return keep what the user wrote.
			refreshed := read(inputs)
			assert.Equal(t, created.ID, refreshed.ID)
			noDrift := diff(refreshed.Properties)
			assert.False(t, noDrift.HasChanges, "unexpected diff: %v", noDrift.DetailedDiff)

			// A change made elsewhere is picked up.
			api.mu.Lock()
			api.automations[created.ID]["name"] = "renamed-elsewhere"
			api.automations[created.ID]["status"] = "paused"
			api.mu.Unlock()
			refreshed = read(inputs)
			assert.Equal(t, "renamed-elsewhere", refreshed.Inputs.Get("name").AsString())
			assert.Equal(t, "paused", refreshed.Properties.Get("status").AsString())
			drift := diff(refreshed.Properties)
			assert.True(t, drift.HasChanges)
			assert.Equal(t, p.Update, drift.DetailedDiff["name"].Kind)
			assert.Len(t, drift.DetailedDiff, 1)

			// An import adopts the remote configuration.
			imported := read(property.Map{})
			assert.Equal(t, "renamed-elsewhere", imported.Inputs.Get("name").AsString())
			assert.False(t, imported.Inputs.Get("target").AsMap().Get("environment").IsNull())

			// An automation deleted elsewhere reads as missing.
			api.mu.Lock()
			delete(api.automations, created.ID)
			api.mu.Unlock()
			assert.Empty(t, read(inputs).ID)
		})
	}
}

func TestOneTimeAutomationLifecycle(t *testing.T) {
	t.Parallel()
	api := newFakeAPI(t)
	server := newServer(t, api)

	postponed := oneTimeInputs()
	postponed["runAt"] = 1900003600

	integration.LifeCycleTest{
		Resource: oneTimeToken,
		Create: integration.Operation{
			Inputs: props(oneTimeInputs()),
			Hook: func(_, output property.Map) {
				create := api.last(http.MethodPost)
				assert.Equal(t, "once", create.Body["type"])
				assert.Equal(t, 1900000000.0, create.Body["run_at"])
				assert.NotContains(t, create.Body, "expression")
				assert.Equal(t, 1900000000.0, output.Get("runAt").AsNumber())
			},
		},
		Updates: []integration.Operation{{
			Inputs: props(postponed),
			Hook: func(_, _ property.Map) {
				assert.Equal(t, map[string]any{"run_at": 1900003600.0}, api.last(http.MethodPost).Body)
			},
		}},
	}.Run(t, server)
	assert.Empty(t, api.automations)
}

func TestAutomationOfTheOtherKindIsRejected(t *testing.T) {
	t.Parallel()
	api := newFakeAPI(t)
	server := newServer(t, api)
	created, err := server.Create(p.CreateRequest{
		Urn: automationURN(periodicToken), Properties: props(periodicInputs()),
	})
	require.NoError(t, err)

	// Importing a periodic automation as a one-time one fails with a pointer to the right resource.
	_, err = server.Read(p.ReadRequest{ID: created.ID, Urn: automationURN(oneTimeToken)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "manage it with the PeriodicAutomation resource")
}

func TestAutomationOutsideWhatIsSupportedIsRejected(t *testing.T) {
	t.Parallel()
	api := newFakeAPI(t)
	server := newServer(t, api)
	urn := automationURN(periodicToken)
	created, err := server.Create(p.CreateRequest{Urn: urn, Properties: props(periodicInputs())})
	require.NoError(t, err)

	readError := func(target map[string]any) string {
		api.mu.Lock()
		api.automations[created.ID]["target"] = target
		api.mu.Unlock()
		_, err := server.Read(p.ReadRequest{ID: created.ID, Urn: urn})
		require.Error(t, err)
		return err.Error()
	}

	// An automation that notifies an existing session, such as one an agent scheduled
	// for itself, cannot be imported.
	assert.Contains(t, readError(map[string]any{
		"type": "existing_session", "session_id": "sess_1", "instructions": "Check the queue.",
	}), "only supports automations that start a new session")

	// Neither can one that configures its agent inline.
	assert.Contains(t, readError(map[string]any{
		"type": "new_session", "agent_id": nil, "agent": map[string]any{"model": "gpt-test"},
		"environment": map[string]any{"type": "none"}, "input": []any{},
	}), "only supports automations that run a saved agent")
}

func TestAutomationRequiredInputs(t *testing.T) {
	t.Parallel()
	server := newServer(t, newFakeAPI(t))

	check := func(token string, inputs property.Map) []string {
		response, err := server.Check(p.CheckRequest{Urn: automationURN(token), Inputs: inputs})
		require.NoError(t, err)
		var properties []string
		for _, failure := range response.Failures {
			properties = append(properties, failure.Property)
		}
		return properties
	}

	assert.Empty(t, check(periodicToken, props(periodicInputs())))
	assert.Empty(t, check(oneTimeToken, props(oneTimeInputs())))

	// The schedule of each resource is required by its schema.
	noExpression := periodicInputs()
	delete(noExpression, "expression")
	assert.Equal(t, []string{"expression"}, check(periodicToken, props(noExpression)))
	noRunAt := oneTimeInputs()
	delete(noRunAt, "runAt")
	assert.Equal(t, []string{"runAt"}, check(oneTimeToken, props(noRunAt)))

	// So is the agent, which the schema reports on the target.
	noAgent := periodicInputs()
	delete(noAgent["target"].(map[string]any), "agentId")
	assert.Equal(t, []string{"target"}, check(periodicToken, props(noAgent)))

	// And so are the messages that start the session: the API rejects a target without them.
	noInput := oneTimeInputs()
	delete(noInput["target"].(map[string]any), "input")
	assert.Equal(t, []string{"target"}, check(oneTimeToken, props(noInput)))

	// The ID of an agent created in the same update is unknown during preview. That is
	// not the same as missing.
	unknownAgent := props(noAgent)
	target := unknownAgent.Get("target").AsMap().Set("agentId", property.New(property.Computed))
	assert.Empty(t, check(periodicToken, unknownAgent.Set("target", property.New(target))))
}
