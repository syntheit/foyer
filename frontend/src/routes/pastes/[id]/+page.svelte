<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { copyToClipboard } from '$lib/utils/clipboard';
	import { api, ApiError } from '$lib/api/client';
	import { getUser } from '$lib/stores/auth.svelte';
	import { Copy, ExternalLink, Pencil, Trash2, ChevronDown, X, Save } from '@lucide/svelte';

	type PasteData = {
		id: string;
		content: string;
		language: string;
		burn_after_read: boolean;
		created_at: string;
		expires_at?: string;
		created_by?: string;
	};

	let paste = $state<PasteData | null>(null);
	let error = $state('');
	let copied = $state(false);

	let editing = $state(false);
	let editContent = $state('');
	let editLanguage = $state('');
	let saving = $state(false);
	let editError = $state('');

	let expanded = $state(false);

	const user = $derived(getUser());
	const canEdit = $derived(
		!!paste && !!user && (paste.created_by === user.username || user.role === 'admin')
	);
	const lineCount = $derived(paste?.content.split('\n').length ?? 0);
	const COLLAPSE_LINES = 30;
	const isLong = $derived(lineCount > COLLAPSE_LINES);
	const showAll = $derived(expanded || !isLong);
	const visibleContent = $derived(
		paste && !showAll ? paste.content.split('\n').slice(0, COLLAPSE_LINES).join('\n') : paste?.content ?? ''
	);

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

	function startEdit() {
		if (!paste) return;
		editContent = paste.content;
		editLanguage = paste.language;
		editError = '';
		editing = true;
	}

	function cancelEdit() {
		editing = false;
		editError = '';
	}

	async function saveEdit() {
		if (!paste) return;
		saving = true;
		editError = '';
		try {
			await api.put(`/api/pastes/${paste.id}`, {
				content: editContent,
				language: editLanguage
			});
			paste = { ...paste, content: editContent, language: editLanguage };
			editing = false;
		} catch (e) {
			editError = e instanceof ApiError ? e.message : 'Failed to save';
		} finally {
			saving = false;
		}
	}

	async function deletePaste() {
		if (!paste) return;
		if (!confirm('Delete this paste?')) return;
		try {
			await api.del(`/api/pastes/${paste.id}`);
			goto('/pastes');
		} catch (e) {
			error = e instanceof ApiError ? e.message : 'Delete failed';
		}
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
					class="inline-flex cursor-pointer items-center gap-1 rounded-md px-2 py-1 text-xs text-muted-foreground hover:bg-accent hover:text-foreground"
				>
					<Copy class="h-3.5 w-3.5" />
					{copied ? 'Copied!' : 'Copy'}
				</button>
				<button
					onclick={copyLink}
					class="inline-flex cursor-pointer items-center gap-1 rounded-md px-2 py-1 text-xs text-muted-foreground hover:bg-accent hover:text-foreground"
				>
					<ExternalLink class="h-3.5 w-3.5" />
					Link
				</button>
				<a
					href="/p/{page.params.id}/raw"
					target="_blank"
					class="inline-flex cursor-pointer items-center gap-1 rounded-md px-2 py-1 text-xs text-muted-foreground hover:bg-accent hover:text-foreground"
				>
					Raw
				</a>
				{#if canEdit && !editing}
					<button
						onclick={startEdit}
						class="inline-flex cursor-pointer items-center gap-1 rounded-md px-2 py-1 text-xs text-muted-foreground hover:bg-accent hover:text-foreground"
					>
						<Pencil class="h-3.5 w-3.5" />
						Edit
					</button>
					<button
						onclick={deletePaste}
						class="inline-flex cursor-pointer items-center gap-1 rounded-md px-2 py-1 text-xs text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
					>
						<Trash2 class="h-3.5 w-3.5" />
						Delete
					</button>
				{/if}
			</div>
		</div>

		{#if editing}
			<div class="space-y-3">
				<div class="flex items-center gap-2">
					<label for="edit-language" class="text-xs text-muted-foreground">Language</label>
					<input
						id="edit-language"
						type="text"
						bind:value={editLanguage}
						class="rounded-md border border-input bg-background px-2 py-1 text-xs"
						placeholder="plaintext"
					/>
				</div>
				<textarea
					bind:value={editContent}
					class="min-h-[24rem] w-full resize-y rounded-lg border border-border bg-card p-4 font-mono text-sm text-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
					spellcheck="false"
				></textarea>
				{#if editError}
					<p class="text-xs text-destructive">{editError}</p>
				{/if}
				<div class="flex items-center justify-end gap-2">
					<button
						onclick={cancelEdit}
						disabled={saving}
						class="inline-flex cursor-pointer items-center gap-1 rounded-md px-3 py-1.5 text-sm text-muted-foreground hover:bg-accent disabled:cursor-not-allowed disabled:opacity-50"
					>
						<X class="h-4 w-4" />
						Cancel
					</button>
					<button
						onclick={saveEdit}
						disabled={saving || !editContent.trim()}
						class="inline-flex cursor-pointer items-center gap-1 rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:cursor-not-allowed disabled:opacity-50"
					>
						<Save class="h-4 w-4" />
						{saving ? 'Saving…' : 'Save'}
					</button>
				</div>
			</div>
		{:else}
			<div class="relative">
				<pre class="overflow-x-auto rounded-lg border border-border bg-card p-4 font-mono text-sm text-foreground"><code>{visibleContent}</code></pre>
				{#if isLong && !expanded}
					<div class="pointer-events-none absolute inset-x-0 bottom-0 h-24 rounded-b-lg bg-gradient-to-t from-card to-transparent"></div>
					<button
						onclick={() => (expanded = true)}
						class="absolute inset-x-0 bottom-2 mx-auto flex w-fit cursor-pointer items-center gap-1 rounded-full border border-border bg-card/90 px-3 py-1 text-xs text-muted-foreground shadow-sm backdrop-blur hover:bg-accent hover:text-foreground"
					>
						<ChevronDown class="h-3.5 w-3.5" />
						Show all {lineCount} lines
					</button>
				{/if}
			</div>
			{#if isLong && expanded}
				<div class="text-center">
					<button
						onclick={() => (expanded = false)}
						class="cursor-pointer text-xs text-muted-foreground hover:text-foreground"
					>
						Collapse
					</button>
				</div>
			{/if}
		{/if}
	{:else}
		<div class="flex h-32 items-center justify-center">
			<div class="h-5 w-5 animate-spin rounded-full border-2 border-muted-foreground border-t-primary"></div>
		</div>
	{/if}
</div>
