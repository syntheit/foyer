<script lang="ts">
	import type { Stats } from '$lib/stores/stats.svelte';
	import { Box, Circle } from '@lucide/svelte';

	type Props = {
		docker: NonNullable<Stats['docker']>;
	};

	let { docker }: Props = $props();

	const running = $derived(docker.containers.filter((c) => c.state === 'running').length);
	const total = $derived(docker.containers.length);
</script>

<div class="rounded-lg border border-border bg-card p-4">
	<div class="mb-3 flex items-center justify-between">
		<div class="flex items-center gap-2">
			<Box class="h-4 w-4 text-muted-foreground" />
			<h3 class="text-xs font-medium tracking-wide text-muted-foreground uppercase">Containers</h3>
		</div>
		<span class="text-xs text-muted-foreground">{running}/{total} running</span>
	</div>
	<div class="max-h-48 space-y-1 overflow-y-auto">
		{#each docker.containers as container}
			<div class="flex items-center gap-2 py-0.5">
				<Circle
					class="h-2 w-2 flex-shrink-0 {container.state === 'running'
						? 'fill-success text-success'
						: 'fill-destructive text-destructive'}"
				/>
				<span class="truncate text-xs text-foreground">{container.name}</span>
				<span class="ml-auto flex-shrink-0 text-xs text-muted-foreground">{container.status}</span>
			</div>
		{/each}
	</div>
</div>
