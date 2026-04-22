<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import { Circle } from '@lucide/svelte';

	type Service = {
		id: number;
		name: string;
		url: string;
		is_healthy: boolean;
		uptime_7d?: number;
		uptime_30d?: number;
		uptime_365d?: number;
	};

	let services = $state<Service[]>([]);
	let error = $state('');

	onMount(async () => {
		try {
			services = (await api.get<Service[]>('/api/services')) || [];
		} catch {
			error = 'Failed to load services';
		}
	});

	function formatUptime(pct: number | undefined): string {
		if (pct === undefined || pct === null) return '—';
		return `${pct.toFixed(1)}%`;
	}

	function uptimeColor(pct: number | undefined): string {
		if (pct === undefined || pct === null) return 'text-muted-foreground';
		if (pct >= 99.5) return 'text-success';
		if (pct >= 95) return 'text-warning';
		return 'text-destructive';
	}

	function dotColor(pct: number | undefined): string {
		if (pct === undefined || pct === null) return 'bg-muted-foreground';
		if (pct >= 99.5) return 'bg-success';
		if (pct >= 95) return 'bg-warning';
		return 'bg-destructive';
	}
</script>

<svelte:head>
	<title>Services — Foyer</title>
</svelte:head>

<div class="space-y-6">
	<h1 class="text-lg font-semibold text-foreground">Services</h1>

	{#if error}
		<p class="text-sm text-destructive">{error}</p>
	{/if}

	<div class="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-3">
		{#each services as service}
			<div class="rounded-lg border border-border bg-card p-3">
				<div class="flex items-center gap-2">
					<Circle
						class="h-2 w-2 flex-shrink-0 {service.is_healthy
							? 'fill-success text-success'
							: 'fill-destructive text-destructive'}"
					/>
					<a
						href={service.url}
						target="_blank"
						rel="noopener"
						class="truncate text-sm font-medium text-foreground hover:text-primary"
					>
						{service.name}
					</a>
				</div>
				<!-- Uptime bars -->
				<div class="mt-2 flex items-center gap-3 text-xs text-muted-foreground">
					<div class="flex items-center gap-1">
						<div class="h-1.5 w-1.5 rounded-full {dotColor(service.uptime_7d)}"></div>
						<span class={uptimeColor(service.uptime_7d)}>7d {formatUptime(service.uptime_7d)}</span>
					</div>
					<div class="flex items-center gap-1">
						<div class="h-1.5 w-1.5 rounded-full {dotColor(service.uptime_30d)}"></div>
						<span class={uptimeColor(service.uptime_30d)}>30d {formatUptime(service.uptime_30d)}</span>
					</div>
					<div class="flex items-center gap-1">
						<div class="h-1.5 w-1.5 rounded-full {dotColor(service.uptime_365d)}"></div>
						<span class={uptimeColor(service.uptime_365d)}>1y {formatUptime(service.uptime_365d)}</span>
					</div>
				</div>
			</div>
		{:else}
			{#if !error}
				<p class="text-sm text-muted-foreground">No services monitored</p>
			{/if}
		{/each}
	</div>
</div>
