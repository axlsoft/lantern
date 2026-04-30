<script lang="ts">
	import { page } from '$app/state';
	import { api } from '$lib/api-client.js';
	import { fetchSource, detectLanguage } from '$lib/github.js';
	import type { FileCoverage, Project, Run } from '$lib/api-client.js';
	import FileViewer from '$lib/components/features/FileViewer.svelte';
	import Skeleton from '$lib/components/ui/Skeleton.svelte';
	import { AlertCircle, ArrowLeft } from 'lucide-svelte';

	const projectId = $derived(page.params.projectId!);
	const filePath = $derived(page.url.searchParams.get('path') ?? '');
	const jumpLine = $derived(parseInt(page.url.searchParams.get('line') ?? '0', 10) || 0);
	const testId = $derived(page.url.searchParams.get('test') ?? undefined);

	let project = $state<Project | null>(null);
	let runs = $state<Run[]>([]);
	let selectedRunId = $state('');
	let selectedSha = $state('');

	let source = $state('');
	let coverage = $state<FileCoverage | null>(null);
	let loading = $state(true);
	let error = $state('');

	$effect(() => {
		if (projectId && filePath) {
			initialLoad();
		}
	});

	async function initialLoad() {
		loading = true;
		error = '';
		try {
			const [projRes, runsRes] = await Promise.all([
				api.getProject(projectId),
				api.listRuns(projectId)
			]);
			project = projRes.project;
			runs = runsRes.runs.filter((r) => r.status === 'completed');
			if (runs.length > 0) {
				selectedRunId = runs[0].id;
				selectedSha = runs[0].commit_sha;
				await loadFileCoverage(selectedSha);
			}
		} catch (err: unknown) {
			error = (err as { message?: string }).message ?? 'Failed to load file.';
		} finally {
			loading = false;
		}
	}

	async function loadFileCoverage(sha: string) {
		error = '';
		try {
			const [cov, src] = await Promise.all([
				api.getFileCoverage(projectId, filePath, sha),
				project?.github_repo_full_name
					? fetchSource(project.github_repo_full_name, filePath, sha)
					: Promise.resolve('')
			]);
			coverage = cov;
			// Source must not be cached — fetch fresh each time.
			source = src;
		} catch (err: unknown) {
			error = (err as { message?: string }).message ?? 'Failed to load file coverage.';
		}
	}

	async function onRunChange(e: Event) {
		const sel = e.currentTarget as HTMLSelectElement;
		selectedRunId = sel.value;
		const run = runs.find((r) => r.id === selectedRunId);
		if (run) {
			selectedSha = run.commit_sha;
			await loadFileCoverage(selectedSha);
		}
	}
</script>

<svelte:head>
	<title>{filePath ? `${filePath} — ` : ''}File View — Lantern</title>
</svelte:head>

<div class="flex h-screen flex-col overflow-hidden">
	<!-- Header -->
	<div class="flex shrink-0 items-center gap-4 border-b border-gray-200 bg-white px-4 py-3">
		<a
			href={`/gap-report/${projectId}`}
			aria-label="Back to gap report"
			class="rounded p-1 text-gray-400 hover:text-gray-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-600"
		>
			<ArrowLeft class="h-4 w-4" aria-hidden="true" />
		</a>

		<h1 class="min-w-0 flex-1 truncate font-mono text-sm font-medium text-gray-900">
			{filePath || 'File View'}
		</h1>

		{#if runs.length > 0}
			<div class="flex items-center gap-2">
				<label for="run-select" class="text-sm text-gray-500">Commit:</label>
				<select
					id="run-select"
					onchange={onRunChange}
					class="rounded border border-gray-200 px-2 py-1 text-sm focus:outline-none focus:ring-2 focus:ring-blue-600"
				>
					{#each runs as run (run.id)}
						<option value={run.id} selected={run.id === selectedRunId}>
							{run.commit_sha.slice(0, 7)}
							{run.branch ? `(${run.branch})` : ''}
						</option>
					{/each}
				</select>
			</div>
		{/if}
	</div>

	<!-- Coverage legend -->
	<div class="flex shrink-0 items-center gap-4 border-b border-gray-100 bg-gray-50 px-4 py-1.5 text-xs">
		<span class="flex items-center gap-1.5">
			<span class="h-3 w-1 rounded-sm bg-green-500" aria-hidden="true"></span>
			Covered
		</span>
		<span class="flex items-center gap-1.5">
			<span class="h-3 w-1 rounded-sm bg-red-500" aria-hidden="true"></span>
			Uncovered
		</span>
		<span class="flex items-center gap-1.5">
			<span class="h-3 w-1 rounded-sm bg-transparent border border-gray-300" aria-hidden="true"></span>
			Not instrumented
		</span>
		{#if testId}
			<span class="ml-4 text-blue-600">Showing coverage for one test</span>
		{/if}
	</div>

	<!-- File viewer -->
	<div class="min-h-0 flex-1 overflow-hidden" aria-live="polite">
		{#if loading}
			<Skeleton class="m-4 h-96 rounded-lg" label="Loading file coverage" />
		{:else if error}
			<div role="alert" class="flex items-center gap-2 p-4 text-red-700">
				<AlertCircle class="h-4 w-4 shrink-0" aria-hidden="true" />
				{error}
			</div>
		{:else if !filePath}
			<p class="p-4 text-gray-500">No file path specified.</p>
		{:else}
			<FileViewer
				{projectId}
				{filePath}
				commitSha={selectedSha}
				{source}
				language={detectLanguage(filePath)}
				coverage={coverage?.lines ?? []}
				{testId}
			/>
		{/if}
	</div>
</div>
