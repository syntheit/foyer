<script lang="ts">
	import { onMount } from 'svelte';
	import { api, ApiError } from '$lib/api/client';
	import { isAdmin } from '$lib/stores/auth.svelte';
	import { timeAgo } from '$lib/utils/format';
	import Toggle from '$lib/components/shared/Toggle.svelte';
	import ConfirmDialog from '$lib/components/shared/ConfirmDialog.svelte';
	import { Pin, PinOff, Plus, Pencil, Trash2, X, Save } from '@lucide/svelte';

	type Message = {
		id: number;
		title: string;
		body: string;
		category: string;
		pinned: boolean;
		author: string;
		created_at: string;
	};

	type Draft = {
		id: number | null;
		title: string;
		body: string;
		category: string;
		pinned: boolean;
	};

	const CATEGORIES = ['info', 'update', 'maintenance'];

	let messages = $state<Message[]>([]);
	let error = $state('');
	const admin = $derived(isAdmin());

	let editor = $state<Draft | null>(null);
	let saving = $state(false);
	let editorError = $state('');

	let deleting = $state<Message | null>(null);
	let deleteError = $state('');

	onMount(reload);

	async function reload() {
		try {
			messages = (await api.get<Message[]>('/api/messages')) || [];
		} catch (e) {
			error = e instanceof ApiError ? e.message : 'Failed to load messages';
		}
	}

	function openNew() {
		editor = { id: null, title: '', body: '', category: 'info', pinned: false };
		editorError = '';
	}

	function openEdit(m: Message) {
		editor = {
			id: m.id,
			title: m.title,
			body: m.body,
			category: m.category,
			pinned: m.pinned
		};
		editorError = '';
	}

	function closeEditor() {
		editor = null;
		editorError = '';
	}

	async function saveMessage() {
		if (!editor) return;
		if (!editor.title.trim() || !editor.body.trim()) {
			editorError = 'Title and body are required';
			return;
		}
		saving = true;
		editorError = '';
		try {
			const body = {
				title: editor.title,
				body: editor.body,
				category: editor.category,
				pinned: editor.pinned
			};
			if (editor.id === null) {
				await api.post('/api/messages', body);
			} else {
				await api.put(`/api/messages/${editor.id}`, body);
			}
			editor = null;
			await reload();
		} catch (e) {
			editorError = e instanceof ApiError ? e.message : 'Save failed';
		} finally {
			saving = false;
		}
	}

	async function togglePin(m: Message) {
		try {
			await api.put(`/api/messages/${m.id}`, {
				title: m.title,
				body: m.body,
				category: m.category,
				pinned: !m.pinned
			});
			await reload();
		} catch (e) {
			error = e instanceof ApiError ? e.message : 'Update failed';
		}
	}

	function askDelete(m: Message) {
		deleting = m;
		deleteError = '';
	}

	async function confirmDelete() {
		if (!deleting) return;
		try {
			await api.del(`/api/messages/${deleting.id}`);
			deleting = null;
			await reload();
		} catch (e) {
			deleteError = e instanceof ApiError ? e.message : 'Delete failed';
		}
	}

	const categoryColors: Record<string, string> = {
		maintenance: 'bg-warning/10 text-warning',
		update: 'bg-primary/10 text-primary',
		info: 'bg-muted text-muted-foreground'
	};
</script>

<svelte:head>
	<title>Messages — Foyer</title>
</svelte:head>

<div class="space-y-6">
	<div class="flex items-center justify-between">
		<h1 class="text-lg font-semibold text-foreground">Messages</h1>
		{#if admin}
			<button
				type="button"
				onclick={openNew}
				class="flex cursor-pointer items-center gap-1.5 rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground hover:bg-primary/90"
			>
				<Plus class="h-4 w-4" />
				New message
			</button>
		{/if}
	</div>

	{#if error}
		<p class="text-sm text-destructive">{error}</p>
	{/if}

	<div class="space-y-3">
		{#each messages as msg (msg.id)}
			<div class="rounded-lg border border-border bg-card p-4">
				<div class="flex items-start justify-between gap-2">
					<div class="flex min-w-0 items-center gap-2">
						{#if msg.pinned}
							<Pin class="h-3.5 w-3.5 flex-shrink-0 text-primary" />
						{/if}
						<h3 class="truncate text-sm font-medium text-foreground">{msg.title}</h3>
						<span
							class="rounded-full px-2 py-0.5 text-xs {categoryColors[msg.category] ||
								categoryColors.info}"
						>
							{msg.category}
						</span>
					</div>
					<div class="flex flex-shrink-0 items-center gap-2">
						<span class="text-xs text-muted-foreground">{timeAgo(msg.created_at)}</span>
						{#if admin}
							<div class="flex items-center gap-1">
								<button
									type="button"
									title={msg.pinned ? 'Unpin' : 'Pin'}
									onclick={() => togglePin(msg)}
									class="cursor-pointer rounded p-1 text-muted-foreground hover:bg-accent hover:text-foreground"
								>
									{#if msg.pinned}
										<PinOff class="h-3.5 w-3.5" />
									{:else}
										<Pin class="h-3.5 w-3.5" />
									{/if}
								</button>
								<button
									type="button"
									title="Edit"
									onclick={() => openEdit(msg)}
									class="cursor-pointer rounded p-1 text-muted-foreground hover:bg-accent hover:text-foreground"
								>
									<Pencil class="h-3.5 w-3.5" />
								</button>
								<button
									type="button"
									title="Delete"
									onclick={() => askDelete(msg)}
									class="cursor-pointer rounded p-1 text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
								>
									<Trash2 class="h-3.5 w-3.5" />
								</button>
							</div>
						{/if}
					</div>
				</div>
				<p class="mt-2 whitespace-pre-wrap text-sm text-muted-foreground">{msg.body}</p>
				<p class="mt-2 text-xs text-muted-foreground">— {msg.author}</p>
			</div>
		{:else}
			{#if !error}
				<p class="text-sm text-muted-foreground">No messages</p>
			{/if}
		{/each}
	</div>
</div>

<!-- Create/edit modal -->
{#if editor}
	{@const e = editor}
	<div class="fixed inset-0 z-50 flex items-center justify-center bg-background/80 p-4">
		<div class="flex max-h-[90vh] w-full max-w-xl flex-col overflow-hidden rounded-lg border border-border bg-card shadow-2xl">
			<header class="flex items-center justify-between gap-3 border-b border-border px-5 py-3">
				<h2 class="text-sm font-medium text-foreground">
					{e.id === null ? 'New message' : 'Edit message'}
				</h2>
				<button
					type="button"
					onclick={closeEditor}
					class="cursor-pointer rounded-md p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground"
					title="Close"
				>
					<X class="h-4 w-4" />
				</button>
			</header>

			<div class="space-y-4 overflow-y-auto p-5">
				<div class="space-y-1.5">
					<label for="msg-title" class="text-xs text-muted-foreground">Title</label>
					<input
						id="msg-title"
						type="text"
						bind:value={editor.title}
						maxlength="500"
						class="w-full rounded-md border border-input bg-background px-3 py-1.5 text-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
						placeholder="What's happening?"
					/>
				</div>

				<div class="space-y-1.5">
					<label for="msg-body" class="text-xs text-muted-foreground">Body</label>
					<textarea
						id="msg-body"
						bind:value={editor.body}
						rows="8"
						class="w-full resize-y rounded-md border border-input bg-background px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
						placeholder="Details. Newlines are preserved."
					></textarea>
				</div>

				<div class="grid grid-cols-2 gap-3">
					<div class="space-y-1.5">
						<label for="msg-category" class="text-xs text-muted-foreground">Category</label>
						<select
							id="msg-category"
							bind:value={editor.category}
							class="w-full rounded-md border border-input bg-background px-3 py-1.5 text-sm"
						>
							{#each CATEGORIES as c}
								<option value={c}>{c}</option>
							{/each}
						</select>
					</div>
					<div class="space-y-1.5">
						<span class="block text-xs text-muted-foreground">Pinned</span>
						<Toggle
							checked={editor.pinned}
							onchange={(next) => (editor!.pinned = next)}
							size="md"
							label="Pinned"
						/>
					</div>
				</div>

				{#if editorError}
					<p class="text-xs text-destructive">{editorError}</p>
				{/if}
			</div>

			<footer class="flex items-center justify-end gap-2 border-t border-border px-5 py-3">
				<button
					type="button"
					onclick={closeEditor}
					disabled={saving}
					class="cursor-pointer rounded-md px-3 py-1.5 text-sm text-muted-foreground hover:bg-accent disabled:cursor-not-allowed disabled:opacity-50"
				>
					Cancel
				</button>
				<button
					type="button"
					onclick={saveMessage}
					disabled={saving || !editor.title.trim() || !editor.body.trim()}
					class="inline-flex cursor-pointer items-center gap-1 rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:cursor-not-allowed disabled:opacity-50"
				>
					<Save class="h-4 w-4" />
					{saving ? 'Saving…' : e.id === null ? 'Post' : 'Save'}
				</button>
			</footer>
		</div>
	</div>
{/if}

<!-- Delete confirm modal -->
<ConfirmDialog
	open={!!deleting}
	title="Delete message?"
	confirmLabel="Delete"
	error={deleteError}
	zIndex={60}
	onCancel={() => (deleting = null)}
	onConfirm={confirmDelete}
>
	{#snippet body()}
		{#if deleting}
			"<span class="text-foreground">{deleting.title}</span>" will be permanently removed.
		{/if}
	{/snippet}
</ConfirmDialog>
