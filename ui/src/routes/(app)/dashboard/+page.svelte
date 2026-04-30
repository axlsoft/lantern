<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import { api } from '$lib/api-client.js';
	import type { DashboardData } from '$lib/api-client.js';
	import { formatDate, pct } from '$lib/utils.js';
	import Badge from '$lib/components/ui/Badge.svelte';
	import Card from '$lib/components/ui/Card.svelte';
	import Skeleton from '$lib/components/ui/Skeleton.svelte';
	import { AlertCircle, GitPullRequest, Clock, TrendingUp, TrendingDown } from 'lucide-svelte';

	let projectId = $derived(page.url.searchParams.get('project'));
	let data = $state<DashboardData | null>(null);
	let loading = $state(true);
	let error = $state('');

	$effect(() => {
		if (projectId) {
			loadDashboard(projectId);
		} else {
			loading = false;
		}
	});

	async function loadDashboard(id: string) {
		loading = true;
		error = '';
		try {
			data = await api.getDashboard(id);
		} catch (err: unknown) {
			error = (err as { message?: string }).message ?? 'Failed to load dashboard.';
		} finally {
			loading = false;
		}
	}

	function runStatusVariant(status: string) {
		if (status === 'completed') return 'success' as const;
		if (status === 'failed' || status === 'aborted') return 'danger' as const;
		if (status === 'in_progress') return 'warning' as const;
		return 'muted' as const;
	}

	function runStatusLabel(status: string) {
		return status.replace('_', ' ');
	}
</script>

<svelte:head>
	<title>Dashboard — Lantern</title>
</svelte:head>

<div class="mx-auto max-w-screen-xl px-4 py-8">
	{#if !projectId}
		<div class="rounded-lg border border-dashed border-gray-300 p-12 text-center">
			<h1 class="text-xl font-semibold text-gray-900">Welcome to Lantern</h1>
			<p class="mt-2 text-sm text-gray-500">
				Select a project from the top navigation, or create one in
				<a href="/admin" class="text-blue-600 hover:underline">Admin</a>.
			</p>
		</div>
	{:else if loading}
		<div aria-live="polite">
			<div class="mb-6 grid grid-cols-4 gap-4">
				{#each [1, 2, 3, 4] as i (i)}
					<Skeleton class="h-28 rounded-lg" label="Loading coverage summary" />
				{/each}
			</div>
			<Skeleton class="h-64 rounded-lg" label="Loading recent runs" />
		</div>
	{:else if error}
		<div role="alert" class="flex items-center gap-2 rounded-md bg-red-50 p-4 text-red-700">
			<AlertCircle class="h-4 w-4 shrink-0" aria-hidden="true" />
			<span>{error}</span>
		</div>
	{:else if data}
		<div class="space-y-6">
			<h1 class="text-2xl font-bold text-gray-900">{data.project.name}</h1>

			<!-- ── Coverage summary cards ──────────────────────────────────── -->
			<section aria-label="Coverage summary">
				<div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
					<Card class="p-4">
						<p class="text-sm text-gray-500">Coverage</p>
						<p class="mt-1 text-3xl font-bold text-gray-900" aria-label="Coverage percentage">
							{pct(data.coverage_summary.covered_lines, data.coverage_summary.total_lines)}
						</p>
						<p class="mt-1 text-xs text-gray-400">
							{data.coverage_summary.covered_lines.toLocaleString()} /
							{data.coverage_summary.total_lines.toLocaleString()} lines
						</p>
					</Card>

					<Card class="p-4">
						<p class="text-sm text-gray-500">Uncovered lines</p>
						<p class="mt-1 text-3xl font-bold text-gray-900">
							{data.coverage_summary.uncovered_lines.toLocaleString()}
						</p>
					</Card>

					<Card class="p-4">
						<p class="text-sm text-gray-500">Gaps</p>
						<p class="mt-1 text-3xl font-bold text-gray-900">
							{data.coverage_summary.gap_count.toLocaleString()}
						</p>
						<a
							href={`/gap-report/${projectId}`}
							class="mt-1 block text-xs text-blue-600 hover:underline"
						>
							View full report
						</a>
					</Card>

					<Card class="p-4">
						<p class="text-sm text-gray-500">Last run</p>
						<p class="mt-1 text-sm font-medium text-gray-700">
							{data.coverage_summary.last_run_at
								? formatDate(data.coverage_summary.last_run_at)
								: 'No runs yet'}
						</p>
					</Card>
				</div>
			</section>

			<div class="grid gap-6 lg:grid-cols-2">
				<!-- ── Recent runs ─────────────────────────────────────────── -->
				<section aria-label="Recent runs">
					<h2 class="mb-3 text-base font-semibold text-gray-900">Recent runs</h2>
					{#if data.recent_runs.length === 0}
						<Card class="p-6 text-center">
							<p class="text-sm text-gray-500">No runs yet. Instrument your tests to get started.</p>
						</Card>
					{:else}
						<Card>
							<ul role="list">
								{#each data.recent_runs as run, i (run.id)}
									<li class="flex items-center justify-between gap-3 px-4 py-3 {i > 0 ? 'border-t border-gray-100' : ''}">
										<div class="min-w-0 flex-1">
											<div class="flex items-center gap-2">
												<Badge variant={runStatusVariant(run.status)}>
													<span class="sr-only">Status:</span>
													{runStatusLabel(run.status)}
												</Badge>
												<span class="truncate text-xs text-gray-500 font-mono">{run.commit_sha.slice(0, 7)}</span>
												{#if run.branch}
													<span class="truncate text-xs text-gray-400">{run.branch}</span>
												{/if}
											</div>
											<p class="mt-0.5 text-xs text-gray-400">
												<Clock class="inline h-3 w-3 align-text-bottom" aria-hidden="true" />
												{formatDate(run.started_at)}
											</p>
										</div>
										<a
											href={`/run-detail/${projectId}/${run.id}`}
											aria-label={`View run from ${run.commit_sha.slice(0, 7)}`}
											class="shrink-0 text-xs text-blue-600 hover:underline"
										>
											Details
										</a>
									</li>
								{/each}
							</ul>
						</Card>
					{/if}
				</section>

				<!-- ── Top gaps ────────────────────────────────────────────── -->
				<section aria-label="Top coverage gaps">
					<h2 class="mb-3 text-base font-semibold text-gray-900">Top gaps</h2>
					{#if data.top_gaps.length === 0}
						<Card class="p-6 text-center">
							<p class="text-sm text-gray-500">No gaps found — great coverage!</p>
						</Card>
					{:else}
						<Card>
							<ul role="list">
								{#each data.top_gaps as gap, i (gap.file_path + gap.line_start)}
									<li class="flex items-start justify-between gap-3 px-4 py-3 {i > 0 ? 'border-t border-gray-100' : ''}">
										<div class="min-w-0 flex-1">
											<p class="truncate text-sm font-medium text-gray-800">{gap.function_name}</p>
											<p class="truncate text-xs text-gray-400">{gap.file_path}</p>
										</div>
										<div class="shrink-0 text-right">
											<p class="text-sm font-semibold text-red-600">
												{gap.uncovered_lines} lines
											</p>
											<p class="text-xs text-gray-400">risk {gap.risk_score.toFixed(1)}</p>
										</div>
									</li>
								{/each}
							</ul>
							<div class="border-t border-gray-100 px-4 py-2">
								<a
									href={`/gap-report/${projectId}`}
									class="text-sm text-blue-600 hover:underline"
								>
									View all gaps →
								</a>
							</div>
						</Card>
					{/if}
				</section>
			</div>

			<!-- ── Open PRs ────────────────────────────────────────────────── -->
			{#if data.open_prs.length > 0}
				<section aria-label="Open pull requests with coverage data">
					<h2 class="mb-3 text-base font-semibold text-gray-900">Open pull requests</h2>
					<Card>
						<ul role="list">
							{#each data.open_prs as pr, i (pr.pr_number)}
								<li class="flex items-center justify-between gap-4 px-4 py-3 {i > 0 ? 'border-t border-gray-100' : ''}">
									<div class="flex items-center gap-3 min-w-0 flex-1">
										<GitPullRequest class="h-4 w-4 shrink-0 text-green-600" aria-hidden="true" />
										<span class="truncate text-sm text-gray-800">
											#{pr.pr_number}
											{#if pr.title}— {pr.title}{/if}
										</span>
									</div>
									<div class="flex shrink-0 items-center gap-3">
										{#if pr.covered_delta !== 0}
											<span
												class="flex items-center gap-0.5 text-sm font-medium {pr.covered_delta > 0 ? 'text-green-600' : 'text-red-600'}"
												aria-label="{pr.covered_delta > 0 ? 'Coverage increased by' : 'Coverage decreased by'} {Math.abs(pr.covered_delta)} lines"
											>
												{#if pr.covered_delta > 0}
													<TrendingUp class="h-3.5 w-3.5" aria-hidden="true" />+{pr.covered_delta}
												{:else}
													<TrendingDown class="h-3.5 w-3.5" aria-hidden="true" />{pr.covered_delta}
												{/if}
											</span>
										{/if}
										<a
											href={`/pr-view/${projectId}/${pr.pr_number}`}
											aria-label={`View coverage for PR #${pr.pr_number}`}
											class="text-xs text-blue-600 hover:underline"
										>
											View
										</a>
									</div>
								</li>
							{/each}
						</ul>
					</Card>
				</section>
			{/if}
		</div>
	{/if}
</div>
