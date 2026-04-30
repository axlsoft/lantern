/**
 * Typed API client for the Lantern collector Management API.
 *
 * Wraps fetch with base URL injection, auth cookie forwarding, and typed
 * request/response shapes.  When an OpenAPI spec is added to the collector,
 * run `pnpm generate:api` to regenerate the schema types and update these
 * types accordingly.
 */

// ── Shared types ─────────────────────────────────────────────────────────────

export interface ApiError {
	error: { code: string; message: string };
	request_id?: string;
}

export type OrgRole = 'owner' | 'admin' | 'member' | 'viewer';
export type TeamRole = 'admin' | 'member';
export type RunStatus = 'in_progress' | 'completed' | 'failed' | 'aborted';
export type TestStatus = 'passed' | 'failed' | 'skipped' | 'timed_out';

export interface User {
	id: string;
	email: string;
	display_name?: string;
	created_at: string;
	email_verified_at?: string | null;
	organizations?: Array<{ id: string; name: string; role?: string }>;
}

export interface Organization {
	id: string;
	name: string;
	slug: string;
	created_at: string;
}

export interface Team {
	id: string;
	organization_id: string;
	name: string;
	slug: string;
	created_at: string;
}

export interface Project {
	id: string;
	organization_id: string;
	team_id: string | null;
	name: string;
	slug: string;
	default_branch: string;
	github_repo_full_name: string | null;
	created_at: string;
	updated_at: string;
}

export interface ApiKey {
	id: string;
	project_id: string;
	name: string;
	key_prefix: string;
	created_at: string;
	last_used_at: string | null;
	revoked_at: string | null;
}

export interface ApiKeyCreated extends ApiKey {
	key: string;
}

export interface Run {
	id: string;
	project_id: string;
	commit_sha: string;
	branch: string | null;
	ci_run_id: string | null;
	github_pr_number: number | null;
	status: RunStatus;
	started_at: string;
	completed_at: string | null;
	total_tests: number;
	passed_tests: number;
	failed_tests: number;
	skipped_tests: number;
}

export interface Test {
	id: string;
	run_id: string;
	name: string;
	suite: string;
	file_path: string;
	status: TestStatus;
	started_at: string;
	completed_at: string | null;
	duration_ms: number | null;
	covered_lines: number;
}

export interface DashboardData {
	project: Project;
	coverage_summary: {
		total_lines: number;
		covered_lines: number;
		uncovered_lines: number;
		gap_count: number;
		last_run_at: string | null;
	};
	recent_runs: Run[];
	top_gaps: GapEntry[];
	open_prs: PrSummary[];
}

export interface GapEntry {
	file_path: string;
	function_name: string;
	line_start: number;
	line_end: number;
	uncovered_lines: number;
	risk_score: number;
}

export interface GapsPage {
	gaps: GapEntry[];
	next_cursor: string | null;
	total: number;
}

export interface FileCoverage {
	file_path: string;
	commit_sha: string;
	lines: LineCoverage[];
}

export interface LineCoverage {
	line: number;
	hit_count: number;
	test_ids: string[];
}

export interface LineTests {
	tests: Array<{ id: string; name: string; suite: string }>;
}

export interface PrSummary {
	pr_number: number;
	title: string;
	head_sha: string;
	covered_delta: number;
	uncovered_delta: number;
	run_id: string | null;
}

export interface PrDetail {
	pr_number: number;
	head_sha: string;
	run_id: string | null;
	pending: boolean;
	changed_files: Array<{
		file_path: string;
		covered_lines: number;
		uncovered_lines: number;
		added_lines: number;
	}>;
}

export interface TestsPage {
	tests: Test[];
	next_cursor: string | null;
}

// ── Client ────────────────────────────────────────────────────────────────────

class ApiClient {
	private base: string;

	constructor(base = '') {
		this.base = base;
	}

	private async request<T>(path: string, init?: RequestInit): Promise<T> {
		console.log('[api] →', init?.method ?? 'GET', path);
		const res = await fetch(`${this.base}${path}`, {
			credentials: 'same-origin',
			headers: { 'Content-Type': 'application/json', ...init?.headers },
			...init
		});
		console.log('[api] ←', res.status, init?.method ?? 'GET', path);

		if (!res.ok) {
			const body = (await res.json().catch(() => ({}))) as Partial<ApiError>;
			const message = body.error?.message || res.statusText;
			const err = new Error(message) as Error & {
				status: number;
				code?: string;
				requestId?: string;
			};
			err.status = res.status;
			err.code = body.error?.code;
			err.requestId = body.request_id;
			throw err;
		}

		// 204 No Content
		if (res.status === 204) return undefined as T;
		const payload = (await res.json()) as { data?: unknown } | unknown;
		// Server wraps successful responses in {"data": ...}; unwrap if present.
		if (
			payload &&
			typeof payload === 'object' &&
			'data' in (payload as Record<string, unknown>)
		) {
			return (payload as { data: T }).data;
		}
		return payload as T;
	}

	// ── Auth ────────────────────────────────────────────────────────────────

	signup(email: string, password: string, displayName: string) {
		return this.request<{ user: User }>('/api/v1/auth/signup', {
			method: 'POST',
			body: JSON.stringify({ email, password, display_name: displayName })
		});
	}

	login(email: string, password: string) {
		return this.request<{ user: User }>('/api/v1/auth/login', {
			method: 'POST',
			body: JSON.stringify({ email, password })
		});
	}

	logout() {
		return this.request<void>('/api/v1/auth/logout', { method: 'POST' });
	}

	me() {
		return this.request<User>('/api/v1/auth/me');
	}

	requestPasswordReset(email: string) {
		return this.request<void>('/api/v1/auth/password-reset/request', {
			method: 'POST',
			body: JSON.stringify({ email })
		});
	}

	completePasswordReset(token: string, password: string) {
		return this.request<void>('/api/v1/auth/password-reset/complete', {
			method: 'POST',
			body: JSON.stringify({ token, password })
		});
	}

	verifyEmail(token: string) {
		return this.request<void>(`/api/v1/auth/verify?token=${encodeURIComponent(token)}`);
	}

	// ── Organizations ───────────────────────────────────────────────────────

	createOrg(name: string) {
		return this.request<Organization>('/api/v1/organizations', {
			method: 'POST',
			body: JSON.stringify({ name })
		}).then((organization) => ({ organization }));
	}

	getOrg(orgId: string) {
		return this.request<Organization>(`/api/v1/organizations/${orgId}`).then((organization) => ({
			organization
		}));
	}

	updateOrg(orgId: string, name: string) {
		return this.request<Organization>(`/api/v1/organizations/${orgId}`, {
			method: 'PATCH',
			body: JSON.stringify({ name })
		}).then((organization) => ({ organization }));
	}

	inviteToOrg(orgId: string, email: string, role: OrgRole) {
		return this.request<void>(`/api/v1/organizations/${orgId}/invites`, {
			method: 'POST',
			body: JSON.stringify({ email, role })
		});
	}

	acceptInvite(token: string) {
		return this.request<void>(`/api/v1/invites/${token}/accept`, { method: 'POST' });
	}

	// ── Teams ───────────────────────────────────────────────────────────────

	createTeam(orgId: string, name: string) {
		return this.request<Team>(`/api/v1/organizations/${orgId}/teams`, {
			method: 'POST',
			body: JSON.stringify({ name })
		}).then((team) => ({ team }));
	}

	listTeams(orgId: string) {
		return this.request<Team[]>(`/api/v1/organizations/${orgId}/teams`).then((teams) => ({ teams }));
	}

	getTeam(teamId: string) {
		return this.request<Team>(`/api/v1/teams/${teamId}`).then((team) => ({ team }));
	}

	addTeamMember(teamId: string, userId: string, role: TeamRole) {
		return this.request<void>(`/api/v1/teams/${teamId}/members`, {
			method: 'POST',
			body: JSON.stringify({ user_id: userId, role })
		});
	}

	// ── Projects ────────────────────────────────────────────────────────────

	createProject(teamId: string, name: string, githubRepo?: string) {
		return this.request<Project>(`/api/v1/teams/${teamId}/projects`, {
			method: 'POST',
			body: JSON.stringify({ name, github_repo_full_name: githubRepo ?? null })
		}).then((project) => ({ project }));
	}

	getProject(projectId: string) {
		return this.request<Project>(`/api/v1/projects/${projectId}`).then((project) => ({ project }));
	}

	updateProject(
		projectId: string,
		patch: Partial<Pick<Project, 'name' | 'github_repo_full_name' | 'default_branch'>>
	) {
		return this.request<Project>(`/api/v1/projects/${projectId}`, {
			method: 'PATCH',
			body: JSON.stringify(patch)
		}).then((project) => ({ project }));
	}

	deleteProject(projectId: string) {
		return this.request<void>(`/api/v1/projects/${projectId}`, { method: 'DELETE' });
	}

	// ── API Keys ────────────────────────────────────────────────────────────

	createApiKey(projectId: string, name: string) {
		return this.request<ApiKeyCreated>(`/api/v1/projects/${projectId}/api-keys`, {
			method: 'POST',
			body: JSON.stringify({ name })
		}).then((api_key) => ({ api_key }));
	}

	listApiKeys(projectId: string) {
		return this.request<ApiKey[]>(`/api/v1/projects/${projectId}/api-keys`).then((api_keys) => ({
			api_keys
		}));
	}

	revokeApiKey(projectId: string, keyId: string) {
		return this.request<void>(`/api/v1/projects/${projectId}/api-keys/${keyId}`, {
			method: 'DELETE'
		});
	}

	rotateApiKey(projectId: string, keyId: string) {
		return this.request<ApiKeyCreated>(
			`/api/v1/projects/${projectId}/api-keys/${keyId}/rotate`,
			{ method: 'POST' }
		).then((api_key) => ({ api_key }));
	}

	// ── Dashboard ───────────────────────────────────────────────────────────

	getDashboard(projectId: string) {
		return this.request<DashboardData>(`/api/v1/projects/${projectId}/dashboard`);
	}

	// ── Runs ────────────────────────────────────────────────────────────────

	listRuns(projectId: string, cursor?: string) {
		const q = cursor ? `?cursor=${encodeURIComponent(cursor)}` : '';
		return this.request<{ runs: Run[]; next_cursor: string | null }>(
			`/api/v1/projects/${projectId}/runs${q}`
		);
	}

	getRun(projectId: string, runId: string) {
		return this.request<{ run: Run }>(`/api/v1/projects/${projectId}/runs/${runId}`);
	}

	listRunTests(projectId: string, runId: string, cursor?: string, status?: TestStatus) {
		const params = new URLSearchParams();
		if (cursor) params.set('cursor', cursor);
		if (status) params.set('status', status);
		const q = params.size ? `?${params}` : '';
		return this.request<TestsPage>(`/api/v1/projects/${projectId}/runs/${runId}/tests${q}`);
	}

	// ── Coverage / Gaps ─────────────────────────────────────────────────────

	getGaps(projectId: string, params?: { cursor?: string; path?: string; min_risk?: number }) {
		const q = new URLSearchParams();
		if (params?.cursor) q.set('cursor', params.cursor);
		if (params?.path) q.set('path', params.path.slice(0, 500));
		if (params?.min_risk != null) q.set('min_risk', String(params.min_risk));
		return this.request<GapsPage>(`/api/v1/projects/${projectId}/gaps${q.size ? `?${q}` : ''}`);
	}

	getFileCoverage(projectId: string, filePath: string, commitSha: string) {
		return this.request<FileCoverage>(
			`/api/v1/projects/${projectId}/files/${encodeURIComponent(filePath)}?commit=${encodeURIComponent(commitSha)}`
		);
	}

	getLineTests(projectId: string, filePath: string, line: number, commitSha: string) {
		return this.request<LineTests>(
			`/api/v1/projects/${projectId}/files/${encodeURIComponent(filePath)}/lines/${line}/tests?commit=${encodeURIComponent(commitSha)}`
		);
	}

	// ── PRs ─────────────────────────────────────────────────────────────────

	getPr(projectId: string, prNumber: number) {
		return this.request<{ pr: PrDetail }>(`/api/v1/projects/${projectId}/prs/${prNumber}`);
	}

	// ── GitHub proxy ────────────────────────────────────────────────────────

	getFileSource(owner: string, repo: string, path: string, ref: string): Promise<string> {
		return fetch(
			`/api/v1/github/repos/${owner}/${repo}/contents/${path}?ref=${encodeURIComponent(ref)}`,
			{ credentials: 'same-origin' }
		).then((r) => {
			if (!r.ok) throw new Error(`GitHub proxy ${r.status}`);
			return r.text();
		});
	}
}

export const api = new ApiClient();
