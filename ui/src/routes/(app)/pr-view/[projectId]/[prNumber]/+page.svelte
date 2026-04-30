<script lang="ts">
	import { page } from '$app/state';
	import { api } from '$lib/api-client.js';
	import type { PrDetail } from '$lib/api-client.js';
	import Card from '$lib/components/ui/Card.svelte';
	import Skeleton from '$lib/components/ui/Skeleton.svelte';
	import { CircleAlert, ArrowLeft, Clock } from 'lucide-svelte';

	const projectId = $derived(page.params.projectId!);
	const prNumber = $derived(parseInt(page.params.prNumber!, 10));

	let pr = $state<PrDetail | null>(null);
	let loading = $state(true);
	let error = $state('');

	$effect(() => {
		if (projectId && prNumber) loadPr();
	});

	async function loadPr() {
		loading = true;
		error = '';
		try {
			const res = await api.getPr(projectId, prNumber);
			pr = res.pr;
		} catch (err: unknown) {
			error = (err as { message?: string }).message ?? 'Failed to load PR.';
		} finally {
			loading = false;
		}
	}
</script>

<svelte:head>
	<title>PR #{prNumber} coverage — Lantern</title>
</svelte:head>

<div class="mx-auto max-w-screen-xl px-4 py-8">
	<div class="mb-6 flex items-center gap-3">
		<a
			href={`/dashboard?project=${projectId}`}
			aria-label="Back to dashboard"
			class="rounded p-1 text-gray-400 hover:text-gray-600 focus-visible:ring-2 focus-visible:ring-blue-600 focus-visible:outline-none"
		>
			<ArrowLeft class="h-4 w-4" aria-hidden="true" />
		</a>
		<h1 class="text-2xl font-bold text-gray-900">PR #{prNumber} coverage</h1>
	</div>

	{#if loading}
		<Skeleton class="h-64 rounded-lg" label="Loading PR coverage" />
	{:else if error}
		<div role="alert" class="flex items-center gap-2 rounded-md bg-red-50 p-4 text-red-700">
			<CircleAlert class="h-4 w-4 shrink-0" aria-hidden="true" />
			{error}
		</div>
	{:else if pr}
		{#if pr.pending}
			<div
				role="status"
				aria-live="polite"
				class="mb-6 flex items-center gap-2 rounded-md bg-yellow-50 p-4 text-yellow-800"
			>
				<Clock class="h-4 w-4 shrink-0" aria-hidden="true" />
				Coverage run has not completed yet for this PR's head commit ({pr.head_sha.slice(0, 7)}).
				Refresh when the CI run completes.
			</div>
		{:else}
			<Card class="mb-4 p-4">
				<p class="mb-1 text-sm text-gray-500">Head commit</p>
				<p class="font-mono text-sm text-gray-900">{pr.head_sha.slice(0, 7)}</p>
			</Card>
		{/if}

		{#if pr.changed_files.length === 0}
			<div class="rounded-lg border border-dashed border-gray-300 p-8 text-center">
				<p class="text-gray-500">No changed files with coverage data for this PR.</p>
			</div>
		{:else}
			<div class="overflow-x-auto rounded-lg border border-gray-200">
				<table class="w-full text-sm">
					<caption class="sr-only">Changed files with coverage data for PR #{prNumber}</caption>
					<thead
						class="bg-gray-50 text-left text-xs font-medium tracking-wide text-gray-500 uppercase"
					>
						<tr>
							<th scope="col" class="px-4 py-3">File</th>
							<th scope="col" class="px-4 py-3 text-right">Added lines</th>
							<th scope="col" class="px-4 py-3 text-right">
								<span>Covered</span>
								<span class="sr-only">(lines covered in this PR)</span>
							</th>
							<th scope="col" class="px-4 py-3 text-right">
								<span>Uncovered</span>
								<span class="sr-only">(lines uncovered in this PR)</span>
							</th>
							<th scope="col" class="px-4 py-3"><span class="sr-only">Actions</span></th>
						</tr>
					</thead>
					<tbody class="divide-y divide-gray-100 bg-white">
						{#each pr.changed_files as file (file.file_path)}
							<tr class="hover:bg-gray-50">
								<td class="px-4 py-3 font-mono text-xs text-gray-800">{file.file_path}</td>
								<td class="px-4 py-3 text-right text-gray-600">
									<span aria-label="{file.added_lines} lines added in this PR"
										>{file.added_lines}</span
									>
								</td>
								<td class="px-4 py-3 text-right font-semibold text-green-600">
									<span aria-label="{file.covered_lines} lines covered in this PR"
										>{file.covered_lines}</span
									>
								</td>
								<td class="px-4 py-3 text-right font-semibold text-red-600">
									<span aria-label="{file.uncovered_lines} lines uncovered in this PR"
										>{file.uncovered_lines}</span
									>
								</td>
								<td class="px-4 py-3 text-right">
									{#if pr.run_id}
										<a
											href={`/file-view/${projectId}?path=${encodeURIComponent(file.file_path)}&commit=${pr.head_sha}`}
											aria-label={`View coverage for ${file.file_path} in PR #${prNumber}`}
											class="text-blue-600 hover:underline"
										>
											View
										</a>
									{/if}
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	{/if}
</div>
