<script lang="ts">
	import type { Snippet } from 'svelte';

	type Props = {
		title: string;
		value: string;
		subtitle?: string;
		icon?: Snippet;
	};

	let { title, value, subtitle, icon }: Props = $props();
</script>

<div class="rounded-lg border border-border bg-card p-4">
	<div class="flex items-center justify-between">
		<p class="text-xs font-medium tracking-wide text-muted-foreground uppercase">{title}</p>
		{#if icon}
			<div class="text-muted-foreground">
				{@render icon()}
			</div>
		{/if}
	</div>
	<p class="mt-1 text-2xl font-semibold text-foreground">{value}</p>
	{#if subtitle}
		<p class="mt-0.5 text-xs text-muted-foreground">{subtitle}</p>
	{/if}
</div>
