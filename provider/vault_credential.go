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
	"strings"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

// VaultCredential is the controller for the VaultCredential resource, a credential stored
// in a vault.
type VaultCredential struct{}

// Annotate describes the VaultCredential resource.
func (c *VaultCredential) Annotate(an infer.Annotator) {
	an.Describe(&c, "A credential stored in a vault: a token for an MCP server, or a secret exposed to "+
		"OpenAI-hosted environments as an environment variable. Sessions use the credentials of the vaults "+
		"attached to them; an MCP tool can pick one with `credentialId`.\n\n"+
		"Secrets can be rotated in place. Everything else about how the credential authenticates is fixed "+
		"once it is created, so changing it replaces the credential.\n\n"+
		"Import a credential with an ID of the form `<vaultId>/<credentialId>`.\n\n"+
		"This resource uses the beta Agents API.")
}

// VaultCredentialState is the persisted state of a VaultCredential.
//
// As with Agent, the embedded inputs are stored as the user wrote them. That includes the
// secrets, which the API never returns.
type VaultCredentialState struct {
	VaultCredentialArgs

	CreatedAt int `pulumi:"createdAt"`
	UpdatedAt int `pulumi:"updatedAt"`
}

// Annotate describes the outputs of the VaultCredential resource.
func (s *VaultCredentialState) Annotate(an infer.Annotator) {
	an.Describe(&s.CreatedAt, "When the credential was created, as a Unix timestamp in seconds.")
	an.Describe(&s.UpdatedAt, "When the credential was last updated, as a Unix timestamp in seconds.")
}

// credentialObject holds the fields of an API credential object that are not inputs.
type credentialObject struct {
	ID        string `json:"id"`
	CreatedAt int    `json:"created_at"`
	UpdatedAt int    `json:"updated_at"`
}

func credentialPath(vaultID, id string) string {
	return vaultPath(vaultID) + "/credentials/" + url.PathEscape(id)
}

func parseCredential(body []byte) (credentialObject, error) {
	var credential credentialObject
	if err := json.Unmarshal(body, &credential); err != nil {
		return credentialObject{}, fmt.Errorf("decoding vault credential: %w", err)
	}
	return credential, nil
}

// Create creates a new credential in a vault.
func (VaultCredential) Create(
	ctx context.Context, req infer.CreateRequest[VaultCredentialArgs],
) (infer.CreateResponse[VaultCredentialState], error) {
	state := VaultCredentialState{VaultCredentialArgs: req.Inputs}
	if req.DryRun {
		return infer.CreateResponse[VaultCredentialState]{Output: state}, nil
	}

	c, err := newClient(ctx)
	if err != nil {
		return infer.CreateResponse[VaultCredentialState]{}, err
	}
	// The vault is part of the path, not the body.
	payload := toAPI(req.Inputs)
	delete(payload, "vault_id")
	body, err := c.do(ctx, http.MethodPost, vaultPath(req.Inputs.VaultID)+"/credentials", payload)
	if err != nil {
		return infer.CreateResponse[VaultCredentialState]{}, fmt.Errorf("creating vault credential: %w", err)
	}
	credential, err := parseCredential(body)
	if err != nil {
		return infer.CreateResponse[VaultCredentialState]{}, err
	}
	if credential.ID == "" {
		return infer.CreateResponse[VaultCredentialState]{}, fmt.Errorf(
			"creating vault credential: the API returned no credential ID")
	}

	state.CreatedAt, state.UpdatedAt = credential.CreatedAt, credential.UpdatedAt
	return infer.CreateResponse[VaultCredentialState]{ID: credential.ID, Output: state}, nil
}

// credentialUpdatable lists the properties the update endpoint can change. A change to
// any other property replaces the credential.
var credentialUpdatable = map[string]bool{
	"metadata":                  true,
	"auth.token":                true,
	"auth.accessToken":          true,
	"auth.expiresAt":            true,
	"auth.secretValue":          true,
	"auth.refresh.refreshToken": true,
	"auth.refresh.scope":        true,
	"auth.refresh.tokenEndpointAuth.clientSecret": true,
}

// Diff reports which changes rotate a credential in place and which replace it. The
// distinction lies within `auth`, below the level that `replaceOnChanges` works at.
func (VaultCredential) Diff(
	_ context.Context, req infer.DiffRequest[VaultCredentialArgs, VaultCredentialState],
) (infer.DiffResponse, error) {
	olds, news := propertyLeaves(req.State.VaultCredentialArgs), propertyLeaves(req.Inputs)
	paths := map[string]bool{}
	for path := range olds {
		paths[path] = true
	}
	for path := range news {
		paths[path] = true
	}

	detailed := map[string]p.PropertyDiff{}
	replace := false
	for path := range paths {
		oldValue, inOld := olds[path]
		newValue, inNew := news[path]
		if reflect.DeepEqual(oldValue, newValue) {
			continue
		}
		kind := p.Update
		switch {
		case !inOld:
			kind = p.Add
		case !inNew:
			kind = p.Delete
		}
		if !credentialUpdatable[path] {
			replace = true
			kind = map[p.DiffKind]p.DiffKind{p.Add: p.AddReplace, p.Delete: p.DeleteReplace, p.Update: p.UpdateReplace}[kind]
		}
		detailed[path] = p.PropertyDiff{Kind: kind, InputDiff: true}
	}

	return infer.DiffResponse{
		HasChanges:          len(detailed) > 0,
		DetailedDiff:        detailed,
		DeleteBeforeReplace: replace && occupiesSameSecretName(req.State.Auth, req.Inputs.Auth),
	}, nil
}

// occupiesSameSecretName reports whether two environment variable credentials use the
// same variable name. A vault holds one active credential per name, so a replacement
// that keeps the name has to delete the old credential first.
func occupiesSameSecretName(before, after CredentialAuth) bool {
	return before.Type == environmentVariableCredential && after.Type == environmentVariableCredential &&
		before.SecretName != nil && after.SecretName != nil && *before.SecretName == *after.SecretName
}

// Update updates a credential in place: it changes its metadata or rotates its secrets.
func (VaultCredential) Update(
	ctx context.Context, req infer.UpdateRequest[VaultCredentialArgs, VaultCredentialState],
) (infer.UpdateResponse[VaultCredentialState], error) {
	state := VaultCredentialState{
		VaultCredentialArgs: req.Inputs,
		CreatedAt:           req.State.CreatedAt,
		UpdatedAt:           req.State.UpdatedAt,
	}
	if req.DryRun {
		return infer.UpdateResponse[VaultCredentialState]{Output: state}, nil
	}

	payload := credentialUpdatePayload(req.State.VaultCredentialArgs, req.Inputs)
	if len(payload) == 0 {
		// Only a secret was removed from the program. The API cannot clear one, so the
		// stored secret stays in use.
		return infer.UpdateResponse[VaultCredentialState]{Output: state}, nil
	}

	c, err := newClient(ctx)
	if err != nil {
		return infer.UpdateResponse[VaultCredentialState]{}, err
	}
	body, err := c.do(ctx, http.MethodPost, credentialPath(req.Inputs.VaultID, req.ID), payload)
	if err != nil {
		return infer.UpdateResponse[VaultCredentialState]{}, fmt.Errorf("updating vault credential %q: %w", req.ID, err)
	}
	credential, err := parseCredential(body)
	if err != nil {
		return infer.UpdateResponse[VaultCredentialState]{}, err
	}

	state.UpdatedAt = credential.UpdatedAt
	return infer.UpdateResponse[VaultCredentialState]{Output: state}, nil
}

// credentialUpdatePayload builds the body of an update request from what changed.
//
// Only what changed is sent. OpenAI refreshes OAuth tokens on its own, so sending an
// unchanged token from the program would overwrite a newer one.
func credentialUpdatePayload(olds, news VaultCredentialArgs) map[string]any {
	payload := map[string]any{}
	if !reflect.DeepEqual(encodeAPI(reflect.ValueOf(olds.Metadata)), encodeAPI(reflect.ValueOf(news.Metadata))) {
		// Supplied metadata replaces all existing metadata, so an empty object clears it.
		metadata := map[string]string{}
		for k, v := range news.Metadata {
			metadata[k] = v
		}
		payload["metadata"] = metadata
	}
	if auth := credentialRotation(olds.Auth, news.Auth); auth != nil {
		payload["auth"] = auth
	}
	return payload
}

// credentialRotation builds the `auth` field of an update request: the new values of the
// secrets that changed. It returns nil if there is nothing to rotate.
func credentialRotation(before, after CredentialAuth) map[string]any {
	auth := map[string]any{}
	switch after.Type {
	case staticBearerCredential:
		if rotated(before.Token, after.Token) {
			auth["token"] = *after.Token
		}
	case environmentVariableCredential:
		if rotated(before.SecretValue, after.SecretValue) {
			auth["secret_value"] = *after.SecretValue
		}
	case mcpOAuthCredential:
		newToken := rotated(before.AccessToken, after.AccessToken)
		if newToken {
			auth["access_token"] = *after.AccessToken
		}
		// A new access token clears the stored expiry unless one is sent with it.
		if newToken || !reflect.DeepEqual(before.ExpiresAt, after.ExpiresAt) {
			auth["expires_at"] = after.ExpiresAt
		}
		if before.Refresh != nil && after.Refresh != nil {
			if refresh := refreshRotation(*before.Refresh, *after.Refresh); len(refresh) > 0 {
				auth["refresh"] = refresh
			}
		}
	}
	if len(auth) == 0 {
		return nil
	}
	auth["type"] = string(after.Type)
	return auth
}

func refreshRotation(before, after CredentialOAuthRefresh) map[string]any {
	refresh := map[string]any{}
	if after.RefreshToken != "" && after.RefreshToken != before.RefreshToken {
		refresh["refresh_token"] = after.RefreshToken
	}
	if !reflect.DeepEqual(before.Scope, after.Scope) {
		// A null scope stops a scope from being sent when refreshing.
		refresh["scope"] = after.Scope
	}
	if rotated(before.TokenEndpointAuth.ClientSecret, after.TokenEndpointAuth.ClientSecret) {
		refresh["token_endpoint_auth"] = map[string]any{
			"type":          string(after.TokenEndpointAuth.Type),
			"client_secret": *after.TokenEndpointAuth.ClientSecret,
		}
	}
	return refresh
}

// rotated reports whether a secret was given a new value. A secret removed from the
// program cannot be cleared, so that is not a rotation.
func rotated(before, after *string) bool {
	return after != nil && (before == nil || *before != *after)
}

// Read refreshes a credential from the API. It also backs `pulumi import`, which takes an
// ID of the form `<vaultId>/<credentialId>`, since a credential is only found through its vault.
func (VaultCredential) Read(
	ctx context.Context, req infer.ReadRequest[VaultCredentialArgs, VaultCredentialState],
) (infer.ReadResponse[VaultCredentialArgs, VaultCredentialState], error) {
	vaultID, id := req.State.VaultID, req.ID
	if vaultID == "" {
		vaultID = req.Inputs.VaultID
	}
	if vaultID == "" {
		var ok bool
		vaultID, id, ok = strings.Cut(req.ID, "/")
		if !ok || vaultID == "" || id == "" {
			return infer.ReadResponse[VaultCredentialArgs, VaultCredentialState]{}, fmt.Errorf(
				"cannot find vault credential %q: import a vault credential with an ID of the form "+
					"`<vaultId>/<credentialId>`", req.ID)
		}
	}

	c, err := newClient(ctx)
	if err != nil {
		return infer.ReadResponse[VaultCredentialArgs, VaultCredentialState]{}, err
	}
	body, err := c.do(ctx, http.MethodGet, credentialPath(vaultID, id), nil)
	if isNotFound(err) {
		// The credential or its whole vault is gone. An empty ID tells the engine so.
		return infer.ReadResponse[VaultCredentialArgs, VaultCredentialState]{}, nil
	}
	if err != nil {
		return infer.ReadResponse[VaultCredentialArgs, VaultCredentialState]{}, fmt.Errorf(
			"reading vault credential %q: %w", id, err)
	}
	credential, err := parseCredential(body)
	if err != nil {
		return infer.ReadResponse[VaultCredentialArgs, VaultCredentialState]{}, err
	}
	inputs, err := refreshInputs(credentialRefresh, req.Inputs, req.Inputs.Name != "", body)
	if err != nil {
		return infer.ReadResponse[VaultCredentialArgs, VaultCredentialState]{}, fmt.Errorf(
			"reading vault credential %q: %w", id, err)
	}
	if req.Inputs.Name != "" {
		inputs.Name = keepUntrimmed(req.Inputs.Name, inputs.Name)
	}

	return infer.ReadResponse[VaultCredentialArgs, VaultCredentialState]{
		ID:     credential.ID,
		Inputs: inputs,
		State: VaultCredentialState{
			VaultCredentialArgs: inputs,
			CreatedAt:           credential.CreatedAt,
			UpdatedAt:           credential.UpdatedAt,
		},
	}, nil
}

// Delete deletes a credential. A credential that is already gone, or whose vault is, counts
// as deleted.
//
// Deleting a credential while its vault is being deleted meets a conflict, which happens
// when the credential does not reference the Vault resource. It is retried until the
// vault, and with it the credential, is gone.
func (VaultCredential) Delete(
	ctx context.Context, req infer.DeleteRequest[VaultCredentialState],
) (infer.DeleteResponse, error) {
	c, err := newClient(ctx)
	if err != nil {
		return infer.DeleteResponse{}, err
	}
	err = retryConflicts(ctx, func() error {
		_, err := c.do(ctx, http.MethodDelete, credentialPath(req.State.VaultID, req.ID), nil)
		if isNotFound(err) {
			return nil
		}
		return err
	})
	if err != nil {
		return infer.DeleteResponse{}, fmt.Errorf("deleting vault credential %q: %w", req.ID, err)
	}
	return infer.DeleteResponse{}, nil
}

// credentialRefresh describes how a credential's inputs are refreshed from the API.
var credentialRefresh = refreshSpec{
	freeForm: [][]string{{"metadata"}},
	writeOnly: [][]string{
		// The API never returns secrets.
		{"auth", "token"},
		{"auth", "access_token"},
		{"auth", "secret_value"},
		{"auth", "refresh", "refresh_token"},
		{"auth", "refresh", "token_endpoint_auth", "client_secret"},
		// OpenAI moves the expiry forward each time it refreshes the access token. That is
		// not drift: sending the program's expiry back would make a fresh token look stale.
		{"auth", "expires_at"},
	},
}
