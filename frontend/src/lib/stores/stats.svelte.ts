import { handleUnauthorized } from '$lib/stores/auth.svelte';

export type Stats = {
	timestamp: string;
	cpu: {
		usage_percent: number;
		cores: number;
		load: number[];
	};
	memory: {
		total_bytes: number;
		used_bytes: number;
		available_bytes: number;
		usage_percent: number;
	};
	disk: {
		pools: Array<{
			name: string;
			total_bytes: number;
			used_bytes: number;
			free_bytes: number;
			usage_percent: number;
			health: string;
		}>;
		mounts: Array<{
			mountpoint: string;
			filesystem: string;
			total_bytes: number;
			used_bytes: number;
			usage_percent: number;
		}>;
	};
	network: {
		interfaces: Array<{
			name: string;
			rx_bytes_per_sec: number;
			tx_bytes_per_sec: number;
		}>;
	};
	temperatures: {
		cpu: number;
		gpu: number;
	};
	gpu: {
		name: string;
		utilization_percent: number;
		memory_used_mb: number;
		memory_total_mb: number;
		temperature: number;
		power_watts: number;
	} | null;
	docker: {
		containers: Array<{
			name: string;
			image: string;
			state: string;
			status: string;
		}>;
	} | null;
	system: {
		hostname: string;
		kernel: string;
		uptime_seconds: number;
		load_avg: number[];
	};
};

let stats = $state<Stats | null>(null);
let connected = $state(false);

let ws: WebSocket | null = null;
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
let reconnectDelay = 1000;
let intentionalClose = false;
let authFailureCount = 0;
const maxReconnectDelay = 30000;

export function getStats() {
	return stats;
}

export function isConnected() {
	return connected;
}

export function connect() {
	if (ws) return;

	intentionalClose = false;
	authFailureCount = 0;

	const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
	const url = `${protocol}//${window.location.host}/ws/stats`;

	ws = new WebSocket(url);

	ws.onopen = () => {
		connected = true;
		reconnectDelay = 1000;
		authFailureCount = 0;
	};

	ws.onmessage = (event) => {
		try {
			const msg = JSON.parse(event.data);
			if (msg.type === 'stats') {
				// Extract stats fields without the `type` property
				const { type: _, ...rest } = msg;
				stats = rest as Stats;
			}
		} catch {
			// Ignore malformed messages
		}
	};

	ws.onclose = (event) => {
		connected = false;
		ws = null;

		if (intentionalClose) return;

		// HTTP 401 during WebSocket upgrade — auth expired
		if (event.code === 4401 || event.code === 1008) {
			authFailureCount++;
			if (authFailureCount >= 3) {
				handleUnauthorized();
				return;
			}
		}

		scheduleReconnect();
	};

	ws.onerror = () => {
		// onerror is always followed by onclose, so just let onclose handle it
	};
}

export function disconnect() {
	intentionalClose = true;
	if (reconnectTimer) {
		clearTimeout(reconnectTimer);
		reconnectTimer = null;
	}
	if (ws) {
		ws.close();
		ws = null;
	}
	connected = false;
	stats = null;
}

function scheduleReconnect() {
	if (reconnectTimer) return;
	reconnectTimer = setTimeout(() => {
		reconnectTimer = null;
		connect();
	}, reconnectDelay);
	reconnectDelay = Math.min(reconnectDelay * 2, maxReconnectDelay);
}
