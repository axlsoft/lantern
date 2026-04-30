<script lang="ts">
	import { Dialog } from 'bits-ui';
	import { cn } from '$lib/utils.js';
	import { X } from 'lucide-svelte';

	interface Props {
		open?: boolean;
		title: string;
		description?: string;
		class?: string;
		onclose?: () => void;
		children: import('svelte').Snippet;
		footer?: import('svelte').Snippet;
	}

	let {
		open = $bindable(false),
		title,
		description,
		class: cls = '',
		onclose,
		children,
		footer
	}: Props = $props();

	function handleClose() {
		open = false;
		onclose?.();
	}
</script>

<Dialog.Root bind:open onOpenChange={(v) => { if (!v) handleClose(); }}>
	<Dialog.Portal>
		<Dialog.Overlay class="fixed inset-0 z-50 bg-black/50" />
		<Dialog.Content
			class={cn(
				'fixed left-1/2 top-1/2 z-50 w-full max-w-lg -translate-x-1/2 -translate-y-1/2 rounded-lg bg-white p-6 shadow-lg',
				cls
			)}
		>
			<div class="mb-4 flex items-start justify-between gap-4">
				<div>
					<Dialog.Title class="text-lg font-semibold text-gray-900">{title}</Dialog.Title>
					{#if description}
						<Dialog.Description class="mt-1 text-sm text-gray-500">{description}</Dialog.Description>
					{/if}
				</div>
				<button
					onclick={handleClose}
					aria-label="Close dialog"
					class="rounded p-1 text-gray-400 hover:bg-gray-100 hover:text-gray-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-600"
				>
					<X class="h-4 w-4" aria-hidden="true" />
				</button>
			</div>

			{@render children()}

			{#if footer}
				<div class="mt-6 flex justify-end gap-3">
					{@render footer()}
				</div>
			{/if}
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>
