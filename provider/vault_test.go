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
	"net/http"
	"slices"
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
	vaultToken      = "openai:index:Vault"
	credentialToken = "openai:index:VaultCredential"
)

func resourceURN(token string) presource.URN {
	return presource.NewURN("test", "provider", "", tokens.Type(token), "test")
}

// The fake API below behaves as the real one was observed to: names are trimmed, secrets
// are never returned, an update can only rotate secrets and change metadata, and a vault
// holds one environment variable credential per variable name.

// credentialSecrets are the paths below `auth` of the values the API never returns.
var credentialSecrets = [][]string{
	{"token"},
	{"access_token"},
	{"secret_value"},
	{"refresh", "refresh_token"},
	{"refresh", "token_endpoint_auth", "client_secret"},
}

// handleVault serves the vaults endpoints of the fake API, credentials included.
func (api *fakeAPI) handleVault(w http.ResponseWriter, r *http.Request, body map[string]any) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/vaults"), "/"), "/")
	vaultID := parts[0]
	if len(parts) > 1 {
		credentialID := ""
		if len(parts) > 2 {
			credentialID = parts[2]
		}
		api.handleCredential(w, r, body, vaultID, credentialID)
		return
	}

	switch {
	case r.Method == http.MethodPost && vaultID == "":
		api.nextID++
		vault := map[string]any{"object": "vault", "name": nil, "metadata": map[string]any{}}
		applyVault(vault, body)
		vault["id"] = fmt.Sprintf("vault_%d", api.nextID)
		vault["created_at"] = api.clock
		api.vaults[vault["id"].(string)] = vault
		api.respond(w, http.StatusCreated, vault)

	case api.vaults[vaultID] == nil:
		api.fail(w, http.StatusNotFound, "No vault resource found: "+vaultID)

	case r.Method == http.MethodGet:
		api.respond(w, http.StatusOK, api.vaults[vaultID])

	case r.Method == http.MethodPost:
		applyVault(api.vaults[vaultID], body)
		api.respond(w, http.StatusOK, api.vaults[vaultID])

	case r.Method == http.MethodDelete:
		// Deleting a vault deletes its credentials.
		delete(api.vaults, vaultID)
		for id, credential := range api.credentials {
			if credential["vault_id"] == vaultID {
				delete(api.credentials, id)
			}
		}
		api.respond(w, http.StatusOK, map[string]any{"id": vaultID, "deleted": true, "object": "vault.deleted"})

	default:
		api.fail(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func applyVault(vault, body map[string]any) {
	for k, v := range body {
		if name, ok := v.(string); ok && k == "name" {
			v = strings.TrimSpace(name)
		}
		vault[k] = v
	}
}

func (api *fakeAPI) handleCredential(
	w http.ResponseWriter, r *http.Request, body map[string]any, vaultID, id string,
) {
	if api.vaults[vaultID] == nil {
		api.fail(w, http.StatusNotFound, "No vault resource found: "+vaultID)
		return
	}
	credential := api.credentials[id]
	if id != "" && (credential == nil || credential["vault_id"] != vaultID) {
		api.fail(w, http.StatusNotFound, "No vault resource found: "+id)
		return
	}

	switch {
	case r.Method == http.MethodPost && id == "":
		name, _ := body["name"].(string)
		auth, _ := body["auth"].(map[string]any)
		if name == "" || auth == nil {
			api.fail(w, http.StatusBadRequest, "Missing required parameter.")
			return
		}
		env := string(environmentVariableCredential)
		if auth["type"] == env {
			for _, other := range api.credentials {
				otherAuth := other["auth"].(map[string]any)
				if other["vault_id"] == vaultID && otherAuth["type"] == env &&
					otherAuth["secret_name"] == auth["secret_name"] {
					api.fail(w, http.StatusConflict, "an active credential with this key already exists in the vault")
					return
				}
			}
		}
		if auth["type"] == string(mcpOAuthCredential) {
			setDefault(auth, "expires_at", nil)
			setDefault(auth, "refresh", nil)
			if refresh, ok := auth["refresh"].(map[string]any); ok {
				setDefault(refresh, "resource", nil)
				setDefault(refresh, "scope", nil)
			}
		}
		api.nextID++
		credential = map[string]any{"object": "vault.credential", "metadata": map[string]any{}}
		for k, v := range body {
			credential[k] = v
		}
		credential["name"] = strings.TrimSpace(name)
		credential["id"] = fmt.Sprintf("credential_%d", api.nextID)
		credential["vault_id"] = vaultID
		credential["created_at"], credential["updated_at"] = api.clock, api.clock
		api.credentials[credential["id"].(string)] = credential
		api.respond(w, http.StatusCreated, withoutSecrets(credential))

	case r.Method == http.MethodGet:
		api.respond(w, http.StatusOK, withoutSecrets(credential))

	case r.Method == http.MethodPost:
		if message := rotateCredential(credential, body); message != "" {
			api.fail(w, http.StatusBadRequest, message)
			return
		}
		credential["updated_at"] = api.clock
		api.respond(w, http.StatusOK, withoutSecrets(credential))

	case r.Method == http.MethodDelete:
		delete(api.credentials, id)
		api.respond(w, http.StatusOK, map[string]any{"id": id, "deleted": true, "object": "vault.credential.deleted"})

	default:
		api.fail(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// rotateCredential applies an update to a stored credential. It returns the API's error
// message for an update the real API rejects.
func rotateCredential(credential, body map[string]any) string {
	for key := range body {
		if key != "metadata" && key != "auth" {
			return "Unknown parameter: '" + key + "'."
		}
	}
	if metadata, ok := body["metadata"]; ok {
		credential["metadata"] = metadata
	}
	auth, ok := body["auth"].(map[string]any)
	if !ok {
		return ""
	}
	stored := credential["auth"].(map[string]any)
	if auth["type"] != stored["type"] {
		return "credential auth type cannot be changed"
	}
	rotatable := map[string][]string{
		"static_bearer":        {"type", "token"},
		"environment_variable": {"type", "secret_value"},
		"mcp_oauth":            {"type", "access_token", "expires_at", "refresh"},
	}[stored["type"].(string)]
	for key := range auth {
		if !slices.Contains(rotatable, key) {
			return "Unknown parameter: 'auth." + key + "'."
		}
	}
	if refresh, ok := auth["refresh"].(map[string]any); ok {
		storedRefresh, ok := stored["refresh"].(map[string]any)
		if !ok {
			return "the credential has no refresh configuration"
		}
		for key, value := range refresh {
			switch key {
			case "refresh_token", "scope":
				storedRefresh[key] = value
			case "token_endpoint_auth":
				storedAuth := storedRefresh["token_endpoint_auth"].(map[string]any)
				if value.(map[string]any)["type"] != storedAuth["type"] {
					return "refresh.token_endpoint_auth.type cannot be changed"
				}
				storedRefresh[key] = value
			default:
				return "Unknown parameter: 'auth.refresh." + key + "'."
			}
		}
	}
	// A new access token clears the stored expiry unless one is sent with it.
	if _, newToken := auth["access_token"]; newToken {
		if _, ok := auth["expires_at"]; !ok {
			stored["expires_at"] = nil
		}
	}
	for key, value := range auth {
		if key != "refresh" {
			stored[key] = value
		}
	}
	return ""
}

func setDefault(object map[string]any, key string, value any) {
	if _, ok := object[key]; !ok {
		object[key] = value
	}
}

func withoutSecrets(credential map[string]any) map[string]any {
	view := deepCopy(credential)
	for _, path := range credentialSecrets {
		object, _ := view["auth"].(map[string]any)
		for _, key := range path[:len(path)-1] {
			object, _ = object[key].(map[string]any)
		}
		delete(object, path[len(path)-1])
	}
	return view
}

func deepCopy(object map[string]any) map[string]any {
	encoded, err := json.Marshal(object)
	if err != nil {
		panic(err)
	}
	var copied map[string]any
	if err := json.Unmarshal(encoded, &copied); err != nil {
		panic(err)
	}
	return copied
}

// with returns a copy of inputs with the value at a dotted path replaced, or removed if
// value is nil.
func with(inputs map[string]any, path string, value any) map[string]any {
	copied := deepCopy(inputs)
	keys := strings.Split(path, ".")
	object := copied
	for _, key := range keys[:len(keys)-1] {
		object = object[key].(map[string]any)
	}
	if value == nil {
		delete(object, keys[len(keys)-1])
	} else {
		object[keys[len(keys)-1]] = value
	}
	return copied
}

// storedAuth returns a credential's auth as the fake API stores it, secrets included.
func (api *fakeAPI) storedAuth(id string) map[string]any {
	api.mu.Lock()
	defer api.mu.Unlock()
	return deepCopy(api.credentials[id])["auth"].(map[string]any)
}

// createVault creates a vault for credentials to go in.
func createVault(t *testing.T, server integration.Server) string {
	created, err := server.Create(p.CreateRequest{
		Urn: resourceURN(vaultToken), Properties: props(map[string]any{"name": "test"}),
	})
	require.NoError(t, err)
	return created.ID
}

func bearerInputs(vaultID string) map[string]any {
	return map[string]any{
		"vaultId":  vaultID,
		"name":     "docs-mcp",
		"metadata": map[string]any{"team": "support"},
		"auth": map[string]any{
			"type":         "static_bearer",
			"mcpServerUrl": "https://mcp.example.com/mcp",
			"token":        "token-1",
		},
	}
}

func oauthInputs(vaultID string) map[string]any {
	return map[string]any{
		"vaultId": vaultID,
		"name":    "github-mcp",
		"auth": map[string]any{
			"type":         "mcp_oauth",
			"mcpServerUrl": "https://mcp.example.com/oauth",
			"accessToken":  "access-1",
			"expiresAt":    "2030-01-01T00:00:00Z",
			"refresh": map[string]any{
				"clientId":      "client-1",
				"refreshToken":  "refresh-1",
				"tokenEndpoint": "https://auth.example.com/token",
				"tokenEndpointAuth": map[string]any{
					"type": "client_secret_basic", "clientSecret": "client-secret-1",
				},
				"scope": "read write",
			},
		},
	}
}

func envInputs(vaultID string) map[string]any {
	return map[string]any{
		"vaultId": vaultID,
		"name":    "github-token",
		"auth": map[string]any{
			"type":        "environment_variable",
			"secretName":  "GITHUB_TOKEN",
			"secretValue": "ghp-1",
			"networking":  map[string]any{"type": "limited", "allowedHosts": []any{"api.github.com"}},
		},
	}
}

func TestVaultLifecycle(t *testing.T) {
	t.Parallel()
	api := newFakeAPI(t)
	server := newServer(t, api)

	integration.LifeCycleTest{
		Resource: vaultToken,
		Create: integration.Operation{
			Inputs: props(map[string]any{"name": "github", "metadata": map[string]any{"team": "support"}}),
			Hook: func(_, output property.Map) {
				assert.Equal(t, "github", output.Get("name").AsString())
				assert.Equal(t, 1700000001.0, output.Get("createdAt").AsNumber())

				create := api.last(http.MethodPost)
				assert.Equal(t, "/v1/vaults", create.Path)
				assert.Equal(t, map[string]any{
					"name": "github", "metadata": map[string]any{"team": "support"},
				}, create.Body)
			},
		},
		Updates: []integration.Operation{
			{
				// Renaming happens in place, and removed metadata is cleared.
				Inputs: props(map[string]any{"name": "github-prod"}),
				Hook: func(_, output property.Map) {
					assert.Equal(t, "github-prod", output.Get("name").AsString())
					assert.Equal(t, 1700000001.0, output.Get("createdAt").AsNumber())

					update := api.last(http.MethodPost)
					assert.Equal(t, "/v1/vaults/vault_1", update.Path)
					assert.Equal(t, map[string]any{"name": "github-prod", "metadata": map[string]any{}}, update.Body)
				},
			},
			{
				// A removed name is cleared too.
				Inputs: props(map[string]any{}),
				Hook: func(_, output property.Map) {
					assert.True(t, output.Get("name").IsNull())
					assert.Equal(t, map[string]any{"name": nil, "metadata": map[string]any{}},
						api.last(http.MethodPost).Body)
				},
			},
		},
	}.Run(t, server)

	assert.Equal(t, "/v1/vaults/vault_1", api.last(http.MethodDelete).Path)
	assert.Empty(t, api.vaults)
}

func TestVaultRead(t *testing.T) {
	t.Parallel()
	api := newFakeAPI(t)
	server := newServer(t, api)
	urn := resourceURN(vaultToken)
	inputs := props(map[string]any{"name": "  github  ", "metadata": map[string]any{"team": "support"}})
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
		response, err := server.Diff(p.DiffRequest{ID: created.ID, Urn: urn, State: state, Inputs: inputs})
		require.NoError(t, err)
		return response
	}

	// The API trims the name. That is not drift.
	assert.Equal(t, "github", api.vaults[created.ID]["name"])
	refreshed := read(inputs)
	assert.Equal(t, created.ID, refreshed.ID)
	noDrift := diff(refreshed.Properties)
	assert.False(t, noDrift.HasChanges, "unexpected diff: %v", noDrift.DetailedDiff)

	// A change made elsewhere is picked up.
	api.mu.Lock()
	api.vaults[created.ID]["metadata"] = map[string]any{"team": "sales"}
	api.mu.Unlock()
	refreshed = read(inputs)
	assert.Equal(t, "sales", refreshed.Inputs.Get("metadata").AsMap().Get("team").AsString())
	drift := diff(refreshed.Properties)
	assert.True(t, drift.HasChanges)
	assert.Equal(t, p.Update, drift.DetailedDiff["metadata.team"].Kind)

	// An import adopts the remote configuration.
	imported := read(property.Map{})
	assert.Equal(t, "github", imported.Inputs.Get("name").AsString())
	assert.Equal(t, "sales", imported.Inputs.Get("metadata").AsMap().Get("team").AsString())

	// A vault deleted elsewhere reads as missing, and deleting it again is a no-op.
	api.mu.Lock()
	delete(api.vaults, created.ID)
	api.mu.Unlock()
	assert.Empty(t, read(inputs).ID)
	require.NoError(t, server.Delete(p.DeleteRequest{ID: created.ID, Urn: urn, Properties: created.Properties}))
}

func TestVaultCredentialLifecycle(t *testing.T) {
	t.Parallel()
	api := newFakeAPI(t)
	server := newServer(t, api)
	vaultID := createVault(t, server)

	rotated := with(bearerInputs(vaultID), "auth.token", "token-2")
	untagged := with(rotated, "metadata", nil)

	integration.LifeCycleTest{
		Resource: credentialToken,
		Create: integration.Operation{
			Inputs: props(bearerInputs(vaultID)),
			Hook: func(_, output property.Map) {
				// The vault is part of the path, not the body.
				create := api.last(http.MethodPost)
				assert.Equal(t, "/v1/vaults/vault_1/credentials", create.Path)
				assert.Equal(t, map[string]any{
					"name":     "docs-mcp",
					"metadata": map[string]any{"team": "support"},
					"auth": map[string]any{
						"type":           "static_bearer",
						"mcp_server_url": "https://mcp.example.com/mcp",
						"token":          "token-1",
					},
				}, create.Body)

				// The token is kept in the state, as a secret.
				token := output.Get("auth").AsMap().Get("token")
				assert.True(t, token.Secret())
				assert.Equal(t, "token-1", token.AsString())
				assert.Equal(t, vaultID, output.Get("vaultId").AsString())
				assert.Equal(t, 1700000002.0, output.Get("createdAt").AsNumber())
			},
		},
		Updates: []integration.Operation{
			{
				// A new token is rotated in place.
				Inputs: props(rotated),
				Hook: func(_, output property.Map) {
					update := api.last(http.MethodPost)
					assert.Equal(t, "/v1/vaults/vault_1/credentials/credential_2", update.Path)
					assert.Equal(t, map[string]any{
						"auth": map[string]any{"type": "static_bearer", "token": "token-2"},
					}, update.Body)
					assert.Equal(t, "token-2", api.storedAuth("credential_2")["token"])
					assert.Greater(t, output.Get("updatedAt").AsNumber(), output.Get("createdAt").AsNumber())
				},
			},
			{
				// A metadata change leaves the token alone, and removed metadata is cleared.
				Inputs: props(untagged),
				Hook: func(_, _ property.Map) {
					assert.Equal(t, map[string]any{"metadata": map[string]any{}}, api.last(http.MethodPost).Body)
				},
			},
		},
	}.Run(t, server)

	assert.Equal(t, "/v1/vaults/vault_1/credentials/credential_2", api.last(http.MethodDelete).Path)
	assert.Empty(t, api.credentials)
}

func TestVaultCredentialOAuthRotation(t *testing.T) {
	t.Parallel()
	api := newFakeAPI(t)
	server := newServer(t, api)
	vaultID := createVault(t, server)

	newToken := with(oauthInputs(vaultID), "auth.accessToken", "access-2")
	noExpiry := with(newToken, "auth.expiresAt", nil)
	newScope := with(with(noExpiry, "auth.refresh.scope", "read"),
		"auth.refresh.tokenEndpointAuth.clientSecret", "client-secret-2")
	noScope := with(newScope, "auth.refresh.scope", nil)

	update := func(want map[string]any) func(_, _ property.Map) {
		return func(_, _ property.Map) {
			assert.Equal(t, map[string]any{"auth": want}, api.last(http.MethodPost).Body)
		}
	}

	integration.LifeCycleTest{
		Resource: credentialToken,
		Create: integration.Operation{
			Inputs: props(oauthInputs(vaultID)),
			Hook: func(_, _ property.Map) {
				assert.Equal(t, map[string]any{
					"name": "github-mcp",
					"auth": map[string]any{
						"type":           "mcp_oauth",
						"mcp_server_url": "https://mcp.example.com/oauth",
						"access_token":   "access-1",
						"expires_at":     "2030-01-01T00:00:00Z",
						"refresh": map[string]any{
							"client_id":      "client-1",
							"refresh_token":  "refresh-1",
							"token_endpoint": "https://auth.example.com/token",
							"token_endpoint_auth": map[string]any{
								"type": "client_secret_basic", "client_secret": "client-secret-1",
							},
							"scope": "read write",
						},
					},
				}, api.last(http.MethodPost).Body)
			},
		},
		Updates: []integration.Operation{
			{
				// A new access token is sent with its expiry, which it would otherwise clear.
				Inputs: props(newToken),
				Hook: func(input, output property.Map) {
					update(map[string]any{
						"type": "mcp_oauth", "access_token": "access-2", "expires_at": "2030-01-01T00:00:00Z",
					})(input, output)
					assert.Equal(t, "2030-01-01T00:00:00Z", api.storedAuth("credential_2")["expires_at"])
				},
			},
			{
				// A removed expiry is cleared.
				Inputs: props(noExpiry),
				Hook:   update(map[string]any{"type": "mcp_oauth", "expires_at": nil}),
			},
			{
				// Only the refresh settings that changed are sent: the stored refresh token,
				// which OpenAI may have replaced, is left alone.
				Inputs: props(newScope),
				Hook: func(input, output property.Map) {
					update(map[string]any{"type": "mcp_oauth", "refresh": map[string]any{
						"scope": "read",
						"token_endpoint_auth": map[string]any{
							"type": "client_secret_basic", "client_secret": "client-secret-2",
						},
					}})(input, output)
					refresh := api.storedAuth("credential_2")["refresh"].(map[string]any)
					assert.Equal(t, "refresh-1", refresh["refresh_token"])
				},
			},
			{
				// A removed scope is cleared.
				Inputs: props(noScope),
				Hook:   update(map[string]any{"type": "mcp_oauth", "refresh": map[string]any{"scope": nil}}),
			},
		},
	}.Run(t, server)
	assert.Empty(t, api.credentials)
}

func TestVaultCredentialReplacement(t *testing.T) {
	t.Parallel()
	api := newFakeAPI(t)
	server := newServer(t, api)
	vaultID := createVault(t, server)

	widened := with(envInputs(vaultID), "auth.networking", map[string]any{"type": "unrestricted"})
	renamedVariable := with(widened, "auth.secretName", "GH_TOKEN")

	integration.LifeCycleTest{
		Resource: credentialToken,
		Create: integration.Operation{
			Inputs: props(envInputs(vaultID)),
			Hook: func(_, _ property.Map) {
				assert.Equal(t, map[string]any{
					"name": "github-token",
					"auth": map[string]any{
						"type":         "environment_variable",
						"secret_name":  "GITHUB_TOKEN",
						"secret_value": "ghp-1",
						"networking":   map[string]any{"type": "limited", "allowed_hosts": []any{"api.github.com"}},
					},
				}, api.last(http.MethodPost).Body)
			},
		},
		Updates: []integration.Operation{
			{
				// The API cannot change the networking, so the credential is replaced. It
				// keeps its variable name, which the vault holds once, so the old credential
				// is deleted first: otherwise the fake API, like the real one, rejects the
				// new credential.
				Inputs: props(widened),
				Hook: func(_, _ property.Map) {
					assert.Equal(t, "/v1/vaults/vault_1/credentials/credential_2", api.last(http.MethodDelete).Path)
					assert.Len(t, api.credentials, 1)
					assert.Equal(t, map[string]any{"type": "unrestricted"},
						api.storedAuth("credential_3")["networking"])
				},
			},
			{
				// A credential under a new variable name is created before the old one is deleted.
				Inputs: props(renamedVariable),
				Hook: func(_, _ property.Map) {
					assert.Len(t, api.credentials, 2)
					assert.Equal(t, "GH_TOKEN", api.storedAuth("credential_4")["secret_name"])
				},
			},
		},
	}.Run(t, server)
	assert.Empty(t, api.credentials)
}

func TestVaultCredentialDiff(t *testing.T) {
	t.Parallel()
	server := newServer(t, newFakeAPI(t))
	urn := resourceURN(credentialToken)

	bearer, oauth, env := bearerInputs("vault_1"), oauthInputs("vault_1"), envInputs("vault_1")
	for name, test := range map[string]struct {
		olds, news  map[string]any
		want        map[string]p.DiffKind
		deleteFirst bool
	}{
		"no change": {olds: bearer, news: bearer, want: map[string]p.DiffKind{}},
		"rotating a token": {
			olds: bearer, news: with(bearer, "auth.token", "token-2"),
			want: map[string]p.DiffKind{"auth.token": p.Update},
		},
		"changing metadata": {
			olds: bearer, news: with(bearer, "metadata", map[string]any{"team": "sales"}),
			want: map[string]p.DiffKind{"metadata": p.Update},
		},
		"renaming": {
			olds: bearer, news: with(bearer, "name", "renamed"),
			want: map[string]p.DiffKind{"name": p.UpdateReplace},
		},
		"moving to another vault": {
			olds: bearer, news: with(bearer, "vaultId", "vault_2"),
			want: map[string]p.DiffKind{"vaultId": p.UpdateReplace},
		},
		"changing the MCP server": {
			olds: bearer, news: with(bearer, "auth.mcpServerUrl", "https://other.example.com/mcp"),
			want: map[string]p.DiffKind{"auth.mcpServerUrl": p.UpdateReplace},
		},
		"changing the type": {
			olds: bearer,
			news: with(with(with(bearer, "auth.type", "mcp_oauth"), "auth.token", nil), "auth.accessToken", "access-1"),
			want: map[string]p.DiffKind{
				"auth.type": p.UpdateReplace, "auth.token": p.Delete, "auth.accessToken": p.Add,
			},
		},
		"rotating OAuth secrets and dropping the scope": {
			olds: oauth,
			news: with(with(with(oauth, "auth.accessToken", "access-2"),
				"auth.refresh.refreshToken", "refresh-2"), "auth.refresh.scope", nil),
			want: map[string]p.DiffKind{
				"auth.accessToken": p.Update, "auth.refresh.refreshToken": p.Update, "auth.refresh.scope": p.Delete,
			},
		},
		"changing the OAuth client": {
			olds: oauth, news: with(oauth, "auth.refresh.clientId", "client-2"),
			want: map[string]p.DiffKind{"auth.refresh.clientId": p.UpdateReplace},
		},
		"changing how the client authenticates": {
			olds: oauth, news: with(oauth, "auth.refresh.tokenEndpointAuth", map[string]any{"type": "none"}),
			want: map[string]p.DiffKind{
				"auth.refresh.tokenEndpointAuth.type":         p.UpdateReplace,
				"auth.refresh.tokenEndpointAuth.clientSecret": p.Delete,
			},
		},
		"rotating an environment secret": {
			olds: env, news: with(env, "auth.secretValue", "ghp-2"),
			want: map[string]p.DiffKind{"auth.secretValue": p.Update},
		},
		"changing the networking of a variable": {
			olds: env, news: with(env, "auth.networking", map[string]any{"type": "unrestricted"}),
			want: map[string]p.DiffKind{
				"auth.networking.type": p.UpdateReplace, "auth.networking.allowedHosts": p.DeleteReplace,
			},
			deleteFirst: true,
		},
		"renaming a variable": {
			olds: env, news: with(env, "auth.secretName", "GH_TOKEN"),
			want: map[string]p.DiffKind{"auth.secretName": p.UpdateReplace},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			state, err := server.Create(p.CreateRequest{Urn: urn, Properties: props(test.olds), DryRun: true})
			require.NoError(t, err)
			diff, err := server.Diff(p.DiffRequest{
				ID: "credential_1", Urn: urn, State: state.Properties, Inputs: props(test.news),
			})
			require.NoError(t, err)

			kinds := map[string]p.DiffKind{}
			for path, change := range diff.DetailedDiff {
				kinds[path] = change.Kind
			}
			assert.Equal(t, test.want, kinds)
			assert.Equal(t, len(test.want) > 0, diff.HasChanges)
			assert.Equal(t, test.deleteFirst, diff.DeleteBeforeReplace)
		})
	}
}

func TestVaultCredentialRead(t *testing.T) {
	t.Parallel()
	api := newFakeAPI(t)
	server := newServer(t, api)
	vaultID := createVault(t, server)
	urn := resourceURN(credentialToken)
	inputs := props(oauthInputs(vaultID))
	created, err := server.Create(p.CreateRequest{Urn: urn, Properties: inputs})
	require.NoError(t, err)

	read := func(id string, inputs, state property.Map) (p.ReadResponse, error) {
		return server.Read(p.ReadRequest{ID: id, Urn: urn, Properties: state, Inputs: inputs})
	}
	refresh := func() p.ReadResponse {
		response, err := read(created.ID, inputs, created.Properties)
		require.NoError(t, err)
		return response
	}
	diff := func(state property.Map) p.DiffResponse {
		response, err := server.Diff(p.DiffRequest{ID: created.ID, Urn: urn, State: state, Inputs: inputs})
		require.NoError(t, err)
		return response
	}

	// Without drift there is no diff: the secrets the API never returns keep their values.
	refreshed := refresh()
	assert.Equal(t, created.ID, refreshed.ID)
	noDrift := diff(refreshed.Properties)
	assert.False(t, noDrift.HasChanges, "unexpected diff: %v", noDrift.DetailedDiff)
	assert.Equal(t, "access-1", refreshed.Inputs.Get("auth").AsMap().Get("accessToken").AsString())

	// OpenAI moves the expiry when it refreshes the access token. That is not drift.
	api.mu.Lock()
	api.credentials[created.ID]["auth"].(map[string]any)["expires_at"] = "2030-06-01T00:00:00Z"
	api.mu.Unlock()
	refreshed = refresh()
	assert.Equal(t, "2030-01-01T00:00:00Z", refreshed.Inputs.Get("auth").AsMap().Get("expiresAt").AsString())
	assert.False(t, diff(refreshed.Properties).HasChanges)

	// A change made elsewhere is picked up, and diffed as a change made in place.
	api.mu.Lock()
	api.credentials[created.ID]["auth"].(map[string]any)["refresh"].(map[string]any)["scope"] = "read"
	api.mu.Unlock()
	refreshed = refresh()
	assert.Equal(t, "read", refreshed.Inputs.Get("auth").AsMap().Get("refresh").AsMap().Get("scope").AsString())
	drift := diff(refreshed.Properties)
	assert.Equal(t, map[string]p.PropertyDiff{"auth.refresh.scope": {Kind: p.Update, InputDiff: true}}, drift.DetailedDiff)

	// An import takes the vault and the credential together, and adopts the remote
	// configuration. The secrets are not returned, so they are left out.
	imported, err := read(vaultID+"/"+created.ID, property.Map{}, property.Map{})
	require.NoError(t, err)
	assert.Equal(t, created.ID, imported.ID)
	assert.Equal(t, vaultID, imported.Inputs.Get("vaultId").AsString())
	assert.Equal(t, "github-mcp", imported.Inputs.Get("name").AsString())
	auth := imported.Inputs.Get("auth").AsMap()
	assert.Equal(t, "mcp_oauth", auth.Get("type").AsString())
	assert.Equal(t, "2030-06-01T00:00:00Z", auth.Get("expiresAt").AsString())
	assert.Equal(t, "client-1", auth.Get("refresh").AsMap().Get("clientId").AsString())
	assert.True(t, auth.Get("accessToken").IsNull())

	// An import ID without the vault is rejected with the form it should take.
	_, err = read(created.ID, property.Map{}, property.Map{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "`<vaultId>/<credentialId>`")

	// A credential whose vault was deleted reads as missing, and deleting it is a no-op.
	api.mu.Lock()
	delete(api.vaults, vaultID)
	api.mu.Unlock()
	assert.Empty(t, refresh().ID)
	require.NoError(t, server.Delete(p.DeleteRequest{ID: created.ID, Urn: urn, Properties: created.Properties}))
}

func TestVaultCredentialWithAVaultCreatedInTheSameUpdate(t *testing.T) {
	t.Parallel()
	api := newFakeAPI(t)
	server := newServer(t, api)
	urn := resourceURN(credentialToken)

	// The ID of a vault created in the same update is unknown during preview. That is not
	// the same as missing, and the preview does not call the API.
	inputs := props(bearerInputs("")).Set("vaultId", property.New(property.Computed))
	check, err := server.Check(p.CheckRequest{Urn: urn, Inputs: inputs})
	require.NoError(t, err)
	assert.Empty(t, check.Failures)
	_, err = server.Create(p.CreateRequest{Urn: urn, Properties: check.Inputs, DryRun: true})
	require.NoError(t, err)
	assert.Empty(t, api.requests)
}

// When a credential does not reference the Vault resource, as after `pulumi import`,
// Pulumi can delete it and its vault at once. The real API then answers with conflicts,
// and even with a 404 for a vault that still exists.
func TestDeletingAVaultAndItsCredentialAtOnce(t *testing.T) {
	t.Parallel()
	api := newFakeAPI(t)
	server := newServer(t, api)
	vault, err := server.Create(p.CreateRequest{Urn: resourceURN(vaultToken), Properties: props(map[string]any{})})
	require.NoError(t, err)
	vaultID := vault.ID
	urn := resourceURN(credentialToken)
	created, err := server.Create(p.CreateRequest{Urn: urn, Properties: props(bearerInputs(vaultID))})
	require.NoError(t, err)

	vaultPath := "/v1/vaults/" + vaultID
	credentialPath := vaultPath + "/credentials/" + created.ID
	api.script(
		scriptedResponse{http.MethodDelete, credentialPath, http.StatusConflict,
			"vault credential changed while deleting; retry the request"},
		scriptedResponse{http.MethodDelete, credentialPath, http.StatusConflict,
			"archived vaults cannot accept credential changes"},
		scriptedResponse{http.MethodDelete, vaultPath, http.StatusNotFound,
			"No vault resource found: " + created.ID},
	)

	// The credential is deleted once the conflicts clear.
	require.NoError(t, server.Delete(p.DeleteRequest{ID: created.ID, Urn: urn, Properties: created.Properties}))
	assert.Equal(t, 3, api.count(http.MethodDelete, credentialPath))
	assert.Empty(t, api.credentials)

	// A 404 is only trusted once the vault is confirmed gone.
	require.NoError(t, server.Delete(p.DeleteRequest{
		ID: vaultID, Urn: resourceURN(vaultToken), Properties: vault.Properties,
	}))
	assert.Equal(t, 2, api.count(http.MethodDelete, vaultPath))
	assert.Empty(t, api.vaults)
}

func TestDeletingACredentialGivesUpOnOtherErrors(t *testing.T) {
	t.Parallel()
	api := newFakeAPI(t)
	server := newServer(t, api)
	vaultID := createVault(t, server)
	urn := resourceURN(credentialToken)
	created, err := server.Create(p.CreateRequest{Urn: urn, Properties: props(bearerInputs(vaultID))})
	require.NoError(t, err)

	path := "/v1/vaults/" + vaultID + "/credentials/" + created.ID
	api.script(scriptedResponse{http.MethodDelete, path, http.StatusForbidden, "Missing scope: api.vaults.write"})
	err = server.Delete(p.DeleteRequest{ID: created.ID, Urn: urn, Properties: created.Properties})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "api.vaults.write")
	assert.Equal(t, 1, api.count(http.MethodDelete, path))
}
