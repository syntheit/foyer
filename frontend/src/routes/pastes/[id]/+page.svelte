<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { copyToClipboard } from '$lib/utils/clipboard';
	import { Copy, ExternalLink } from '@lucide/svelte';

	type PasteData = {
		id: string;
		content: string;
		language: string;
		burn_after_read: boolean;
		created_at: string;
		expires_at?: string;
	};

	let paste = $state<PasteData | null>(null);
	let error = $state('');
	let copied = $state(false);

	onMount(async () => {
		const id = page.params.id;
		try {
			const res = await fetch(`/p/${id}`);
			if (!res.ok) {
				error = await res.text();
				return;
			}
			paste = await res.json();
		} catch {
			error = 'Failed to load paste';
		}
	});

	function copyContent() {
		if (!paste) return;
		copyToClipboard(paste.content);
		copied = true;
		setTimeout(() => (copied = false), 2000);
	}

	function copyLink() {
		copyToClipboard(window.location.origin + `/p/${page.params.id}`);
	}
</script>

<svelte:head>
	<title>Paste {page.params.id} — Foyer</title>
</svelte:head>

<div class="space-y-4">
	{#if error}
		<div class="rounded-lg border border-border bg-card p-8 text-center">
			<p class="text-sm text-destructive">{error}</p>
		</div>
	{:else if paste}
		<div class="flex items-center justify-between">
			<div class="flex items-center gap-2">
				<h1 class="font-mono text-sm text-foreground">{paste.id}</h1>
				<span class="rounded bg-muted px-1.5 py-0.5 text-xs text-muted-foreground">{paste.language}</span>
				{#if paste.burn_after_read}
					<span class="rounded bg-destructive/10 px-1.5 py-0.5 text-xs text-destructive">burned</span>
				{/if}
			</div>
			<div class="flex items-center gap-1">
				<button
					onclick={copyContent}
					class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs text-muted-foreground hover:bg-accent hover:text-foreground"
				>
					<Copy class="h-3.5 w-3.5" />
					{copied ? 'Copied!' : 'Copy'}
				</button>
				<button
					onclick={copyLink}
					class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs text-muted-foreground hover:bg-accent hover:text-foreground"
				>
					<ExternalLink class="h-3.5 w-3.5" />
					Link
				</button>
				<a
					href="/p/{page.params.id}/raw"
					target="_blank"
					class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs text-muted-foreground hover:bg-accent hover:text-foreground"
				>
					Raw
				</a>
			</div>
		</div>

		<pre class="overflow-x-auto rounded-lg border border-border bg-card p-4 font-mono text-sm text-foreground"><code>{paste.content}</code></pre>
	{:else}
		<div class="flex h-32 items-center justify-center">
			<div class="h-5 w-5 animate-spin rounded-full border-2 border-muted-foreground border-t-primary"></div>
		</div>
	{/if}
</div>
