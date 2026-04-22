<script lang="ts">
	import { goto } from '$app/navigation';
	import { api } from '$lib/api/client';

	let content = $state('');
	let language = $state('plaintext');
	let expiresIn = $state('7d');
	let burnAfterRead = $state(false);
	let error = $state('');
	let submitting = $state(false);

	const languages = [
		'plaintext', 'javascript', 'typescript', 'python', 'rust', 'go', 'bash', 'json',
		'yaml', 'toml', 'html', 'css', 'sql', 'nix', 'dockerfile', 'markdown', 'c', 'cpp', 'java'
	];

	async function handleSubmit(e: SubmitEvent) {
		e.preventDefault();
		if (!content.trim()) return;
		submitting = true;
		error = '';

		try {
			const result = await api.post<{ id: string; url: string }>('/api/pastes', {
				content,
				language,
				expires_in: expiresIn,
				burn_after_read: burnAfterRead
			});
			goto(`/pastes/${result.id}`);
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to create paste';
		} finally {
			submitting = false;
		}
	}
</script>

<svelte:head>
	<title>New Paste — Foyer</title>
</svelte:head>

<div class="space-y-6">
	<h1 class="text-lg font-semibold text-foreground">New Paste</h1>

	{#if error}
		<p class="text-sm text-destructive">{error}</p>
	{/if}

	<form onsubmit={handleSubmit} class="space-y-4">
		<textarea
			bind:value={content}
			placeholder="Paste your content here…"
			rows="16"
			class="w-full resize-y rounded-md border border-input bg-background p-3 font-mono text-sm text-foreground placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
		></textarea>

		<div class="flex flex-wrap items-center gap-4 text-sm">
			<label class="flex items-center gap-2 text-muted-foreground">
				Language
				<select bind:value={language} class="rounded border border-input bg-background px-2 py-1 text-foreground text-sm">
					{#each languages as lang}
						<option value={lang}>{lang}</option>
					{/each}
				</select>
			</label>

			<label class="flex items-center gap-2 text-muted-foreground">
				Expires
				<select bind:value={expiresIn} class="rounded border border-input bg-background px-2 py-1 text-foreground text-sm">
					<option value="1h">1 hour</option>
					<option value="1d">1 day</option>
					<option value="7d">7 days</option>
					<option value="30d">30 days</option>
					<option value="">Never</option>
				</select>
			</label>

			<label class="flex items-center gap-2 text-muted-foreground">
				<input type="checkbox" bind:checked={burnAfterRead} class="rounded border-input" />
				Burn after read
			</label>
		</div>

		<button
			type="submit"
			disabled={submitting || !content.trim()}
			class="inline-flex items-center rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
		>
			{submitting ? 'Creating…' : 'Create Paste'}
		</button>
	</form>
</div>
