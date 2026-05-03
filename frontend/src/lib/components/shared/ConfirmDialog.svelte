<script lang="ts">
	import type { Snippet } from 'svelte';

	type Variant = 'destructive' | 'primary';

	type Props = {
		open: boolean;
		title: string;
		body?: Snippet;
		confirmLabel?: string;
		variant?: Variant;
		error?: string;
		busy?: boolean;
		zIndex?: number;
		onConfirm: () => void;
		onCancel: () => void;
	};

	let {
		open,
		title,
		body,
		confirmLabel = 'Confirm',
		variant = 'destructive',
		error = '',
		busy = false,
		zIndex = 50,
		onConfirm,
		onCancel
	}: Props = $props();

	const confirmClass = $derived(
		variant === 'destructive'
			? 'bg-destructive hover:bg-destructive/90 text-destructive-foreground'
			: 'bg-primary hover:bg-primary/90 text-primary-foreground'
	);
</script>

{#if open}
	<div
		class="fixed inset-0 flex items-center justify-center bg-background/80 p-4"
		style="z-index: {zIndex}"
	>
		<div class="w-full max-w-sm rounded-lg border border-border bg-card p-4">
			<h2 class="text-sm font-medium text-foreground">{title}</h2>
			{#if body}
				<div class="mt-2 text-xs text-muted-foreground">
					{@render body()}
				</div>
			{/if}
			{#if error}
				<p class="mt-2 text-xs text-destructive">{error}</p>
			{/if}
			<div class="mt-3 flex justify-end gap-2">
				<button
					type="button"
					onclick={onCancel}
					disabled={busy}
					class="cursor-pointer rounded-md px-3 py-1.5 text-sm text-muted-foreground hover:bg-accent disabled:cursor-not-allowed disabled:opacity-50"
				>
					Cancel
				</button>
				<button
					type="button"
					onclick={onConfirm}
					disabled={busy}
					class="cursor-pointer rounded-md px-3 py-1.5 text-sm font-medium {confirmClass} disabled:cursor-not-allowed disabled:opacity-50"
				>
					{busy ? 'Working…' : confirmLabel}
				</button>
			</div>
		</div>
	</div>
{/if}
