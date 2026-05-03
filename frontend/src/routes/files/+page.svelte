<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import { formatBytes, timeAgo } from '$lib/utils/format';
	import { copyToClipboard } from '$lib/utils/clipboard';
	import { Upload, Copy, Trash2, Link } from '@lucide/svelte';

	type FileEntry = {
		id: string;
		filename: string;
		size_bytes: number;
		mime_type: string;
		download_count: number;
		max_downloads?: number;
		expires_at: string;
		created_at: string;
		url: string;
	};

	let files = $state<FileEntry[]>([]);
	let error = $state('');
	let uploading = $state(false);
	let uploadProgress = $state(0); // 0-100
	let uploadError = $state('');
	let dragOver = $state(false);

	// Upload form state
	let expiryHours = $state(24);
	let maxDownloads = $state('');
	let password = $state('');

	onMount(() => loadFiles());

	async function loadFiles() {
		try {
			files = (await api.get<FileEntry[]>('/api/files')) || [];
		} catch {
			error = 'Failed to load files';
		}
	}

	function upload(file: File) {
		uploading = true;
		uploadProgress = 0;
		uploadError = '';

		const form = new FormData();
		form.append('file', file);
		form.append('expiry_hours', String(expiryHours));
		if (maxDownloads) form.append('max_downloads', maxDownloads);
		if (password) form.append('password', password);

		const xhr = new XMLHttpRequest();
		xhr.open('POST', '/api/files');
		xhr.withCredentials = true;

		xhr.upload.onprogress = (e) => {
			if (e.lengthComputable) {
				uploadProgress = Math.round((e.loaded / e.total) * 100);
			}
		};

		xhr.onload = () => {
			uploading = false;
			if (xhr.status >= 200 && xhr.status < 300) {
				password = '';
				maxDownloads = '';
				uploadProgress = 100;
				loadFiles();
			} else {
				uploadError = xhr.responseText || 'Upload failed';
			}
		};

		xhr.onerror = () => {
			uploading = false;
			uploadError = 'Upload failed — connection error';
		};

		xhr.send(form);
	}

	function handleDrop(e: DragEvent) {
		e.preventDefault();
		dragOver = false;
		const file = e.dataTransfer?.files[0];
		if (file) upload(file);
	}

	function handleFileInput(e: Event) {
		const input = e.target as HTMLInputElement;
		const file = input.files?.[0];
		if (file) upload(file);
		input.value = '';
	}

	async function deleteFile(id: string) {
		try {
			await api.del(`/api/files/${id}`);
			files = files.filter((f) => f.id !== id);
		} catch {
			// ignore
		}
	}

	function copyLink(url: string) {
		copyToClipboard(window.location.origin + url);
	}
</script>

<svelte:head>
	<title>Files — Foyer</title>
</svelte:head>

<div class="space-y-6">
	<h1 class="text-lg font-semibold text-foreground">Files</h1>

	{#if error}
		<p class="text-sm text-destructive">{error}</p>
	{/if}

	<!-- Upload zone -->
	<div
		class="rounded-lg border-2 border-dashed transition-colors
			{dragOver ? 'border-primary bg-primary/5' : 'border-border'}"
		role="button"
		tabindex="0"
		ondragover={(e) => { e.preventDefault(); dragOver = true; }}
		ondragleave={() => (dragOver = false)}
		ondrop={handleDrop}
	>
		<label class="flex cursor-pointer flex-col items-center gap-2 p-8">
			<Upload class="h-8 w-8 text-muted-foreground" />
			<span class="text-sm text-muted-foreground">Drop a file or click to upload</span>
			<input type="file" class="hidden" onchange={handleFileInput} />
		</label>
	</div>

	<!-- Upload options -->
	<div class="flex flex-wrap gap-4 text-sm">
		<label class="flex items-center gap-2 text-muted-foreground">
			Expires in
			<select bind:value={expiryHours} class="rounded border border-input bg-background px-2 py-1 text-foreground text-sm">
				<option value={1}>1 hour</option>
				<option value={24}>1 day</option>
				<option value={72}>3 days</option>
				<option value={168}>7 days</option>
			</select>
		</label>
		<label class="flex items-center gap-2 text-muted-foreground">
			Max downloads
			<input
				type="number"
				min="1"
				bind:value={maxDownloads}
				placeholder="unlimited"
				class="w-24 rounded border border-input bg-background px-2 py-1 text-foreground text-sm"
			/>
		</label>
		<label class="flex items-center gap-2 text-muted-foreground">
			Password
			<input
				type="password"
				bind:value={password}
				placeholder="optional"
				class="w-32 rounded border border-input bg-background px-2 py-1 text-foreground text-sm"
			/>
		</label>
	</div>

	{#if uploading}
		<div class="space-y-1">
			<div class="flex items-center justify-between text-xs text-muted-foreground">
				<span>Uploading…</span>
				<span>{uploadProgress}%</span>
			</div>
			<div class="h-2 w-full overflow-hidden rounded-full bg-muted">
				<div
					class="h-full rounded-full bg-primary transition-all duration-300"
					style="width: {uploadProgress}%"
				></div>
			</div>
		</div>
	{/if}
	{#if uploadError}
		<p class="text-sm text-destructive">{uploadError}</p>
	{/if}

	<!-- File list -->
	<div class="space-y-2">
		{#each files as file}
			<div class="flex items-center justify-between rounded-lg border border-border bg-card px-4 py-3">
				<div class="min-w-0">
					<p class="truncate text-sm font-medium text-foreground">{file.filename}</p>
					<p class="text-xs text-muted-foreground">
						{formatBytes(file.size_bytes)} · {file.download_count} downloads · expires {timeAgo(file.expires_at)}
					</p>
				</div>
				<div class="flex items-center gap-1">
					<button
						onclick={() => copyLink(file.url)}
						class="cursor-pointer rounded-md p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground"
						title="Copy link"
					>
						<Link class="h-4 w-4" />
					</button>
					<button
						onclick={() => deleteFile(file.id)}
						class="cursor-pointer rounded-md p-1.5 text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
						title="Delete"
					>
						<Trash2 class="h-4 w-4" />
					</button>
				</div>
			</div>
		{:else}
			{#if !error}
				<p class="text-sm text-muted-foreground">No files uploaded</p>
			{/if}
		{/each}
	</div>
</div>
