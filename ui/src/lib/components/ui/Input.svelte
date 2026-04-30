<script lang="ts">
	import { cn } from '$lib/utils.js';

	interface Props {
		id?: string;
		type?: string;
		value?: string;
		placeholder?: string;
		disabled?: boolean;
		required?: boolean;
		autocomplete?: HTMLInputElement['autocomplete'];
		'aria-describedby'?: string;
		'aria-label'?: string;
		'aria-invalid'?: boolean | 'true' | 'false';
		class?: string;
		oninput?: (e: Event & { currentTarget: HTMLInputElement }) => void;
		onchange?: (e: Event & { currentTarget: HTMLInputElement }) => void;
	}

	let {
		id,
		type = 'text',
		value = $bindable(''),
		placeholder,
		disabled = false,
		required = false,
		autocomplete,
		'aria-describedby': ariaDescribedBy,
		'aria-label': ariaLabel,
		'aria-invalid': ariaInvalid,
		class: cls = '',
		oninput,
		onchange
	}: Props = $props();
</script>

<input
	{id}
	{type}
	bind:value
	{placeholder}
	{disabled}
	{required}
	{autocomplete}
	aria-describedby={ariaDescribedBy}
	aria-label={ariaLabel}
	aria-invalid={ariaInvalid}
	class={cn(
		'flex h-9 w-full rounded-md border border-gray-300 bg-white px-3 py-1 text-sm shadow-sm',
		'placeholder:text-gray-400',
		'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-600 focus-visible:ring-offset-1',
		'disabled:cursor-not-allowed disabled:opacity-50',
		'aria-invalid:border-red-500 aria-invalid:focus-visible:ring-red-500',
		cls
	)}
	{oninput}
	{onchange}
/>
