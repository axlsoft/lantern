<script lang="ts">
	import { cn } from '$lib/utils.js';

	type Variant = 'default' | 'success' | 'warning' | 'danger' | 'muted';

	interface Props {
		variant?: Variant;
		class?: string;
		children: import('svelte').Snippet;
	}

	let { variant = 'default', class: cls = '', children }: Props = $props();

	const variants: Record<Variant, string> = {
		default: 'bg-blue-100 text-blue-800',
		success: 'bg-green-100 text-green-800',
		warning: 'bg-yellow-100 text-yellow-800',
		danger: 'bg-red-100 text-red-800',
		muted: 'bg-gray-100 text-gray-600'
	};
</script>

<span class={cn('inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium', variants[variant], cls)}>
	{@render children()}
</span>
