import { describe, expect, it, vi } from 'vitest';
import { ApiClient, ApiError, IdempotentOperation, type PendingStorage } from './api';

const keyA = '11111111-1111-4111-8111-111111111111';
const keyB = '22222222-2222-4222-8222-222222222222';
const resourceID = '33333333-3333-4333-8333-333333333333';
const response = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
function storage(): PendingStorage {
  const values = new Map<string, string>();
  return { getItem: key => values.get(key) ?? null, setItem: (key, value) => { values.set(key, value); }, removeItem: key => { values.delete(key); } };
}

describe('idempotent writes', () => {
  it('retains the exact body/key on response loss and creates a new key only after success', async () => {
    const fetcher = vi.fn<typeof fetch>().mockRejectedValueOnce(new TypeError('network'))
      .mockResolvedValueOnce(response({ data: { id: resourceID } }))
      .mockResolvedValueOnce(response({ data: { id: resourceID } }, 201));
    const ids = vi.fn().mockReturnValueOnce(keyA).mockReturnValueOnce(keyB);
    const operation = new IdempotentOperation('/stock-receipts', new ApiClient(fetcher), storage(), ids);
    await expect(operation.submit({ quantity: 7 })).rejects.toMatchObject({ code: 'NETWORK_ERROR' });
    expect(operation.pending?.key).toBe(keyA);
    await operation.submit({ quantity: 900 });
    expect(fetcher.mock.calls[1][1]?.body).toBe(JSON.stringify({ quantity: 7 }));
    expect(fetcher.mock.calls[1][1]?.headers).toMatchObject({ 'Idempotency-Key': keyA });
    expect(operation.pending).toBeNull();
    await operation.submit({ quantity: 8 });
    expect(fetcher.mock.calls[2][1]?.headers).toMatchObject({ 'Idempotency-Key': keyB });
    expect(fetcher.mock.calls[2][1]?.body).toBe(JSON.stringify({ quantity: 8 }));
  });

  it('restores pending keys after a reload and keeps route targets isolated', async () => {
    const persisted = storage();
    const fetcher = vi.fn<typeof fetch>().mockRejectedValueOnce(new TypeError('lost'))
      .mockResolvedValueOnce(response({ data: { id: resourceID } }));
    const client = new ApiClient(fetcher);
    const path = `/production-orders/${keyA}/results`;
    const first = new IdempotentOperation(path, client, persisted, () => keyB);
    await expect(first.submit({ good_quantity: 1, defective_quantity: 0 })).rejects.toBeInstanceOf(ApiError);
    const restored = new IdempotentOperation(path, client, persisted, () => resourceID);
    expect(restored.pending?.key).toBe(keyB);
    expect(restored.draft).toEqual({ good_quantity: 1, defective_quantity: 0 });
    const other = new IdempotentOperation(`/production-orders/${resourceID}/results`, client, persisted);
    expect(other.pending).toBeNull();
    await restored.submit({ good_quantity: 99, defective_quantity: 1 });
    expect(fetcher.mock.calls[1][1]?.body).toBe(JSON.stringify({ good_quantity: 1, defective_quantity: 0 }));
    expect(restored.pending).toBeNull();
  });

  it.each([null, {}, { id: 'invalid' }, [], { id: 123 }])('does not clear a key for malformed successful data %j', async data => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(response({ data }));
    const operation = new IdempotentOperation('/stock-receipts', new ApiClient(fetcher), storage(), () => keyA);
    await expect(operation.submit({ quantity: 1 })).rejects.toMatchObject({ code: 'INVALID_RESPONSE' });
    expect(operation.pending?.key).toBe(keyA);
  });

  it.each([500, 503])('retains a key for HTTP %d', async code => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(response({ error: { code: 'FAILED', message: 'Try again' } }, code));
    const operation = new IdempotentOperation('/stock-receipts', new ApiClient(fetcher), storage(), () => keyA);
    await expect(operation.submit({ quantity: 1 })).rejects.toMatchObject({ status: code, definitive: false });
    expect(operation.pending?.key).toBe(keyA);
  });

  it.each([400, 404, 409])('clears a key after a structured, definitive HTTP %d outcome', async code => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(response({ error: { code: 'REJECTED', message: 'Not committed' } }, code));
    const operation = new IdempotentOperation('/stock-receipts', new ApiClient(fetcher), storage(), () => keyA);
    await expect(operation.submit({ quantity: 1 })).rejects.toMatchObject({ status: code, definitive: true });
    expect(operation.pending).toBeNull();
  });

  it('treats an invalid JSON error response as uncertain', async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(new Response('<html>proxy error</html>', { status: 400 }));
    const operation = new IdempotentOperation('/stock-receipts', new ApiClient(fetcher), storage(), () => keyA);
    await expect(operation.submit({ quantity: 1 })).rejects.toMatchObject({ code: 'INVALID_RESPONSE', definitive: false });
    expect(operation.pending?.key).toBe(keyA);
  });

  it('prevents simultaneous local submissions and resetting an in-flight request', async () => {
    let release!: (response: Response) => void;
    const fetcher = vi.fn<typeof fetch>().mockImplementation(() => new Promise(resolve => { release = resolve; }));
    const operation = new IdempotentOperation('/stock-receipts', new ApiClient(fetcher), storage(), () => keyA);
    const first = operation.submit({ quantity: 1 });
    await expect(operation.submit({ quantity: 2 })).rejects.toThrow('이미');
    expect(() => operation.reset()).toThrow('처리 중');
    expect(fetcher).toHaveBeenCalledTimes(1);
    release(response({ data: { id: resourceID } }, 201));
    await first;
    expect(operation.pending).toBeNull();
  });
});

describe('complete paginated selectors', () => {
  it('loads beyond the first 100 rows', async () => {
    const first = Array.from({ length: 100 }, (_, id) => ({ id }));
    const fetcher = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(response({ data: first, pagination: { page: 1, page_size: 100, total: 101 } }))
      .mockResolvedValueOnce(response({ data: [{ id: 100 }], pagination: { page: 2, page_size: 100, total: 101 } }));
    const rows = await new ApiClient(fetcher).all('/items');
    expect(rows).toHaveLength(101);
    expect(String(fetcher.mock.calls[1][0])).toContain('page=2');
  });

  it('rejects silent truncation', async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(response({ data: [{ id: 1 }], pagination: { page: 1, page_size: 100, total: 2 } }));
    await expect(new ApiClient(fetcher).all('/items')).rejects.toMatchObject({ code: 'INCOMPLETE_LIST' });
  });

  it('rejects changing totals instead of presenting an incomplete list', async () => {
    const fetcher = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(response({ data: Array.from({ length: 100 }, (_, id) => ({ id })), pagination: { page: 1, page_size: 100, total: 101 } }))
      .mockResolvedValueOnce(response({ data: [{ id: 100 }, { id: 101 }], pagination: { page: 2, page_size: 100, total: 102 } }));
    await expect(new ApiClient(fetcher).all('/items')).rejects.toMatchObject({ code: 'INCOMPLETE_LIST' });
  });
});
