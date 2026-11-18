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

// The types in this file are the Pulumi inputs of the VaultCredential resource. As with
// the Agent resource, each API field is the snake_case form of the Pulumi property name,
// and API unions are flattened into one object with a `type` discriminator.

// VaultCredentialArgs are the inputs of the VaultCredential resource.
type VaultCredentialArgs struct {
	VaultID  string            `pulumi:"vaultId" provider:"replaceOnChanges"`
	Name     string            `pulumi:"name" provider:"replaceOnChanges"`
	Auth     CredentialAuth    `pulumi:"auth"`
	Metadata map[string]string `pulumi:"metadata,optional"`
}

// Annotate describes the inputs of the VaultCredential resource.
func (a *VaultCredentialArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.VaultID, "The ID of the vault that holds the credential, such as the `id` of a `Vault` resource.")
	an.Describe(&a.Name, "A human-readable name for the credential, 1 to 256 bytes once leading and "+
		"trailing whitespace is trimmed. Names need not be unique.")
	an.Describe(&a.Auth, "How the credential authenticates, and its secrets.")
	an.Describe(&a.Metadata, "Up to 16 custom key-value pairs. "+
		"Keys can be up to 64 characters and values up to 512 characters.")
}

// CredentialAuth is how a credential authenticates.
//
// The API models it as a union discriminated by `type`. Its secrets are write-only: the
// API never returns them.
type CredentialAuth struct {
	Type CredentialAuthType `pulumi:"type"`

	// MCP credentials: `mcp_oauth` and `static_bearer`.
	McpServerURL *string `pulumi:"mcpServerUrl,optional"`

	// Static bearer credentials.
	Token *string `pulumi:"token,optional" provider:"secret"`

	// MCP OAuth credentials.
	AccessToken *string                 `pulumi:"accessToken,optional" provider:"secret"`
	ExpiresAt   *string                 `pulumi:"expiresAt,optional"`
	Refresh     *CredentialOAuthRefresh `pulumi:"refresh,optional"`

	// Environment variable credentials.
	SecretName  *string               `pulumi:"secretName,optional"`
	SecretValue *string               `pulumi:"secretValue,optional" provider:"secret"`
	Networking  *CredentialNetworking `pulumi:"networking,optional"`
}

// Annotate describes CredentialAuth.
func (a *CredentialAuth) Annotate(an infer.Annotator) {
	an.Describe(&a, "How a credential authenticates. `type` selects the kind of credential; "+
		"each other property applies only to the credential types named in its description. "+
		"Secrets are write-only: the API never returns them, so they are not refreshed.")
	an.Describe(&a.Type, "The kind of credential. It cannot be changed: changing it replaces the credential.")

	an.Describe(&a.McpServerURL, "`mcp_oauth` and `static_bearer` credentials: the HTTPS URL of the MCP "+
		"server the credential authorizes. Required. An MCP tool uses the credential whose server URL "+
		"matches, or the one its `credentialId` names. Changing it replaces the credential.")

	an.Describe(&a.Token, "`static_bearer` credentials: the bearer token, which is not refreshed automatically. "+
		"Required. Changing it rotates the token in place.")

	an.Describe(&a.AccessToken, "`mcp_oauth` credentials: the OAuth access token. Required. "+
		"Changing it rotates the token in place.")
	an.Describe(&a.ExpiresAt, "`mcp_oauth` credentials: when the access token expires, as an RFC 3339 "+
		"timestamp, if known. OpenAI updates the expiry when it refreshes the token; that is not "+
		"reported as drift.")
	an.Describe(&a.Refresh, "`mcp_oauth` credentials: how OpenAI refreshes the access token. "+
		"When unset, the token is not refreshed. Adding or removing it replaces the credential.")

	an.Describe(&a.SecretName, "`environment_variable` credentials: the name of the environment variable "+
		"that receives a placeholder for the secret. Required. Use ASCII letters, digits and underscores, "+
		"starting with a letter or underscore; names starting with `CODEX_` are reserved. "+
		"It must be unique among the vault's environment variable credentials. "+
		"Changing it replaces the credential.")
	an.Describe(&a.SecretValue, "`environment_variable` credentials: the secret. Required. It must not contain "+
		"carriage returns, newlines or NUL bytes. Sandbox code only sees a placeholder; OpenAI's egress "+
		"proxy substitutes the secret in requests to allowed hosts. Changing it rotates the secret in place, "+
		"for sessions created afterwards.")
	an.Describe(&a.Networking, "`environment_variable` credentials: where the secret may be sent. Required. "+
		"Changing it replaces the credential.")
}

// CredentialOAuthRefresh configures how an OAuth access token is refreshed.
type CredentialOAuthRefresh struct {
	ClientID          string                      `pulumi:"clientId"`
	RefreshToken      string                      `pulumi:"refreshToken" provider:"secret"`
	TokenEndpoint     string                      `pulumi:"tokenEndpoint"`
	TokenEndpointAuth CredentialTokenEndpointAuth `pulumi:"tokenEndpointAuth"`
	Resource          *string                     `pulumi:"resource,optional"`
	Scope             *string                     `pulumi:"scope,optional"`
}

// Annotate describes CredentialOAuthRefresh.
func (r *CredentialOAuthRefresh) Annotate(an infer.Annotator) {
	an.Describe(&r.ClientID, "The OAuth client ID used to request a new token. Changing it replaces the credential.")
	an.Describe(&r.RefreshToken, "The OAuth refresh token. Changing it rotates the token in place.")
	an.Describe(&r.TokenEndpoint, "The HTTPS token endpoint the refresh token is exchanged at. "+
		"Changing it replaces the credential.")
	an.Describe(&r.TokenEndpointAuth, "How the client authenticates to the token endpoint.")
	an.Describe(&r.Resource, "The resource URI sent when refreshing, if the server requires one. "+
		"Changing it replaces the credential.")
	an.Describe(&r.Scope, "The space-separated OAuth scopes requested when refreshing.")
}

// CredentialTokenEndpointAuth is how an OAuth client authenticates to a token endpoint.
type CredentialTokenEndpointAuth struct {
	Type         TokenEndpointAuthType `pulumi:"type"`
	ClientSecret *string               `pulumi:"clientSecret,optional" provider:"secret"`
}

// Annotate describes CredentialTokenEndpointAuth.
func (a *CredentialTokenEndpointAuth) Annotate(an infer.Annotator) {
	an.Describe(&a.Type, "How the client authenticates: `none` sends only the client ID, "+
		"`client_secret_basic` sends the client secret with HTTP Basic authentication, and "+
		"`client_secret_post` sends it in the request body. Changing it replaces the credential.")
	an.Describe(&a.ClientSecret, "`client_secret_basic` and `client_secret_post`: the OAuth client secret. "+
		"Required. Changing it rotates the secret in place.")
}

// CredentialNetworking is where an environment variable credential's secret may be sent.
type CredentialNetworking struct {
	Type         CredentialNetworkingType `pulumi:"type"`
	AllowedHosts []string                 `pulumi:"allowedHosts,optional"`
}

// Annotate describes CredentialNetworking.
func (n *CredentialNetworking) Annotate(an infer.Annotator) {
	an.Describe(&n.Type, "`limited` sends the secret only to `allowedHosts`. `unrestricted` sends it to any "+
		"host the session's network policy allows, which must then be `restricted` with explicit allowed domains.")
	an.Describe(&n.AllowedHosts, "`limited` networking: the hosts the secret may be sent to, 1 to 16 lowercase "+
		"host names or IPv4 addresses, without a scheme, path, port or wildcard. They must also be allowed by "+
		"the session's network policy. Required.")
}

// CredentialAuthType is the kind of a vault credential.
type CredentialAuthType string

// The kinds of vault credential.
//
//nolint:gosec // These name kinds of credential; they are not credentials.
const (
	mcpOAuthCredential            CredentialAuthType = "mcp_oauth"
	staticBearerCredential        CredentialAuthType = "static_bearer"
	environmentVariableCredential CredentialAuthType = "environment_variable"
)

// Values lists the allowed credential types.
func (CredentialAuthType) Values() []infer.EnumValue[CredentialAuthType] {
	return []infer.EnumValue[CredentialAuthType]{
		{
			Name: "McpOAuth", Value: mcpOAuthCredential,
			Description: "An OAuth access token for an MCP server, optionally refreshed by OpenAI.",
		},
		{Name: "StaticBearer", Value: staticBearerCredential, Description: "A bearer token for an MCP server."},
		{
			Name: "EnvironmentVariable", Value: environmentVariableCredential,
			Description: "A secret exposed to OpenAI-hosted environments as an environment variable placeholder.",
		},
	}
}

// TokenEndpointAuthType is how an OAuth client authenticates to a token endpoint.
type TokenEndpointAuthType string

// Values lists the allowed token endpoint authentication methods.
func (TokenEndpointAuthType) Values() []infer.EnumValue[TokenEndpointAuthType] {
	return []infer.EnumValue[TokenEndpointAuthType]{
		{Name: "None", Value: "none"},
		{Name: "ClientSecretBasic", Value: "client_secret_basic"},
		{Name: "ClientSecretPost", Value: "client_secret_post"},
	}
}

// CredentialNetworkingType is where an environment variable credential's secret may be sent.
type CredentialNetworkingType string

// Values lists the allowed networking types.
func (CredentialNetworkingType) Values() []infer.EnumValue[CredentialNetworkingType] {
	return []infer.EnumValue[CredentialNetworkingType]{
		{Name: "Limited", Value: "limited"},
		{Name: "Unrestricted", Value: "unrestricted"},
	}
}
