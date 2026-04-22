export async function copyToClipboard(text: string): Promise<boolean> {
	// Try the modern clipboard API first
	if (navigator.clipboard) {
		try {
			await navigator.clipboard.writeText(text);
			return true;
		} catch {
			// Falls through to fallback
		}
	}

	// Fallback for non-secure contexts (HTTP over Tailscale)
	try {
		const textarea = document.createElement('textarea');
		textarea.value = text;
		textarea.style.position = 'fixed';
		textarea.style.opacity = '0';
		document.body.appendChild(textarea);
		textarea.select();
		document.execCommand('copy');
		document.body.removeChild(textarea);
		return true;
	} catch {
		return false;
	}
}
