<script lang="ts">
	import { cn } from '$lib/utils.js';

	interface Props {
		class?: string;
		label?: string;
	}
	let { class: cls = '', label = 'Loading…' }: Props = $props();
</script>

<div
	role="status"
	aria-busy="true"
	aria-label={label}
	class={cn('animate-pulse rounded bg-gray-200', cls)}
></div>
