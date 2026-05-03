<script lang="ts">
	import { onMount } from 'svelte';
	import { api, ApiError } from '$lib/api/client';
	import { timeAgo } from '$lib/utils/format';
	import { copyToClipboard } from '$lib/utils/clipboard';
	import { getUser } from '$lib/stores/auth.svelte';
	import ConfirmDialog from '$lib/components/shared/ConfirmDialog.svelte';
	import {
		Plus,
		Link as LinkIcon,
		Trash2,
		Flame,
		Copy,
		ExternalLink,
		Pencil,
		Save,
		X,
		ChevronDown
	} from '@lucide/svelte';

	type Paste = {
		id: string;
		language: string;
		burn_after_read: boolean;
		created_at: string;
		expires_at?: string;
		url: string;
	};

	type PasteContent = Paste & {
		content: string;
		created_by?: string;
	};

	let pastes = $state<Paste[]>([]);
	let error = $state('');

	// Modal state
	let viewing = $state<PasteContent | null>(null);
	let viewLoading = $state(false);
	let viewError = $state('');
	let copied = $state(false);
	let expanded = $state(false);

	let editing = $state(false);
	let editContent = $state('');
	let editLanguage = $state('');
	let saving = $state(false);
	let editError = $state('');

	let deleting = $state<Paste | null>(null);
	let deleteError = $state('');

	const user = $derived(getUser());
	const canEdit = $derived(
		!!viewing && !!user && (viewing.created_by === user.username || user.role === 'admin')
	);
	const lineCount = $derived(viewing?.content.split('\n').length ?? 0);
	const COLLAPSE_LINES = 30;
	const isLong = $derived(lineCount > COLLAPSE_LINES);
	const showAll = $derived(expanded || !isLong);
	const visibleContent = $derived(
		viewing && !showAll
			? viewing.content.split('\n').slice(0, COLLAPSE_LINES).join('\n')
			: viewing?.content ?? ''
	);

	onMount(async () => {
		try {
			pastes = (await api.get<Paste[]>('/api/pastes')) || [];
		} catch {
			error = 'Failed to load pastes';
		}
	});

	async function openPaste(p: Paste) {
		viewing = { ...p, content: '' };
		viewLoading = true;
		viewError = '';
		expanded = false;
		editing = false;
		try {
			// Hits the public view endpoint; for burn-after-read pastes this will burn them.
			const res = await fetch(`/p/${p.id}`);
			if (!res.ok) {
				viewError = await res.text();
				return;
			}
			const data = await res.json();
			viewing = { ...p, ...data };
		} catch {
			viewError = 'Failed to load paste';
		} finally {
			viewLoading = false;
		}
	}

	function closePaste() {
		viewing = null;
		editing = false;
	}

	function copyContent() {
		if (!viewing) return;
		copyToClipboard(viewing.content);
		copied = true;
		setTimeout(() => (copied = false), 2000);
	}

	function copyLink(url: string) {
		copyToClipboard(window.location.origin + url);
	}

	function startEdit() {
		if (!viewing) return;
		editContent = viewing.content;
		editLanguage = viewing.language;
		editError = '';
		editing = true;
	}

	function cancelEdit() {
		editing = false;
		editError = '';
	}

	async function saveEdit() {
		if (!viewing) return;
		saving = true;
		editError = '';
		try {
			await api.put(`/api/pastes/${viewing.id}`, {
				content: editContent,
				language: editLanguage
			});
			viewing = { ...viewing, content: editContent, language: editLanguage };
			pastes = pastes.map((p) =>
				p.id === viewing!.id ? { ...p, language: editLanguage } : p
			);
			editing = false;
		} catch (e) {
			editError = e instanceof ApiError ? e.message : 'Failed to save';
		} finally {
			saving = false;
		}
	}

	function askDelete(p: Paste, e: Event) {
		e.stopPropagation();
		deleting = p;
		deleteError = '';
	}

	async function confirmDelete() {
		if (!deleting) return;
		try {
			await api.del(`/api/pastes/${deleting.id}`);
			pastes = pastes.filter((p) => p.id !== deleting!.id);
			if (viewing?.id === deleting.id) viewing = null;
			deleting = null;
		} catch (e) {
			deleteError = e instanceof ApiError ? e.message : 'Delete failed';
		}
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
			class="inline-flex cursor-pointer items-center gap-1.5 rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground hover:bg-primary/90"
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
			<div
				role="button"
				tabindex="0"
				onclick={() => openPaste(paste)}
				onkeydown={(e) => {
					if (e.key === 'Enter' || e.key === ' ') {
						e.preventDefault();
						openPaste(paste);
					}
				}}
				class="flex w-full cursor-pointer items-center justify-between rounded-lg border border-border bg-card px-4 py-3 text-left transition-colors hover:border-primary/40 hover:bg-card/80 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
			>
				<div class="min-w-0">
					<div class="flex items-center gap-2">
						<p class="font-mono text-sm font-medium text-foreground">{paste.id}</p>
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
					<button
						type="button"
						onclick={(e) => {
							e.stopPropagation();
							copyLink(paste.url);
						}}
						class="cursor-pointer rounded-md p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground"
						title="Copy link"
					>
						<LinkIcon class="h-4 w-4" />
					</button>
					<button
						type="button"
						onclick={(e) => askDelete(paste, e)}
						class="cursor-pointer rounded-md p-1.5 text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
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

<!-- Paste view modal -->
{#if viewing}
	{@const v = viewing}
	<div class="fixed inset-0 z-50 flex items-center justify-center bg-background/80 p-4">
		<div class="flex max-h-[90vh] w-full max-w-4xl flex-col overflow-hidden rounded-lg border border-border bg-card shadow-2xl">
			<header class="flex items-center justify-between gap-3 border-b border-border px-5 py-3">
				<div class="flex min-w-0 items-center gap-2">
					<h2 class="font-mono text-sm text-foreground">{v.id}</h2>
					<span class="rounded bg-muted px-1.5 py-0.5 text-xs text-muted-foreground">{v.language}</span>
					{#if v.burn_after_read}
						<span class="rounded bg-destructive/10 px-1.5 py-0.5 text-xs text-destructive">burned</span>
					{/if}
				</div>
				<div class="flex items-center gap-1">
					{#if !editing}
						<button
							type="button"
							onclick={copyContent}
							disabled={!v.content}
							class="inline-flex cursor-pointer items-center gap-1 rounded-md px-2 py-1 text-xs text-muted-foreground hover:bg-accent hover:text-foreground disabled:cursor-not-allowed disabled:opacity-50"
						>
							<Copy class="h-3.5 w-3.5" />
							{copied ? 'Copied!' : 'Copy'}
						</button>
						<button
							type="button"
							onclick={() => copyLink(v.url)}
							class="inline-flex cursor-pointer items-center gap-1 rounded-md px-2 py-1 text-xs text-muted-foreground hover:bg-accent hover:text-foreground"
						>
							<ExternalLink class="h-3.5 w-3.5" />
							Link
						</button>
						<a
							href="/p/{v.id}/raw"
							target="_blank"
							class="inline-flex cursor-pointer items-center gap-1 rounded-md px-2 py-1 text-xs text-muted-foreground hover:bg-accent hover:text-foreground"
						>
							Raw
						</a>
						{#if canEdit}
							<button
								type="button"
								onclick={startEdit}
								class="inline-flex cursor-pointer items-center gap-1 rounded-md px-2 py-1 text-xs text-muted-foreground hover:bg-accent hover:text-foreground"
							>
								<Pencil class="h-3.5 w-3.5" />
								Edit
							</button>
							<button
								type="button"
								onclick={(e) => askDelete(v, e)}
								class="inline-flex cursor-pointer items-center gap-1 rounded-md px-2 py-1 text-xs text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
							>
								<Trash2 class="h-3.5 w-3.5" />
								Delete
							</button>
						{/if}
					{/if}
					<button
						type="button"
						onclick={closePaste}
						class="cursor-pointer rounded-md p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground"
						title="Close"
					>
						<X class="h-4 w-4" />
					</button>
				</div>
			</header>

			<div class="flex-1 overflow-y-auto p-5">
				{#if viewLoading}
					<div class="flex h-32 items-center justify-center">
						<div class="h-5 w-5 animate-spin rounded-full border-2 border-muted-foreground border-t-primary"></div>
					</div>
				{:else if viewError}
					<p class="text-sm text-destructive">{viewError}</p>
				{:else if editing}
					<div class="space-y-3">
						<div class="flex items-center gap-2">
							<label for="modal-edit-language" class="text-xs text-muted-foreground">Language</label>
							<input
								id="modal-edit-language"
								type="text"
								bind:value={editLanguage}
								class="rounded-md border border-input bg-background px-2 py-1 text-xs"
								placeholder="plaintext"
							/>
						</div>
						<textarea
							bind:value={editContent}
							class="min-h-[24rem] w-full resize-y rounded-lg border border-border bg-background p-4 font-mono text-sm text-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
							spellcheck="false"
						></textarea>
						{#if editError}
							<p class="text-xs text-destructive">{editError}</p>
						{/if}
						<div class="flex items-center justify-end gap-2">
							<button
								type="button"
								onclick={cancelEdit}
								disabled={saving}
								class="inline-flex cursor-pointer items-center gap-1 rounded-md px-3 py-1.5 text-sm text-muted-foreground hover:bg-accent disabled:cursor-not-allowed disabled:opacity-50"
							>
								<X class="h-4 w-4" />
								Cancel
							</button>
							<button
								type="button"
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
						<pre class="overflow-x-auto rounded-lg border border-border bg-background p-4 font-mono text-sm text-foreground"><code>{visibleContent}</code></pre>
						{#if isLong && !expanded}
							<div class="pointer-events-none absolute inset-x-0 bottom-0 h-24 rounded-b-lg bg-gradient-to-t from-background to-transparent"></div>
							<button
								type="button"
								onclick={() => (expanded = true)}
								class="absolute inset-x-0 bottom-2 mx-auto flex w-fit cursor-pointer items-center gap-1 rounded-full border border-border bg-card/90 px-3 py-1 text-xs text-muted-foreground shadow-sm backdrop-blur hover:bg-accent hover:text-foreground"
							>
								<ChevronDown class="h-3.5 w-3.5" />
								Show all {lineCount} lines
							</button>
						{/if}
					</div>
					{#if isLong && expanded}
						<div class="mt-2 text-center">
							<button
								type="button"
								onclick={() => (expanded = false)}
								class="cursor-pointer text-xs text-muted-foreground hover:text-foreground"
							>
								Collapse
							</button>
						</div>
					{/if}
				{/if}
			</div>
		</div>
	</div>
{/if}

<!-- Delete confirm modal -->
<ConfirmDialog
	open={!!deleting}
	title="Delete paste?"
	confirmLabel="Delete"
	error={deleteError}
	zIndex={60}
	onCancel={() => (deleting = null)}
	onConfirm={confirmDelete}
>
	{#snippet body()}
		{#if deleting}
			Paste <span class="font-mono text-foreground">{deleting.id}</span> will be permanently removed.
		{/if}
	{/snippet}
</ConfirmDialog>
