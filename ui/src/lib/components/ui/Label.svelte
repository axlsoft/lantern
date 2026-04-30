<script lang="ts">
	import { cn } from '$lib/utils.js';

	interface Props {
		for?: string;
		class?: string;
		children: import('svelte').Snippet;
	}
	let { for: forAttr, class: cls = '', children }: Props = $props();
</script>

<label for={forAttr} class={cn('block text-sm font-medium text-gray-700', cls)}>
	{@render children()}
</label>
