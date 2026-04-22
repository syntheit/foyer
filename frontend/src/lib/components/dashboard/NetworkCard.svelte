<script lang="ts">
	import type { Stats } from '$lib/stores/stats.svelte';
	import { formatBytesPerSec } from '$lib/utils/format';
	import { ArrowDown, ArrowUp, Network } from '@lucide/svelte';

	type Props = {
		network: Stats['network'];
	};

	let { network }: Props = $props();
</script>

<div class="rounded-lg border border-border bg-card p-4">
	<div class="mb-3 flex items-center gap-2">
		<Network class="h-4 w-4 text-muted-foreground" />
		<h3 class="text-xs font-medium tracking-wide text-muted-foreground uppercase">Network</h3>
	</div>
	<div class="space-y-2">
		{#each network.interfaces as iface}
			<div class="flex items-center justify-between">
				<span class="text-sm font-medium text-foreground">{iface.name}</span>
				<div class="flex items-center gap-3 text-xs text-muted-foreground">
					<span class="flex items-center gap-1">
						<ArrowDown class="h-3 w-3 text-success" />
						{formatBytesPerSec(iface.rx_bytes_per_sec)}
					</span>
					<span class="flex items-center gap-1">
						<ArrowUp class="h-3 w-3 text-primary" />
						{formatBytesPerSec(iface.tx_bytes_per_sec)}
					</span>
				</div>
			</div>
		{/each}
		{#if network.interfaces.length === 0}
			<p class="text-xs text-muted-foreground">No interfaces</p>
		{/if}
	</div>
</div>
