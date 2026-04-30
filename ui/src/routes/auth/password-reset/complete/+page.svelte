<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import { api } from '$lib/api-client.js';
	import Button from '$lib/components/ui/Button.svelte';
	import FormField from '$lib/components/ui/FormField.svelte';
	import Input from '$lib/components/ui/Input.svelte';

	let token = $state('');
	let password = $state('');
	let error = $state('');
	let loading = $state(false);
	let done = $state(false);

	onMount(() => {
		const t = page.url.searchParams.get('token') ?? '';
		token = t;
		// Strip token from URL immediately.
		if (t) history.replaceState({}, '', '/auth/password-reset/complete');
	});

	async function handleSubmit(e: SubmitEvent) {
		e.preventDefault();
		error = '';
		if (!token) {
			error = 'Invalid or expired reset link.';
			return;
		}
		loading = true;
		try {
			await api.completePasswordReset(token, password);
			done = true;
		} catch (err: unknown) {
			error = (err as { message?: string }).message ?? 'Reset failed. The link may have expired.';
		} finally {
			loading = false;
		}
	}
</script>

<svelte:head>
	<title>Set new password — Lantern</title>
</svelte:head>

<main class="flex min-h-screen items-center justify-center bg-gray-50 px-4">
	<div class="w-full max-w-sm">
		<h1 class="mb-6 text-center text-2xl font-bold text-gray-900">Set new password</h1>

		{#if done}
			<div role="status" aria-live="polite" class="rounded-md bg-green-50 p-4 text-sm text-green-800">
				<p class="font-medium">Password updated</p>
				<button
					onclick={() => goto('/auth/login')}
					class="mt-2 font-medium text-green-700 underline-offset-2 hover:underline"
				>
					Sign in with your new password
				</button>
			</div>
		{:else}
			<form onsubmit={handleSubmit} novalidate class="flex flex-col gap-4">
				{#if error}
					<div role="alert" aria-live="assertive" class="rounded-md bg-red-50 p-3 text-sm text-red-700">
						{error}
					</div>
				{/if}

				<FormField id="password" label="New password">
					<Input
						id="password"
						type="password"
						bind:value={password}
						autocomplete="new-password"
						required
					/>
				</FormField>

				<Button type="submit" {loading} class="w-full">
					{loading ? 'Saving…' : 'Save password'}
				</Button>
			</form>
		{/if}
	</div>
</main>
