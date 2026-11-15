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

// The types in this file are the Pulumi inputs of the two automation resources. The API
// models an automation as a union of a periodic and a one-time schedule; here each is a
// resource of its own, and they share the target types. As with the Agent resource, each API field is the snake_case form of the Pulumi property name, and
// API unions are flattened into one object with a `type` discriminator.

// PeriodicAutomationArgs are the inputs of the PeriodicAutomation resource.
type PeriodicAutomationArgs struct {
	Name       string            `pulumi:"name"`
	Expression string            `pulumi:"expression"`
	Timezone   string            `pulumi:"timezone"`
	Target     AutomationTarget  `pulumi:"target"`
	Metadata   map[string]string `pulumi:"metadata,optional"`
}

// Annotate describes the inputs of the PeriodicAutomation resource.
func (a *PeriodicAutomationArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.Name, "A display name for the automation, up to 256 characters.")
	an.Describe(&a.Expression, "A five-field cron expression that sets when the automation runs.")
	an.Describe(&a.Timezone, "The IANA timezone the cron expression is interpreted in, such as `Europe/Amsterdam`.")
	an.Describe(&a.Target, "What each invocation of the automation does.")
	an.Describe(&a.Metadata, "Custom key-value pairs attached to the automation.")
}

// OneTimeAutomationArgs are the inputs of the OneTimeAutomation resource.
type OneTimeAutomationArgs struct {
	Name     string            `pulumi:"name"`
	RunAt    int               `pulumi:"runAt"`
	Target   AutomationTarget  `pulumi:"target"`
	Metadata map[string]string `pulumi:"metadata,optional"`
}

// Annotate describes the inputs of the OneTimeAutomation resource.
func (a *OneTimeAutomationArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.Name, "A display name for the automation, up to 256 characters.")
	an.Describe(&a.RunAt, "When the automation runs, as a Unix timestamp in seconds. It must be in the future, "+
		"and can only be changed until the automation has run.")
	an.Describe(&a.Target, "What the automation does when it runs.")
	an.Describe(&a.Metadata, "Custom key-value pairs attached to the automation.")
}

// AutomationTarget is what an automation invocation does: it starts a new session of a
// saved agent.
//
// The API's target is wider than this. It can also configure an agent inline, and it has
// an `existing_session` variant that notifies a session that already exists. Neither is
// exposed: an agent is managed with the Agent resource, and sessions are not managed by
// this provider.
type AutomationTarget struct {
	AgentID     string            `pulumi:"agentId"`
	Environment EnvironmentConfig `pulumi:"environment"`
	Input       []InputMessage    `pulumi:"input"`
	Metadata    map[string]string `pulumi:"metadata,optional"`
	VaultIDs    []string          `pulumi:"vaultIds,optional"`
}

// Annotate describes AutomationTarget.
func (t *AutomationTarget) Annotate(an infer.Annotator) {
	an.Describe(&t, "What an automation invocation does: it starts a new session of an agent.")
	an.Describe(&t.AgentID, "The ID of the agent to run, such as the `id` of an `Agent` resource.")
	an.Describe(&t.Environment, "The execution environment of each session.")
	an.Describe(&t.Input, "The user messages that start each session. At least one message is required.")
	an.Describe(&t.Metadata, "Custom key-value pairs applied to each new session.")
	an.Describe(&t.VaultIDs, "The IDs of vaults available to each session.")
}

// InputMessage is a user message that starts a session.
type InputMessage struct {
	Content []InputContent `pulumi:"content"`
}

// Annotate describes InputMessage.
func (m *InputMessage) Annotate(an infer.Annotator) {
	an.Describe(&m.Content, "The parts of the message.")
}

// InputContent is one part of an input message.
type InputContent struct {
	Type     InputContentType `pulumi:"type"`
	Text     *string          `pulumi:"text,optional"`
	ImageURL *string          `pulumi:"imageUrl,optional"`
}

// Annotate describes InputContent.
func (c *InputContent) Annotate(an infer.Annotator) {
	an.Describe(&c.Type, "The kind of content: `input_text` or `input_image`.")
	an.Describe(&c.Text, "`input_text` content: the text. Required.")
	an.Describe(&c.ImageURL, "`input_image` content: the URL of the image, which may be a base64 data URL. Required.")
}

// EnvironmentConfig is the execution environment of a session.
type EnvironmentConfig struct {
	Type EnvironmentType `pulumi:"type"`

	// Hosted and self-hosted environments.
	CapabilityDirectories []string `pulumi:"capabilityDirectories,optional"`

	// Self-hosted environments.
	WorkspaceDirectory *string `pulumi:"workspaceDirectory,optional"`

	// Hosted environments.
	EnvironmentID         *string                   `pulumi:"environmentId,optional"`
	EnvironmentTemplateID *string                   `pulumi:"environmentTemplateId,optional"`
	ContainerSize         *ContainerSize            `pulumi:"containerSize,optional"`
	Desktop               *EnvironmentDesktop       `pulumi:"desktop,optional"`
	Env                   map[string]string         `pulumi:"env,optional" provider:"secret"`
	Files                 []EnvironmentFile         `pulumi:"files,optional"`
	Network               *EnvironmentNetwork       `pulumi:"network,optional"`
	Packages              *EnvironmentPackages      `pulumi:"packages,optional"`
	Plugins               []EnvironmentPlugin       `pulumi:"plugins,optional"`
	SetupCommands         []EnvironmentSetupCommand `pulumi:"setupCommands,optional"`
	Skills                []EnvironmentSkill        `pulumi:"skills,optional"`
}

// Annotate describes EnvironmentConfig.
func (e *EnvironmentConfig) Annotate(an infer.Annotator) {
	an.Describe(&e, "The execution environment of a session. `type` selects the kind of environment; "+
		"each other property applies only to the environment types named in its description.")
	an.Describe(&e.Type, "The kind of environment: `none` for no execution environment, "+
		"`openai_hosted` for a container hosted by OpenAI, or `self_hosted`.")
	an.Describe(&e.CapabilityDirectories, "`openai_hosted` and `self_hosted` environments: capability directories.")
	an.Describe(&e.WorkspaceDirectory, "`self_hosted` environments: the absolute path of the workspace directory. "+
		"Required.")
	an.Describe(&e.EnvironmentID, "`openai_hosted` environments: the ID of an existing prewarmed environment. "+
		"Cannot be combined with a template or inline configuration.")
	an.Describe(&e.EnvironmentTemplateID, "`openai_hosted` environments: the ID of a hosted environment template. "+
		"Properties left unset inherit from the template.")
	an.Describe(&e.ContainerSize, "`openai_hosted` environments: the size of the container. Defaults to `medium`.")
	an.Describe(&e.Desktop, "`openai_hosted` environments: desktop settings.")
	an.Describe(&e.Env, "`openai_hosted` environments: environment variables. "+
		"They are secret and never refreshed from the API.")
	an.Describe(&e.Files, "`openai_hosted` environments: files placed in the workspace, up to 50.")
	an.Describe(&e.Network, "`openai_hosted` environments: the network policy.")
	an.Describe(&e.Packages, "`openai_hosted` environments: packages to install.")
	an.Describe(&e.Plugins, "`openai_hosted` environments: plugins to install, up to 32.")
	an.Describe(&e.SetupCommands, "`openai_hosted` environments: commands run to set up the environment, up to 16. "+
		"They are never refreshed from the API.")
	an.Describe(&e.Skills, "`openai_hosted` environments: skills to install, up to 200.")
}

// EnvironmentDesktop holds desktop settings of a hosted environment.
type EnvironmentDesktop struct {
	Enabled bool `pulumi:"enabled"`
}

// Annotate describes EnvironmentDesktop.
func (d *EnvironmentDesktop) Annotate(an infer.Annotator) {
	an.Describe(&d.Enabled, "Whether the environment has a desktop.")
}

// EnvironmentFile is a file placed in a hosted environment.
type EnvironmentFile struct {
	Type   EnvironmentFileType `pulumi:"type"`
	Path   string              `pulumi:"path"`
	FileID *string             `pulumi:"fileId,optional"`
	Data   *string             `pulumi:"data,optional"`
}

// Annotate describes EnvironmentFile.
func (f *EnvironmentFile) Annotate(an infer.Annotator) {
	an.Describe(&f.Type, "Where the file's content comes from: `file_id` for an uploaded file, "+
		"or `inline` for content given in `data`.")
	an.Describe(&f.Path, "The absolute path of the file, which must be under `/workspace`.")
	an.Describe(&f.FileID, "`file_id` files: the ID of the uploaded file. Required.")
	an.Describe(&f.Data, "`inline` files: the content of the file, base64-encoded. Required. "+
		"It is never refreshed from the API.")
}

// EnvironmentNetwork is the network policy of a hosted environment.
type EnvironmentNetwork struct {
	Access         NetworkAccess `pulumi:"access"`
	AllowedDomains []string      `pulumi:"allowedDomains,optional"`
	BlockedDomains []string      `pulumi:"blockedDomains,optional"`
}

// Annotate describes EnvironmentNetwork.
func (n *EnvironmentNetwork) Annotate(an infer.Annotator) {
	an.Describe(&n.Access, "Whether network access is `enabled` (unrestricted), `disabled`, "+
		"or `restricted` by the domain rules.")
	an.Describe(&n.AllowedDomains, "The domains the environment may reach.")
	an.Describe(&n.BlockedDomains, "The domains the environment may not reach, up to 100. Requires `restricted` "+
		"access and cannot be combined with `allowedDomains`. Wildcards are not supported. "+
		"They are never refreshed from the API.")
}

// EnvironmentPackages lists the packages installed in a hosted environment.
type EnvironmentPackages struct {
	Npm    []string `pulumi:"npm,optional"`
	Python []string `pulumi:"python,optional"`
	System []string `pulumi:"system,optional"`
}

// Annotate describes EnvironmentPackages.
func (p *EnvironmentPackages) Annotate(an infer.Annotator) {
	an.Describe(&p.Npm, "npm packages to install.")
	an.Describe(&p.Python, "Python packages to install.")
	an.Describe(&p.System, "System packages to install.")
}

// EnvironmentPlugin is a plugin installed in a hosted environment.
type EnvironmentPlugin struct {
	Name        string `pulumi:"name"`
	Description string `pulumi:"description"`
	Source      string `pulumi:"source"`
}

// Annotate describes EnvironmentPlugin.
func (p *EnvironmentPlugin) Annotate(an infer.Annotator) {
	an.Describe(&p.Name, "The name of the plugin, as in its manifest.")
	an.Describe(&p.Description, "A description of the plugin.")
	an.Describe(&p.Source, "The plugin as a base64-encoded ZIP archive. It is never refreshed from the API.")
}

// EnvironmentSetupCommand is a command that sets up a hosted environment.
type EnvironmentSetupCommand struct {
	Command string  `pulumi:"command" provider:"secret"`
	Cwd     *string `pulumi:"cwd,optional"`
}

// Annotate describes EnvironmentSetupCommand.
func (c *EnvironmentSetupCommand) Annotate(an infer.Annotator) {
	an.Describe(&c.Command, "The command to run. The API treats it as confidential and never returns it.")
	an.Describe(&c.Cwd, "The absolute path of the directory the command runs in. Defaults to `/workspace`.")
}

// EnvironmentSkill is a skill installed in a hosted environment.
type EnvironmentSkill struct {
	Type EnvironmentSkillType `pulumi:"type"`

	// Skill references.
	SkillID *string `pulumi:"skillId,optional"`
	Version *string `pulumi:"version,optional"`

	// Inline skills.
	Name        *string `pulumi:"name,optional"`
	Description *string `pulumi:"description,optional"`
	Source      *string `pulumi:"source,optional"`
}

// Annotate describes EnvironmentSkill.
func (s *EnvironmentSkill) Annotate(an infer.Annotator) {
	an.Describe(&s.Type, "The kind of skill: `skill_reference` for a saved skill, or `inline`.")
	an.Describe(&s.SkillID, "`skill_reference` skills: the ID of the skill. Required.")
	an.Describe(&s.Version, "`skill_reference` skills: the version, as a positive integer or `latest`. "+
		"When unset, the skill's default version is used.")
	an.Describe(&s.Name, "`inline` skills: the name of the skill. Required.")
	an.Describe(&s.Description, "`inline` skills: a description of the skill. Required.")
	an.Describe(&s.Source, "`inline` skills: the skill as a base64-encoded ZIP archive. Required. "+
		"It is never refreshed from the API.")
}

// AutomationCreatedBy records who created an automation.
type AutomationCreatedBy struct {
	Type       string  `pulumi:"type"`
	SessionID  *string `pulumi:"sessionId,optional"`
	ToolCallID *string `pulumi:"toolCallId,optional"`
}

// Annotate describes AutomationCreatedBy.
func (c *AutomationCreatedBy) Annotate(an infer.Annotator) {
	an.Describe(&c.Type, "Who created the automation: `developer` or `agent`.")
	an.Describe(&c.SessionID, "For automations created by an agent, the session that created it.")
	an.Describe(&c.ToolCallID, "For automations created by an agent, the tool call that created it.")
}

// InputContentType is the kind of an input message part.
type InputContentType string

// Values lists the allowed input content types.
func (InputContentType) Values() []infer.EnumValue[InputContentType] {
	return []infer.EnumValue[InputContentType]{
		{Name: "InputText", Value: "input_text"},
		{Name: "InputImage", Value: "input_image"},
	}
}

// EnvironmentType is the kind of an execution environment.
type EnvironmentType string

// Values lists the allowed environment types.
func (EnvironmentType) Values() []infer.EnumValue[EnvironmentType] {
	return []infer.EnumValue[EnvironmentType]{
		{Name: "None", Value: "none", Description: "No execution environment."},
		{Name: "OpenAIHosted", Value: "openai_hosted", Description: "A container hosted by OpenAI."},
		{Name: "SelfHosted", Value: "self_hosted", Description: "An environment you host."},
	}
}

// ContainerSize is the size of a hosted container.
type ContainerSize string

// Values lists the allowed container sizes.
func (ContainerSize) Values() []infer.EnumValue[ContainerSize] {
	return []infer.EnumValue[ContainerSize]{
		{Name: "Small", Value: "small"},
		{Name: "Medium", Value: "medium"},
		{Name: "Large", Value: "large"},
	}
}

// NetworkAccess is the network access of a hosted environment.
type NetworkAccess string

// Values lists the allowed network access modes.
func (NetworkAccess) Values() []infer.EnumValue[NetworkAccess] {
	return []infer.EnumValue[NetworkAccess]{
		{Name: "Enabled", Value: "enabled"},
		{Name: "Disabled", Value: "disabled"},
		{Name: "Restricted", Value: "restricted"},
	}
}

// EnvironmentFileType is where an environment file's content comes from.
type EnvironmentFileType string

// Values lists the allowed file types.
func (EnvironmentFileType) Values() []infer.EnumValue[EnvironmentFileType] {
	return []infer.EnumValue[EnvironmentFileType]{
		{Name: "FileID", Value: "file_id"},
		{Name: "Inline", Value: "inline"},
	}
}

// EnvironmentSkillType is the kind of an environment skill.
type EnvironmentSkillType string

// Values lists the allowed skill types.
func (EnvironmentSkillType) Values() []infer.EnumValue[EnvironmentSkillType] {
	return []infer.EnumValue[EnvironmentSkillType]{
		{Name: "SkillReference", Value: "skill_reference"},
		{Name: "Inline", Value: "inline"},
	}
}
