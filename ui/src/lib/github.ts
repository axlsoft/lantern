import { api } from './api-client.js';

/**
 * Fetch source code for a file from the GitHub API via the collector proxy.
 * The proxy enforces tenant scoping and GitHub App credential isolation.
 * Source is NOT cached — fetch fresh each visit.
 */
export async function fetchSource(
	githubRepoFullName: string,
	filePath: string,
	commitSha: string
): Promise<string> {
	const [owner, repo] = githubRepoFullName.split('/');
	if (!owner || !repo) throw new Error(`Invalid github_repo_full_name: ${githubRepoFullName}`);
	return api.getFileSource(owner, repo, filePath, commitSha);
}

/** Detect Shiki language id from file extension. */
export function detectLanguage(filePath: string): string {
	const ext = filePath.split('.').pop()?.toLowerCase() ?? '';
	const map: Record<string, string> = {
		ts: 'typescript',
		tsx: 'tsx',
		js: 'javascript',
		jsx: 'jsx',
		mjs: 'javascript',
		cjs: 'javascript',
		cs: 'csharp',
		go: 'go',
		py: 'python',
		rb: 'ruby',
		java: 'java',
		kt: 'kotlin',
		swift: 'swift',
		rs: 'rust',
		cpp: 'cpp',
		c: 'c',
		h: 'c',
		hpp: 'cpp',
		php: 'php',
		vue: 'vue',
		svelte: 'svelte',
		html: 'html',
		css: 'css',
		scss: 'scss',
		json: 'json',
		yaml: 'yaml',
		yml: 'yaml',
		toml: 'toml',
		md: 'markdown',
		sh: 'bash',
		bash: 'bash',
		zsh: 'bash',
		sql: 'sql',
		xml: 'xml'
	};
	return map[ext] ?? 'text';
}
