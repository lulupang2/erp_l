import { describe, expect, it } from 'vitest';
import { allowsWork, readyToClose, sourceCandidates, sourceRemaining, workHref } from './workflow';

describe('factory work navigation', () => {
  it('preserves the order, source and active work in links', () => {
    const url = new URL(workHref('documents', 'order/1', 'production', 'source&1', 'session1'), 'https://example.test');
    expect(Object.fromEntries(url.searchParams)).toEqual({order:'order/1',kind:'production',source:'source&1',session:'session1'});
  });
  it('keeps held orders available for inspection and return, but blocks production and receipt', () => {
    const order = {status:'held'};
    expect(allowsWork(order,'inspection')).toBe(true);
    expect(allowsWork(order,'return')).toBe(true);
    expect(allowsWork(order,'production')).toBe(false);
    expect(allowsWork(order,'goods_receipt')).toBe(false);
    expect(allowsWork({status:'issued'},'production')).toBe(false);
  });
  it('uses authoritative remaining balances even when history is incomplete', () => {
    const source = {id:'old',kind:'production',status:'posted',quantity:100,available_pending_quantity:3};
    expect(sourceRemaining(source,'inspection',[])).toBe(3);
    expect(sourceRemaining({...source,available_pending_quantity:undefined},'inspection',[])).toBe(0);
  });
  it('excludes exhausted, reversed and other-order records', () => {
    const base = {kind:'inspection',status:'posted',order_id:'one',available_accepted_quantity:3};
    const rows = [{...base,id:'ok'},{...base,id:'other',order_id:'two'},{...base,id:'reversed',status:'reversed'},{...base,id:'empty',available_accepted_quantity:0}];
    expect(sourceCandidates(rows,'goods_receipt','one').map(row=>row.id)).toEqual(['ok']);
  });
  it('separates rejected stock and rework stock', () => {
    expect(sourceRemaining({kind:'inspection',status:'posted',available_rejected_quantity:2},'disposition')).toBe(2);
    expect(sourceRemaining({kind:'disposition',status:'posted',disposition:'disposal',available_rework_quantity:2},'rework')).toBe(0);
  });
  it('does not mark incomplete or unknown cleanup states as ready to close', () => {
    const order = {status:'in_progress',remaining_quantity:0,pending_quantity:0,accepted_quantity:0,rejected_quantity:0,rework_quantity:0,floor_quantity:0,active_sessions:0};
    expect(readyToClose(order)).toBe(true);
    expect(readyToClose({...order,floor_quantity:1})).toBe(false);
    expect(readyToClose({...order,active_sessions:undefined})).toBe(false);
    expect(readyToClose({...order,status:'held'})).toBe(false);
  });
});
