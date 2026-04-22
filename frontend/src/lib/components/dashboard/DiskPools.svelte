<script lang="ts">
	import type { Stats } from '$lib/stores/stats.svelte';
	import UsageBar from './UsageBar.svelte';
	import { formatBytes } from '$lib/utils/format';
	import { HardDrive } from '@lucide/svelte';

	type Props = {
		disk: Stats['disk'];
	};

	let { disk }: Props = $props();
</script>

<div class="rounded-lg border border-border bg-card p-4">
	<div class="mb-3 flex items-center gap-2">
		<HardDrive class="h-4 w-4 text-muted-foreground" />
		<h3 class="text-xs font-medium tracking-wide text-muted-foreground uppercase">Storage</h3>
	</div>
	<div class="space-y-3">
		{#each disk.pools as pool}
			<UsageBar
				label={pool.name}
				percent={pool.usage_percent}
				detail="{formatBytes(pool.used_bytes)} / {formatBytes(pool.total_bytes)}"
			/>
		{/each}
		{#each disk.mounts as mount}
			<UsageBar
				label={mount.mountpoint}
				percent={mount.usage_percent}
				detail="{formatBytes(mount.used_bytes)} / {formatBytes(mount.total_bytes)}"
			/>
		{/each}
	</div>
</div>
