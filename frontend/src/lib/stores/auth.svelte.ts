import { goto } from '$app/navigation';
import { api } from '$lib/api/client';

type User = {
	username: string;
	role: string;
};

let user = $state<User | null>(null);
let checked = $state(false);

// Deduplication: only one checkAuth in flight at a time
let checkPromise: Promise<boolean> | null = null;

export function getUser() {
	return user;
}

export function isChecked() {
	return checked;
}

export function isAdmin() {
	return user?.role === 'admin';
}

export async function checkAuth(): Promise<boolean> {
	if (checkPromise) return checkPromise;
	checkPromise = doCheckAuth();
	try {
		return await checkPromise;
	} finally {
		checkPromise = null;
	}
}

async function doCheckAuth(): Promise<boolean> {
	try {
		user = await api.get<User>('/api/auth/me');
		checked = true;
		return true;
	} catch {
		user = null;
		checked = true;
		return false;
	}
}

export async function login(username: string, password: string): Promise<User> {
	const result = await api.post<User>('/api/auth/login', { username, password });
	user = result;
	checked = true;
	return result;
}

export async function logout() {
	try {
		await api.post('/api/auth/logout');
	} finally {
		user = null;
	}
}

// Centralized 401 redirect — prevents multiple concurrent redirects
let redirecting = false;
export function handleUnauthorized() {
	if (redirecting) return;
	redirecting = true;
	user = null;
	goto('/login').finally(() => {
		redirecting = false;
	});
}
