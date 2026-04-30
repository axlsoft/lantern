<script lang="ts">
	import { page } from '$app/state';
	import { api } from '$lib/api-client.js';
	import type { Run, Test, TestStatus } from '$lib/api-client.js';
	import { formatDate, formatDuration } from '$lib/utils.js';
	import Badge from '$lib/components/ui/Badge.svelte';
	import Button from '$lib/components/ui/Button.svelte';
	import Card from '$lib/components/ui/Card.svelte';
	import Input from '$lib/components/ui/Input.svelte';
	import Skeleton from '$lib/components/ui/Skeleton.svelte';
	import { AlertCircle, Clock, ArrowLeft } from 'lucide-svelte';

	const projectId = $derived(page.params.projectId!);
	const runId = $derived(page.params.runId!);

	let run = $state<Run | null>(null);
	let tests = $state<Test[]>([]);
	let nextCursor = $state<string | null>(null);
	let loading = $state(true);
	let loadingMore = $state(false);
	let error = $state('');

	let nameFilter = $state('');
	let statusFilter = $state<TestStatus | ''>('');
	let resultAnnouncement = $state('');

	let debounceTimer: ReturnType<typeof setTimeout> | undefined;

	$effect(() => {
		if (projectId && runId) initialLoad();
	});

	async function initialLoad() {
		loading = true;
		error = '';
		try {
			const [runRes, testsRes] = await Promise.all([
				api.getRun(projectId, runId),
				api.listRunTests(projectId, runId)
			]);
			run = runRes.run;
			tests = testsRes.tests;
			nextCursor = testsRes.next_cursor;
			resultAnnouncement = `Showing ${tests.length} tests`;
		} catch (err: unknown) {
			error = (err as { message?: string }).message ?? 'Failed to load run.';
		} finally {
			loading = false;
		}
	}

	async function loadMore() {
		if (!nextCursor || loadingMore) return;
		loadingMore = true;
		try {
			const res = await api.listRunTests(
				projectId,
				runId,
				nextCursor,
				statusFilter || undefined
			);
			tests = [...tests, ...res.tests];
			nextCursor = res.next_cursor;
		} finally {
			loadingMore = false;
		}
	}

	function scheduleFilter() {
		clearTimeout(debounceTimer);
		debounceTimer = setTimeout(applyFilter, 300);
	}

	async function applyFilter() {
		loading = true;
		tests = [];
		nextCursor = null;
		try {
			const res = await api.listRunTests(
				projectId,
				runId,
				undefined,
				statusFilter || undefined
			);
			// Client-side name filter (name filter endpoint not separate — apply locally).
			const all = res.tests;
			const filtered = nameFilter
				? all.filter((t) =>
						`${t.suite} ${t.name}`.toLowerCase().includes(nameFilter.toLowerCase())
					)
				: all;
			tests = filtered;
			nextCursor = res.next_cursor;
			resultAnnouncement = `Showing ${tests.length} tests`;
		} catch {
			// Ignore filter errors.
		} finally {
			loading = false;
		}
	}

	function statusVariant(status: TestStatus) {
		if (status === 'passed') return 'success' as const;
		if (status === 'failed') return 'danger' as const;
		if (status === 'timed_out') return 'warning' as const;
		return 'muted' as const;
	}
</script>

<svelte:head>
	<title>Run detail — Lantern</title>
</svelte:head>

<div class="mx-auto max-w-screen-xl px-4 py-8">
	<div class="mb-6 flex items-center gap-3">
		<a
			href={`/dashboard?project=${projectId}`}
			aria-label="Back to dashboard"
			class="rounded p-1 text-gray-400 hover:text-gray-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-600"
		>
			<ArrowLeft class="h-4 w-4" aria-hidden="true" />
		</a>
		<h1 class="text-2xl font-bold text-gray-900">Run detail</h1>
	</div>

	{#if loading && !run}
		<Skeleton class="h-40 rounded-lg mb-6" label="Loading run summary" />
	{:else if error}
		<div role="alert" class="flex items-center gap-2 rounded-md bg-red-50 p-4 text-red-700 mb-6">
			<AlertCircle class="h-4 w-4 shrink-0" aria-hidden="true" />
			{error}
		</div>
	{:else if run}
		<!-- Run summary card -->
		<Card class="mb-6 p-5">
			<div class="flex flex-wrap items-start justify-between gap-4">
				<div class="space-y-1">
					<div class="flex items-center gap-2">
						<Badge variant={run.status === 'completed' ? 'success' : run.status === 'failed' ? 'danger' : 'warning'}>
							<span class="sr-only">Status:</span>
							{run.status.replace('_', ' ')}
						</Badge>
						<span class="font-mono text-sm text-gray-600">{run.commit_sha.slice(0, 7)}</span>
						{#if run.branch}
							<span class="text-sm text-gray-400">{run.branch}</span>
						{/if}
					</div>
					<p class="text-xs text-gray-400">
						<Clock class="inline h-3 w-3 align-text-bottom" aria-hidden="true" />
						Started {formatDate(run.started_at)}
						{#if run.completed_at}
							· Duration {formatDuration(new Date(run.completed_at).getTime() - new Date(run.started_at).getTime())}
						{/if}
					</p>
				</div>

				<div class="flex gap-6 text-center">
					{#each [
						{ label: 'Total', val: run.total_tests },
						{ label: 'Passed', val: run.passed_tests, cls: 'text-green-600' },
						{ label: 'Failed', val: run.failed_tests, cls: 'text-red-600' },
						{ label: 'Skipped', val: run.skipped_tests, cls: 'text-gray-400' }
					] as stat (stat.label)}
						<div>
							<p class="text-2xl font-bold {stat.cls ?? 'text-gray-900'}">{stat.val}</p>
							<p class="text-xs text-gray-500">{stat.label}</p>
						</div>
					{/each}
				</div>
			</div>
		</Card>

		<!-- Filters -->
		<div class="mb-4 flex flex-wrap items-end gap-4">
			<div class="flex flex-col gap-1">
				<label for="name-filter" class="text-sm font-medium text-gray-700">Test name</label>
				<Input
					id="name-filter"
					type="text"
					bind:value={nameFilter}
					placeholder="Filter by name…"
					class="w-64"
					oninput={scheduleFilter}
				/>
			</div>
			<div class="flex flex-col gap-1">
				<label for="status-filter" class="text-sm font-medium text-gray-700">Status</label>
				<select
					id="status-filter"
					bind:value={statusFilter}
					onchange={applyFilter}
					class="h-9 rounded-md border border-gray-300 px-3 text-sm focus:outline-none focus:ring-2 focus:ring-blue-600"
				>
					<option value="">All</option>
					<option value="passed">Passed</option>
					<option value="failed">Failed</option>
					<option value="skipped">Skipped</option>
					<option value="timed_out">Timed out</option>
				</select>
			</div>
		</div>

		<!-- Result count announcement -->
		<div aria-live="polite" class="sr-only">{resultAnnouncement}</div>

		{#if tests.length === 0}
			<div class="rounded-lg border border-dashed border-gray-300 p-8 text-center">
				<p class="text-gray-500">No tests match the current filters.</p>
			</div>
		{:else}
			<Card>
				<table class="w-full text-sm">
					<thead class="bg-gray-50 text-left text-xs font-medium text-gray-500 uppercase tracking-wide">
						<tr>
							<th scope="col" class="px-4 py-3">Test</th>
							<th scope="col" class="px-4 py-3">Status</th>
							<th scope="col" class="px-4 py-3 text-right">Duration</th>
							<th scope="col" class="px-4 py-3 text-right">Coverage</th>
							<th scope="col" class="px-4 py-3"><span class="sr-only">Actions</span></th>
						</tr>
					</thead>
					<tbody class="divide-y divide-gray-100 bg-white">
						{#each tests as test (test.id)}
							<tr class="hover:bg-gray-50">
								<td class="px-4 py-3">
									<p class="font-medium text-gray-900">{test.name}</p>
									{#if test.suite}
										<p class="text-xs text-gray-400">{test.suite}</p>
									{/if}
								</td>
								<td class="px-4 py-3">
									<Badge variant={statusVariant(test.status)}>
										<span class="sr-only">Status:</span>
										{test.status.replace('_', ' ')}
									</Badge>
								</td>
								<td class="px-4 py-3 text-right text-gray-600">
									{test.duration_ms != null ? formatDuration(test.duration_ms) : '—'}
								</td>
								<td class="px-4 py-3 text-right text-gray-600">
									{test.covered_lines > 0 ? `${test.covered_lines} lines` : '—'}
								</td>
								<td class="px-4 py-3 text-right">
									<a
										href={`/file-view/${projectId}?test=${test.id}`}
										aria-label={`View coverage for ${test.name}`}
										class="text-blue-600 hover:underline"
									>
										Coverage
									</a>
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</Card>

			{#if nextCursor}
				<div class="mt-4 text-center">
					<Button variant="outline" loading={loadingMore} onclick={loadMore}>
						{loadingMore ? 'Loading…' : 'Load more'}
					</Button>
				</div>
			{/if}
		{/if}
	{/if}
</div>
