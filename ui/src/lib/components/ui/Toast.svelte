<script lang="ts">
	import { cn } from '$lib/utils.js';

	type ToastVariant = 'default' | 'success' | 'error';

	interface Props {
		message: string;
		variant?: ToastVariant;
		onclose?: () => void;
	}

	let { message, variant = 'default', onclose }: Props = $props();

	const variants: Record<ToastVariant, string> = {
		default: 'bg-gray-900 text-white',
		success: 'bg-green-700 text-white',
		error: 'bg-red-700 text-white'
	};
</script>

<div
	role="status"
	aria-live="polite"
	class={cn(
		'pointer-events-auto flex items-center justify-between gap-3 rounded-md px-4 py-3 text-sm shadow-lg',
		variants[variant]
	)}
>
	<span>{message}</span>
	{#if onclose}
		<button
			onclick={onclose}
			aria-label="Dismiss notification"
			class="text-current opacity-70 hover:opacity-100 focus-visible:ring-1 focus-visible:ring-white focus-visible:outline-none"
		>
			✕
		</button>
	{/if}
</div>
