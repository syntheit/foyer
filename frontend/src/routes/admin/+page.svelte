<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { api, ApiError } from '$lib/api/client';
	import { copyToClipboard } from '$lib/utils/clipboard';
	import { isAdmin, getUser } from '$lib/stores/auth.svelte';
	import Toggle from '$lib/components/shared/Toggle.svelte';
	import ConfirmDialog from '$lib/components/shared/ConfirmDialog.svelte';
	import {
		Trash2,
		KeyRound,
		Plus,
		Shield,
		ShieldOff,
		Power,
		PowerOff,
		Copy,
		Link as LinkIcon,
		Server
	} from '@lucide/svelte';

	type User = {
		id: number;
		username: string;
		role: 'admin' | 'user';
		active: boolean;
		created_at: string;
	};

	type Invite = {
		id: number;
		code: string;
		created_at: string;
		created_by: string;
		used_at: string | null;
		used_by: string | null;
		expires_at: string | null;
	};

	type Assignment = {
		id: number;
		vm_name: string;
		user_id: number;
		username: string;
		created_at: string;
	};

	let users = $state<User[]>([]);
	let invites = $state<Invite[]>([]);
	let inviteOnly = $state(false);
	let inviteFilter = $state<'active' | 'used'>('active');
	let assignments = $state<Assignment[]>([]);
	let allVMs = $state<string[]>([]);
	let error = $state('');
	let loading = $state(true);
	const me = $derived(getUser());

	let assignVM = $state('');
	let assignUser = $state('');
	let assignError = $state('');
	let deletingAssignment = $state<Assignment | null>(null);
	let deletingAssignmentError = $state('');

	let showCreate = $state(false);
	let newUsername = $state('');
	let newPassword = $state('');
	let newRole = $state<'admin' | 'user'>('user');
	let createError = $state('');

	let resettingUser = $state<User | null>(null);
	let resetPassword = $state('');
	let resetError = $state('');

	let deletingInvite = $state<Invite | null>(null);
	let deletingInviteError = $state('');

	let copiedKey = $state<string | null>(null);

	onMount(async () => {
		if (!isAdmin()) {
			goto('/');
			return;
		}
		await Promise.all([
			reloadUsers(),
			reloadInvites(),
			reloadSettings(),
			reloadAssignments(),
			reloadAllVMs()
		]);
		loading = false;
	});

	async function reloadAssignments() {
		try {
			assignments = (await api.get<Assignment[]>('/api/admin/vm-assignments')) || [];
		} catch (e) {
			// VM controller may be off — silently leave empty.
			assignments = [];
		}
	}

	async function reloadAllVMs() {
		try {
			allVMs = (await api.get<string[]>('/api/admin/vms')) || [];
		} catch {
			allVMs = [];
		}
	}

	async function createAssignment() {
		assignError = '';
		if (!assignVM || !assignUser) {
			assignError = 'Pick a VM and a user';
			return;
		}
		try {
			await api.post('/api/admin/vm-assignments', {
				username: assignUser,
				vm_name: assignVM
			});
			assignVM = '';
			assignUser = '';
			await reloadAssignments();
		} catch (e) {
			assignError = e instanceof ApiError ? e.message : 'Assign failed';
		}
	}

	function askDeleteAssignment(a: Assignment) {
		deletingAssignment = a;
		deletingAssignmentError = '';
	}

	async function confirmDeleteAssignment() {
		if (!deletingAssignment) return;
		try {
			await api.del(`/api/admin/vm-assignments/${deletingAssignment.id}`);
			deletingAssignment = null;
			await reloadAssignments();
		} catch (e) {
			deletingAssignmentError = e instanceof ApiError ? e.message : 'Delete failed';
		}
	}

	async function reloadUsers() {
		try {
			users = (await api.get<User[]>('/api/admin/users')) || [];
		} catch (e) {
			error = e instanceof ApiError ? e.message : 'Failed to load users';
		}
	}

	async function reloadInvites() {
		try {
			invites = (await api.get<Invite[]>(`/api/admin/invites?status=${inviteFilter}`)) || [];
		} catch (e) {
			error = e instanceof ApiError ? e.message : 'Failed to load invites';
		}
	}

	async function reloadSettings() {
		try {
			const s = await api.get<{ invite_only_enabled: boolean }>('/api/admin/settings');
			inviteOnly = s.invite_only_enabled;
		} catch (e) {
			error = e instanceof ApiError ? e.message : 'Failed to load settings';
		}
	}

	async function setInviteOnly(next: boolean) {
		try {
			await api.patch('/api/admin/settings', { invite_only_enabled: next });
			inviteOnly = next;
		} catch (e) {
			error = e instanceof ApiError ? e.message : 'Failed to update settings';
		}
	}

	async function createUser() {
		createError = '';
		try {
			await api.post('/api/admin/users', {
				username: newUsername,
				password: newPassword,
				role: newRole
			});
			showCreate = false;
			newUsername = '';
			newPassword = '';
			newRole = 'user';
			await reloadUsers();
		} catch (e) {
			createError = e instanceof ApiError ? e.message : 'Failed to create user';
		}
	}

	async function patchUser(u: User, body: Record<string, unknown>) {
		error = '';
		try {
			await api.patch(`/api/admin/users/${u.id}`, body);
			await reloadUsers();
		} catch (e) {
			error = e instanceof ApiError ? e.message : 'Update failed';
		}
	}

	async function deleteUser(u: User) {
		if (!confirm(`Delete user ${u.username}?`)) return;
		try {
			await api.del(`/api/admin/users/${u.id}`);
			await reloadUsers();
		} catch (e) {
			error = e instanceof ApiError ? e.message : 'Delete failed';
		}
	}

	async function submitPasswordReset() {
		if (!resettingUser) return;
		resetError = '';
		try {
			await api.patch(`/api/admin/users/${resettingUser.id}`, { password: resetPassword });
			resettingUser = null;
			resetPassword = '';
		} catch (e) {
			resetError = e instanceof ApiError ? e.message : 'Reset failed';
		}
	}

	async function generateInvite() {
		try {
			await api.post<{ code: string }>('/api/admin/invites');
			inviteFilter = 'active';
			await reloadInvites();
		} catch (e) {
			error = e instanceof ApiError ? e.message : 'Generate failed';
		}
	}

	function askDeleteInvite(inv: Invite) {
		deletingInvite = inv;
		deletingInviteError = '';
	}

	async function confirmDeleteInvite() {
		if (!deletingInvite) return;
		try {
			await api.del(`/api/admin/invites/${deletingInvite.id}`);
			deletingInvite = null;
			await reloadInvites();
		} catch (e) {
			deletingInviteError = e instanceof ApiError ? e.message : 'Delete failed';
		}
	}

	async function copyText(key: string, text: string) {
		if (await copyToClipboard(text)) {
			copiedKey = key;
			setTimeout(() => {
				if (copiedKey === key) copiedKey = null;
			}, 1500);
		}
	}

	function signupLink(code: string) {
		return `${window.location.origin}/register?code=${code}`;
	}

	$effect(() => {
		// Re-fetch when filter changes
		inviteFilter;
		if (!loading) reloadInvites();
	});
</script>

<svelte:head>
	<title>Admin — Foyer</title>
</svelte:head>

<div class="space-y-8">
	<h1 class="text-lg font-semibold text-foreground">Admin</h1>

	{#if error}
		<p class="text-sm text-destructive">{error}</p>
	{/if}

	{#if loading}
		<p class="text-sm text-muted-foreground">Loading…</p>
	{:else}
		<!-- Settings -->
		<section class="space-y-3">
			<h2 class="text-sm font-medium text-foreground">Settings</h2>
			<div class="rounded-lg border border-border bg-card p-4">
				<div class="flex items-start justify-between gap-4">
					<div class="min-w-0">
						<p class="text-sm font-medium text-foreground">Invite-only registration</p>
						<p class="mt-0.5 text-xs text-muted-foreground">
							When enabled, new accounts require a valid invite code. The first user (admin) is exempt.
						</p>
					</div>
					<div class="mt-0.5">
						<Toggle
							checked={inviteOnly}
							onchange={setInviteOnly}
							label="Invite-only registration"
						/>
					</div>
				</div>
			</div>
		</section>

		<!-- Users -->
		<section class="space-y-3">
			<div class="flex items-center justify-between">
				<h2 class="text-sm font-medium text-foreground">Users</h2>
				<button
					onclick={() => (showCreate = !showCreate)}
					class="flex cursor-pointer items-center gap-1.5 rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground hover:bg-primary/90"
				>
					<Plus class="h-4 w-4" />
					New user
				</button>
			</div>

			{#if showCreate}
				<div class="rounded-lg border border-border bg-card p-4">
					<div class="grid gap-3 sm:grid-cols-2">
						<div>
							<label for="new-username" class="mb-1 block text-xs text-muted-foreground">Username</label>
							<input
								id="new-username"
								type="text"
								bind:value={newUsername}
								class="w-full rounded-md border border-border bg-background px-3 py-1.5 text-sm"
								placeholder="alice"
							/>
						</div>
						<div>
							<label for="new-password" class="mb-1 block text-xs text-muted-foreground">Password</label>
							<input
								id="new-password"
								type="password"
								bind:value={newPassword}
								class="w-full rounded-md border border-border bg-background px-3 py-1.5 text-sm"
								placeholder="min. 8 characters"
							/>
						</div>
						<div>
							<label for="new-role" class="mb-1 block text-xs text-muted-foreground">Role</label>
							<select
								id="new-role"
								bind:value={newRole}
								class="w-full rounded-md border border-border bg-background px-3 py-1.5 text-sm"
							>
								<option value="user">user</option>
								<option value="admin">admin</option>
							</select>
						</div>
					</div>
					{#if createError}
						<p class="mt-2 text-xs text-destructive">{createError}</p>
					{/if}
					<div class="mt-3 flex justify-end gap-2">
						<button
							onclick={() => (showCreate = false)}
							class="cursor-pointer rounded-md px-3 py-1.5 text-sm text-muted-foreground hover:bg-accent"
						>
							Cancel
						</button>
						<button
							onclick={createUser}
							class="cursor-pointer rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground hover:bg-primary/90"
						>
							Create
						</button>
					</div>
				</div>
			{/if}

			<div class="overflow-hidden rounded-lg border border-border bg-card">
				<table class="w-full text-sm">
					<thead class="border-b border-border bg-muted/30 text-xs text-muted-foreground">
						<tr>
							<th class="px-4 py-2 text-left font-medium">Username</th>
							<th class="px-4 py-2 text-left font-medium">Role</th>
							<th class="px-4 py-2 text-left font-medium">Status</th>
							<th class="px-4 py-2 text-left font-medium">Created</th>
							<th class="px-4 py-2 text-right font-medium">Actions</th>
						</tr>
					</thead>
					<tbody>
						{#each users as u (u.id)}
							<tr class="border-b border-border last:border-0">
								<td class="px-4 py-2 font-medium text-foreground">
									{u.username}
									{#if u.username === me?.username}
										<span class="ml-1 text-xs text-muted-foreground">(you)</span>
									{/if}
								</td>
								<td class="px-4 py-2">
									<span class="rounded px-2 py-0.5 text-xs {u.role === 'admin' ? 'bg-primary/10 text-primary' : 'bg-muted text-muted-foreground'}">
										{u.role}
									</span>
								</td>
								<td class="px-4 py-2">
									<span class="text-xs {u.active ? 'text-foreground' : 'text-muted-foreground'}">
										{u.active ? 'active' : 'disabled'}
									</span>
								</td>
								<td class="px-4 py-2 text-xs text-muted-foreground">{u.created_at.slice(0, 10)}</td>
								<td class="px-4 py-2">
									<div class="flex items-center justify-end gap-1">
										<button
											title={u.role === 'admin' ? 'Demote to user' : 'Promote to admin'}
											onclick={() => patchUser(u, { role: u.role === 'admin' ? 'user' : 'admin' })}
											disabled={u.username === me?.username}
											class="cursor-pointer rounded p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground disabled:cursor-not-allowed disabled:opacity-30 disabled:hover:bg-transparent"
										>
											{#if u.role === 'admin'}
												<ShieldOff class="h-4 w-4" />
											{:else}
												<Shield class="h-4 w-4" />
											{/if}
										</button>
										<button
											title={u.active ? 'Deactivate' : 'Activate'}
											onclick={() => patchUser(u, { active: !u.active })}
											disabled={u.username === me?.username}
											class="cursor-pointer rounded p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground disabled:cursor-not-allowed disabled:opacity-30 disabled:hover:bg-transparent"
										>
											{#if u.active}
												<PowerOff class="h-4 w-4" />
											{:else}
												<Power class="h-4 w-4" />
											{/if}
										</button>
										<button
											title="Reset password"
											onclick={() => {
												resettingUser = u;
												resetPassword = '';
												resetError = '';
											}}
											class="cursor-pointer rounded p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground"
										>
											<KeyRound class="h-4 w-4" />
										</button>
										<button
											title="Delete"
											onclick={() => deleteUser(u)}
											disabled={u.username === me?.username}
											class="cursor-pointer rounded p-1.5 text-muted-foreground hover:bg-destructive/10 hover:text-destructive disabled:cursor-not-allowed disabled:opacity-30 disabled:hover:bg-transparent disabled:hover:text-muted-foreground"
										>
											<Trash2 class="h-4 w-4" />
										</button>
									</div>
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		</section>

		<!-- Invite codes -->
		<section class="space-y-3">
			<div class="flex items-center justify-between">
				<div>
					<h2 class="text-sm font-medium text-foreground">Invite codes</h2>
					<p class="mt-0.5 text-xs text-muted-foreground">
						Generate codes to share. Each code can be used once to register a new account.
					</p>
				</div>
				<button
					onclick={generateInvite}
					class="flex cursor-pointer items-center gap-1.5 rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground hover:bg-primary/90"
				>
					<Plus class="h-4 w-4" />
					Generate
				</button>
			</div>

			<div class="flex items-center gap-1 border-b border-border">
				{#each ['active', 'used'] as f}
					<button
						onclick={() => (inviteFilter = f as 'active' | 'used')}
						class="cursor-pointer border-b-2 px-3 py-1.5 text-sm capitalize {inviteFilter === f
							? 'border-primary text-foreground'
							: 'border-transparent text-muted-foreground hover:text-foreground'}"
					>
						{f}
					</button>
				{/each}
			</div>

			<div class="overflow-hidden rounded-lg border border-border bg-card">
				<table class="w-full text-sm">
					<thead class="border-b border-border bg-muted/30 text-xs text-muted-foreground">
						<tr>
							<th class="px-4 py-2 text-left font-medium">Code</th>
							<th class="px-4 py-2 text-left font-medium">Created by</th>
							<th class="px-4 py-2 text-left font-medium">Created</th>
							{#if inviteFilter === 'used'}
								<th class="px-4 py-2 text-left font-medium">Used by</th>
								<th class="px-4 py-2 text-left font-medium">Used at</th>
							{/if}
							<th class="px-4 py-2 text-right font-medium">Actions</th>
						</tr>
					</thead>
					<tbody>
						{#each invites as inv (inv.id)}
							<tr class="border-b border-border last:border-0">
								<td class="px-4 py-2 font-mono font-medium text-foreground">{inv.code}</td>
								<td class="px-4 py-2 text-muted-foreground">{inv.created_by}</td>
								<td class="px-4 py-2 text-xs text-muted-foreground">{inv.created_at.slice(0, 16).replace('T', ' ')}</td>
								{#if inviteFilter === 'used'}
									<td class="px-4 py-2 text-muted-foreground">{inv.used_by ?? '—'}</td>
									<td class="px-4 py-2 text-xs text-muted-foreground">
										{inv.used_at ? inv.used_at.slice(0, 16).replace('T', ' ') : '—'}
									</td>
								{/if}
								<td class="px-4 py-2">
									<div class="flex items-center justify-end gap-1">
										<button
											title={copiedKey === `code-${inv.id}` ? 'Copied' : 'Copy code'}
											onclick={() => copyText(`code-${inv.id}`, inv.code)}
											class="cursor-pointer rounded p-1.5 {copiedKey === `code-${inv.id}` ? 'text-primary' : 'text-muted-foreground'} hover:bg-accent hover:text-foreground"
										>
											<Copy class="h-4 w-4" />
										</button>
										<button
											title={copiedKey === `link-${inv.id}` ? 'Copied' : 'Copy signup link'}
											onclick={() => copyText(`link-${inv.id}`, signupLink(inv.code))}
											class="cursor-pointer rounded p-1.5 {copiedKey === `link-${inv.id}` ? 'text-primary' : 'text-muted-foreground'} hover:bg-accent hover:text-foreground"
										>
											<LinkIcon class="h-4 w-4" />
										</button>
										<button
											title="Delete"
											onclick={() => askDeleteInvite(inv)}
											class="cursor-pointer rounded p-1.5 text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
										>
											<Trash2 class="h-4 w-4" />
										</button>
									</div>
								</td>
							</tr>
						{:else}
							<tr>
								<td colspan="6" class="px-4 py-6 text-center text-xs text-muted-foreground">
									{inviteFilter === 'active' ? 'No active invite codes' : 'No used invite codes'}
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		</section>

		<!-- VM assignments -->
		<section class="space-y-3">
			<div>
				<h2 class="flex items-center gap-2 text-sm font-medium text-foreground">
					<Server class="h-4 w-4" />
					VM assignments
				</h2>
				<p class="mt-0.5 text-xs text-muted-foreground">
					Assign a libvirt VM to a foyer user. They'll see it under "VMs" in the nav and be able to view stats and gracefully reboot/shutdown.
				</p>
			</div>

			<div class="rounded-lg border border-border bg-card p-4">
				<div class="grid gap-3 sm:grid-cols-3">
					<div>
						<label for="assign-vm" class="mb-1 block text-xs text-muted-foreground">VM</label>
						<select
							id="assign-vm"
							bind:value={assignVM}
							class="w-full rounded-md border border-border bg-background px-3 py-1.5 text-sm"
						>
							<option value="">— pick a VM —</option>
							{#each allVMs as name}
								<option value={name}>{name}</option>
							{/each}
						</select>
					</div>
					<div>
						<label for="assign-user" class="mb-1 block text-xs text-muted-foreground">User</label>
						<select
							id="assign-user"
							bind:value={assignUser}
							class="w-full rounded-md border border-border bg-background px-3 py-1.5 text-sm"
						>
							<option value="">— pick a user —</option>
							{#each users.filter((u) => u.active) as u}
								<option value={u.username}>{u.username}</option>
							{/each}
						</select>
					</div>
					<div class="flex items-end">
						<button
							type="button"
							onclick={createAssignment}
							class="flex cursor-pointer items-center gap-1.5 rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground hover:bg-primary/90"
						>
							<Plus class="h-4 w-4" />
							Assign
						</button>
					</div>
				</div>
				{#if assignError}
					<p class="mt-2 text-xs text-destructive">{assignError}</p>
				{/if}
			</div>

			<div class="overflow-hidden rounded-lg border border-border bg-card">
				<table class="w-full text-sm">
					<thead class="border-b border-border bg-muted/30 text-xs text-muted-foreground">
						<tr>
							<th class="px-4 py-2 text-left font-medium">VM</th>
							<th class="px-4 py-2 text-left font-medium">User</th>
							<th class="px-4 py-2 text-left font-medium">Assigned</th>
							<th class="px-4 py-2 text-right font-medium">Actions</th>
						</tr>
					</thead>
					<tbody>
						{#each assignments as a (a.id)}
							<tr class="border-b border-border last:border-0">
								<td class="px-4 py-2 font-mono text-foreground">{a.vm_name}</td>
								<td class="px-4 py-2 text-foreground">{a.username}</td>
								<td class="px-4 py-2 text-xs text-muted-foreground">{a.created_at.slice(0, 10)}</td>
								<td class="px-4 py-2">
									<div class="flex items-center justify-end gap-1">
										<button
											type="button"
											title="Unassign"
											onclick={() => askDeleteAssignment(a)}
											class="cursor-pointer rounded p-1.5 text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
										>
											<Trash2 class="h-4 w-4" />
										</button>
									</div>
								</td>
							</tr>
						{:else}
							<tr>
								<td colspan="4" class="px-4 py-6 text-center text-xs text-muted-foreground">
									No VM assignments
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		</section>
	{/if}
</div>

<ConfirmDialog
	open={!!deletingAssignment}
	title="Unassign VM?"
	confirmLabel="Unassign"
	error={deletingAssignmentError}
	onCancel={() => (deletingAssignment = null)}
	onConfirm={confirmDeleteAssignment}
>
	{#snippet body()}
		{#if deletingAssignment}
			<span class="text-foreground">{deletingAssignment.username}</span> will lose access to
			<span class="font-mono text-foreground">{deletingAssignment.vm_name}</span>. The VM itself is unaffected.
		{/if}
	{/snippet}
</ConfirmDialog>

<ConfirmDialog
	open={!!deletingInvite}
	title="Delete invite code?"
	confirmLabel="Delete"
	error={deletingInviteError}
	onCancel={() => (deletingInvite = null)}
	onConfirm={confirmDeleteInvite}
>
	{#snippet body()}
		{#if deletingInvite}
			The code <span class="font-mono text-foreground">{deletingInvite.code}</span> will be removed.
			{#if deletingInvite.used_at}
				This code has already been used; deleting it won't affect the registered account.
			{:else}
				Anyone holding this link will no longer be able to register.
			{/if}
		{/if}
	{/snippet}
</ConfirmDialog>

{#if resettingUser}
	<div class="fixed inset-0 z-50 flex items-center justify-center bg-background/80 p-4">
		<div class="w-full max-w-sm rounded-lg border border-border bg-card p-4">
			<h2 class="mb-3 text-sm font-medium text-foreground">
				Reset password for {resettingUser.username}
			</h2>
			<input
				type="password"
				bind:value={resetPassword}
				class="w-full rounded-md border border-border bg-background px-3 py-1.5 text-sm"
				placeholder="new password (min. 8 characters)"
			/>
			{#if resetError}
				<p class="mt-2 text-xs text-destructive">{resetError}</p>
			{/if}
			<div class="mt-3 flex justify-end gap-2">
				<button
					onclick={() => (resettingUser = null)}
					class="cursor-pointer rounded-md px-3 py-1.5 text-sm text-muted-foreground hover:bg-accent"
				>
					Cancel
				</button>
				<button
					onclick={submitPasswordReset}
					class="cursor-pointer rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground hover:bg-primary/90"
				>
					Reset
				</button>
			</div>
		</div>
	</div>
{/if}
