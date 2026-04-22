<script lang="ts">
	import { onMount } from 'svelte';
	import { getStats } from '$lib/stores/stats.svelte';
	import { formatPercent, formatBytes, formatUptime, formatTemp, formatLoad, formatBytesPerSec } from '$lib/utils/format';
	import StatsCard from '$lib/components/dashboard/StatsCard.svelte';
	import DiskPools from '$lib/components/dashboard/DiskPools.svelte';
	import GpuCard from '$lib/components/dashboard/GpuCard.svelte';
	import { Cpu, MemoryStick, Clock, Thermometer, Box, ArrowDownUp, Tv } from '@lucide/svelte';
	import { api } from '$lib/api/client';

	const stats = $derived(getStats());

	// Pick the main network interface (highest traffic, skip lo/veth/docker/br-)
	const mainIface = $derived.by(() => {
		if (!stats?.network?.interfaces?.length) return null;
		return stats.network.interfaces.reduce((best, iface) => {
			const total = iface.rx_bytes_per_sec + iface.tx_bytes_per_sec;
			const bestTotal = best ? best.rx_bytes_per_sec + best.tx_bytes_per_sec : -1;
			return total > bestTotal ? iface : best;
		}, stats.network.interfaces[0]);
	});

	const runningContainers = $derived(
		stats?.docker?.containers?.filter((c) => c.state === 'running').length ?? 0
	);
	const totalContainers = $derived(stats?.docker?.containers?.length ?? 0);

	// Jellyfin streams
	let jellyfinStreams = $state<number | null>(null);
	onMount(() => {
		async function fetchStreams() {
			try {
				const res = await api.get<{ active_streams: number }>('/api/jellyfin/streams');
				jellyfinStreams = res.active_streams;
			} catch {
				// Jellyfin not configured or unreachable — hide the card
			}
		}
		fetchStreams();
		const interval = setInterval(fetchStreams, 30000);
		return () => clearInterval(interval);
	});
</script>

<svelte:head>
	<title>Dashboard — Foyer</title>
</svelte:head>

{#if !stats}
	<div class="flex h-64 items-center justify-center">
		<div class="text-center">
			<div class="mx-auto h-6 w-6 animate-spin rounded-full border-2 border-muted-foreground border-t-primary"></div>
			<p class="mt-3 text-sm text-muted-foreground">Connecting…</p>
		</div>
	</div>
{:else}
	<div class="space-y-6">
		<h1 class="text-lg font-semibold text-foreground">{stats.system.hostname}</h1>

		<!-- Stats cards -->
		<div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
			<StatsCard
				title="CPU"
				value={formatPercent(stats.cpu.usage_percent)}
				subtitle="{stats.cpu.cores} cores · Load {formatLoad(stats.cpu.load)}"
			>
				{#snippet icon()}<Cpu class="h-4 w-4" />{/snippet}
			</StatsCard>

			<StatsCard
				title="Memory"
				value={formatPercent(stats.memory.usage_percent)}
				subtitle="{formatBytes(stats.memory.used_bytes)} / {formatBytes(stats.memory.total_bytes)}"
			>
				{#snippet icon()}<MemoryStick class="h-4 w-4" />{/snippet}
			</StatsCard>

			<StatsCard
				title="Uptime"
				value={formatUptime(stats.system.uptime_seconds)}
			>
				{#snippet icon()}<Clock class="h-4 w-4" />{/snippet}
			</StatsCard>

			<StatsCard
				title="Temperature"
				value={stats.temperatures.cpu > 0 ? formatTemp(stats.temperatures.cpu) : '—'}
				subtitle={stats.temperatures.gpu > 0 ? `GPU: ${formatTemp(stats.temperatures.gpu)}` : undefined}
			>
				{#snippet icon()}<Thermometer class="h-4 w-4" />{/snippet}
			</StatsCard>
		</div>

		<!-- Second row: Network, Containers, Jellyfin -->
		<div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
			<StatsCard
				title="Network"
				value={mainIface ? formatBytesPerSec(mainIface.rx_bytes_per_sec + mainIface.tx_bytes_per_sec) : '—'}
				subtitle={mainIface ? `↓ ${formatBytesPerSec(mainIface.rx_bytes_per_sec)} · ↑ ${formatBytesPerSec(mainIface.tx_bytes_per_sec)}` : undefined}
			>
				{#snippet icon()}<ArrowDownUp class="h-4 w-4" />{/snippet}
			</StatsCard>

			<StatsCard
				title="Containers"
				value="{runningContainers}/{totalContainers}"
				subtitle="running"
			>
				{#snippet icon()}<Box class="h-4 w-4" />{/snippet}
			</StatsCard>

			{#if jellyfinStreams !== null}
				<StatsCard
					title="Jellyfin"
					value="{jellyfinStreams}"
					subtitle={jellyfinStreams === 1 ? 'active stream' : 'active streams'}
				>
					{#snippet icon()}<Tv class="h-4 w-4" />{/snippet}
				</StatsCard>
			{/if}
		</div>

		<!-- Storage & GPU -->
		<div class="grid gap-4 lg:grid-cols-2 xl:grid-cols-3">
			{#if stats.disk?.pools?.length > 0 || stats.disk?.mounts?.length > 0}
				<DiskPools disk={stats.disk} />
			{/if}

			{#if stats.gpu}
				<GpuCard gpu={stats.gpu} />
			{/if}
		</div>
	</div>
{/if}
