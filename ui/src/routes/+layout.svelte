<script lang="ts">
	import '../app.css';
	import { onMount } from 'svelte';
	import { initSession } from '$lib/auth.svelte.js';

	let { children } = $props();

	onMount(() => {
		initSession();
	});
</script>

{@render children()}
