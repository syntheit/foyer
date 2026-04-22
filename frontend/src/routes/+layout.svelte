<script lang="ts">
	import '../app.css';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { checkAuth, getUser } from '$lib/stores/auth.svelte';
	import { connect, disconnect } from '$lib/stores/stats.svelte';
	import Navbar from '$lib/components/shared/Navbar.svelte';

	let { children } = $props();

	const user = $derived(getUser());
	const isPublicPage = $derived(
		page.url.pathname === '/login' || page.url.pathname === '/register'
	);

	let authReady = $state(false);

	onMount(() => {
		checkAuth().then((authed) => {
			authReady = true;
			if (!authed && !isPublicPage) {
				goto('/login');
			}
		});

		return () => {
			disconnect();
		};
	});

	// Connect/disconnect WS based on user state
	$effect(() => {
		if (user) {
			connect();
		} else {
			disconnect();
		}
	});
</script>

<svelte:head>
	<link rel="preconnect" href="https://fonts.googleapis.com" />
	<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin="anonymous" />
	<link
		href="https://fonts.googleapis.com/css2?family=DM+Sans:ital,opsz,wght@0,9..40,100..1000;1,9..40,100..1000&family=JetBrains+Mono:wght@400;500;600&display=swap"
		rel="stylesheet"
	/>
</svelte:head>

{#if !authReady}
	<div class="flex min-h-screen items-center justify-center">
		<div class="h-6 w-6 animate-spin rounded-full border-2 border-muted-foreground border-t-primary"></div>
	</div>
{:else if isPublicPage}
	{@render children()}
{:else if user}
	<div class="flex h-screen flex-col overflow-hidden md:flex-row">
		<Navbar />
		<main class="min-w-0 flex-1 overflow-y-auto p-4 md:p-6">
			{@render children()}
		</main>
	</div>
{:else}
	<!-- Redirecting to login -->
	<div class="flex min-h-screen items-center justify-center">
		<div class="h-6 w-6 animate-spin rounded-full border-2 border-muted-foreground border-t-primary"></div>
	</div>
{/if}
