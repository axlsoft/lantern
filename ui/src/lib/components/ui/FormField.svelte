<script lang="ts">
	interface Props {
		id: string;
		label: string;
		error?: string;
		children: import('svelte').Snippet;
	}
	let { id, label, error, children }: Props = $props();
	const errorId = $derived(`${id}-error`);
</script>

<div class="flex flex-col gap-1">
	<label for={id} class="block text-sm font-medium text-gray-700">{label}</label>
	{@render children()}
	{#if error}
		<p id={errorId} role="alert" class="text-sm text-red-600">{error}</p>
	{/if}
</div>
