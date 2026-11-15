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
	"reflect"

	"github.com/pulumi/pulumi-go-provider/infer"
)

// The API has a single automation object whose `type` is `periodic` or `once`. Each type
// is exposed as its own resource, so that the schedule properties of each are plainly
// required. The two resources share everything in this file except their thin methods.
const (
	periodicSchedule = "periodic"
	oneTimeSchedule  = "once"

	// newSessionTarget is the only kind of target this provider exposes.
	newSessionTarget = "new_session"
)

// PeriodicAutomation is the controller for the PeriodicAutomation resource.
type PeriodicAutomation struct{}

// Annotate describes the PeriodicAutomation resource.
func (a *PeriodicAutomation) Annotate(an infer.Annotator) {
	an.Describe(&a, "A schedule that runs an agent repeatedly, on a cron expression. "+
		"Each invocation starts a new session.\n\n"+
		"This resource uses the beta Agents API.")
}

// OneTimeAutomation is the controller for the OneTimeAutomation resource.
type OneTimeAutomation struct{}

// Annotate describes the OneTimeAutomation resource.
func (a *OneTimeAutomation) Annotate(an infer.Annotator) {
	an.Describe(&a, "A schedule that runs an agent once, at a given time. "+
		"The invocation starts a new session.\n\n"+
		"This resource uses the beta Agents API.")
}

// AutomationOutputs are the outputs shared by both automation resources.
type AutomationOutputs struct {
	Status    string              `pulumi:"status"`
	CreatedAt int                 `pulumi:"createdAt"`
	CreatedBy AutomationCreatedBy `pulumi:"createdBy"`
}

// Annotate describes the outputs of the automation resources.
func (o *AutomationOutputs) Annotate(an infer.Annotator) {
	an.Describe(&o.Status, "The status of the automation: `active`, `paused`, or, for a one-time automation "+
		"whose schedule has finished, `completed`.")
	an.Describe(&o.CreatedAt, "When the automation was created, as a Unix timestamp in seconds.")
	an.Describe(&o.CreatedBy, "Who created the automation.")
}

// PeriodicAutomationState is the persisted state of a PeriodicAutomation.
//
// As with Agent, the embedded inputs are stored as the user wrote them.
type PeriodicAutomationState struct {
	PeriodicAutomationArgs
	AutomationOutputs
}

// OneTimeAutomationState is the persisted state of a OneTimeAutomation.
type OneTimeAutomationState struct {
	OneTimeAutomationArgs
	AutomationOutputs
}

// Create creates a new periodic automation.
func (PeriodicAutomation) Create(
	ctx context.Context, req infer.CreateRequest[PeriodicAutomationArgs],
) (infer.CreateResponse[PeriodicAutomationState], error) {
	state := PeriodicAutomationState{PeriodicAutomationArgs: req.Inputs}
	if req.DryRun {
		return infer.CreateResponse[PeriodicAutomationState]{Output: state}, nil
	}
	id, outputs, err := createAutomation(ctx, periodicSchedule, req.Inputs)
	state.AutomationOutputs = outputs
	return infer.CreateResponse[PeriodicAutomationState]{ID: id, Output: state}, err
}

// Update updates a periodic automation in place.
func (PeriodicAutomation) Update(
	ctx context.Context, req infer.UpdateRequest[PeriodicAutomationArgs, PeriodicAutomationState],
) (infer.UpdateResponse[PeriodicAutomationState], error) {
	state := PeriodicAutomationState{PeriodicAutomationArgs: req.Inputs, AutomationOutputs: req.State.AutomationOutputs}
	if req.DryRun {
		return infer.UpdateResponse[PeriodicAutomationState]{Output: state}, nil
	}
	outputs, err := updateAutomation(ctx, req.ID, periodicSchedule, req.State.PeriodicAutomationArgs, req.Inputs)
	state.AutomationOutputs = outputs
	return infer.UpdateResponse[PeriodicAutomationState]{Output: state}, err
}

// Read refreshes a periodic automation from the API. It also backs `pulumi import`.
func (PeriodicAutomation) Read(
	ctx context.Context, req infer.ReadRequest[PeriodicAutomationArgs, PeriodicAutomationState],
) (infer.ReadResponse[PeriodicAutomationArgs, PeriodicAutomationState], error) {
	inputs, outputs, found, err := readAutomation(ctx, req.ID, periodicSchedule, req.Inputs, req.Inputs.Name != "")
	if err != nil || !found {
		// An empty ID tells the engine the resource no longer exists.
		return infer.ReadResponse[PeriodicAutomationArgs, PeriodicAutomationState]{}, err
	}
	return infer.ReadResponse[PeriodicAutomationArgs, PeriodicAutomationState]{
		ID:     req.ID,
		Inputs: inputs,
		State:  PeriodicAutomationState{PeriodicAutomationArgs: inputs, AutomationOutputs: outputs},
	}, nil
}

// Delete deletes a periodic automation.
func (PeriodicAutomation) Delete(
	ctx context.Context, req infer.DeleteRequest[PeriodicAutomationState],
) (infer.DeleteResponse, error) {
	return infer.DeleteResponse{}, deleteAutomation(ctx, req.ID)
}

// Create creates a new one-time automation.
func (OneTimeAutomation) Create(
	ctx context.Context, req infer.CreateRequest[OneTimeAutomationArgs],
) (infer.CreateResponse[OneTimeAutomationState], error) {
	state := OneTimeAutomationState{OneTimeAutomationArgs: req.Inputs}
	if req.DryRun {
		return infer.CreateResponse[OneTimeAutomationState]{Output: state}, nil
	}
	id, outputs, err := createAutomation(ctx, oneTimeSchedule, req.Inputs)
	state.AutomationOutputs = outputs
	return infer.CreateResponse[OneTimeAutomationState]{ID: id, Output: state}, err
}

// Update updates a one-time automation in place.
func (OneTimeAutomation) Update(
	ctx context.Context, req infer.UpdateRequest[OneTimeAutomationArgs, OneTimeAutomationState],
) (infer.UpdateResponse[OneTimeAutomationState], error) {
	state := OneTimeAutomationState{OneTimeAutomationArgs: req.Inputs, AutomationOutputs: req.State.AutomationOutputs}
	if req.DryRun {
		return infer.UpdateResponse[OneTimeAutomationState]{Output: state}, nil
	}
	outputs, err := updateAutomation(ctx, req.ID, oneTimeSchedule, req.State.OneTimeAutomationArgs, req.Inputs)
	state.AutomationOutputs = outputs
	return infer.UpdateResponse[OneTimeAutomationState]{Output: state}, err
}

// Read refreshes a one-time automation from the API. It also backs `pulumi import`.
func (OneTimeAutomation) Read(
	ctx context.Context, req infer.ReadRequest[OneTimeAutomationArgs, OneTimeAutomationState],
) (infer.ReadResponse[OneTimeAutomationArgs, OneTimeAutomationState], error) {
	inputs, outputs, found, err := readAutomation(ctx, req.ID, oneTimeSchedule, req.Inputs, req.Inputs.Name != "")
	if err != nil || !found {
		// An empty ID tells the engine the resource no longer exists.
		return infer.ReadResponse[OneTimeAutomationArgs, OneTimeAutomationState]{}, err
	}
	return infer.ReadResponse[OneTimeAutomationArgs, OneTimeAutomationState]{
		ID:     req.ID,
		Inputs: inputs,
		State:  OneTimeAutomationState{OneTimeAutomationArgs: inputs, AutomationOutputs: outputs},
	}, nil
}

// Delete deletes a one-time automation.
func (OneTimeAutomation) Delete(
	ctx context.Context, req infer.DeleteRequest[OneTimeAutomationState],
) (infer.DeleteResponse, error) {
	return infer.DeleteResponse{}, deleteAutomation(ctx, req.ID)
}

// automationObject holds the fields of an API automation object that are not inputs.
type automationObject struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Status    string `json:"status"`
	CreatedAt int    `json:"created_at"`
	CreatedBy struct {
		Type       string  `json:"type"`
		SessionID  *string `json:"session_id"`
		ToolCallID *string `json:"tool_call_id"`
	} `json:"created_by"`
	Target struct {
		Type    string  `json:"type"`
		AgentID *string `json:"agent_id"`
	} `json:"target"`
}

func automationPath(id string) string {
	return "/agents/automations/" + url.PathEscape(id)
}

// parseAutomation extracts the ID, schedule type and outputs from an API response.
func parseAutomation(body []byte) (automationObject, AutomationOutputs, error) {
	var automation automationObject
	if err := json.Unmarshal(body, &automation); err != nil {
		return automationObject{}, AutomationOutputs{}, fmt.Errorf("decoding automation: %w", err)
	}
	return automation, AutomationOutputs{
		Status:    automation.Status,
		CreatedAt: automation.CreatedAt,
		CreatedBy: AutomationCreatedBy{
			Type:       automation.CreatedBy.Type,
			SessionID:  automation.CreatedBy.SessionID,
			ToolCallID: automation.CreatedBy.ToolCallID,
		},
	}, nil
}

// fieldChange names a top-level input that differs between two sets of inputs.
type fieldChange struct {
	apiName  string
	property string
}

// changedFields lists the top-level inputs that differ. Both arguments must be input
// structs of the same type.
func changedFields(olds, news any) []fieldChange {
	oldFields, newFields := toAPI(olds), toAPI(news)
	var changes []fieldChange
	fields := reflect.TypeOf(news)
	for i := range fields.NumField() {
		apiField, _ := apiName(fields.Field(i))
		if !reflect.DeepEqual(oldFields[apiField], newFields[apiField]) {
			changes = append(changes, fieldChange{apiName: apiField, property: pulumiName(fields.Field(i))})
		}
	}
	return changes
}

func createAutomation(ctx context.Context, schedule string, inputs any) (string, AutomationOutputs, error) {
	c, err := newClient(ctx)
	if err != nil {
		return "", AutomationOutputs{}, err
	}
	payload := automationPayload(inputs)
	payload["type"] = schedule
	body, err := c.do(ctx, http.MethodPost, "/agents/automations", payload)
	if err != nil {
		return "", AutomationOutputs{}, fmt.Errorf("creating automation: %w", err)
	}
	automation, outputs, err := parseAutomation(body)
	if err != nil {
		return "", AutomationOutputs{}, err
	}
	if automation.ID == "" {
		return "", AutomationOutputs{}, fmt.Errorf("creating automation: the API returned no ID")
	}
	return automation.ID, outputs, nil
}

func updateAutomation(ctx context.Context, id, schedule string, olds, news any) (AutomationOutputs, error) {
	// The update endpoint leaves omitted fields unchanged, so only what changed is sent.
	// This matters for `runAt`, which the API rejects once the automation has run.
	fields := automationPayload(news)
	payload := map[string]any{}
	for _, change := range changedFields(olds, news) {
		value, set := fields[change.apiName]
		if !set && change.apiName == "metadata" {
			// Supplied metadata replaces all existing metadata, so an empty object clears it.
			value = map[string]any{}
		}
		payload[change.apiName] = value
	}

	c, err := newClient(ctx)
	if err != nil {
		return AutomationOutputs{}, err
	}
	body, err := c.do(ctx, http.MethodPost, automationPath(id), payload)
	if err != nil {
		return AutomationOutputs{}, fmt.Errorf("updating automation %q: %w", id, err)
	}
	automation, outputs, err := parseAutomation(body)
	if err != nil {
		return AutomationOutputs{}, err
	}
	return outputs, checkSchedule(id, schedule, automation)
}

// readAutomation fetches an automation. It reports found as false if it no longer exists.
func readAutomation[I any](
	ctx context.Context, id, schedule string, prior I, hasPrior bool,
) (inputs I, outputs AutomationOutputs, found bool, err error) {
	c, err := newClient(ctx)
	if err != nil {
		return inputs, outputs, false, err
	}
	body, err := c.do(ctx, http.MethodGet, automationPath(id), nil)
	if isNotFound(err) {
		return inputs, outputs, false, nil
	}
	if err != nil {
		return inputs, outputs, false, fmt.Errorf("reading automation %q: %w", id, err)
	}
	automation, outputs, err := parseAutomation(body)
	if err != nil {
		return inputs, outputs, false, err
	}
	if err := checkSchedule(id, schedule, automation); err != nil {
		return inputs, outputs, false, err
	}
	// Automations made outside Pulumi can use parts of the API this provider leaves out.
	if automation.Target.Type != newSessionTarget {
		return inputs, outputs, false, fmt.Errorf("automation %q has a target of type %q: "+
			"this provider only supports automations that start a new session", id, automation.Target.Type)
	}
	if automation.Target.AgentID == nil {
		return inputs, outputs, false, fmt.Errorf("automation %q configures its agent inline: "+
			"this provider only supports automations that run a saved agent", id)
	}
	inputs, err = refreshInputs(automationRefresh, prior, hasPrior, body)
	if err != nil {
		return inputs, outputs, false, fmt.Errorf("reading automation %q: %w", id, err)
	}
	return inputs, outputs, true, nil
}

// checkSchedule guards against managing an automation with the wrong resource type,
// which can otherwise happen on import.
func checkSchedule(id, schedule string, automation automationObject) error {
	if automation.Type == schedule {
		return nil
	}
	resources := map[string]string{periodicSchedule: "PeriodicAutomation", oneTimeSchedule: "OneTimeAutomation"}
	want, ok := resources[automation.Type]
	if !ok {
		return fmt.Errorf("automation %q has unsupported type %q", id, automation.Type)
	}
	return fmt.Errorf("automation %q is a %s automation: manage it with the %s resource, not %s",
		id, automation.Type, want, resources[schedule])
}

// deleteAutomation deletes an automation, which prevents future invocations. Sessions it
// started are not deleted. An automation that is already gone counts as deleted.
func deleteAutomation(ctx context.Context, id string) error {
	c, err := newClient(ctx)
	if err != nil {
		return err
	}
	if _, err := c.do(ctx, http.MethodDelete, automationPath(id), nil); err != nil && !isNotFound(err) {
		return fmt.Errorf("deleting automation %q: %w", id, err)
	}
	return nil
}

// automationRefresh describes how an automation's inputs are refreshed from the API.
var automationRefresh = refreshSpec{
	freeForm: [][]string{
		{"metadata"},
		{"target", "metadata"},
	},
	// The API returns targets without confidential values and without uploaded content.
	writeOnly: [][]string{
		{"target", "environment", "env"},
		{"target", "environment", "setup_commands"},
		{"target", "environment", "files", "*", "data"},
		{"target", "environment", "plugins", "*", "source"},
		{"target", "environment", "skills", "*", "source"},
		{"target", "environment", "network", "blocked_domains"},
	},
}

// automationPayload builds the API form of an automation's inputs. On top of the generic
// conversion, it fills in the parts of the API shape that the inputs leave implicit.
func automationPayload(inputs any) map[string]any {
	payload := toAPI(inputs)
	target, _ := payload["target"].(map[string]any)
	if target == nil {
		return payload
	}
	target["type"] = newSessionTarget
	completeSessionFields(target)
	return payload
}

// completeSessionFields fills in the constant parts of the API shape of the `input` and
// `environment` fields of a target.
func completeSessionFields(object map[string]any) {
	// Every input message is a user message.
	messages, _ := object["input"].([]any)
	for _, message := range messages {
		message := message.(map[string]any)
		message["role"] = "user"
		message["type"] = "message"
	}

	// Plugins and inline skills are uploaded as base64-encoded ZIP archives.
	environment, _ := object["environment"].(map[string]any)
	archive := func(item any) {
		object, _ := item.(map[string]any)
		if data, ok := object["source"].(string); ok {
			object["source"] = map[string]any{"type": "base64", "media_type": "application/zip", "data": data}
		}
	}
	plugins, _ := environment["plugins"].([]any)
	for _, plugin := range plugins {
		plugin.(map[string]any)["type"] = "inline"
		archive(plugin)
	}
	skills, _ := environment["skills"].([]any)
	for _, skill := range skills {
		archive(skill)
	}
}
