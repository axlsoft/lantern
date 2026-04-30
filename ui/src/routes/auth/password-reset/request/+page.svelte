<script lang="ts">
	import { api } from '$lib/api-client.js';
	import Button from '$lib/components/ui/Button.svelte';
	import FormField from '$lib/components/ui/FormField.svelte';
	import Input from '$lib/components/ui/Input.svelte';

	let email = $state('');
	let error = $state('');
	let loading = $state(false);
	let done = $state(false);

	async function handleSubmit(e: SubmitEvent) {
		e.preventDefault();
		error = '';
		loading = true;
		try {
			await api.requestPasswordReset(email);
			done = true;
		} catch (err: unknown) {
			const apiErr = err as { status?: number; message?: string };
			if (apiErr.status === 429) {
				error = 'Too many attempts. Please wait a moment and try again.';
			} else {
				error = apiErr.message ?? 'Request failed. Please try again.';
			}
		} finally {
			loading = false;
		}
	}
</script>

<svelte:head>
	<title>Reset password — Lantern</title>
</svelte:head>

<main class="flex min-h-screen items-center justify-center bg-gray-50 px-4">
	<div class="w-full max-w-sm">
		<h1 class="mb-6 text-center text-2xl font-bold text-gray-900">Reset your password</h1>

		{#if done}
			<div role="status" aria-live="polite" class="rounded-md bg-green-50 p-4 text-sm text-green-800">
				<p class="font-medium">Check your inbox</p>
				<p class="mt-1">If an account exists for {email}, you'll receive a reset link shortly.</p>
			</div>
		{:else}
			<form onsubmit={handleSubmit} novalidate class="flex flex-col gap-4">
				{#if error}
					<div role="alert" aria-live="assertive" class="rounded-md bg-red-50 p-3 text-sm text-red-700">
						{error}
					</div>
				{/if}

				<FormField id="email" label="Email address">
					<Input
						id="email"
						type="email"
						bind:value={email}
						autocomplete="email"
						required
						placeholder="you@example.com"
					/>
				</FormField>

				<Button type="submit" {loading} class="w-full">
					{loading ? 'Sending…' : 'Send reset link'}
				</Button>
			</form>

			<p class="mt-4 text-center text-sm">
				<a href="/auth/login" class="font-medium text-blue-600 hover:underline">Back to sign in</a>
			</p>
		{/if}
	</div>
</main>
