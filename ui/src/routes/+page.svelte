<script lang="ts">
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { session } from '$lib/auth.js';

	onMount(() => {
		if (!session.loading) {
			goto(session.user ? '/dashboard' : '/auth/login', { replaceState: true });
		}
	});

	$effect(() => {
		if (!session.loading) {
			goto(session.user ? '/dashboard' : '/auth/login', { replaceState: true });
		}
	});
</script>
