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
      model: gpt-5
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

Existing resources can be adopted with `pulumi import`:

```bash
pulumi import openai:index:Agent <name> <agent-id>
pulumi import openai:index:PeriodicAutomation <name> <automation-id>
pulumi import openai:index:OneTimeAutomation <name> <automation-id>
```

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

## Developing

Prerequisites: Go, the Pulumi CLI and `pulumictl`. The language SDK builds also need
Node.js with Yarn, Python, .NET and Gradle.

```bash
make provider        # build bin/pulumi-resource-openai
make test_provider   # run the provider unit tests against a fake API
make codegen         # regenerate schema.json and the SDKs under sdk/
make build           # build the provider and all SDKs
```

The provider code is in [`provider/`](provider/):

- `provider.go` declares the provider, its configuration and its resources.
- `agent.go` and `automation.go` implement the lifecycle of the resources: create, read,
  update and delete. Both automation resources share the logic in `automation.go`.
- `agent_types.go` and `automation_types.go` define their inputs.
- `wire.go` converts between Pulumi inputs and API JSON.
- `refresh.go` computes refreshed inputs from API responses.
- `client.go` is the HTTP client for the OpenAI API.

The tests under [`examples/`](examples/) run real Pulumi programs and create real agents and automations,
so they need `OPENAI_API_KEY` to be set.
