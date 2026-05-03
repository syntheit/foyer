const BASE = '';

type FetchOptions = {
	method?: string;
	body?: unknown;
	headers?: Record<string, string>;
};

class ApiError extends Error {
	constructor(
		public status: number,
		message: string
	) {
		super(message);
		this.name = 'ApiError';
	}
}

async function request<T>(path: string, opts: FetchOptions = {}): Promise<T> {
	const headers: Record<string, string> = {
		...opts.headers
	};

	const init: RequestInit = {
		method: opts.method || 'GET',
		headers,
		credentials: 'include'
	};

	if (opts.body !== undefined) {
		headers['Content-Type'] = 'application/json';
		init.body = JSON.stringify(opts.body);
	}

	const res = await fetch(`${BASE}${path}`, init);

	if (res.status === 401) {
		throw new ApiError(401, 'Unauthorized');
	}

	if (!res.ok) {
		const text = await res.text();
		throw new ApiError(res.status, text);
	}

	if (res.status === 204) {
		return undefined as T;
	}

	return res.json();
}

export const api = {
	get: <T>(path: string) => request<T>(path),
	post: <T>(path: string, body?: unknown) => request<T>(path, { method: 'POST', body }),
	put: <T>(path: string, body?: unknown) => request<T>(path, { method: 'PUT', body }),
	patch: <T>(path: string, body?: unknown) => request<T>(path, { method: 'PATCH', body }),
	del: <T>(path: string) => request<T>(path, { method: 'DELETE' })
};

export { ApiError };
