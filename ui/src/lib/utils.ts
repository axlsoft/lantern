import { type ClassValue, clsx } from 'clsx';
import { twMerge } from 'tailwind-merge';

export function cn(...inputs: ClassValue[]) {
	return twMerge(clsx(inputs));
}

export function formatDate(iso: string): string {
	return new Intl.DateTimeFormat('en-US', {
		month: 'short',
		day: 'numeric',
		year: 'numeric',
		hour: '2-digit',
		minute: '2-digit'
	}).format(new Date(iso));
}

export function formatDuration(ms: number): string {
	if (ms < 1000) return `${ms}ms`;
	if (ms < 60_000) return `${(ms / 1000).toFixed(1)}s`;
	const m = Math.floor(ms / 60_000);
	const s = Math.floor((ms % 60_000) / 1000);
	return `${m}m ${s}s`;
}

export function pct(covered: number, total: number): string {
	if (total === 0) return '—';
	return `${Math.round((covered / total) * 100)}%`;
}

/** Validate that a redirect target is relative (starts with / but not // or a scheme). */
export function isSafeRedirect(url: string): boolean {
	return /^\/(?!\/)/.test(url);
}
