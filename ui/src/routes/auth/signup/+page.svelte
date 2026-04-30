<script lang="ts">
	import { goto } from '$app/navigation';
	import { api } from '$lib/api-client.js';
	import Button from '$lib/components/ui/Button.svelte';
	import FormField from '$lib/components/ui/FormField.svelte';
	import Input from '$lib/components/ui/Input.svelte';

	let email = $state('');
	let password = $state('');
	let displayName = $state('');
	let error = $state('');
	let loading = $state(false);
	let done = $state(false);

	async function handleSubmit(e: SubmitEvent) {
		e.preventDefault();
		error = '';
		loading = true;
		try {
			await api.signup(email, password, displayName);
			done = true;
		} catch (err: unknown) {
			const apiErr = err as { message?: string };
			error = apiErr.message ?? 'Signup failed. Please try again.';
		} finally {
			loading = false;
		}
	}
</script>

<svelte:head>
	<title>Create account — Lantern</title>
</svelte:head>

<main class="flex min-h-screen items-center justify-center bg-gray-50 px-4">
	<div class="w-full max-w-sm">
		<h1 class="mb-6 text-center text-2xl font-bold text-gray-900">Create your account</h1>

		{#if done}
			<div
				role="status"
				aria-live="polite"
				class="rounded-md bg-green-50 p-4 text-sm text-green-800"
			>
				<p class="font-medium">Check your inbox</p>
				<p class="mt-1">
					We sent a verification link to {email}. Click it to activate your account.
				</p>
				<button
					onclick={() => goto('/auth/login')}
					class="mt-3 font-medium text-green-700 underline-offset-2 hover:underline"
				>
					Back to sign in
				</button>
			</div>
		{:else}
			<form onsubmit={handleSubmit} novalidate class="flex flex-col gap-4">
				{#if error}
					<div
						role="alert"
						aria-live="assertive"
						class="rounded-md bg-red-50 p-3 text-sm text-red-700"
					>
						{error}
					</div>
				{/if}

				<FormField id="name" label="Full name">
					<Input
						id="name"
						type="text"
						bind:value={displayName}
						autocomplete="name"
						required
						placeholder="Ada Lovelace"
					/>
				</FormField>

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

				<FormField id="password" label="Password">
					<Input
						id="password"
						type="password"
						bind:value={password}
						autocomplete="new-password"
						required
					/>
				</FormField>

				<Button type="submit" {loading} class="w-full">
					{loading ? 'Creating account…' : 'Create account'}
				</Button>
			</form>

			<p class="mt-4 text-center text-sm text-gray-600">
				Already have an account?
				<a href="/auth/login" class="font-medium text-blue-600 hover:underline">Sign in</a>
			</p>
		{/if}
	</div>
</main>
