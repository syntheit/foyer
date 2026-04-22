<script lang="ts">
	type Props = {
		label: string;
		percent: number;
		detail?: string;
		color?: string;
	};

	let { label, percent, detail, color = 'bg-primary' }: Props = $props();

	const clampedPercent = $derived(Math.min(100, Math.max(0, percent)));
	const barColor = $derived(
		clampedPercent > 90 ? 'bg-destructive' : clampedPercent > 75 ? 'bg-warning' : color
	);
</script>

<div class="space-y-1.5">
	<div class="flex items-center justify-between text-xs">
		<span class="font-medium text-foreground">{label}</span>
		<span class="text-muted-foreground">{percent.toFixed(1)}%{detail ? ` · ${detail}` : ''}</span>
	</div>
	<div class="h-2 w-full overflow-hidden rounded-full bg-muted">
		<div
			class="h-full rounded-full transition-all duration-500 {barColor}"
			style="width: {clampedPercent}%"
		></div>
	</div>
</div>
