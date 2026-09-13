<script lang="ts">
  import { onMount } from 'svelte';
  import { Resource, request, can, label, name, quantity, time, short } from '$lib/v2/client.svelte';
  import type { Row } from '$lib/v2/client.svelte';

  type Tab = 'inventory' | 'output-lots' | 'ledger' | 'reconciliation' | 'trace' | 'audit';
  let tab = $state<Tab>('inventory');
  let error = $state('');
  let inventoryResource = new Resource<Row[]>();
  let lotsResource = new Resource<Row[]>();
  let ledgerResource = new Resource<Row[]>();
  let reconResource = new Resource<Row>();
  let traceResource = new Resource<Row>();
  let auditResource = new Resource<Row[]>();
  let invFilter = $state({ item: '', location: '', order: '' });
  let lotFilterOrder = $state('');
  let ledgerFilter = $state({ order: '', lot: '' });
  let traceLotId = $state('');

  onMount(() => { void loadTab(); });
  function switchTab(next: Tab) { tab = next; error = ''; void loadTab(); }
  async function loadTab() {
    error = '';
    if (tab === 'inventory') await inventoryResource.load(() => request<Row[]>('/inventory'));
    if (tab === 'output-lots') await lotsResource.load(() => request<Row[]>('/output-lots'));
    if (tab === 'ledger') await ledgerResource.load(() => request<Row[]>('/ledger'));
    if (tab === 'reconciliation') await reconResource.load(() => request<Row>('/reconciliation'));
    if (tab === 'audit') {
      if (!can('admin')) { error = '감사 조회는 관리자 전용입니다.'; return; }
      await auditResource.load(() => request<Row[]>('/audit'));
    }
    error = [inventoryResource.error, lotsResource.error, ledgerResource.error, reconResource.error, auditResource.error].filter(Boolean).join(' · ');
  }
  function inventoryRows() {
    return (inventoryResource.value ?? []).filter(row =>
      (!invFilter.item || String(row.item_code ?? '').includes(invFilter.item) || String(row.item_name ?? '').includes(invFilter.item)) &&
      (!invFilter.location || String(row.location_code ?? '').includes(invFilter.location)) &&
      (!invFilter.order || String(row.order_id ?? '') === invFilter.order)
    );
  }
  function lotRows() { return (lotsResource.value ?? []).filter(row => !lotFilterOrder || String(row.order_id ?? '') === lotFilterOrder); }
  function ledgerRows() { return (ledgerResource.value ?? []).filter(row => (!ledgerFilter.order || String(row.order_id ?? '') === ledgerFilter.order) && (!ledgerFilter.lot || String(row.lot_id ?? '') === ledgerFilter.lot)); }
  async function lookupTrace() {
    error = '';
    if (!traceLotId) return;
    await traceResource.load(() => request<Row>(`/trace?lot_id=${encodeURIComponent(traceLotId)}`));
    error = traceResource.error;
  }
  function rows(value: unknown): Row[] { return Array.isArray(value) ? value as Row[] : []; }
</script>

<svelte:head><title>공장 재고 · 조립 제조 ERP</title></svelte:head>
<div class="heading"><div><p class="eyebrow">INVENTORY</p><h1>공장 재고</h1><p>재고 현황, 산출 LOT, 원장 조정, LOT 추적, 감사 로그를 조회합니다.</p></div></div>
{#if error}<p class="notice notice-error" role="alert">{error}</p>{/if}
<nav class="tabs" aria-label="재고 탭">{#each [{ key: 'inventory', label: '재고' }, { key: 'output-lots', label: '산출 LOT' }, { key: 'ledger', label: '원장' }, { key: 'reconciliation', label: '조정' }, { key: 'trace', label: '추적' }, { key: 'audit', label: '감사' }] as item}<button class:selected={tab === item.key} onclick={() => switchTab(item.key as Tab)}>{item.label}</button>{/each}</nav>

{#if tab === 'inventory'}<section class="card"><h2>재고 조회</h2><div class="filter"><div class="field"><label for="v2-inv-item">품목</label><input id="v2-inv-item" bind:value={invFilter.item} placeholder="code 또는 이름" /></div><div class="field"><label for="v2-inv-location">창고/위치</label><input id="v2-inv-location" bind:value={invFilter.location} placeholder="code" /></div><div class="field"><label for="v2-inv-order">지시 ID</label><input id="v2-inv-order" bind:value={invFilter.order} /></div><div class="actions"><button class="button" disabled={inventoryResource.loading} onclick={() => void loadTab()}>서버 갱신</button></div></div>{#if inventoryResource.loading}<p>로딩 중…</p>{:else if inventoryRows().length === 0}<p class="empty">조건에 맞는 재고가 없습니다.</p>{:else}<div class="table-scroll"><table><thead><tr><th>품목</th><th>LOT</th><th>위치</th><th>지시</th><th>수량</th></tr></thead><tbody>{#each inventoryRows() as row}<tr><td>{row.item_code ?? name(undefined, row.item_id)}{row.item_name ? ` · ${row.item_name}` : ''}</td><td>{row.lot_code ?? short(row.lot_id)}</td><td>{row.location_code ?? short(row.location_id)} · {label(row.location_kind)}</td><td>{short(row.order_id)}</td><td class="number">{quantity(row.quantity)}</td></tr>{/each}</tbody></table></div>{/if}</section>{/if}

{#if tab === 'output-lots'}<section class="card"><h2>산출 LOT</h2><div class="filter"><div class="field"><label for="v2-lot-order">지시 ID</label><input id="v2-lot-order" bind:value={lotFilterOrder} /></div><div class="actions"><button class="button" disabled={lotsResource.loading} onclick={() => void loadTab()}>서버 갱신</button></div></div><p class="subtext">현재 서버 목록 API는 지시 필터 파라미터를 처리하지 않으므로 조회 결과를 화면에서 필터링합니다.</p>{#if lotsResource.loading}<p>로딩 중…</p>{:else if lotRows().length === 0}<p class="empty">조건에 맞는 산출 LOT가 없습니다.</p>{:else}<div class="table-scroll"><table><thead><tr><th>LOT</th><th>지시</th><th>생산</th><th>대기</th><th>합격</th><th>부적합</th><th>재작업</th><th>폐기</th><th>입고</th></tr></thead><tbody>{#each lotRows() as row}<tr><td>{row.code ?? short(row.id)}</td><td>{short(row.order_id)}</td><td>{quantity(row.new_output_quantity)}</td><td>{quantity(row.pending_quantity)}</td><td>{quantity(row.accepted_quantity)}</td><td>{quantity(row.rejected_quantity)}</td><td>{quantity(row.rework_quantity)}</td><td>{quantity(row.disposed_quantity)}</td><td>{quantity(row.received_quantity)}</td></tr>{/each}</tbody></table></div>{/if}</section>{/if}

{#if tab === 'ledger'}<section class="card"><h2>자재 원장</h2><div class="filter"><div class="field"><label for="v2-led-order">지시 ID</label><input id="v2-led-order" bind:value={ledgerFilter.order} /></div><div class="field"><label for="v2-led-lot">LOT ID</label><input id="v2-led-lot" bind:value={ledgerFilter.lot} /></div><div class="actions"><button class="button" disabled={ledgerResource.loading} onclick={() => void loadTab()}>서버 갱신</button></div></div><p class="subtext">현재 서버 원장 API는 이 필터를 처리하지 않으므로 최대 1,000건의 서버 결과 안에서 화면 필터를 적용합니다.</p>{#if ledgerResource.loading}<p>로딩 중…</p>{:else if ledgerRows().length === 0}<p class="empty">조건에 맞는 원장 기록이 없습니다.</p>{:else}<div class="table-scroll"><table><thead><tr><th>#</th><th>문서</th><th>LOT</th><th>품목</th><th>위치</th><th>지시</th><th>수량</th><th>구분</th><th>일시</th></tr></thead><tbody>{#each ledgerRows() as row}<tr><td>{row.id ?? '—'}</td><td>{short(row.document_id)}</td><td>{short(row.lot_id)}</td><td>{name(undefined, row.item_id)}</td><td>{short(row.location_id)}</td><td>{short(row.order_id)}</td><td class="number">{quantity(row.quantity)}</td><td>{label(row.movement)}</td><td>{time(row.recorded_at)}</td></tr>{/each}</tbody></table></div>{/if}</section>{/if}

{#if tab === 'reconciliation'}<section class="card"><div class="heading"><div><h2>원장 조정</h2><p>저장된 잔액과 append-only 원장 합계를 비교합니다.</p></div><button class="button" disabled={reconResource.loading} onclick={() => void loadTab()}>갱신</button></div>{#if reconResource.loading}<p>로딩 중…</p>{:else if reconResource.value}{@const value = reconResource.value}<div class="notice notice-info"><strong>{value.balanced ? '균형' : '불균형'}</strong></div>{#each [['inventory_mismatches','재고 불일치'],['output_mismatches','산출 불일치'],['lot_mismatches','LOT 불일치']] as entry}{@const mismatchRows = rows(value[entry[0]])}{#if mismatchRows.length}<h3>{entry[1]}</h3><div class="table-scroll"><table><thead><tr><th>식별자</th><th>저장값</th><th>원장/상태값</th></tr></thead><tbody>{#each mismatchRows as row}<tr><td>{row.key ?? short(row.lot_id ?? row.source_document_id)}</td><td>{quantity(row.balance ?? row.new_output_quantity)}</td><td>{quantity(row.ledger_quantity ?? row.state_quantity)}</td></tr>{/each}</tbody></table></div>{/if}{/each}{/if}</section>{/if}

{#if tab === 'trace'}<section class="card"><h2>LOT 추적</h2><div class="filter"><div class="field"><label for="v2-trace-lot">LOT ID</label><input id="v2-trace-lot" bind:value={traceLotId} placeholder="UUID" /></div><div class="actions"><button class="button" disabled={traceResource.loading || !traceLotId} onclick={() => void lookupTrace()}>조회</button></div></div>{#if traceResource.loading}<p>로딩 중…</p>{:else if traceResource.value}{@const trace = traceResource.value}{#each [['documents','문서'],['ledger','원장'],['output_lots','산출 LOT'],['audit','감사']] as entry}<h3>{entry[1]}</h3>{#if rows(trace[entry[0]]).length}<div class="record-list">{#each rows(trace[entry[0]]) as row}<div><strong>{label(row.kind ?? row.movement ?? row.operation ?? row.bucket)}</strong><span class="subtext">{short(row.id)}</span></div><div>{row.quantity != null ? quantity(row.quantity) : row.reason ?? row.status ?? '—'}</div>{/each}</div>{:else}<p class="subtext">기록 없음</p>{/if}{/each}{:else}<p>LOT ID를 입력해 조회하세요.</p>{/if}</section>{/if}

{#if tab === 'audit'}{#if !can('admin')}<section class="card"><p class="notice notice-info">관리자 전용입니다.</p></section>{:else}<section class="card"><div class="heading"><div><h2>감사 로그</h2></div><button class="button" disabled={auditResource.loading} onclick={() => void loadTab()}>갱신</button></div>{#if auditResource.loading}<p>로딩 중…</p>{:else if auditResource.value}<div class="table-scroll"><table><thead><tr><th>#</th><th>주체</th><th>역할</th><th>작업</th><th>개체</th><th>사유</th><th>일시</th></tr></thead><tbody>{#each auditResource.value as row}<tr><td>{row.id ?? '—'}</td><td>{short(row.actor_id)}</td><td>{row.actor_role ?? '—'}</td><td>{row.operation}</td><td>{short(row.entity_id)}</td><td>{row.reason ?? '—'}</td><td>{time(row.recorded_at)}</td></tr>{/each}</tbody></table></div>{/if}</section>{/if}{/if}
