<script lang="ts">
	/**
	 * FileViewer renders a source file with line-level coverage overlay.
	 *
	 * - Syntax highlighted via Shiki (only approved {@html} usage in this codebase).
	 * - Lines > VIRTUALIZE_THRESHOLD are rendered via @tanstack/svelte-virtual.
	 * - Coverage state (covered/uncovered/not-instrumented) is conveyed by
	 *   both colour and a visually-hidden label on each line.
	 * - Line tooltips appear on both hover and keyboard focus.
	 */
	import { createVirtualizer, type SvelteVirtualizer } from '@tanstack/svelte-virtual';
	import { codeToHtml } from 'shiki';
	import type { LineCoverage, LineTests } from '$lib/api-client.js';
	import { api } from '$lib/api-client.js';

	const VIRTUALIZE_THRESHOLD = 1000;
	const LINE_HEIGHT = 22; // px — matches pre line-height

	interface Props {
		projectId: string;
		filePath: string;
		commitSha: string;
		source: string;
		language: string;
		coverage: LineCoverage[];
		testId?: string;
	}

	let { projectId, filePath, commitSha, source, language, coverage, testId }: Props = $props();

	// Build a line→coverage map for O(1) lookup.
	const coverageMap = $derived(
		new Map(coverage.map((c) => [c.line, c]))
	);

	const sourceLines = $derived(source.split('\n'));
	const lineCount = $derived(sourceLines.length);

	// Shiki highlighted HTML lines (array, one element per source line).
	let htmlLines = $state<string[]>([]);
	let highlightLoading = $state(true);

	$effect(() => {
		if (!source) return;
		highlightAsync(source, language);
	});

	async function highlightAsync(src: string, lang: string) {
		highlightLoading = true;
		try {
			// Shiki sanitizes its own HTML output — approved {@html} use site.
			const html = await codeToHtml(src, {
				lang,
				theme: 'github-light',
				structure: 'inline'
			});
			// Split the pre/code wrapper into per-line spans.
			// Shiki inline mode returns bare spans per token; we wrap per line.
			htmlLines = splitToLines(html, src);
		} catch {
			// Fallback: render plain text lines.
			htmlLines = sourceLines.map((l) => escapeHtml(l));
		} finally {
			highlightLoading = false;
		}
	}

	function splitToLines(highlighted: string, rawSrc: string): string[] {
		// Shiki's inline structure returns a string of spans without line wrapping.
		// We split by newlines in the raw source and match token boundaries.
		// For simplicity, split the highlighted output by \n-equivalent spans.
		// This works because Shiki preserves newline positions.
		return highlighted.split('\n').map((l) => l || ' ');
	}

	function escapeHtml(s: string): string {
		return s
			.replace(/&/g, '&amp;')
			.replace(/</g, '&lt;')
			.replace(/>/g, '&gt;')
			.replace(/"/g, '&quot;');
	}

	function lineStatus(lineNum: number): 'covered' | 'uncovered' | 'none' {
		const c = coverageMap.get(lineNum);
		if (!c) return 'none';
		// When filtering by testId, only count lines where that test contributed.
		if (testId && !c.test_ids.includes(testId)) return 'none';
		return c.hit_count > 0 ? 'covered' : 'uncovered';
	}

	const statusClasses: Record<string, string> = {
		covered: 'border-l-4 border-green-500 bg-green-50',
		uncovered: 'border-l-4 border-red-500 bg-red-50',
		none: 'border-l-4 border-transparent'
	};

	const statusLabels: Record<string, string> = {
		covered: 'covered',
		uncovered: 'uncovered',
		none: 'not instrumented'
	};

	// Tooltip state
	let tooltipLine = $state<number | null>(null);
	let tooltipTests = $state<LineTests['tests']>([]);
	let tooltipLoading = $state(false);
	let tooltipEl = $state<HTMLDivElement | null>(null);

	async function showTooltip(lineNum: number) {
		const c = coverageMap.get(lineNum);
		if (!c || c.hit_count === 0) return;
		tooltipLine = lineNum;
		tooltipLoading = true;
		tooltipTests = [];
		try {
			const res = await api.getLineTests(projectId, filePath, lineNum, commitSha);
			tooltipTests = res.tests;
		} catch {
			tooltipTests = [];
		} finally {
			tooltipLoading = false;
		}
	}

	function hideTooltip() {
		tooltipLine = null;
		tooltipTests = [];
	}

	// Virtualizer (activated above threshold).
	// createVirtualizer returns a Readable<SvelteVirtualizer>; we subscribe via
	// $effect to unwrap the store value before calling its methods.
	const useVirtualizer = $derived(lineCount > VIRTUALIZE_THRESHOLD);
	let scrollerEl = $state<HTMLDivElement | null>(null);

	let virtualItems = $state<ReturnType<SvelteVirtualizer<HTMLDivElement, Element>['getVirtualItems']> | null>(null);
	let totalSize = $state(0);

	$effect(() => {
		if (!useVirtualizer || !scrollerEl) {
			virtualItems = null;
			totalSize = 0;
			return;
		}
		const store = createVirtualizer({
			count: lineCount,
			getScrollElement: () => scrollerEl,
			estimateSize: () => LINE_HEIGHT,
			overscan: 20
		});
		const unsub = store.subscribe((v) => {
			virtualItems = v.getVirtualItems();
			totalSize = v.getTotalSize();
		});
		return unsub;
	});
</script>

<div class="relative text-sm" aria-live="polite">
	{#if highlightLoading}
		<p class="p-4 text-gray-400" aria-busy="true">Highlighting source…</p>
	{/if}

	{#if useVirtualizer}
		<!-- Virtualized rendering for large files -->
		<div
			bind:this={scrollerEl}
			class="h-[70vh] overflow-auto"
			role="list"
			aria-label="Source file lines"
		>
			<div style:height="{totalSize}px" class="relative w-full font-mono">
				{#if virtualItems}
					{#each virtualItems as vRow (vRow.index)}
						{@const lineNum = vRow.index + 1}
						{@const status = lineStatus(lineNum)}
						{@const htmlLine = htmlLines[vRow.index] ?? ''}
						<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
						<div
							role="listitem"
							class="absolute top-0 left-0 flex w-full items-start {statusClasses[status]}"
							style:transform="translateY({vRow.start}px)"
							style:height="{LINE_HEIGHT}px"
							aria-label="Line {lineNum} — {statusLabels[status]}"
							tabindex={status === 'covered' ? 0 : -1}
							onfocus={() => showTooltip(lineNum)}
							onblur={hideTooltip}
							onmouseenter={() => showTooltip(lineNum)}
							onmouseleave={hideTooltip}
						>
							<span aria-hidden="true" class="w-12 shrink-0 select-none pr-3 text-right text-xs text-gray-300 leading-[22px]">
								{lineNum}
							</span>
							<span class="sr-only">Line {lineNum} — {statusLabels[status]}</span>
							<pre class="min-w-0 flex-1 leading-[22px]">{@html htmlLine}</pre>

							{#if tooltipLine === lineNum}
								<div
									bind:this={tooltipEl}
									role="tooltip"
									class="absolute left-16 top-6 z-50 min-w-[200px] rounded-md border border-gray-200 bg-white p-2 shadow-lg text-xs"
								>
									{#if tooltipLoading}
										<p class="text-gray-400">Loading tests…</p>
									{:else if tooltipTests.length === 0}
										<p class="text-gray-400">No tests recorded for this line.</p>
									{:else}
										<ul>
											{#each tooltipTests as t (t.id)}
												<li class="py-0.5">{t.suite ? `${t.suite} › ` : ''}{t.name}</li>
											{/each}
										</ul>
									{/if}
								</div>
							{/if}
						</div>
					{/each}
				{/if}
			</div>
		</div>
	{:else}
		<!-- Direct rendering for small files -->
		<div
			class="overflow-auto font-mono"
			role="list"
			aria-label="Source file lines"
		>
			{#each sourceLines as rawLine, idx (idx)}
				{@const lineNum = idx + 1}
				{@const status = lineStatus(lineNum)}
				{@const htmlLine = htmlLines[idx] ?? escapeHtml(rawLine)}
				<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
				<div
					role="listitem"
					class="flex items-start {statusClasses[status]}"
					style:min-height="{LINE_HEIGHT}px"
					aria-label="Line {lineNum} — {statusLabels[status]}"
					tabindex={status === 'covered' ? 0 : -1}
					onfocus={() => showTooltip(lineNum)}
					onblur={hideTooltip}
					onmouseenter={() => showTooltip(lineNum)}
					onmouseleave={hideTooltip}
				>
					<span aria-hidden="true" class="w-12 shrink-0 select-none pr-3 text-right text-xs text-gray-300 leading-[22px]">
						{lineNum}
					</span>
					<span class="sr-only">Line {lineNum} — {statusLabels[status]}</span>
					<pre class="min-w-0 flex-1 leading-[22px] whitespace-pre">{@html htmlLine}</pre>

					{#if tooltipLine === lineNum}
						<div
							role="tooltip"
							class="absolute left-16 z-50 min-w-[200px] rounded-md border border-gray-200 bg-white p-2 shadow-lg text-xs"
						>
							{#if tooltipLoading}
								<p class="text-gray-400">Loading tests…</p>
							{:else if tooltipTests.length === 0}
								<p class="text-gray-400">No tests recorded for this line.</p>
							{:else}
								<ul>
									{#each tooltipTests as t (t.id)}
										<li class="py-0.5">{t.suite ? `${t.suite} › ` : ''}{t.name}</li>
									{/each}
								</ul>
							{/if}
						</div>
					{/if}
				</div>
			{/each}
		</div>
	{/if}
</div>
