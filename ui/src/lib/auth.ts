import { goto } from '$app/navigation';
import type { User } from './api-client.js';
import { api } from './api-client.js';
import { isSafeRedirect } from './utils.js';

// ── Session state (Svelte 5 runes, module-level) ─────────────────────────────

let _user = $state<User | null>(null);
let _loading = $state(true);
let _initialized = false;

export const session = {
	get user() {
		return _user;
	},
	get loading() {
		return _loading;
	}
};

/** Call once on app init (root layout). */
export async function initSession() {
	if (_initialized) return;
	_initialized = true;
	try {
		const res = await api.me();
		_user = res.user;
	} catch {
		_user = null;
	} finally {
		_loading = false;
	}
}

export async function login(email: string, password: string, next?: string): Promise<void> {
	const res = await api.login(email, password);
	_user = res.user;
	const dest = next && isSafeRedirect(next) ? next : '/dashboard';
	await goto(dest);
}

export async function logout(): Promise<void> {
	await api.logout().catch(() => {});
	_user = null;
	await goto('/auth/login');
}

/** Guard: redirect to /auth/login if not authenticated. */
export function requireAuth(next?: string): void {
	if (_loading) return;
	if (!_user) {
		const dest = next ? `/auth/login?next=${encodeURIComponent(next)}` : '/auth/login';
		goto(dest);
	}
}
