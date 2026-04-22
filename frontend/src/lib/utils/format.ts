export function formatBytes(bytes: number): string {
	if (!bytes || bytes <= 0) return '0 B';
	const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
	const i = Math.floor(Math.log(bytes) / Math.log(1024));
	const value = bytes / Math.pow(1024, i);
	return `${value.toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}

export function formatBytesPerSec(bytes: number): string {
	return `${formatBytes(bytes)}/s`;
}

export function formatPercent(value: number): string {
	return `${value.toFixed(1)}%`;
}

export function formatUptime(seconds: number): string {
	const days = Math.floor(seconds / 86400);
	const hours = Math.floor((seconds % 86400) / 3600);
	const mins = Math.floor((seconds % 3600) / 60);

	if (days > 0) return `${days}d ${hours}h`;
	if (hours > 0) return `${hours}h ${mins}m`;
	return `${mins}m`;
}

export function formatTemp(celsius: number): string {
	return `${celsius}°C`;
}

export function formatLoad(load: number[]): string {
	if (!load || load.length === 0) return '—';
	return load.map((l) => l.toFixed(2)).join(' · ');
}

export function timeAgo(dateStr: string): string {
	const date = new Date(dateStr);
	const now = new Date();
	const seconds = Math.floor((now.getTime() - date.getTime()) / 1000);

	if (seconds < 60) return 'just now';
	if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`;
	if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`;
	return `${Math.floor(seconds / 86400)}d ago`;
}
