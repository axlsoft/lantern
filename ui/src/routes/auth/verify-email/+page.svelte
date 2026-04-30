<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import { api } from '$lib/api-client.js';

	let status = $state<'pending' | 'success' | 'error'>('pending');
	let errorMsg = $state('');

	onMount(async () => {
		const token = page.url.searchParams.get('token');
		if (!token) {
			status = 'error';
			errorMsg = 'Missing verification token.';
			return;
		}

		// Strip token from URL immediately so it doesn't persist in history.
		history.replaceState({}, '', '/auth/verify-email');

		try {
			await api.verifyEmail(token);
			status = 'success';
		} catch (err: unknown) {
			status = 'error';
			errorMsg = (err as { message?: string }).message ?? 'Verification failed or link expired.';
		}
	});
</script>

<svelte:head>
	<title>Verify email — Lantern</title>
</svelte:head>

<main class="flex min-h-screen items-center justify-center bg-gray-50 px-4">
	<div class="w-full max-w-sm text-center">
		{#if status === 'pending'}
			<p class="text-gray-600" aria-live="polite" aria-busy="true">Verifying your email…</p>
		{:else if status === 'success'}
			<div role="status" aria-live="polite" class="rounded-md bg-green-50 p-6">
				<h1 class="text-xl font-semibold text-green-800">Email verified!</h1>
				<p class="mt-2 text-sm text-green-700">Your account is active. You can now sign in.</p>
				<button
					onclick={() => goto('/auth/login')}
					class="mt-4 font-medium text-green-700 underline-offset-2 hover:underline"
				>
					Go to sign in
				</button>
			</div>
		{:else}
			<div role="alert" class="rounded-md bg-red-50 p-6">
				<h1 class="text-xl font-semibold text-red-800">Verification failed</h1>
				<p class="mt-2 text-sm text-red-700">{errorMsg}</p>
				<a href="/auth/signup" class="mt-4 block font-medium text-red-700 hover:underline">
					Try signing up again
				</a>
			</div>
		{/if}
	</div>
</main>
