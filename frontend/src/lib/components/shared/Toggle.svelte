<script lang="ts">
	type Size = 'sm' | 'md';

	type Props = {
		checked: boolean;
		onchange: (next: boolean) => void;
		size?: Size;
		label?: string;
	};

	let { checked, onchange, size = 'sm', label }: Props = $props();

	// Track + thumb sizes — "sm" matches the previous admin/messages toggle dims.
	const trackClass = $derived(size === 'md' ? 'h-6 w-10' : 'h-5 w-9');
	const thumbClass = $derived(size === 'md' ? 'h-5 w-5' : 'h-4 w-4');
	const onTranslate = $derived(size === 'md' ? 'translate-x-[18px]' : 'translate-x-[18px]');
	const offTranslate = 'translate-x-[2px]';
</script>

<button
	type="button"
	role="switch"
	aria-checked={checked}
	aria-label={label}
	onclick={() => onchange(!checked)}
	class="relative inline-flex shrink-0 cursor-pointer items-center rounded-full transition-colors focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background focus-visible:outline-none {trackClass} {checked ? 'bg-primary' : 'bg-muted'}"
>
	<span
		class="inline-block transform rounded-full bg-background shadow-sm transition-transform {thumbClass} {checked ? onTranslate : offTranslate}"
	></span>
</button>
