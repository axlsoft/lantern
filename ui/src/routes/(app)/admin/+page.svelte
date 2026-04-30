<script lang="ts">
	import { api } from '$lib/api-client.js';
	import type { Organization, Team, Project, ApiKey, ApiKeyCreated } from '$lib/api-client.js';
	import Card from '$lib/components/ui/Card.svelte';
	import Button from '$lib/components/ui/Button.svelte';
	import Dialog from '$lib/components/ui/Dialog.svelte';
	import FormField from '$lib/components/ui/FormField.svelte';
	import Input from '$lib/components/ui/Input.svelte';
	import Skeleton from '$lib/components/ui/Skeleton.svelte';
	import { Plus, Key, Trash2, Copy, Check } from 'lucide-svelte';
	import { onMount } from 'svelte';

	// ── State ────────────────────────────────────────────────────────────────

	let org = $state<Organization | null>(null);
	let teams = $state<Team[]>([]);
	let projects = $state<Project[]>([]);
	let loading = $state(true);

	// Dialog state
	let showCreateOrg = $state(false);
	let showInviteUser = $state(false);
	let showCreateTeam = $state(false);
	let showCreateProject = $state(false);
	let showApiKeyModal = $state(false);
	let showDeleteConfirm = $state(false);

	let newApiKey = $state<ApiKeyCreated | null>(null);
	let apiKeyCopied = $state(false);
	let selectedProjectId = $state('');
	let deleteTarget = $state<{ type: string; id: string; name: string } | null>(null);
	let deleteConfirmText = $state('');
	let apiKeys = $state<ApiKey[]>([]);

	// Form fields
	let orgName = $state('');
	let inviteEmail = $state('');
	let inviteRole = $state<'admin' | 'member' | 'viewer'>('member');
	let teamName = $state('');
	let projectName = $state('');
	let projectRepo = $state('');
	let apiKeyName = $state('');
	let selectedTeamId = $state('');

	let formError = $state('');
	let formLoading = $state(false);

	// ── Load ─────────────────────────────────────────────────────────────────

	onMount(async () => {
		loading = true;
		try {
			// For MVP, we need to know the user's org. Until "list my orgs" exists,
			// we try to create one if none exists, or fetch details via the session.
			// The user's org is accessible after they've created one.
		} catch {
			// ignore — user may not have an org yet
		} finally {
			loading = false;
		}
	});

	// ── Handlers ─────────────────────────────────────────────────────────────

	async function createOrg(e: SubmitEvent) {
		e.preventDefault();
		formError = '';
		formLoading = true;
		try {
			const res = await api.createOrg(orgName);
			org = res.organization;
			showCreateOrg = false;
			orgName = '';
		} catch (err: unknown) {
			formError = (err as { message?: string }).message ?? 'Failed to create organization.';
		} finally {
			formLoading = false;
		}
	}

	async function inviteUser(e: SubmitEvent) {
		e.preventDefault();
		if (!org) return;
		formError = '';
		formLoading = true;
		try {
			await api.inviteToOrg(org.id, inviteEmail, inviteRole);
			showInviteUser = false;
			inviteEmail = '';
		} catch (err: unknown) {
			formError = (err as { message?: string }).message ?? 'Failed to send invitation.';
		} finally {
			formLoading = false;
		}
	}

	async function createTeam(e: SubmitEvent) {
		e.preventDefault();
		if (!org) return;
		formError = '';
		formLoading = true;
		try {
			const res = await api.createTeam(org.id, teamName);
			teams = [...teams, res.team];
			showCreateTeam = false;
			teamName = '';
		} catch (err: unknown) {
			formError = (err as { message?: string }).message ?? 'Failed to create team.';
		} finally {
			formLoading = false;
		}
	}

	async function createProject(e: SubmitEvent) {
		e.preventDefault();
		if (!selectedTeamId) return;
		formError = '';
		formLoading = true;
		try {
			const res = await api.createProject(selectedTeamId, projectName, projectRepo || undefined);
			projects = [...projects, res.project];
			showCreateProject = false;
			projectName = '';
			projectRepo = '';
		} catch (err: unknown) {
			formError = (err as { message?: string }).message ?? 'Failed to create project.';
		} finally {
			formLoading = false;
		}
	}

	async function openApiKeyModal(projectId: string) {
		selectedProjectId = projectId;
		apiKeyName = '';
		const res = await api.listApiKeys(projectId).catch(() => ({ api_keys: [] }));
		apiKeys = res.api_keys;
		newApiKey = null;
		showApiKeyModal = true;
	}

	async function createApiKey(e: SubmitEvent) {
		e.preventDefault();
		formError = '';
		formLoading = true;
		try {
			const res = await api.createApiKey(selectedProjectId, apiKeyName);
			newApiKey = res.api_key;
			apiKeys = [res.api_key, ...apiKeys];
			apiKeyName = '';
		} catch (err: unknown) {
			formError = (err as { message?: string }).message ?? 'Failed to create API key.';
		} finally {
			formLoading = false;
		}
	}

	async function copyApiKey() {
		if (!newApiKey) return;
		await navigator.clipboard.writeText(newApiKey.key);
		apiKeyCopied = true;
		setTimeout(() => (apiKeyCopied = false), 2000);
	}

	function closeApiKeyModal() {
		// Destroy the key from state — it must not be retrievable after close.
		newApiKey = null;
		apiKeyCopied = false;
		showApiKeyModal = false;
	}

	function promptDelete(type: string, id: string, name: string) {
		deleteTarget = { type, id, name };
		deleteConfirmText = '';
		showDeleteConfirm = true;
	}

	async function confirmDelete() {
		if (!deleteTarget || deleteConfirmText !== deleteTarget.name) return;
		formLoading = true;
		try {
			if (deleteTarget.type === 'project') {
				await api.deleteProject(deleteTarget.id);
				projects = projects.filter((p) => p.id !== deleteTarget!.id);
			} else if (deleteTarget.type === 'apikey') {
				await api.revokeApiKey(selectedProjectId, deleteTarget.id);
				apiKeys = apiKeys.filter((k) => k.id !== deleteTarget!.id);
			}
			showDeleteConfirm = false;
			deleteTarget = null;
		} catch (err: unknown) {
			formError = (err as { message?: string }).message ?? 'Delete failed.';
		} finally {
			formLoading = false;
		}
	}
</script>

<svelte:head>
	<title>Admin — Lantern</title>
</svelte:head>

<div class="mx-auto max-w-screen-xl px-4 py-8">
	<h1 class="mb-6 text-2xl font-bold text-gray-900">Organization Settings</h1>

	{#if loading}
		<Skeleton class="h-48 rounded-lg" label="Loading organization settings" />
	{:else if !org}
		<!-- No org yet — create one -->
		<Card class="p-6 text-center">
			<p class="mb-4 text-gray-600">You don't have an organization yet.</p>
			<Button onclick={() => (showCreateOrg = true)}>
				<Plus class="h-4 w-4" aria-hidden="true" />
				Create organization
			</Button>
		</Card>
	{:else}
		<div class="space-y-8">
			<!-- Org details -->
			<section aria-label="Organization details">
				<div class="mb-3 flex items-center justify-between">
					<h2 class="text-lg font-semibold text-gray-900">{org.name}</h2>
					<Button variant="outline" size="sm" onclick={() => (showInviteUser = true)}>
						Invite user
					</Button>
				</div>
			</section>

			<!-- Teams -->
			<section aria-label="Teams">
				<div class="mb-3 flex items-center justify-between">
					<h2 class="text-lg font-semibold text-gray-900">Teams</h2>
					<Button variant="outline" size="sm" onclick={() => (showCreateTeam = true)}>
						<Plus class="h-4 w-4" aria-hidden="true" />
						New team
					</Button>
				</div>
				{#if teams.length === 0}
					<p class="text-sm text-gray-500">No teams yet.</p>
				{:else}
					<Card>
						<ul role="list">
							{#each teams as team, i (team.id)}
								<li
									class="flex items-center justify-between px-4 py-3 {i > 0
										? 'border-t border-gray-100'
										: ''}"
								>
									<span class="font-medium text-gray-800">{team.name}</span>
									<Button
										variant="outline"
										size="sm"
										onclick={() => {
											selectedTeamId = team.id;
											showCreateProject = true;
										}}
									>
										<Plus class="h-3.5 w-3.5" aria-hidden="true" />
										New project
									</Button>
								</li>
							{/each}
						</ul>
					</Card>
				{/if}
			</section>

			<!-- Projects -->
			<section aria-label="Projects">
				<div class="mb-3 flex items-center justify-between">
					<h2 class="text-lg font-semibold text-gray-900">Projects</h2>
				</div>
				{#if projects.length === 0}
					<p class="text-sm text-gray-500">No projects yet. Create a team first, then a project.</p>
				{:else}
					<Card>
						<ul role="list">
							{#each projects as project, i (project.id)}
								<li
									class="flex flex-wrap items-center justify-between gap-3 px-4 py-3 {i > 0
										? 'border-t border-gray-100'
										: ''}"
								>
									<div>
										<p class="font-medium text-gray-800">{project.name}</p>
										{#if project.github_repo_full_name}
											<p class="text-xs text-gray-400">{project.github_repo_full_name}</p>
										{:else}
											<p class="text-xs text-yellow-600">No GitHub repo associated</p>
										{/if}
									</div>
									<div class="flex gap-2">
										<Button variant="outline" size="sm" onclick={() => openApiKeyModal(project.id)}>
											<Key class="h-3.5 w-3.5" aria-hidden="true" />
											API keys
										</Button>
										<Button
											variant="ghost"
											size="sm"
											onclick={() => promptDelete('project', project.id, project.name)}
										>
											<Trash2 class="h-3.5 w-3.5 text-red-500" aria-hidden="true" />
											<span class="sr-only">Delete project {project.name}</span>
										</Button>
									</div>
								</li>
							{/each}
						</ul>
					</Card>
				{/if}
			</section>
		</div>
	{/if}
</div>

<!-- ── Dialogs ─────────────────────────────────────────────────────────────── -->

<!-- Create org -->
<Dialog bind:open={showCreateOrg} title="Create organization" onclose={() => (formError = '')}>
	<form onsubmit={createOrg} class="flex flex-col gap-4">
		{#if formError}
			<p role="alert" class="text-sm text-red-600">{formError}</p>
		{/if}
		<FormField id="org-name" label="Organization name">
			<Input id="org-name" bind:value={orgName} required placeholder="Acme Corp" />
		</FormField>
		<!-- eslint-disable-next-line @typescript-eslint/no-unused-vars -->
		{#snippet footer()}
			<Button variant="outline" onclick={() => (showCreateOrg = false)}>Cancel</Button>
			<Button type="submit" loading={formLoading}>Create</Button>
		{/snippet}
	</form>
</Dialog>

<!-- Invite user -->
<Dialog bind:open={showInviteUser} title="Invite user" onclose={() => (formError = '')}>
	<form onsubmit={inviteUser} class="flex flex-col gap-4">
		{#if formError}
			<p role="alert" class="text-sm text-red-600">{formError}</p>
		{/if}
		<FormField id="invite-email" label="Email address">
			<Input id="invite-email" type="email" bind:value={inviteEmail} required autocomplete="off" />
		</FormField>
		<fieldset>
			<legend class="mb-1 block text-sm font-medium text-gray-700">Role</legend>
			<div class="flex gap-4">
				{#each ['admin', 'member', 'viewer'] as r (r)}
					<label class="flex items-center gap-2 text-sm">
						<input
							type="radio"
							name="invite-role"
							value={r}
							bind:group={inviteRole}
							class="text-blue-600 focus:ring-blue-600"
						/>
						{r.charAt(0).toUpperCase() + r.slice(1)}
					</label>
				{/each}
			</div>
		</fieldset>
		<!-- eslint-disable-next-line @typescript-eslint/no-unused-vars -->
		{#snippet footer()}
			<Button variant="outline" onclick={() => (showInviteUser = false)}>Cancel</Button>
			<Button type="submit" loading={formLoading}>Send invite</Button>
		{/snippet}
	</form>
</Dialog>

<!-- Create team -->
<Dialog bind:open={showCreateTeam} title="Create team" onclose={() => (formError = '')}>
	<form onsubmit={createTeam} class="flex flex-col gap-4">
		{#if formError}
			<p role="alert" class="text-sm text-red-600">{formError}</p>
		{/if}
		<FormField id="team-name" label="Team name">
			<Input id="team-name" bind:value={teamName} required placeholder="Platform" />
		</FormField>
		<!-- eslint-disable-next-line @typescript-eslint/no-unused-vars -->
		{#snippet footer()}
			<Button variant="outline" onclick={() => (showCreateTeam = false)}>Cancel</Button>
			<Button type="submit" loading={formLoading}>Create</Button>
		{/snippet}
	</form>
</Dialog>

<!-- Create project -->
<Dialog bind:open={showCreateProject} title="Create project" onclose={() => (formError = '')}>
	<form onsubmit={createProject} class="flex flex-col gap-4">
		{#if formError}
			<p role="alert" class="text-sm text-red-600">{formError}</p>
		{/if}
		<FormField id="proj-name" label="Project name">
			<Input id="proj-name" bind:value={projectName} required placeholder="My Service" />
		</FormField>
		<FormField id="proj-repo" label="GitHub repo (optional)">
			<Input id="proj-repo" bind:value={projectRepo} placeholder="owner/repo" />
		</FormField>
		<!-- eslint-disable-next-line @typescript-eslint/no-unused-vars -->
		{#snippet footer()}
			<Button variant="outline" onclick={() => (showCreateProject = false)}>Cancel</Button>
			<Button type="submit" loading={formLoading}>Create</Button>
		{/snippet}
	</form>
</Dialog>

<!-- API Keys modal -->
<Dialog bind:open={showApiKeyModal} title="API Keys" onclose={closeApiKeyModal}>
	<div class="space-y-4">
		{#if newApiKey}
			<div class="rounded-md bg-green-50 p-4">
				<p class="mb-2 text-sm font-medium text-green-800">
					Copy this key now — it will not be shown again.
				</p>
				<div class="flex items-center gap-2">
					<code
						class="flex-1 truncate rounded bg-green-100 px-2 py-1 font-mono text-xs text-green-900"
					>
						{newApiKey.key}
					</code>
					<button
						onclick={copyApiKey}
						aria-label={apiKeyCopied ? 'Copied to clipboard' : 'Copy API key to clipboard'}
						class="rounded p-1 text-green-700 hover:bg-green-100 focus-visible:ring-2 focus-visible:ring-green-600 focus-visible:outline-none"
					>
						{#if apiKeyCopied}
							<Check class="h-4 w-4" aria-hidden="true" />
						{:else}
							<Copy class="h-4 w-4" aria-hidden="true" />
						{/if}
					</button>
				</div>
				<div aria-live="polite" class="sr-only">
					{apiKeyCopied ? 'Copied to clipboard' : ''}
				</div>
			</div>
		{/if}

		<form onsubmit={createApiKey} class="flex gap-2">
			<Input
				id="api-key-name"
				bind:value={apiKeyName}
				required
				placeholder="Key name (e.g. CI)"
				class="flex-1"
				aria-label="API key name"
			/>
			<Button type="submit" size="sm" loading={formLoading}>
				<Plus class="h-4 w-4" aria-hidden="true" />
				Create
			</Button>
		</form>
		{#if formError}
			<p role="alert" class="text-sm text-red-600">{formError}</p>
		{/if}

		{#if apiKeys.length > 0}
			<ul role="list" class="divide-y divide-gray-100 rounded-md border border-gray-200">
				{#each apiKeys as key (key.id)}
					<li class="flex items-center justify-between px-3 py-2">
						<div>
							<p class="text-sm font-medium text-gray-800">{key.name}</p>
							<p class="font-mono text-xs text-gray-400">{key.key_prefix}…</p>
						</div>
						{#if !key.revoked_at}
							<Button
								variant="ghost"
								size="sm"
								onclick={() => promptDelete('apikey', key.id, key.name)}
							>
								<Trash2 class="h-3.5 w-3.5 text-red-500" aria-hidden="true" />
								<span class="sr-only">Revoke API key {key.name}</span>
							</Button>
						{:else}
							<span class="text-xs text-gray-400 italic">Revoked</span>
						{/if}
					</li>
				{/each}
			</ul>
		{/if}
	</div>
</Dialog>

<!-- Destructive confirmation -->
<Dialog
	bind:open={showDeleteConfirm}
	title="Confirm deletion"
	onclose={() => (deleteTarget = null)}
>
	{#if deleteTarget}
		<div class="space-y-4">
			<p class="text-sm text-gray-700">
				This action is irreversible. Type <strong>{deleteTarget.name}</strong> to confirm.
			</p>
			<FormField id="confirm-name" label={`Type "${deleteTarget.name}" to confirm`}>
				<Input
					id="confirm-name"
					bind:value={deleteConfirmText}
					placeholder={deleteTarget.name}
					autocomplete="off"
					aria-describedby="confirm-instructions"
				/>
			</FormField>
			<p id="confirm-instructions" class="sr-only">
				Type {deleteTarget.name} exactly to enable the delete button.
			</p>
			{#if formError}
				<p role="alert" class="text-sm text-red-600">{formError}</p>
			{/if}
		</div>
		<!-- eslint-disable-next-line @typescript-eslint/no-unused-vars -->
		{#snippet footer()}
			<Button variant="outline" onclick={() => (showDeleteConfirm = false)}>Cancel</Button>
			<Button
				variant="destructive"
				loading={formLoading}
				disabled={deleteConfirmText !== deleteTarget!.name}
				onclick={confirmDelete}
			>
				Delete
			</Button>
		{/snippet}
	{/if}
</Dialog>
