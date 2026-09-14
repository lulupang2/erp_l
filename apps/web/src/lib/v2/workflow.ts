import type { Row } from './client.svelte';

export type WorkKind = 'issue' | 'return' | 'production' | 'inspection' | 'disposition' | 'rework' | 'goods_receipt';
export function workHref(path: 'orders' | 'sessions' | 'documents' | 'work', orderId?: unknown, kind?: string, sourceId?: unknown, sessionId?: unknown) {
  const query = new URLSearchParams();
  if (orderId) query.set('order', String(orderId));
  if (kind) query.set('kind', kind);
  if (sourceId) query.set('source', String(sourceId));
  if (sessionId) query.set('session', String(sessionId));
  return `/v2/${path}${query.size ? `?${query}` : ''}`;
}
export function allowsWork(order: Row, kind: WorkKind) {
  const active = ['issued', 'in_progress'].includes(String(order.status));
  if (['return', 'inspection', 'disposition'].includes(kind)) return active || order.status === 'held';
  if (['production', 'rework'].includes(kind)) return order.status === 'in_progress';
  return active;
}
// The list is a snapshot. Posting still checks the authoritative output balance.
export function sourceRemaining(source: Row, kind: WorkKind, _documents: Row[] = []): number {
  if (source.status !== 'posted') return 0;
  const balances: Partial<Record<WorkKind, string>> = { inspection: 'available_pending_quantity', goods_receipt: 'available_accepted_quantity', disposition: 'available_rejected_quantity', rework: 'available_rework_quantity' };
  const balance = balances[kind];
  const validSource = kind === 'inspection' ? ['production', 'rework'].includes(String(source.kind)) : kind === 'rework' ? source.kind === 'disposition' && source.disposition === 'rework' : ['goods_receipt', 'disposition'].includes(kind) && source.kind === 'inspection';
  if (!balance || !validSource || source[balance] == null) return 0;
  const remaining = Number(source[balance]);
  return Number.isFinite(remaining) ? Math.max(0, remaining) : 0;
}
export function sourceCandidates(documents: Row[], kind: WorkKind, orderId?: string) {
  return documents.filter(row => (!orderId || String(row.order_id) === orderId) && sourceRemaining(row, kind, documents) > 0);
}
export function readyToClose(order: Row) {
  return ['issued', 'in_progress'].includes(String(order.status)) &&
    ['remaining_quantity', 'pending_quantity', 'accepted_quantity', 'rejected_quantity', 'rework_quantity', 'floor_quantity', 'active_sessions'].every(key => order[key] != null && Number(order[key]) === 0);
}
