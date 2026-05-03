<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { api, ApiError } from '$lib/api/client';
	import { checkAuth } from '$lib/stores/auth.svelte';

	let username = $state('');
	let password = $state('');
	let confirmPassword = $state('');
	let inviteCode = $state('');
	let inviteState = $state<'idle' | 'validating' | 'success' | 'error'>('idle');
	let inviteError = $state('');
	let error = $state('');
	let loading = $state(false);
	let signupsEnabled = $state(true);
	let inviteOnly = $state(false);
	let checking = $state(true);

	onMount(async () => {
		try {
			const res = await api.get<{ enabled: boolean; invite_only_enabled: boolean }>(
				'/api/auth/signups'
			);
			signupsEnabled = res.enabled;
			inviteOnly = res.invite_only_enabled;
		} catch {
			signupsEnabled = false;
		}
		checking = false;

		// Pre-fill from ?code= and validate
		const codeParam = page.url.searchParams.get('code');
		if (codeParam && inviteState === 'idle') {
			const upper = codeParam.toUpperCase();
			inviteCode = upper;
			if (upper.length === 8) await validateCode(upper);
		}
	});

	async function validateCode(code: string) {
		inviteState = 'validating';
		inviteError = '';
		try {
			const res = await api.get<{ valid: boolean }>(
				`/api/auth/invites/validate?code=${encodeURIComponent(code)}`
			);
			if (res.valid) {
				inviteState = 'success';
			} else {
				inviteState = 'error';
				inviteError = 'Invalid or expired invite code';
			}
		} catch {
			inviteState = 'error';
			inviteError = 'Failed to validate invite code';
		}
	}

	function handleInviteCodeInput(e: Event) {
		const upper = (e.target as HTMLInputElement).value.toUpperCase();
		inviteCode = upper;
		inviteError = '';
		if (upper.length === 8) {
			validateCode(upper);
		} else if (inviteState !== 'idle') {
			inviteState = 'idle';
		}
	}

	async function handleSubmit(e: SubmitEvent) {
		e.preventDefault();
		error = '';

		if (inviteOnly && inviteState !== 'success') {
			error = 'Please enter a valid invite code';
			return;
		}

		if (password !== confirmPassword) {
			error = 'Passwords do not match';
			return;
		}

		loading = true;
		try {
			await api.post('/api/auth/register', {
				username: username.trim(),
				password,
				invite_code: inviteOnly ? inviteCode : undefined
			});
			await checkAuth();
			goto('/');
		} catch (err) {
			if (err instanceof ApiError) {
				error = err.message.trim();
			} else {
				error = 'Connection error';
			}
		} finally {
			loading = false;
		}
	}
</script>

<svelte:head>
	<title>Register — Foyer</title>
</svelte:head>

<div class="flex min-h-screen items-center justify-center px-4">
	<div class="w-full max-w-sm">
		<div class="mb-8 text-center">
			<h1 class="text-2xl font-semibold tracking-tight text-foreground">Foyer</h1>
			<p class="mt-1 text-sm text-muted-foreground">Create an account</p>
		</div>

		{#if checking}
			<div class="flex justify-center">
				<div class="h-6 w-6 animate-spin rounded-full border-2 border-muted-foreground border-t-primary"></div>
			</div>
		{:else if !signupsEnabled}
			<div class="rounded-md bg-muted px-4 py-3 text-center text-sm text-muted-foreground">
				Signups are currently disabled.
			</div>
			<p class="mt-4 text-center text-sm text-muted-foreground">
				Already have an account? <a href="/login" class="text-primary hover:underline">Sign in</a>
			</p>
		{:else}
			<form onsubmit={handleSubmit} class="space-y-4">
				{#if error}
					<div class="rounded-md bg-destructive/10 px-3 py-2 text-sm text-destructive">
						{error}
					</div>
				{/if}

				{#if inviteOnly}
					<div class="space-y-2">
						<div class="flex items-center justify-between">
							<label for="invite" class="text-sm font-medium text-foreground">Invite code</label>
							{#if inviteState === 'success'}
								<span class="rounded-full bg-primary/10 px-2 py-0.5 text-xs text-primary">Accepted</span>
							{/if}
						</div>
						<input
							id="invite"
							type="text"
							value={inviteCode}
							oninput={handleInviteCodeInput}
							maxlength="8"
							autocomplete="off"
							spellcheck="false"
							class="flex h-9 w-full rounded-md border border-input bg-background px-3 py-1 font-mono text-sm uppercase tracking-widest text-foreground shadow-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
							placeholder="XXXXXXXX"
						/>
						{#if inviteState === 'validating'}
							<p class="text-xs text-muted-foreground">Validating…</p>
						{:else if inviteState === 'error' && inviteError}
							<p class="text-xs text-destructive">{inviteError}</p>
						{:else}
							<p class="text-xs text-muted-foreground">Enter the 8-character code you were given.</p>
						{/if}
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
						minlength="2"
						maxlength="32"
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
						autocomplete="new-password"
						required
						minlength="8"
						class="flex h-9 w-full rounded-md border border-input bg-background px-3 py-1 text-sm text-foreground shadow-sm transition-colors placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
						placeholder="••••••••"
					/>
				</div>

				<div class="space-y-2">
					<label for="confirm" class="text-sm font-medium text-foreground">Confirm Password</label>
					<input
						id="confirm"
						type="password"
						bind:value={confirmPassword}
						autocomplete="new-password"
						required
						class="flex h-9 w-full rounded-md border border-input bg-background px-3 py-1 text-sm text-foreground shadow-sm transition-colors placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
						placeholder="••••••••"
					/>
				</div>

				<button
					type="submit"
					disabled={loading || !username.trim() || !password || !confirmPassword || (inviteOnly && inviteState !== 'success')}
					class="inline-flex h-9 w-full cursor-pointer items-center justify-center rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground shadow transition-colors hover:bg-primary/90 disabled:pointer-events-none disabled:opacity-50"
				>
					{loading ? 'Creating account…' : 'Create account'}
				</button>
			</form>

			<p class="mt-4 text-center text-sm text-muted-foreground">
				Already have an account? <a href="/login" class="text-primary hover:underline">Sign in</a>
			</p>
		{/if}
	</div>
</div>
