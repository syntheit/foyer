<script lang="ts">
	import { goto } from '$app/navigation';
	import { login } from '$lib/stores/auth.svelte';
	import { ApiError } from '$lib/api/client';

	let username = $state('');
	let password = $state('');
	let error = $state('');
	let loading = $state(false);

	async function handleSubmit(e: SubmitEvent) {
		e.preventDefault();
		error = '';
		loading = true;

		try {
			await login(username, password);
			goto('/');
		} catch (err) {
			if (err instanceof ApiError) {
				error = 'Invalid username or password';
			} else {
				error = 'Connection error';
			}
		} finally {
			loading = false;
		}
	}
</script>

<svelte:head>
	<title>Login — Foyer</title>
</svelte:head>

<div class="flex min-h-screen items-center justify-center px-4">
	<div class="w-full max-w-sm">
		<div class="mb-8 text-center">
			<h1 class="text-2xl font-semibold tracking-tight text-foreground">Foyer</h1>
			<p class="mt-1 text-sm text-muted-foreground">Sign in to your server</p>
		</div>

		<form onsubmit={handleSubmit} class="space-y-4">
			{#if error}
				<div class="rounded-md bg-destructive/10 px-3 py-2 text-sm text-destructive">
					{error}
				</div>
			{/if}

			<div class="space-y-2">
				<label for="username" class="text-sm font-medium text-foreground">Username</label>
				<input
					id="username"
					type="text"
					bind:value={username}
					autocomplete="username"
					required
					class="flex h-9 w-full rounded-md border border-input bg-background px-3 py-1 text-sm text-foreground shadow-sm transition-colors placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
					placeholder="username"
				/>
			</div>

			<div class="space-y-2">
				<label for="password" class="text-sm font-medium text-foreground">Password</label>
				<input
					id="password"
					type="password"
					bind:value={password}
					autocomplete="current-password"
					required
					class="flex h-9 w-full rounded-md border border-input bg-background px-3 py-1 text-sm text-foreground shadow-sm transition-colors placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
					placeholder="••••••••"
				/>
			</div>

			<button
				type="submit"
				disabled={loading || !username || !password}
				class="inline-flex h-9 w-full items-center justify-center rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground shadow transition-colors hover:bg-primary/90 disabled:pointer-events-none disabled:opacity-50"
			>
				{loading ? 'Signing in…' : 'Sign in'}
			</button>
		</form>

		<p class="mt-4 text-center text-sm text-muted-foreground">
			Don't have an account? <a href="/register" class="text-primary hover:underline">Create one</a>
		</p>
	</div>
</div>
