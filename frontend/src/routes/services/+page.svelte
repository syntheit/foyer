<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import { ExternalLink, X } from '@lucide/svelte';

	type Service = {
		id: number;
		name: string;
		url: string;
		is_healthy: boolean;
		uptime_7d?: number;
		uptime_30d?: number;
		uptime_90d?: number;
		uptime_365d?: number;
	};

	type Bucket = {
		hour: string;
		total: number;
		healthy: number;
		uptime: number;
		status: 'up' | 'degraded' | 'down' | 'unknown';
	};

	type DailyHistory = {
		date: string;
		total_checks: number;
		healthy_checks: number;
		avg_response_time_ms: number;
		uptime_percentage: number;
	};

	let services = $state<Service[]>([]);
	let recentByService = $state<Record<number, Bucket[]>>({});
	let error = $state('');

	// Display order. Names not in this list fall to the end, sorted alphabetically.
	const SERVICE_PRIORITY = [
		'Retrospend',
		'Jellyfin',
		'Immich',
		'Seafile',
		'Vaultwarden',
		'Memos',
		'Seerr',
		'Website'
	];
	function priorityIndex(name: string): number {
		const i = SERVICE_PRIORITY.indexOf(name);
		return i === -1 ? Number.MAX_SAFE_INTEGER : i;
	}
	const sortedServices = $derived(
		[...services].sort((a, b) => {
			const pa = priorityIndex(a.name);
			const pb = priorityIndex(b.name);
			if (pa !== pb) return pa - pb;
			return a.name.localeCompare(b.name);
		})
	);

	const overallUptime90d = $derived.by(() => {
		const vals = services
			.map((s) => s.uptime_90d)
			.filter((v): v is number => typeof v === 'number');
		if (vals.length === 0) return null;
		return vals.reduce((a, b) => a + b, 0) / vals.length;
	});

	// Side panel state
	let openService = $state<Service | null>(null);
	let history = $state<DailyHistory[]>([]);
	let historyLoading = $state(false);

	onMount(async () => {
		try {
			services = (await api.get<Service[]>('/api/services')) || [];
			// Fetch recent buckets for all services in parallel.
			await Promise.all(
				services.map(async (s) => {
					try {
						const buckets = (await api.get<Bucket[]>(`/api/services/${s.id}/recent`)) || [];
						recentByService[s.id] = buckets;
					} catch {
						recentByService[s.id] = [];
					}
				})
			);
		} catch {
			error = 'Failed to load services';
		}
	});

	function formatUptime(pct: number | undefined | null): string {
		if (pct === undefined || pct === null) return '—';
		return `${pct.toFixed(2)}%`;
	}

	function uptimeColor(pct: number | undefined | null): string {
		if (pct === undefined || pct === null) return 'text-muted-foreground';
		if (pct >= 99.5) return 'text-success';
		if (pct >= 95) return 'text-warning';
		return 'text-destructive';
	}

	function statusBarClass(status: Bucket['status']): string {
		switch (status) {
			case 'up':
				return 'bg-success';
			case 'degraded':
				return 'bg-warning';
			case 'down':
				return 'bg-destructive';
			default:
				return 'bg-muted';
		}
	}

	function bucketTitle(b: Bucket): string {
		const t = new Date(b.hour);
		const local = t.toLocaleString(undefined, {
			month: 'short',
			day: 'numeric',
			hour: 'numeric'
		});
		if (b.status === 'unknown') return `${local} — no data`;
		return `${local} — ${b.uptime.toFixed(1)}% (${b.healthy}/${b.total})`;
	}

	async function openPanel(svc: Service) {
		openService = svc;
		historyLoading = true;
		history = [];
		try {
			history = (await api.get<DailyHistory[]>(`/api/services/${svc.id}/history?days=365`)) || [];
		} finally {
			historyLoading = false;
		}
	}

	function closePanel() {
		openService = null;
	}

	function dayBarClass(uptime: number): string {
		if (uptime >= 99.5) return 'bg-success';
		if (uptime >= 95) return 'bg-warning';
		if (uptime > 0) return 'bg-destructive';
		return 'bg-muted';
	}
</script>

<svelte:head>
	<title>Services — Foyer</title>
</svelte:head>

<div class="space-y-6">
	<div class="flex items-end justify-between gap-4">
		<div>
			<h1 class="text-lg font-semibold text-foreground">Services</h1>
			<p class="mt-0.5 text-xs text-muted-foreground">Uptime over the past 90 days</p>
		</div>
		{#if overallUptime90d !== null}
			<p class="text-3xl font-semibold tabular-nums {uptimeColor(overallUptime90d)}">
				{overallUptime90d.toFixed(2)}%
			</p>
		{/if}
	</div>

	{#if error}
		<p class="text-sm text-destructive">{error}</p>
	{/if}

	<div class="grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
		{#each sortedServices as service (service.id)}
			{@const buckets = recentByService[service.id] ?? []}
			<button
				type="button"
				onclick={() => openPanel(service)}
				class="group block w-full cursor-pointer rounded-md border border-border bg-card p-2.5 text-left transition-colors hover:border-primary/40 hover:bg-card/80"
			>
				<div class="flex items-center justify-between gap-2">
					<div class="flex min-w-0 items-center gap-1.5">
						<div
							class="h-2 w-2 flex-shrink-0 rounded-full {service.is_healthy
								? 'bg-success'
								: 'bg-destructive'}"
						></div>
						<span class="truncate text-sm font-medium text-foreground">{service.name}</span>
					</div>
					<span class="font-mono text-[10px] {uptimeColor(service.uptime_90d)}">
						{formatUptime(service.uptime_90d)}
					</span>
				</div>

				<!-- 48-hour bar strip -->
				<div class="mt-2 flex h-3 items-stretch gap-px" aria-label="Last 48 hours">
					{#each buckets as b}
						<div
							class="flex-1 rounded-[1px] {statusBarClass(b.status)}"
							title={bucketTitle(b)}
						></div>
					{:else}
						<div class="flex-1 rounded-[1px] bg-muted"></div>
					{/each}
				</div>
			</button>
		{:else}
			{#if !error}
				<p class="text-sm text-muted-foreground">No services monitored</p>
			{/if}
		{/each}
	</div>
</div>

<!-- Side panel -->
{#if openService}
	{@const svc = openService}
	<div class="fixed inset-0 z-50 flex">
		<button
			type="button"
			onclick={closePanel}
			aria-label="Close"
			class="flex-1 cursor-pointer bg-background/60"
		></button>
		<aside class="flex h-full w-full max-w-2xl flex-col overflow-y-auto border-l border-border bg-card shadow-2xl">
			<header class="sticky top-0 flex items-center justify-between gap-3 border-b border-border bg-card px-5 py-4">
				<div class="flex min-w-0 items-center gap-2">
					<div
						class="h-2.5 w-2.5 flex-shrink-0 rounded-full {svc.is_healthy
							? 'bg-success'
							: 'bg-destructive'}"
					></div>
					<h2 class="truncate text-base font-semibold text-foreground">{svc.name}</h2>
					<a
						href={svc.url}
						target="_blank"
						rel="noopener"
						class="text-muted-foreground hover:text-foreground"
						title="Open"
					>
						<ExternalLink class="h-4 w-4" />
					</a>
				</div>
				<button
					type="button"
					onclick={closePanel}
					class="cursor-pointer rounded-md p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground"
					title="Close"
				>
					<X class="h-4 w-4" />
				</button>
			</header>

			<div class="space-y-6 p-5">
				<!-- Headline uptime -->
				<div class="grid grid-cols-2 gap-3 sm:grid-cols-4">
					<div class="rounded-md border border-border bg-background p-3">
						<p class="text-xs text-muted-foreground">7-day</p>
						<p class="mt-1 text-lg font-semibold {uptimeColor(svc.uptime_7d)}">
							{formatUptime(svc.uptime_7d)}
						</p>
					</div>
					<div class="rounded-md border border-border bg-background p-3">
						<p class="text-xs text-muted-foreground">30-day</p>
						<p class="mt-1 text-lg font-semibold {uptimeColor(svc.uptime_30d)}">
							{formatUptime(svc.uptime_30d)}
						</p>
					</div>
					<div class="rounded-md border border-border bg-background p-3">
						<p class="text-xs text-muted-foreground">90-day</p>
						<p class="mt-1 text-lg font-semibold {uptimeColor(svc.uptime_90d)}">
							{formatUptime(svc.uptime_90d)}
						</p>
					</div>
					<div class="rounded-md border border-border bg-background p-3">
						<p class="text-xs text-muted-foreground">365-day</p>
						<p class="mt-1 text-lg font-semibold {uptimeColor(svc.uptime_365d)}">
							{formatUptime(svc.uptime_365d)}
						</p>
					</div>
				</div>

				<!-- Last 48h bar -->
				<section class="space-y-2">
					<h3 class="text-xs font-medium tracking-wide text-muted-foreground uppercase">
						Last 48 hours
					</h3>
					<div class="flex h-8 items-stretch gap-[2px]">
						{#each recentByService[svc.id] ?? [] as b}
							<div
								class="flex-1 rounded-sm {statusBarClass(b.status)}"
								title={bucketTitle(b)}
							></div>
						{/each}
					</div>
				</section>

				<!-- Daily history (up to 365 days) -->
				<section class="space-y-3">
					<div class="flex items-center justify-between">
						<h3 class="text-xs font-medium tracking-wide text-muted-foreground uppercase">
							Daily history
						</h3>
						<span class="text-xs text-muted-foreground">
							{historyLoading ? 'Loading…' : `${history.length} day${history.length === 1 ? '' : 's'}`}
						</span>
					</div>

					{#if historyLoading}
						<div class="flex h-8 items-center">
							<div class="h-4 w-4 animate-spin rounded-full border-2 border-muted-foreground border-t-primary"></div>
						</div>
					{:else if history.length > 0}
						<!-- Reverse: oldest on the left, newest on the right -->
						<div class="flex h-8 items-stretch gap-[2px]" dir="ltr">
							{#each history.slice().reverse() as day}
								<div
									class="flex-1 rounded-sm {dayBarClass(day.uptime_percentage)}"
									title="{day.date} — {day.uptime_percentage.toFixed(2)}% ({day.healthy_checks}/{day.total_checks})"
								></div>
							{/each}
						</div>
						<div class="flex items-center justify-between text-[10px] text-muted-foreground">
							<span>{history[history.length - 1]?.date}</span>
							<span>{history[0]?.date}</span>
						</div>

						<!-- Recent days table -->
						<div class="mt-4 overflow-hidden rounded-md border border-border">
							<table class="w-full text-xs">
								<thead class="border-b border-border bg-muted/30 text-muted-foreground">
									<tr>
										<th class="px-3 py-2 text-left font-medium">Date</th>
										<th class="px-3 py-2 text-right font-medium">Uptime</th>
										<th class="px-3 py-2 text-right font-medium">Checks</th>
										<th class="px-3 py-2 text-right font-medium">Avg response</th>
									</tr>
								</thead>
								<tbody>
									{#each history.slice(0, 30) as day}
										<tr class="border-b border-border last:border-0">
											<td class="px-3 py-1.5 text-muted-foreground">{day.date}</td>
											<td class="px-3 py-1.5 text-right font-medium {uptimeColor(day.uptime_percentage)}">
												{day.uptime_percentage.toFixed(2)}%
											</td>
											<td class="px-3 py-1.5 text-right text-muted-foreground">
												{day.healthy_checks}/{day.total_checks}
											</td>
											<td class="px-3 py-1.5 text-right text-muted-foreground">
												{day.avg_response_time_ms ? `${day.avg_response_time_ms}ms` : '—'}
											</td>
										</tr>
									{/each}
								</tbody>
							</table>
						</div>
					{:else}
						<p class="text-xs text-muted-foreground">No daily history yet (rolled up after midnight).</p>
					{/if}
				</section>
			</div>
		</aside>
	</div>
{/if}
