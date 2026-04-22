<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import { timeAgo } from '$lib/utils/format';
	import { copyToClipboard } from '$lib/utils/clipboard';
	import { Plus, Link, Trash2, Flame } from '@lucide/svelte';

	type Paste = {
		id: string;
		language: string;
		burn_after_read: boolean;
		created_at: string;
		expires_at?: string;
		url: string;
	};

	let pastes = $state<Paste[]>([]);
	let error = $state('');

	onMount(async () => {
		try {
			pastes = (await api.get<Paste[]>('/api/pastes')) || [];
		} catch {
			error = 'Failed to load pastes';
		}
	});

	async function deletePaste(id: string) {
		try {
			await api.del(`/api/pastes/${id}`);
			pastes = pastes.filter((p) => p.id !== id);
		} catch {
			// ignore
		}
	}

	function copyLink(url: string) {
		copyToClipboard(window.location.origin + url);
	}
</script>

<svelte:head>
	<title>Pastes — Foyer</title>
</svelte:head>

<div class="space-y-6">
	<div class="flex items-center justify-between">
		<h1 class="text-lg font-semibold text-foreground">Pastes</h1>
		<a
			href="/pastes/new"
			class="inline-flex items-center gap-1.5 rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground hover:bg-primary/90"
		>
			<Plus class="h-4 w-4" />
			New Paste
		</a>
	</div>

	{#if error}
		<p class="text-sm text-destructive">{error}</p>
	{/if}

	<div class="space-y-2">
		{#each pastes as paste}
			<div class="flex items-center justify-between rounded-lg border border-border bg-card px-4 py-3">
				<div class="min-w-0">
					<div class="flex items-center gap-2">
						<p class="text-sm font-medium text-foreground font-mono">{paste.id}</p>
						<span class="rounded bg-muted px-1.5 py-0.5 text-xs text-muted-foreground">{paste.language}</span>
						{#if paste.burn_after_read}
							<Flame class="h-3.5 w-3.5 text-destructive" />
						{/if}
					</div>
					<p class="text-xs text-muted-foreground">
						Created {timeAgo(paste.created_at)}
						{#if paste.expires_at}
							 · expires {timeAgo(paste.expires_at)}
						{/if}
					</p>
				</div>
				<div class="flex items-center gap-1">
					<a
						href={paste.url}
						target="_blank"
						class="rounded-md p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground"
						title="Open"
					>
						<Link class="h-4 w-4" />
					</a>
					<button
						onclick={() => deletePaste(paste.id)}
						class="rounded-md p-1.5 text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
						title="Delete"
					>
						<Trash2 class="h-4 w-4" />
					</button>
				</div>
			</div>
		{:else}
			{#if !error}
				<p class="text-sm text-muted-foreground">No pastes yet</p>
			{/if}
		{/each}
	</div>
</div>
