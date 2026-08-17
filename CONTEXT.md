# Herdr Web Access

Vocabulary for browser access to a running Herdr environment without replacing Herdr’s control model.

## Language

**Herdr Web TUI**:
Browser-facing companion to Herdr that exposes its terminal experience and browser-only bridges.
_Avoid_: Herdr plugin, Web Herdr

**Herdr Session**:
Persistent isolated Herdr namespace that owns workspaces, tabs, panes, focus, and agent state.
_Avoid_: Web session, user session

**Workspace**:
Project-level container within a Herdr Session that groups related tabs and panes.
_Avoid_: Project, repository

**Tab**:
Named layout within a Workspace containing one or more panes.
_Avoid_: Page, browser tab

**Pane**:
Independently addressable terminal surface within a Tab.
_Avoid_: Window, shell

**Agent**:
Coding-agent process recognized by Herdr and associated with a Pane.
_Avoid_: User, assistant

**Content Label**:
Human-readable Tab name describing its current work; notification identity prefers this over Agent name.
_Avoid_: Agent name, terminal title

**Promptbox**:
Browser composer containing ordered text and artifact references before submission to a Pane.
_Avoid_: Terminal input, chat box

**Artifact**:
Browser-supplied file referenced from Promptbox content and made available to the target Pane.
_Avoid_: Attachment, upload

## Delivery Language

**Deployment**:
Applying a selected revision to a managed environment for runtime validation or use. A Deployment may use an untagged commit and does not publish a public version.
_Avoid_: Release, publish

**Release**:
Publishing a versioned Herdr Web TUI revision through GitHub for downstream consumers. A Release does not imply that any managed environment changed.
_Avoid_: Deployment, rollout

**Accepted Deployment**:
Deployment that the Owner explicitly declares satisfactory after observing its verification results. Passing technical health checks alone does not make a Deployment accepted.
_Avoid_: Healthy Deployment, successful build

## Trust Language

**Owner**:
Single trusted operator of one deployed Herdr Web TUI instance.
_Avoid_: User, account

**Gateway**:
External trust boundary that accepts browser traffic before forwarding trusted requests to Herdr Web TUI.
_Avoid_: Application authentication
