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
	"strings"

	"github.com/pulumi/pulumi-go-provider/infer"
)

// Vault is the controller for the Vault resource, a store of credentials.
type Vault struct{}

// Annotate describes the Vault resource.
func (v *Vault) Annotate(an infer.Annotator) {
	an.Describe(&v, "A vault: a store of credentials that sessions use without the secrets appearing "+
		"in an agent's configuration. Add credentials to it with the `VaultCredential` resource, and make "+
		"it available to the sessions an automation starts with `target.vaultIds`.\n\n"+
		"Deleting a vault also deletes all of its credentials.\n\n"+
		"This resource uses the beta Agents API.")
}

// VaultArgs are the inputs of the Vault resource.
type VaultArgs struct {
	Name     *string           `pulumi:"name,optional"`
	Metadata map[string]string `pulumi:"metadata,optional"`
}

// Annotate describes the inputs of the Vault resource.
func (a *VaultArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.Name, "A human-readable name for the vault, 1 to 256 bytes once leading and "+
		"trailing whitespace is trimmed.")
	an.Describe(&a.Metadata, "Up to 16 custom key-value pairs, such as an application or team identifier. "+
		"Keys can be up to 64 characters and values up to 512 characters.")
}

// VaultState is the persisted state of a Vault.
type VaultState struct {
	VaultArgs

	CreatedAt int `pulumi:"createdAt"`
}

// Annotate describes the outputs of the Vault resource.
func (s *VaultState) Annotate(an infer.Annotator) {
	an.Describe(&s.CreatedAt, "When the vault was created, as a Unix timestamp in seconds.")
}

// vaultObject is an API vault object.
type vaultObject struct {
	ID        string            `json:"id"`
	Name      *string           `json:"name"`
	Metadata  map[string]string `json:"metadata"`
	CreatedAt int               `json:"created_at"`
}

func vaultPath(id string) string {
	return "/vaults/" + url.PathEscape(id)
}

func parseVault(body []byte) (vaultObject, error) {
	var vault vaultObject
	if err := json.Unmarshal(body, &vault); err != nil {
		return vaultObject{}, fmt.Errorf("decoding vault: %w", err)
	}
	return vault, nil
}

// Create creates a new vault.
func (Vault) Create(ctx context.Context, req infer.CreateRequest[VaultArgs]) (infer.CreateResponse[VaultState], error) {
	state := VaultState{VaultArgs: req.Inputs}
	if req.DryRun {
		return infer.CreateResponse[VaultState]{Output: state}, nil
	}

	c, err := newClient(ctx)
	if err != nil {
		return infer.CreateResponse[VaultState]{}, err
	}
	body, err := c.do(ctx, http.MethodPost, "/vaults", toAPI(req.Inputs))
	if err != nil {
		return infer.CreateResponse[VaultState]{}, fmt.Errorf("creating vault: %w", err)
	}
	vault, err := parseVault(body)
	if err != nil {
		return infer.CreateResponse[VaultState]{}, err
	}
	if vault.ID == "" {
		return infer.CreateResponse[VaultState]{}, fmt.Errorf("creating vault: the API returned no vault ID")
	}

	state.CreatedAt = vault.CreatedAt
	return infer.CreateResponse[VaultState]{ID: vault.ID, Output: state}, nil
}

// Update updates a vault in place. Both inputs can be changed without replacement.
func (Vault) Update(
	ctx context.Context, req infer.UpdateRequest[VaultArgs, VaultState],
) (infer.UpdateResponse[VaultState], error) {
	state := VaultState{VaultArgs: req.Inputs, CreatedAt: req.State.CreatedAt}
	if req.DryRun {
		return infer.UpdateResponse[VaultState]{Output: state}, nil
	}

	// The update endpoint leaves omitted fields unchanged, so both are always sent: a null
	// name clears the name, and an empty object clears the metadata.
	payload := map[string]any{"name": nil, "metadata": map[string]string{}}
	if req.Inputs.Name != nil {
		payload["name"] = *req.Inputs.Name
	}
	if len(req.Inputs.Metadata) > 0 {
		payload["metadata"] = req.Inputs.Metadata
	}

	c, err := newClient(ctx)
	if err != nil {
		return infer.UpdateResponse[VaultState]{}, err
	}
	if _, err := c.do(ctx, http.MethodPost, vaultPath(req.ID), payload); err != nil {
		return infer.UpdateResponse[VaultState]{}, fmt.Errorf("updating vault %q: %w", req.ID, err)
	}
	return infer.UpdateResponse[VaultState]{Output: state}, nil
}

// Read refreshes a vault from the API. It also backs `pulumi import`.
//
// The API fills in no defaults for a vault, so its remote configuration is adopted as is.
func (Vault) Read(
	ctx context.Context, req infer.ReadRequest[VaultArgs, VaultState],
) (infer.ReadResponse[VaultArgs, VaultState], error) {
	c, err := newClient(ctx)
	if err != nil {
		return infer.ReadResponse[VaultArgs, VaultState]{}, err
	}
	body, err := c.do(ctx, http.MethodGet, vaultPath(req.ID), nil)
	if isNotFound(err) {
		// An empty ID tells the engine the resource no longer exists.
		return infer.ReadResponse[VaultArgs, VaultState]{}, nil
	}
	if err != nil {
		return infer.ReadResponse[VaultArgs, VaultState]{}, fmt.Errorf("reading vault %q: %w", req.ID, err)
	}
	vault, err := parseVault(body)
	if err != nil {
		return infer.ReadResponse[VaultArgs, VaultState]{}, err
	}

	inputs := VaultArgs{Name: vault.Name}
	if req.Inputs.Name != nil && vault.Name != nil {
		inputs.Name = ptr(keepUntrimmed(*req.Inputs.Name, *vault.Name))
	}
	if len(vault.Metadata) > 0 {
		inputs.Metadata = vault.Metadata
	}
	return infer.ReadResponse[VaultArgs, VaultState]{
		ID:     req.ID,
		Inputs: inputs,
		State:  VaultState{VaultArgs: inputs, CreatedAt: vault.CreatedAt},
	}, nil
}

// Delete deletes a vault and all of its credentials. A vault that is already gone counts
// as deleted.
//
// Pulumi deletes a credential before its vault when the credential references the Vault
// resource. When it does not, the two deletions can run at once, and the API then answers
// with conflicts, or with a 404 for a vault that still exists. Both are retried.
func (Vault) Delete(ctx context.Context, req infer.DeleteRequest[VaultState]) (infer.DeleteResponse, error) {
	c, err := newClient(ctx)
	if err != nil {
		return infer.DeleteResponse{}, err
	}
	err = retryConflicts(ctx, func() error {
		_, err := c.do(ctx, http.MethodDelete, vaultPath(req.ID), nil)
		if !isNotFound(err) {
			return err
		}
		_, err = c.do(ctx, http.MethodGet, vaultPath(req.ID), nil)
		if isNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		return &apiError{StatusCode: http.StatusConflict, Message: "the vault still exists after it was deleted"}
	})
	if err != nil {
		return infer.DeleteResponse{}, fmt.Errorf("deleting vault %q: %w", req.ID, err)
	}
	return infer.DeleteResponse{}, nil
}

// keepUntrimmed returns the name the user wrote if the API stored the same name with the
// surrounding whitespace trimmed, so that the trimming is not reported as drift.
func keepUntrimmed(prior, remote string) string {
	if strings.TrimSpace(prior) == remote {
		return prior
	}
	return remote
}

func ptr[T any](v T) *T {
	return &v
}
