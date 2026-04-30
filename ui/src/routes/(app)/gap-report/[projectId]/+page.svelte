<script lang="ts">
	import { page } from '$app/state';
	import { api } from '$lib/api-client.js';
	import type { GapEntry } from '$lib/api-client.js';
	import { CircleAlert, ThumbsUp, ThumbsDown } from 'lucide-svelte';
	import Button from '$lib/components/ui/Button.svelte';
	import Input from '$lib/components/ui/Input.svelte';
	import Skeleton from '$lib/components/ui/Skeleton.svelte';

	const projectId = $derived(page.params.projectId!);

	let gaps = $state<GapEntry[]>([]);
	let total = $state(0);
	let nextCursor = $state<string | null>(null);
	let loading = $state(true);
	let loadingMore = $state(false);
	let error = $state('');

	// Filters
	let pathFilter = $state('');
	let minRisk = $state<number | undefined>(undefined);
	let filterResultAnnouncement = $state('');

	let debounceTimer: ReturnType<typeof setTimeout> | undefined;

	function scheduleLoad() {
		clearTimeout(debounceTimer);
		debounceTimer = setTimeout(() => loadGaps(true), 350);
	}

	$effect(() => {
		if (projectId) loadGaps(true);
	});

	async function loadGaps(reset: boolean) {
		if (reset) {
			loading = true;
			gaps = [];
			nextCursor = null;
		} else {
			loadingMore = true;
		}
		error = '';
		try {
			const res = await api.getGaps(projectId, {
				cursor: reset ? undefined : (nextCursor ?? undefined),
				path: pathFilter || undefined,
				min_risk: minRisk
			});
			if (reset) {
				gaps = res.gaps;
				total = res.total;
			} else {
				gaps = [...gaps, ...res.gaps];
			}
			nextCursor = res.next_cursor;
			filterResultAnnouncement = `Showing ${gaps.length} of ${total} gaps`;
		} catch (err: unknown) {
			error = (err as { message?: string }).message ?? 'Failed to load gaps.';
		} finally {
			loading = false;
			loadingMore = false;
		}
	}

	function handlePathInput() {
		scheduleLoad();
	}

	function handleMinRiskChange(e: Event) {
		const val = (e.currentTarget as HTMLInputElement).value;
		minRisk = val ? parseFloat(val) : undefined;
		scheduleLoad();
	}
</script>

<svelte:head>
	<title>Gap Report — Lantern</title>
</svelte:head>

<div class="mx-auto max-w-screen-xl px-4 py-8">
	<div class="mb-6 flex items-center justify-between">
		<h1 class="text-2xl font-bold text-gray-900">Gap Report</h1>
		<span class="text-sm text-gray-500" aria-live="polite">
			{total > 0 ? `${total.toLocaleString()} gaps` : ''}
		</span>
	</div>

	<!-- Filters -->
	<div class="mb-4 flex flex-wrap items-end gap-4">
		<div class="flex flex-col gap-1">
			<label for="path-filter" class="text-sm font-medium text-gray-700">File path</label>
			<Input
				id="path-filter"
				type="text"
				bind:value={pathFilter}
				placeholder="Filter by path…"
				class="w-64"
				oninput={handlePathInput}
			/>
		</div>
		<div class="flex flex-col gap-1">
			<label for="min-risk" class="text-sm font-medium text-gray-700">Min risk score</label>
			<Input
				id="min-risk"
				type="number"
				placeholder="0.0"
				class="w-28"
				onchange={handleMinRiskChange}
			/>
		</div>
	</div>

	<!-- Live region for filter result count -->
	<div aria-live="polite" class="sr-only">{filterResultAnnouncement}</div>

	{#if error}
		<div role="alert" class="mb-4 flex items-center gap-2 rounded-md bg-red-50 p-4 text-red-700">
			<CircleAlert class="h-4 w-4 shrink-0" aria-hidden="true" />
			{error}
		</div>
	{/if}

	{#if loading}
		<div aria-live="polite">
			<Skeleton class="h-96 w-full rounded-lg" label="Loading gap report" />
		</div>
	{:else if gaps.length === 0}
		<div class="rounded-lg border border-dashed border-gray-300 p-12 text-center">
			<p class="text-gray-500">
				No gaps found — your coverage is excellent, or you need more functional tests.
			</p>
		</div>
	{:else}
		<div class="overflow-x-auto rounded-lg border border-gray-200">
			<table class="w-full text-sm">
				<thead
					class="bg-gray-50 text-left text-xs font-medium tracking-wide text-gray-500 uppercase"
				>
					<tr>
						<th scope="col" class="px-4 py-3">File / Function</th>
						<th scope="col" class="px-4 py-3 text-right">Uncovered</th>
						<th scope="col" class="px-4 py-3 text-right" aria-sort="descending"> Risk score </th>
						<th scope="col" class="px-4 py-3 text-right">Feedback</th>
						<th scope="col" class="px-4 py-3"><span class="sr-only">Actions</span></th>
					</tr>
				</thead>
				<tbody class="divide-y divide-gray-100 bg-white">
					{#each gaps as gap (gap.file_path + ':' + gap.line_start)}
						<tr class="hover:bg-gray-50">
							<td class="px-4 py-3">
								<p class="font-medium text-gray-900">{gap.function_name}</p>
								<p class="font-mono text-xs text-gray-400">{gap.file_path}:{gap.line_start}</p>
							</td>
							<td class="px-4 py-3 text-right font-semibold text-red-600">
								{gap.uncovered_lines}
							</td>
							<td class="px-4 py-3 text-right text-gray-700">
								{gap.risk_score.toFixed(2)}
							</td>
							<td class="px-4 py-3 text-right">
								<div class="flex justify-end gap-1">
									<button
										aria-label={`This gap is useful — ${gap.function_name} in ${gap.file_path}`}
										class="rounded p-1 text-gray-400 hover:text-green-600 focus-visible:ring-2 focus-visible:ring-blue-600 focus-visible:outline-none"
									>
										<ThumbsUp class="h-3.5 w-3.5" aria-hidden="true" />
									</button>
									<button
										aria-label={`This gap is not useful — ${gap.function_name} in ${gap.file_path}`}
										class="rounded p-1 text-gray-400 hover:text-red-500 focus-visible:ring-2 focus-visible:ring-blue-600 focus-visible:outline-none"
									>
										<ThumbsDown class="h-3.5 w-3.5" aria-hidden="true" />
									</button>
								</div>
							</td>
							<td class="px-4 py-3 text-right">
								<a
									href={`/file-view/${projectId}?path=${encodeURIComponent(gap.file_path)}&line=${gap.line_start}`}
									aria-label={`View file coverage for ${gap.function_name} in ${gap.file_path}`}
									class="text-blue-600 hover:underline"
								>
									View
								</a>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>

		{#if nextCursor}
			<div class="mt-4 text-center">
				<Button variant="outline" loading={loadingMore} onclick={() => loadGaps(false)}>
					{loadingMore ? 'Loading…' : 'Load more'}
				</Button>
			</div>
		{/if}
	{/if}
</div>
