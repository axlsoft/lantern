<script lang="ts">
	import { cn } from '$lib/utils.js';

	interface Props {
		variant?: 'default' | 'outline' | 'ghost' | 'destructive' | 'link';
		size?: 'sm' | 'md' | 'lg' | 'icon';
		type?: 'button' | 'submit' | 'reset';
		form?: string;
		disabled?: boolean;
		loading?: boolean;
		class?: string;
		onclick?: (e: MouseEvent) => void;
		children: import('svelte').Snippet;
	}

	let {
		variant = 'default',
		size = 'md',
		type = 'button',
		form,
		disabled = false,
		loading = false,
		class: cls = '',
		onclick,
		children
	}: Props = $props();

	const base =
		'inline-flex items-center justify-center gap-2 rounded-md font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-offset-2 disabled:pointer-events-none disabled:opacity-50 select-none';

	const variants: Record<string, string> = {
		default: 'bg-blue-600 text-white hover:bg-blue-700 focus-visible:ring-blue-600',
		outline: 'border border-gray-300 bg-white hover:bg-gray-50 focus-visible:ring-blue-600',
		ghost: 'hover:bg-gray-100 focus-visible:ring-gray-400',
		destructive: 'bg-red-600 text-white hover:bg-red-700 focus-visible:ring-red-600',
		link: 'text-blue-600 underline-offset-4 hover:underline focus-visible:ring-blue-600'
	};

	const sizes: Record<string, string> = {
		sm: 'h-8 px-3 text-sm',
		md: 'h-9 px-4 text-sm',
		lg: 'h-10 px-6 text-base',
		icon: 'h-9 w-9'
	};
</script>

<button
	{type}
	{form}
	disabled={disabled || loading}
	aria-busy={loading}
	class={cn(base, variants[variant], sizes[size], cls)}
	{onclick}
>
	{#if loading}
		<svg
			class="h-4 w-4 animate-spin"
			xmlns="http://www.w3.org/2000/svg"
			fill="none"
			viewBox="0 0 24 24"
			aria-hidden="true"
		>
			<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"
			></circle>
			<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"
			></path>
		</svg>
		<span class="sr-only">Loading</span>
	{/if}
	{@render children()}
</button>
