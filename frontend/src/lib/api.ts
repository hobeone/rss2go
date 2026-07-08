let onErrorCallback: ((msg: string) => void) | null = null;

export function registerOnError(callback: (msg: string) => void) {
  onErrorCallback = callback;
}

// Fetch Wrapper
async function apiFetch(path: string, options: RequestInit = {}) {
  try {
    const resp = await fetch(path, {
      cache: 'no-store',
      ...options
    });
    if (!resp.ok) {
      const text = await resp.text();
      throw new Error(text || `HTTP error ${resp.status}`);
    }
    if (resp.headers.get('content-type')?.includes('application/json')) {
      return await resp.json();
    }
    return resp;
  } catch (err: any) {
    console.error(`API Fetch Error [${path}]:`, err);
    if (onErrorCallback) {
      onErrorCallback(err.message || 'Unknown network error');
    }
    throw err;
  }
}

export async function fetchStats(): Promise<any> {
  return await apiFetch('/api/v1/stats');
}

export async function fetchFeeds(): Promise<any[]> {
  return (await apiFetch('/api/v1/feeds')) || [];
}

export async function fetchUsers(): Promise<any[]> {
  return (await apiFetch('/api/v1/users')) || [];
}

export async function fetchOutbox(): Promise<any[]> {
  return (await apiFetch('/api/v1/outbox')) || [];
}

export async function fetchFeedItems(feedId: number): Promise<any[]> {
  return (await apiFetch(`/api/v1/feeds/${feedId}/items`)) || [];
}

export async function addFeed(feed: any): Promise<any> {
  return await apiFetch('/api/v1/feeds', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(feed)
  });
}

export async function updateFeed(feedId: number, feed: any): Promise<any> {
  return await apiFetch(`/api/v1/feeds/${feedId}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(feed)
  });
}

export async function deleteFeed(feedId: number): Promise<any> {
  return await apiFetch(`/api/v1/feeds/${feedId}`, { method: 'DELETE' });
}

export async function testFeed(feed: any): Promise<any> {
  return await apiFetch(`/api/v1/feeds/${feed.id}/test`, { method: 'POST' });
}

export async function catchupFeed(feedId: number): Promise<any> {
  return await apiFetch(`/api/v1/feeds/${feedId}/catchup`, { method: 'POST' });
}

export async function rewindFeed(feedId: number, body: any): Promise<any> {
  return await apiFetch(`/api/v1/feeds/${feedId}/rewind`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body)
  });
}

export async function scanFeed(feedId: number): Promise<any> {
  return await apiFetch(`/api/v1/feeds/${feedId}/scan`, { method: 'POST' });
}

export async function addUser(email: string): Promise<any> {
  return await apiFetch('/api/v1/users', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email })
  });
}

export async function deleteUser(id: number): Promise<any> {
  return await apiFetch(`/api/v1/users/${id}`, { method: 'DELETE' });
}

export async function addSubscription(userId: number, feedId: number): Promise<any> {
  return await apiFetch('/api/v1/subscriptions', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ user_id: userId, feed_id: feedId })
  });
}

export async function deleteSubscription(userId: number, feedId: number): Promise<any> {
  return await apiFetch('/api/v1/subscriptions', {
    method: 'DELETE',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ user_id: userId, feed_id: feedId })
  });
}
