<script lang="ts">
	import { api, ApiError } from '$lib/api/client';
	import { Search, Globe } from '@lucide/svelte';

	type IPResult = {
		query: string;
		status: string;
		country: string;
		countryCode: string;
		region: string;
		regionName: string;
		city: string;
		zip: string;
		lat: number;
		lon: number;
		timezone: string;
		isp: string;
		org: string;
		as: string;
	};

	let address = $state('');
	let result = $state<IPResult | null>(null);
	let error = $state('');
	let loading = $state(false);

	async function lookupSelf() {
		loading = true;
		error = '';
		try {
			result = await api.get<IPResult>('/api/tools/ip');
			address = result?.query || '';
		} catch {
			error = 'Lookup failed';
		} finally {
			loading = false;
		}
	}

	async function lookup(e?: SubmitEvent) {
		e?.preventDefault();
		if (!address.trim()) return;
		loading = true;
		error = '';
		try {
			result = await api.get<IPResult>(`/api/tools/ip/${address.trim()}`);
		} catch (err) {
			if (err instanceof ApiError) {
				error = err.message;
			} else {
				error = 'Lookup failed';
			}
			result = null;
		} finally {
			loading = false;
		}
	}
</script>

<svelte:head>
	<title>IP Lookup — Foyer</title>
</svelte:head>

<div class="space-y-6">
	<h1 class="text-lg font-semibold text-foreground">IP Lookup</h1>

	<form onsubmit={lookup} class="flex gap-2">
		<input
			type="text"
			bind:value={address}
			placeholder="Enter IP address"
			class="flex-1 rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
		/>
		<button
			type="submit"
			disabled={loading || !address.trim()}
			class="inline-flex items-center gap-1.5 rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
		>
			<Search class="h-4 w-4" />
			Lookup
		</button>
		<button
			type="button"
			onclick={lookupSelf}
			disabled={loading}
			class="inline-flex items-center gap-1.5 rounded-md border border-input bg-background px-4 py-2 text-sm text-foreground hover:bg-accent disabled:opacity-50"
		>
			<Globe class="h-4 w-4" />
			My IP
		</button>
	</form>

	{#if error}
		<p class="text-sm text-destructive">{error}</p>
	{/if}

	{#if result}
		<div class="rounded-lg border border-border bg-card p-4">
			<div class="grid grid-cols-2 gap-3 text-sm">
				<div>
					<p class="text-xs text-muted-foreground uppercase">IP</p>
					<p class="font-mono text-foreground">{result.query}</p>
				</div>
				<div>
					<p class="text-xs text-muted-foreground uppercase">Location</p>
					<p class="text-foreground">{result.city}, {result.regionName}, {result.country}</p>
				</div>
				<div>
					<p class="text-xs text-muted-foreground uppercase">ISP</p>
					<p class="text-foreground">{result.isp}</p>
				</div>
				<div>
					<p class="text-xs text-muted-foreground uppercase">Organization</p>
					<p class="text-foreground">{result.org}</p>
				</div>
				<div>
					<p class="text-xs text-muted-foreground uppercase">AS</p>
					<p class="text-foreground">{result.as}</p>
				</div>
				<div>
					<p class="text-xs text-muted-foreground uppercase">Timezone</p>
					<p class="text-foreground">{result.timezone}</p>
				</div>
				<div>
					<p class="text-xs text-muted-foreground uppercase">Coordinates</p>
					<p class="text-foreground">{result.lat}, {result.lon}</p>
				</div>
				<div>
					<p class="text-xs text-muted-foreground uppercase">Zip</p>
					<p class="text-foreground">{result.zip}</p>
				</div>
			</div>
		</div>
	{/if}
</div>
