<script lang="ts">
	import { cn } from '$lib/utils.js';

	interface Props {
		class?: string;
		children: import('svelte').Snippet;
	}
	let { class: cls = '', children }: Props = $props();
</script>

<div class={cn('rounded-lg border border-gray-200 bg-white shadow-sm', cls)}>
	{@render children()}
</div>
