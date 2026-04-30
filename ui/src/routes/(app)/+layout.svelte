<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import { session, logout } from '$lib/auth.js';
	import { api } from '$lib/api-client.js';
	import type { Project } from '$lib/api-client.js';
	import { ChevronDown, LogOut, Settings } from 'lucide-svelte';

	let { children } = $props();

	// Redirect unauthenticated visitors after session resolves.
	$effect(() => {
		if (!session.loading && !session.user) {
			goto(`/auth/login?next=${encodeURIComponent(page.url.pathname)}`, { replaceState: true });
		}
	});

	// Active project stored in sessionStorage for persistence across navigations.
	let projects = $state<Project[]>([]);
	let activeProjectId = $state<string | null>(null);
	let projectsLoading = $state(false);

	function getStoredProjectId(): string | null {
		try {
			return sessionStorage.getItem('lantern_project_id');
		} catch {
			return null;
		}
	}

	function storeProjectId(id: string) {
		try {
			sessionStorage.setItem('lantern_project_id', id);
		} catch {
			// Storage blocked — non-fatal.
		}
	}

	onMount(async () => {
		// Wait for session before loading projects.
		await new Promise<void>((resolve) => {
			const interval = setInterval(() => {
				if (!session.loading) {
					clearInterval(interval);
					resolve();
				}
			}, 50);
		});

		if (!session.user) return;

		projectsLoading = true;
		try {
			// Load orgs → teams → projects for the current user.
			// For MVP, fetch the user's orgs via their first org membership.
			// A proper "list my projects" endpoint is the cleaner solution (Phase 2 cleanup).
			// For now, try to use the stored project id to avoid a full re-fetch.
			const storedId = getStoredProjectId();
			if (storedId) {
				activeProjectId = storedId;
			}
		} finally {
			projectsLoading = false;
		}
	});

	function selectProject(id: string) {
		activeProjectId = id;
		storeProjectId(id);
		goto(`/dashboard?project=${id}`);
	}

	let menuOpen = $state(false);
</script>

{#if session.loading}
	<div class="flex min-h-screen items-center justify-center">
		<p class="text-sm text-gray-500" aria-live="polite" aria-busy="true">Loading…</p>
	</div>
{:else if session.user}
	<div class="flex min-h-screen flex-col">
		<!-- ── Top navigation bar ─────────────────────────────────────────── -->
		<header class="border-b border-gray-200 bg-white">
			<nav
				aria-label="Main navigation"
				class="mx-auto flex h-14 max-w-screen-xl items-center gap-4 px-4"
			>
				<a href="/dashboard" class="flex items-center gap-2 font-semibold text-gray-900">
					<span aria-label="Lantern">🔦</span>
					<span>Lantern</span>
				</a>

				<div class="flex flex-1 items-center gap-1">
					<a
						href={activeProjectId ? `/dashboard?project=${activeProjectId}` : '/dashboard'}
						class="rounded px-3 py-1.5 text-sm text-gray-700 hover:bg-gray-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-600
							{page.url.pathname === '/dashboard' ? 'bg-gray-100 font-medium' : ''}"
					>
						Dashboard
					</a>
					{#if activeProjectId}
						<a
							href={`/gap-report/${activeProjectId}`}
							class="rounded px-3 py-1.5 text-sm text-gray-700 hover:bg-gray-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-600
								{page.url.pathname.startsWith('/gap-report') ? 'bg-gray-100 font-medium' : ''}"
						>
							Gap Report
						</a>
					{/if}
				</div>

				<!-- Project selector -->
				{#if activeProjectId || projects.length > 0}
					<div class="relative">
						<button
							onclick={() => (menuOpen = !menuOpen)}
							aria-haspopup="listbox"
							aria-expanded={menuOpen}
							aria-label={`Selected project: ${activeProjectId ?? 'none'}`}
							class="flex items-center gap-1 rounded border border-gray-200 px-3 py-1.5 text-sm hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-600"
						>
							<span class="max-w-[160px] truncate">
								{projects.find((p) => p.id === activeProjectId)?.name ?? 'Select project'}
							</span>
							<ChevronDown class="h-3 w-3 text-gray-400" aria-hidden="true" />
						</button>

						{#if menuOpen}
							<ul
								role="listbox"
								aria-label="Projects"
								class="absolute right-0 z-50 mt-1 min-w-[180px] rounded-md border border-gray-200 bg-white py-1 shadow-lg"
							>
								{#each projects as project (project.id)}
									<li role="option" aria-selected={project.id === activeProjectId}>
										<button
											onclick={() => { selectProject(project.id); menuOpen = false; }}
											class="w-full px-3 py-1.5 text-left text-sm hover:bg-gray-50
												{project.id === activeProjectId ? 'font-medium text-blue-600' : 'text-gray-700'}"
										>
											{project.name}
										</button>
									</li>
								{/each}
							</ul>
						{/if}
					</div>
				{/if}

				<!-- User menu -->
				<div class="flex items-center gap-2">
					<a
						href="/admin"
						aria-label="Organization settings"
						class="rounded p-1.5 text-gray-500 hover:bg-gray-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-600"
					>
						<Settings class="h-4 w-4" aria-hidden="true" />
					</a>
					<button
						onclick={logout}
						aria-label="Sign out"
						class="rounded p-1.5 text-gray-500 hover:bg-gray-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-600"
					>
						<LogOut class="h-4 w-4" aria-hidden="true" />
					</button>
				</div>
			</nav>
		</header>

		<!-- ── Page content ───────────────────────────────────────────────── -->
		<main id="main-content" class="flex-1">
			{@render children()}
		</main>
	</div>
{/if}
