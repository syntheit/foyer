<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import { isAdmin } from '$lib/stores/auth.svelte';
	import { timeAgo } from '$lib/utils/format';
	import { Pin } from '@lucide/svelte';

	type Message = {
		id: number;
		title: string;
		body: string;
		category: string;
		pinned: boolean;
		author: string;
		created_at: string;
	};

	let messages = $state<Message[]>([]);
	let error = $state('');
	const admin = $derived(isAdmin());

	onMount(async () => {
		try {
			messages = (await api.get<Message[]>('/api/messages')) || [];
		} catch {
			error = 'Failed to load messages';
		}
	});

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
	</div>

	{#if error}
		<p class="text-sm text-destructive">{error}</p>
	{/if}

	<div class="space-y-3">
		{#each messages as msg}
			<div class="rounded-lg border border-border bg-card p-4">
				<div class="flex items-start justify-between gap-2">
					<div class="flex items-center gap-2">
						{#if msg.pinned}
							<Pin class="h-3.5 w-3.5 text-primary" />
						{/if}
						<h3 class="text-sm font-medium text-foreground">{msg.title}</h3>
						<span
							class="rounded-full px-2 py-0.5 text-xs {categoryColors[msg.category] ||
								categoryColors.info}"
						>
							{msg.category}
						</span>
					</div>
					<span class="flex-shrink-0 text-xs text-muted-foreground"
						>{timeAgo(msg.created_at)}</span
					>
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
