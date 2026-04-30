<script lang="ts">
	import { page } from '$app/state';
	import { login } from '$lib/auth.js';
	import Button from '$lib/components/ui/Button.svelte';
	import FormField from '$lib/components/ui/FormField.svelte';
	import Input from '$lib/components/ui/Input.svelte';

	let email = $state('');
	let password = $state('');
	let error = $state('');
	let loading = $state(false);

	const next = $derived(page.url.searchParams.get('next') ?? undefined);

	async function handleSubmit(e: SubmitEvent) {
		e.preventDefault();
		error = '';
		loading = true;
		try {
			await login(email, password, next);
		} catch (err: unknown) {
			const apiErr = err as { status?: number; message?: string };
			if (apiErr.status === 429) {
				error = 'Too many attempts. Please wait a moment and try again.';
			} else {
				error = apiErr.message ?? 'Login failed. Check your email and password.';
			}
		} finally {
			loading = false;
		}
	}
</script>

<svelte:head>
	<title>Sign in — Lantern</title>
</svelte:head>

<main class="flex min-h-screen items-center justify-center bg-gray-50 px-4">
	<div class="w-full max-w-sm">
		<h1 class="mb-6 text-center text-2xl font-bold text-gray-900">Sign in to Lantern</h1>

		<form onsubmit={handleSubmit} novalidate class="flex flex-col gap-4">
			{#if error}
				<div role="alert" aria-live="assertive" class="rounded-md bg-red-50 p-3 text-sm text-red-700">
					{error}
				</div>
			{/if}

			<FormField id="email" label="Email address" error={undefined}>
				<Input
					id="email"
					type="email"
					bind:value={email}
					autocomplete="email"
					required
					placeholder="you@example.com"
					aria-describedby={error ? 'login-error' : undefined}
				/>
			</FormField>

			<FormField id="password" label="Password" error={undefined}>
				<Input
					id="password"
					type="password"
					bind:value={password}
					autocomplete="current-password"
					required
				/>
			</FormField>

			<Button type="submit" {loading} class="w-full">
				{loading ? 'Signing in…' : 'Sign in'}
			</Button>
		</form>

		<p class="mt-4 text-center text-sm text-gray-600">
			No account?
			<a href="/auth/signup" class="font-medium text-blue-600 hover:underline">Create one</a>
		</p>
		<p class="mt-2 text-center text-sm text-gray-600">
			<a href="/auth/password-reset/request" class="font-medium text-blue-600 hover:underline">
				Forgot password?
			</a>
		</p>
	</div>
</main>
