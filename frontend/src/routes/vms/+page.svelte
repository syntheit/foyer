<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { api, ApiError } from '$lib/api/client';
	import { formatBytes, formatBytesPerSec } from '$lib/utils/format';
	import ConfirmDialog from '$lib/components/shared/ConfirmDialog.svelte';
	import { Server, Power, RotateCw } from '@lucide/svelte';

	type VM = {
		name: string;
		state: string;
		assigned_at: string;
	};

	type VMStats = {
		state: string;
		vcpus: number;
		cpu_time_ns: number;
		mem_max_kib: number;
		mem_rss_kib: number;
		mem_usable_kib: number;
		mem_unused_kib: number;
		disk_capacity_b: number;
		disk_alloc_b: number;
		disk_read_b: number;
		disk_written_b: number;
		net_rx_bytes: number;
		net_tx_bytes: number;
		sampled_at: number;
	};

	let vms = $state<VM[]>([]);
	let statsByVM = $state<Record<string, VMStats | null>>({});
	let cpuPctByVM = $state<Record<string, number>>({});
	let netRateByVM = $state<Record<string, { rx: number; tx: number }>>({});
	// Used only inside refreshOne to compute deltas; not read from the template,
	// so it doesn't need to be reactive.
	const prevStatsByVM: Record<string, { stats: VMStats; t: number } | null> = {};

	let error = $state('');
	let confirmAction = $state<{ vm: string; action: 'reboot' | 'shutdown' } | null>(null);
	let confirmError = $state('');
	let actionInFlight = $state(false);

	let pollTimer: ReturnType<typeof setInterval> | null = null;

	onMount(async () => {
		try {
			vms = (await api.get<VM[]>('/api/vms')) || [];
		} catch (e) {
			error = e instanceof ApiError ? e.message : 'Failed to load VMs';
			return;
		}
		await refreshAll();
		pollTimer = setInterval(refreshAll, 5000);
	});

	onDestroy(() => {
		if (pollTimer) clearInterval(pollTimer);
	});

	async function refreshAll() {
		await Promise.all(vms.map((v) => refreshOne(v.name)));
	}

	async function refreshOne(name: string) {
		try {
			const fresh = await api.get<VMStats>(`/api/vms/${encodeURIComponent(name)}/stats`);
			const now = Date.now();
			const prev = prevStatsByVM[name];

			// CPU% — derive from cpu_time_ns delta over wall-clock delta.
			if (prev && fresh) {
				const dtNs = (now - prev.t) * 1_000_000;
				const cpuDeltaNs = Math.max(0, fresh.cpu_time_ns - prev.stats.cpu_time_ns);
				const vcpus = fresh.vcpus || 1;
				if (dtNs > 0) {
					cpuPctByVM[name] = Math.min(100, (cpuDeltaNs / dtNs / vcpus) * 100);
				}
				const rxRate = Math.max(0, fresh.net_rx_bytes - prev.stats.net_rx_bytes) / ((now - prev.t) / 1000);
				const txRate = Math.max(0, fresh.net_tx_bytes - prev.stats.net_tx_bytes) / ((now - prev.t) / 1000);
				netRateByVM[name] = { rx: rxRate, tx: txRate };
			}

			prevStatsByVM[name] = { stats: fresh, t: now };
			statsByVM[name] = fresh;

			// Only swap the list reference when state actually changed; otherwise
			// the array allocation forces a full {#each} reconciliation each tick.
			const idx = vms.findIndex((v) => v.name === name);
			if (idx >= 0 && vms[idx].state !== fresh.state) {
				vms = vms.with(idx, { ...vms[idx], state: fresh.state });
			}
		} catch {
			// stats endpoint may transiently fail (VM stopping, etc.); leave previous
		}
	}

	function askAction(vm: string, action: 'reboot' | 'shutdown') {
		confirmAction = { vm, action };
		confirmError = '';
	}

	async function performAction() {
		if (!confirmAction) return;
		actionInFlight = true;
		confirmError = '';
		try {
			await api.post(`/api/vms/${encodeURIComponent(confirmAction.vm)}/power`, {
				action: confirmAction.action
			});
			confirmAction = null;
			// Give libvirt a moment, then refresh state.
			setTimeout(refreshAll, 500);
		} catch (e) {
			confirmError = e instanceof ApiError ? e.message : 'Operation failed';
		} finally {
			actionInFlight = false;
		}
	}

	function stateClass(state: string): string {
		const s = state.toLowerCase();
		if (s.includes('running')) return 'bg-success/10 text-success';
		if (s.includes('shut') || s.includes('off')) return 'bg-muted text-muted-foreground';
		if (s.includes('paused') || s.includes('idle')) return 'bg-warning/10 text-warning';
		return 'bg-muted text-muted-foreground';
	}

</script>

<svelte:head>
	<title>VMs — Foyer</title>
</svelte:head>

<div class="space-y-6">
	<h1 class="text-lg font-semibold text-foreground">VMs</h1>

	{#if error}
		<p class="text-sm text-destructive">{error}</p>
	{/if}

	{#if vms.length === 0 && !error}
		<p class="text-sm text-muted-foreground">No VMs assigned.</p>
	{/if}

	<div class="space-y-4">
		{#each vms as vm (vm.name)}
			{@const stats = statsByVM[vm.name]}
			{@const cpuPct = cpuPctByVM[vm.name] ?? 0}
			{@const netRate = netRateByVM[vm.name]}
			{@const memUsedKib = stats ? stats.mem_max_kib - stats.mem_usable_kib : 0}
			{@const memPct = stats && stats.mem_max_kib > 0 ? (memUsedKib / stats.mem_max_kib) * 100 : 0}
			{@const diskPct = stats && stats.disk_capacity_b > 0
				? (stats.disk_alloc_b / stats.disk_capacity_b) * 100
				: 0}
			{@const isRunning = (stats?.state ?? vm.state ?? '').toLowerCase().includes('running')}

			<div class="rounded-lg border border-border bg-card p-4">
				<div class="flex items-center justify-between">
					<div class="flex items-center gap-2">
						<Server class="h-4 w-4 text-muted-foreground" />
						<h2 class="text-sm font-medium text-foreground">{vm.name}</h2>
						<span class="rounded-full px-2 py-0.5 text-xs {stateClass(stats?.state ?? vm.state)}">
							{stats?.state ?? vm.state ?? 'unknown'}
						</span>
					</div>
					<div class="flex items-center gap-1">
						<button
							type="button"
							onclick={() => askAction(vm.name, 'reboot')}
							disabled={!isRunning}
							class="inline-flex cursor-pointer items-center gap-1 rounded-md px-2 py-1 text-xs text-muted-foreground hover:bg-accent hover:text-foreground disabled:cursor-not-allowed disabled:opacity-50"
						>
							<RotateCw class="h-3.5 w-3.5" />
							Reboot
						</button>
						<button
							type="button"
							onclick={() => askAction(vm.name, 'shutdown')}
							disabled={!isRunning}
							class="inline-flex cursor-pointer items-center gap-1 rounded-md px-2 py-1 text-xs text-muted-foreground hover:bg-destructive/10 hover:text-destructive disabled:cursor-not-allowed disabled:opacity-50"
						>
							<Power class="h-3.5 w-3.5" />
							Shutdown
						</button>
					</div>
				</div>

				{#if stats}
					<div class="mt-4 grid grid-cols-2 gap-3 sm:grid-cols-4">
						<div class="rounded-md border border-border bg-background p-3">
							<p class="text-xs text-muted-foreground">CPU</p>
							<p class="mt-1 text-lg font-semibold text-foreground">{cpuPct.toFixed(1)}%</p>
							<p class="text-[10px] text-muted-foreground">{stats.vcpus} vCPU</p>
						</div>
						<div class="rounded-md border border-border bg-background p-3">
							<p class="text-xs text-muted-foreground">Memory</p>
							<p class="mt-1 text-lg font-semibold text-foreground">{memPct.toFixed(0)}%</p>
							<p class="text-[10px] text-muted-foreground">
								{formatBytes(memUsedKib * 1024)} / {formatBytes(stats.mem_max_kib * 1024)}
							</p>
						</div>
						<div class="rounded-md border border-border bg-background p-3">
							<p class="text-xs text-muted-foreground">Disk</p>
							<p class="mt-1 text-lg font-semibold text-foreground">{diskPct.toFixed(0)}%</p>
							<p class="text-[10px] text-muted-foreground">
								{formatBytes(stats.disk_alloc_b)} / {formatBytes(stats.disk_capacity_b)}
							</p>
						</div>
						<div class="rounded-md border border-border bg-background p-3">
							<p class="text-xs text-muted-foreground">Network</p>
							{#if netRate}
								<p class="mt-1 text-sm font-medium text-foreground">
									↓ {formatBytesPerSec(netRate.rx)}
								</p>
								<p class="text-xs text-muted-foreground">↑ {formatBytesPerSec(netRate.tx)}</p>
							{:else}
								<p class="mt-1 text-sm text-muted-foreground">—</p>
							{/if}
						</div>
					</div>
				{/if}
			</div>
		{/each}
	</div>
</div>

<ConfirmDialog
	open={!!confirmAction}
	title={confirmAction
		? `${confirmAction.action === 'reboot' ? 'Reboot' : 'Shut down'} ${confirmAction.vm}?`
		: ''}
	confirmLabel={confirmAction?.action === 'reboot' ? 'Reboot' : 'Shut down'}
	variant={confirmAction?.action === 'reboot' ? 'primary' : 'destructive'}
	error={confirmError}
	busy={actionInFlight}
	onCancel={() => (confirmAction = null)}
	onConfirm={performAction}
>
	{#snippet body()}
		{#if confirmAction?.action === 'reboot'}
			Sends an ACPI reboot signal. The VM should come back online in a moment.
		{:else}
			Sends an ACPI shutdown signal. The VM will power off; you'll need to ask an admin to start it again.
		{/if}
	{/snippet}
</ConfirmDialog>
