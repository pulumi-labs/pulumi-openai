# Pulumi OpenAI Provider

A native Pulumi provider for managing [OpenAI](https://platform.openai.com) resources.
It is built with [`pulumi-go-provider`](https://github.com/pulumi/pulumi-go-provider).

> This provider is experimental. Its resources use OpenAI's beta Agents API.

## Resources

| Resource | Description |
|---|---|
| `openai:index:Agent` | A reusable agent: a model with its instructions, tools and settings. |
| `openai:index:PeriodicAutomation` | A schedule that runs an agent repeatedly, on a cron expression. |
| `openai:index:OneTimeAutomation` | A schedule that runs an agent once, at a given time. |
| `openai:index:Vault` | A store of credentials that sessions use without the secrets appearing in an agent's configuration. |
| `openai:index:VaultCredential` | A credential in a vault: a token for an MCP server, or a secret for OpenAI-hosted environments. |

## Configuration

| Key | Environment variable | Description |
|---|---|---|
| `openai:apiToken` | `OPENAI_API_KEY` | The OpenAI API key. Stored as a secret. |
| `openai:baseUrl` | `OPENAI_BASE_URL` | The API base URL. Defaults to `https://api.openai.com/v1`. |

## Example

```yaml
name: my-agents
runtime: yaml

resources:
  supportAgent:
    type: openai:Agent
    properties:
      model: gpt-5.5
      name: support-agent
      instructions: You answer customer questions about orders.
      reasoning:
        effort: low
      tools:
        - type: web_search

  dailyDigest:
    type: openai:PeriodicAutomation
    properties:
      name: daily-digest
      expression: 0 9 * * 1-5
      timezone: Europe/Amsterdam
      target:
        agentId: ${supportAgent.id}
        environment:
          type: none
        input:
          - content:
              - type: input_text
                text: Summarize yesterday's open orders.

outputs:
  agentId: ${supportAgent.id}
```

The same program in TypeScript, Python, Go and C# is under [`examples/`](examples/).

Credentials live in a vault. An MCP tool picks the credential to use with `credentialId`,
and an automation makes vaults available to the sessions it starts with `target.vaultIds`:

```yaml
config:
  docsToken:
    type: string
    secret: true
  githubPat:
    type: string
    secret: true

resources:
  vault:
    type: openai:Vault
    properties:
      name: support-credentials

  docsMcp:
    type: openai:VaultCredential
    properties:
      vaultId: ${vault.id}
      name: docs-mcp
      auth:
        type: static_bearer
        mcpServerUrl: https://mcp.example.com/mcp
        token: ${docsToken}

  githubToken:
    type: openai:VaultCredential
    properties:
      vaultId: ${vault.id}
      name: github-token
      auth:
        type: environment_variable
        secretName: GITHUB_TOKEN
        secretValue: ${githubPat}
        networking:
          type: limited
          allowedHosts:
            - api.github.com

  docsAgent:
    type: openai:Agent
    properties:
      model: gpt-5.5
      tools:
        - type: mcp
          serverLabel: docs
          transport:
            type: http
            serverUrl: https://mcp.example.com/mcp
          credentialId: ${docsMcp.id}
```

Existing resources can be adopted with `pulumi import`:

```bash
pulumi import openai:index:Agent <name> <agent-id>
pulumi import openai:index:PeriodicAutomation <name> <automation-id>
pulumi import openai:index:OneTimeAutomation <name> <automation-id>
pulumi import openai:index:Vault <name> <vault-id>
pulumi import openai:index:VaultCredential <name> <vault-id>/<credential-id>
```

A credential can only be looked up through its vault, so its import ID names both. Once
imported, the resource's ID is the credential ID. The API never returns secrets, so the
generated code has none: add them to your program, and the next update rotates them in.
Replace the generated `vaultId` with a reference to the `Vault` resource, such as
`${vault.id}`, so that Pulumi orders operations on the credential after its vault.

The API has one automation object whose type is `periodic` or `once`. Each type is its own
resource here, so import an automation with the resource that matches its type; the
provider reports an error that names the right one otherwise.

## How the resources behave

All resources:

- `pulumi refresh` only refreshes the properties you set, so defaults filled in by the
  API are not reported as changes.
- A resource deleted outside Pulumi is detected on refresh and recreated on the next update.

`Agent`:

- Every property can be updated in place. Nothing forces a replacement.
- Removing an optional property from your program resets it to the API default.

`PeriodicAutomation` and `OneTimeAutomation`:

- Every property can be updated in place. Nothing forces a replacement.
- Each run starts a new session of a saved agent, given by `target.agentId`. Two parts of
  the API's target are not supported: configuring an agent inline, and the
  `existing_session` target that notifies a session that already exists. An automation
  that uses either cannot be imported.
- The `runAt` of a `OneTimeAutomation` can only be changed until it has run.
- The API never returns confidential values or uploaded content, so these are not
  refreshed: environment `env`, `setupCommands`, inline file `data`, plugin and skill
  `source`, and network `blockedDomains`.
- Deleting an automation stops future runs. It does not delete sessions it started.

`Vault`:

- Every property can be updated in place. Nothing forces a replacement.
- The API fills in no defaults for a vault, so a refresh picks up every property.
- Deleting a vault deletes all of its credentials.

`VaultCredential`:

- Secrets are rotated in place: `token`, `accessToken`, `secretValue`, and under
  `refresh`, `refreshToken` and `tokenEndpointAuth.clientSecret`. So are `expiresAt`,
  `refresh.scope` and `metadata`. Only what changed is sent, so a metadata change never
  resends a token that OpenAI has since refreshed.
- Everything else replaces the credential: `vaultId`, `name`, `auth.type`,
  `mcpServerUrl`, `secretName`, `networking`, and under `refresh`, `clientId`,
  `tokenEndpoint`, `resource` and `tokenEndpointAuth.type`. The API cannot update them.
- A vault holds one `environment_variable` credential per `secretName`, so replacing one
  that keeps its `secretName` deletes the old credential before creating the new one.
- Secrets are stored in the Pulumi state as secrets. The API never returns them, so they
  are not refreshed. Neither is `expiresAt`, which OpenAI moves forward whenever it
  refreshes an OAuth access token.

## Developing

Prerequisites: Go, the Pulumi CLI and `jq`. The language SDK builds also need
Node.js, Python, .NET and Gradle.

```bash
make provider        # build bin/pulumi-resource-openai
make test            # run the provider unit tests against a fake API
make schema          # regenerate schema.json
make sdks            # regenerate schema.json and the SDKs under sdk/
make build_nodejs_sdk build_python_sdk build_dotnet_sdk build_java_sdk   # compile the SDKs
```

If you change the schema, run `make sdks` and commit the regenerated files. CI fails when
the committed `schema.json` is out of date.

The provider code is in [`provider/`](provider/):

- `provider.go` declares the provider, its configuration and its resources.
- `agent.go` and `automation.go` implement the lifecycle of the resources: create, read,
  update and delete. Both automation resources share the logic in `automation.go`.
- `agent_types.go` and `automation_types.go` define their inputs.
- `wire.go` converts between Pulumi inputs and API JSON.
- `refresh.go` computes refreshed inputs from API responses.
- `client.go` is the HTTP client for the OpenAI API.

The tests under [`examples/`](examples/) run real Pulumi programs and create real agents and automations,
so they need `OPENAI_API_KEY` to be set. Run them with `make test_examples`. CI does not run them.

## Releasing

Releases are tag-driven. Pushing a `v*.*.*` tag runs
[`.github/workflows/release.yml`](.github/workflows/release.yml), which:

1. Cross-compiles the provider for Linux, macOS and Windows.
2. Generates the SDKs in all five languages with the version of the tag.
3. Uploads the provider tarballs and SDK source archives to a GitHub Release. The
   Pulumi CLI downloads the provider plugin from there.
4. Publishes the SDKs:

   | Language | Package | How |
   | --- | --- | --- |
   | Node.js | `@pulumi-labs/openai` on npm | Trusted publishing, `publish-npm` environment |
   | Python | `pulumi_labs_openai` on PyPI | Trusted publishing, `publish-pypi` environment |
   | .NET | `Pulumi.Labs.Openai` on NuGet | Trusted publishing, `publish-nuget` environment |
   | Java | `com.pulumi.labs:openai` on Maven Central | Signing credentials from Pulumi ESC |
   | Go | `github.com/pulumi-labs/pulumi-openai/sdk/go/pulumi-openai` | A `sdk/go/pulumi-openai/vX.Y.Z` tag |

The NuGet and Maven Central jobs read their settings from the Pulumi ESC environment
`github-secrets/pulumi-labs-pulumi-openai`. The workflows follow the structure of
[pulumi-labs/pulumi-modal](https://github.com/pulumi-labs/pulumi-modal).
