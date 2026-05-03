<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { getUser, isAdmin, logout } from '$lib/stores/auth.svelte';
	import { isConnected } from '$lib/stores/stats.svelte';
	import {
		Server,
		LayoutDashboard,
		MessageSquare,
		Upload,
		FileCode,
		Activity,
		Wrench,
		LogOut,
		Circle,
		Menu,
		X,
		Users,
		MonitorCog
	} from '@lucide/svelte';

	const user = $derived(getUser());
	const wsConnected = $derived(isConnected());
	const admin = $derived(isAdmin());

	let mobileOpen = $state(false);
	let hasVMs = $state(false);

	onMount(async () => {
		// Show the VMs link only if the user actually has assignments. Treat any
		// failure as "no VMs" — the link is purely a UX shortcut.
		try {
			const res = await api.get<unknown[]>('/api/vms');
			hasVMs = Array.isArray(res) && res.length > 0;
		} catch {
			hasVMs = false;
		}
	});

	const links = $derived([
		{ href: '/', label: 'Dashboard', icon: LayoutDashboard },
		{ href: '/services', label: 'Services', icon: Activity },
		{ href: '/messages', label: 'Messages', icon: MessageSquare },
		{ href: '/files', label: 'Files', icon: Upload },
		{ href: '/pastes', label: 'Pastes', icon: FileCode },
		{ href: '/tools/ip', label: 'Tools', icon: Wrench },
		...(hasVMs ? [{ href: '/vms', label: 'VMs', icon: MonitorCog }] : []),
		...(admin ? [{ href: '/admin', label: 'Admin', icon: Users }] : [])
	]);

	function isActive(href: string): boolean {
		if (href === '/') return page.url.pathname === '/';
		return page.url.pathname.startsWith(href);
	}

	async function handleLogout() {
		await logout();
		goto('/login');
	}

	function handleNavClick() {
		mobileOpen = false;
	}
</script>

<!-- Mobile header bar -->
<div class="flex h-12 items-center border-b border-border bg-card px-4 md:hidden">
	<button onclick={() => (mobileOpen = !mobileOpen)} class="cursor-pointer text-muted-foreground">
		{#if mobileOpen}
			<X class="h-5 w-5" />
		{:else}
			<Menu class="h-5 w-5" />
		{/if}
	</button>
	<div class="ml-3 flex items-center gap-2">
		<Server class="h-4 w-4 text-primary" />
		<span class="text-sm font-semibold text-foreground">Foyer</span>
	</div>
</div>

<!-- Sidebar: always visible on desktop, overlay on mobile -->
{#if mobileOpen}
	<!-- Mobile overlay backdrop -->
	<button
		class="fixed inset-0 z-40 cursor-pointer bg-background/80 md:hidden"
		onclick={() => (mobileOpen = false)}
		aria-label="Close navigation"
	></button>
{/if}

<nav
	class="fixed z-50 flex h-screen w-56 flex-col border-r border-border bg-card transition-transform md:static md:translate-x-0
		{mobileOpen ? 'translate-x-0' : '-translate-x-full'}"
>
	<!-- Header -->
	<div class="flex items-center gap-2 border-b border-border px-4 py-4">
		<Server class="h-5 w-5 text-primary" />
		<span class="text-sm font-semibold text-foreground">Foyer</span>
	</div>

	<!-- Links -->
	<div class="flex-1 space-y-1 px-2 py-3">
		{#each links as link}
			<a
				href={link.href}
				onclick={handleNavClick}
				class="flex items-center gap-2.5 rounded-md px-3 py-2 text-sm transition-colors
					{isActive(link.href)
					? 'bg-accent text-accent-foreground font-medium'
					: 'text-muted-foreground hover:bg-accent/50 hover:text-foreground'}"
			>
				<link.icon class="h-4 w-4" />
				{link.label}
			</a>
		{/each}
	</div>

	<!-- User -->
	<div class="border-t border-border px-3 py-3">
		<div class="flex items-center justify-between">
			<div class="min-w-0">
				<p class="truncate text-sm font-medium text-foreground">{user?.username}</p>
				<p class="text-xs text-muted-foreground">{admin ? 'Admin' : 'User'}</p>
			</div>
			<button
				onclick={handleLogout}
				class="cursor-pointer rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
				title="Sign out"
			>
				<LogOut class="h-4 w-4" />
			</button>
		</div>
	</div>
</nav>
