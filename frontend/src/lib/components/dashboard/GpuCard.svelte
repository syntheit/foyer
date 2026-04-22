<script lang="ts">
	import type { Stats } from '$lib/stores/stats.svelte';
	import UsageBar from './UsageBar.svelte';
	import { formatTemp } from '$lib/utils/format';
	import { Monitor } from '@lucide/svelte';

	type Props = {
		gpu: NonNullable<Stats['gpu']>;
	};

	let { gpu }: Props = $props();
</script>

<div class="rounded-lg border border-border bg-card p-4">
	<div class="mb-3 flex items-center justify-between">
		<div class="flex items-center gap-2">
			<Monitor class="h-4 w-4 text-muted-foreground" />
			<h3 class="text-xs font-medium tracking-wide text-muted-foreground uppercase">GPU</h3>
		</div>
		<span class="text-xs text-muted-foreground">{gpu.name}</span>
	</div>
	<div class="space-y-3">
		<UsageBar label="Utilization" percent={gpu.utilization_percent} />
		<UsageBar
			label="VRAM"
			percent={gpu.memory_total_mb > 0 ? (gpu.memory_used_mb / gpu.memory_total_mb) * 100 : 0}
			detail="{gpu.memory_used_mb} / {gpu.memory_total_mb} MB"
		/>
		<div class="flex items-center justify-between text-xs text-muted-foreground">
			<span>Temp: {formatTemp(gpu.temperature ?? 0)}</span>
			<span>Power: {(gpu.power_watts ?? 0).toFixed(0)}W</span>
		</div>
	</div>
</div>
