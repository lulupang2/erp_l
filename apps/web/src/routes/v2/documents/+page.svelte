<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { beforeNavigate } from '$app/navigation';
  import { page } from '$app/state';
  import OrderFlow from '$lib/v2/OrderFlow.svelte';
  import { allowsWork, sourceCandidates, sourceRemaining, workHref, type WorkKind } from '$lib/v2/workflow';
  import { Resource, Command, request, workDocuments, can, documentRoles, kinds, label, name, quantity, time, short, session } from '$lib/v2/client.svelte';
  import type { Row, References } from '$lib/v2/client.svelte';
  import Feedback from '$lib/v2/Feedback.svelte';

  type DocKind = 'component_receipt' | 'issue' | 'return' | 'material_loss' | 'production' | 'inspection' | 'disposition' | 'rework' | 'goods_receipt';
  type Line = { lot_id: string; location_id: string; quantity: string };
  type InspectionResult = { item_code: string; result: '' | 'pass' | 'fail'; note: string };
  const allKinds = Object.keys(kinds) as DocKind[];

  let documents = $state<Row[]>([]);
  let orders = $state<Row[]>([]);
  let refs = $state<References | null>(null);
  let sessions = $state<Row[]>([]);
  let inventory = $state<Row[]>([]);
  let docResource = new Resource<Row[]>();
  let orderResource = new Resource<Row[]>();
  let refsResource = new Resource<References>();
  let sessionResource = new Resource<Row[]>();
  let inventoryResource = new Resource<Row[]>();
  let detailResource = new Resource<Row>();
  let command = new Command();
  beforeNavigate(({ cancel }) => { if (command.busy || command.locked) cancel(); });

  let selectedDoc = $state<Row | null>(null);
  let orderFilter = $state('');
  let kindFilter = $state<DocKind | ''>('');
  let kind = $state<DocKind>('component_receipt');
  let form = $state<Record<string, string>>({});
  let lines = $state<Line[]>([{ lot_id: '', location_id: '', quantity: '1' }]);
  let inspectionResults = $state<InspectionResult[]>([]);
  let tab = $state<'list' | 'form'>('list');
  let localError = $state('');

  let contextOrder = $derived(orders.find(row => String(row.id) === (form.order_id || orderFilter)));
  let completedOrder = $state('');
  let pendingOperation: 'create' | 'post' | 'reverse' = 'create';
  async function recovered(saved: Row) {
    if (pendingOperation === 'create') {
      orderFilter = String(saved.order_id ?? form.order_id ?? ''); tab = 'list';
      await loadAll(); await openDocument(saved);
    } else {
      completedOrder = String(selectedDoc?.order_id ?? saved.order_id ?? ''); selectedDoc = null;
      await loadAll();
    }
  }
  onMount(() => { void initialize(); });
  async function initialize() {
    await loadAll();
    if (localError) return;
    const id = page.url.searchParams.get('order') ?? '';
    if (id && !orders.some(row => String(row.id) === id)) { localError ||= '선택한 작업 지시를 찾을 수 없습니다. 업무 대기 목록에서 다시 선택하세요.'; return; }
    orderFilter = id;
    const requested = page.url.searchParams.get('kind') as DocKind | null;
    if (requested && availableKinds().includes(requested)) {
      resetForm(requested);
      if (contextOrder && !orderAllowed(contextOrder)) { tab = 'list'; localError = '현재 지시 상태에서는 이 작업을 진행할 수 없습니다. 지시 상태를 확인하세요.'; return; }
      tab = 'form';
      orderChanged();
      const sourceId = page.url.searchParams.get('source');
      if (sourceId && sourceOptions().some(row => String(row.id) === sourceId)) { form.source_document_id = sourceId; sourceChanged(); }
      else if (sourceId) localError = '선택한 기록은 처리 가능한 잔량이 없거나 이 지시에 속하지 않습니다. 대상 기록을 다시 선택하세요.';
      else if (sourceOptions().length === 1) { form.source_document_id = String(sourceOptions()[0].id); sourceChanged(); }
      const sessionId = page.url.searchParams.get('session');
      if (sessionId && ownSessions().some(row => String(row.id) === sessionId)) form.work_session_id = sessionId;
    }
  }
  function ownSessions() { return sessions.filter(row => row.status === 'active' && String(row.order_id) === form.order_id && row.user_id === session.user.id); }
  function orderChanged() {
    form.source_document_id = ''; form.work_session_id = ''; inspectionResults = [];
    lines = [{ lot_id: '', location_id: '', quantity: '1' }];
    const active = ownSessions(); if (active.length === 1) form.work_session_id = String(active[0].id);
  }

  async function loadAll() {
    localError = '';
    await Promise.all([
      docResource.load(workDocuments),
      orderResource.load(() => request<Row[]>('/orders')),
      inventoryResource.load(() => request<Row[]>('/inventory')),
      refsResource.load(() => request<References>('/reference')),
      can('admin', 'planner', 'operator') ? sessionResource.load(() => request<Row[]>('/work-sessions')) : Promise.resolve()
    ]);
    documents = docResource.value ?? [];
    orders = orderResource.value ?? [];
    inventory = inventoryResource.value ?? [];
    refs = refsResource.value;
    sessions = sessionResource.value ?? [];
    const errors = [docResource.error, orderResource.error, inventoryResource.error, refsResource.error, sessionResource.error].filter(Boolean);
    localError = errors.join(' · ');
  }

  function availableKinds() { return allKinds.filter(value => can(...documentRoles[value])); }
  function ensureKind() {
    const available = availableKinds();
    if (!available.includes(kind) && available.length) kind = available[0];
  }
  function resetForm(nextKind?: DocKind) {
    selectedDoc = null;
    form = { order_id: orderFilter };
    completedOrder = '';
    lines = [{ lot_id: '', location_id: '', quantity: '1' }];
    inspectionResults = [];
    if (nextKind) kind = nextKind;
    ensureKind();
    localError = '';
  }
  function chooseKind(next: DocKind) { const id = form.order_id; resetForm(next); form.order_id = id; orderChanged(); }
  function filteredDocuments() { return documents.filter(doc => (!kindFilter || doc.kind === kindFilter) && (!orderFilter || String(doc.order_id ?? '') === orderFilter)); }

  function orderAllowed(row: Row) {
    if (kind === 'component_receipt') return true;
    if (kind === 'material_loss') return ['issued', 'in_progress', 'held'].includes(String(row.status));
    return allowsWork(row, kind);
  }
  function activeOrders() { return orders.filter(orderAllowed); }
  function selectedSource() { return documents.find(doc => String(doc.id) === form.source_document_id); }
  function sourceOptions() {
    return sourceCandidates(documents, kind as WorkKind, form.order_id).filter(source => orders.some(order => order.id === source.order_id && orderAllowed(order)));
  }
  function sourceChanged() {
    const source = selectedSource();
    if (source?.order_id) form.order_id = String(source.order_id);
    if (kind === 'inspection') {
      const order = orders.find(row => String(row.id) === String(source?.order_id ?? ''));
      const revision = refs?.inspection_revisions.find(row => String(row.id) === String(order?.inspection_revision_id ?? ''));
      const items = Array.isArray(revision?.items) ? revision.items as Row[] : [];
      inspectionResults = items.map(item => ({ item_code: String(item.item_code ?? ''), result: '', note: '' }));
    }
  }

  function uniqueById(rows: Row[]) {
    const seen = new Set<string>();
    return rows.filter(row => { const id = String(row.id ?? ''); if (!id || seen.has(id)) return false; seen.add(id); return true; });
  }
  function lots(): Row[] {
    const options = refs?.lots?.length ? refs.lots.filter(row => row.active !== false) : uniqueById(inventory.map(row => ({ id: row.lot_id, code: row.lot_code, item_id: row.item_id })));
    if (kind === 'component_receipt') return options;
    return options.filter(lot => inventory.some(row => row.lot_id === lot.id && Number(row.quantity) > 0 &&
      (kind === 'issue' ? row.location_kind === 'warehouse' && !row.order_id : row.location_kind === 'floor' && String(row.order_id) === form.order_id)));
  }
  function locations(): Row[] {
    if (refs?.locations?.length) return refs.locations.filter(row => row.active !== false);
    return uniqueById(inventory.map(row => ({ id: row.location_id, code: row.location_code, name: row.location_name, kind: row.location_kind })));
  }
  function locationOptions(target: 'warehouse' | 'floor' | 'finished') { return locations().filter(row => row.kind === target); }
  function lineLocationKind(): 'warehouse' | 'floor' {
    if (kind === 'component_receipt' || kind === 'issue' || kind === 'return') return 'warehouse';
    return 'floor';
  }
  function lineKinds() { return ['component_receipt', 'issue', 'return', 'material_loss', 'production', 'rework'].includes(kind); }
  function linesRequired() { return ['component_receipt', 'issue', 'return', 'material_loss'].includes(kind); }
  function relevantLines() { return lines.filter(line => line.lot_id && line.location_id && Number(line.quantity) > 0); }
  function receiptQuantity() { return relevantLines().reduce((sum, line) => sum + Number(line.quantity), 0); }

  function occurredAt(): string | undefined {
    if (!form.occurred_at) return undefined;
    const parsed = new Date(form.occurred_at);
    return Number.isNaN(parsed.getTime()) ? undefined : parsed.toISOString();
  }
  function payload(): Record<string, unknown> {
    const body: Record<string, unknown> = { kind };
    const source = selectedSource();
    const orderId = form.order_id || String(source?.order_id ?? '');
    if (kind !== 'component_receipt' && orderId) body.order_id = orderId;
    if (form.source_document_id) body.source_document_id = form.source_document_id;
    if (form.work_session_id) body.work_session_id = form.work_session_id;
    if (form.location_id) body.location_id = form.location_id;
    if (kind === 'component_receipt') body.quantity = receiptQuantity();
    if (['production', 'disposition', 'rework', 'goods_receipt'].includes(kind)) body.quantity = Number(form.quantity);
    if (kind === 'inspection') {
      body.accepted_quantity = Number(form.accepted_quantity || 0);
      body.rejected_quantity = Number(form.rejected_quantity || 0);
      if (form.defect_reason_id) body.defect_reason_id = form.defect_reason_id;
      body.inspection_results = inspectionResults.filter(row => row.item_code).map(row => ({ ...row }));
    }
    if (['production', 'material_loss', 'disposition', 'rework'].includes(kind) && form.reason?.trim()) body.reason = form.reason.trim();
    if (kind === 'disposition') body.disposition = form.disposition;
    const at = occurredAt(); if (at) body.occurred_at = at;
    if (lineKinds()) body.lines = relevantLines().map(line => ({ lot_id: line.lot_id, location_id: line.location_id, quantity: Number(line.quantity) }));
    return body;
  }

  function validate(): string {
    const validQuantity = (value: string | undefined, min: number) => value !== undefined && value !== '' && Number.isSafeInteger(Number(value)) && Number(value) >= min && Number(value) <= 1000000;
    if (['production','disposition','rework','goods_receipt'].includes(kind) && !validQuantity(form.quantity, 1)) return '수량은 1~1,000,000 사이의 정수로 입력하세요.';
    if (kind === 'inspection' && (!validQuantity(form.accepted_quantity, 0) || !validQuantity(form.rejected_quantity, 0))) return '합격과 부적합 수량을 0 이상의 정수로 입력하세요.';
    if (kind === 'inspection' && inspectionResults.some(row => !row.item_code || !row.result)) return '모든 검사 항목의 판정을 선택하세요.';
    if (lineKinds() && lines.some(line => (line.lot_id || line.location_id) && (!line.lot_id || !line.location_id || !validQuantity(line.quantity, 1)))) return '자재 행의 로트, 위치와 수량을 모두 입력하세요.';
    if (kind !== 'component_receipt' && !activeOrders().some(row => String(row.id) === form.order_id)) return '현재 처리 가능한 작업 지시를 선택하세요.';
    if (['production','rework'].includes(kind) && !ownSessions().some(row => String(row.id) === form.work_session_id)) return '이 지시의 작업을 먼저 시작하세요.';
    if (['inspection','disposition','rework','goods_receipt'].includes(kind)) {
      const source = sourceOptions().find(row => String(row.id) === form.source_document_id);
      if (!source) return '처리 가능한 대상 기록을 선택하세요.';
      const requested = kind === 'inspection' ? Number(form.accepted_quantity || 0) + Number(form.rejected_quantity || 0) : Number(form.quantity);
      if (requested > sourceRemaining(source, kind as WorkKind, documents)) return '처리 대기 잔량을 초과했습니다.';
    }
    if (!can(...documentRoles[kind])) return '현재 계정 역할로 이 전표를 작성할 수 없습니다.';
    if (linesRequired() && relevantLines().length === 0) return '로트와 위치가 지정된 자재 행이 하나 이상 필요합니다.';
    if (kind === 'component_receipt' && receiptQuantity() < 1) return '입고 수량은 1 이상이어야 합니다.';
    if (['production', 'disposition', 'rework', 'goods_receipt'].includes(kind) && !(Number(form.quantity) >= 1 && Number(form.quantity) <= 1000000)) return '수량은 1~1,000,000 정수 범위여야 합니다.';
    if (kind === 'inspection' && Number(form.accepted_quantity || 0) + Number(form.rejected_quantity || 0) < 1) return '검사 합격/부적합 합계는 1 이상이어야 합니다.';
    if (kind === 'inspection' && Number(form.rejected_quantity || 0) > 0 && !form.defect_reason_id) return '부적합 수량이 있으면 부적합 사유를 선택하세요.';
    if (kind === 'disposition' && (!form.reason?.trim() || !form.disposition)) return '부적합 처분에는 처분 방식과 사유가 필요합니다.';
    return '';
  }

  async function createDraft(event: SubmitEvent) {
    event.preventDefault();
    const invalid = validate();
    if (invalid) { localError = invalid; return; }
    localError = '';
    pendingOperation = 'create';
    const saved = await command.run<Row>('/documents', payload());
    if (saved) {
      command.success = `${kinds[kind]} 초안을 저장했습니다.`;
      await loadAll();
      orderFilter = String(saved.order_id ?? form.order_id ?? '');
      tab = 'list';
      await openDocument(saved);
    }
  }

  async function openDocument(doc: Row) {
    selectedDoc = null;
    await detailResource.load(() => request<Row>(`/documents/${String(doc.id)}`));
    if (detailResource.error) { localError = '등록 내용을 불러오지 못했습니다. 처리 이력에서 다시 열어주세요.'; return; }
    selectedDoc = detailResource.value;
    form = { reason: '' };
    await tick();
    document.getElementById('document-review-title')?.focus();
    document.getElementById('document-review-title')?.scrollIntoView({ block: 'start' });
  }
  async function postDocument() {
    if (!selectedDoc) return;
    pendingOperation = 'post';
    const saved = await command.run(`/documents/${String(selectedDoc.id)}/post`, {});
    if (saved) { completedOrder = String(selectedDoc.order_id ?? ''); command.success = '등록을 확정했습니다. 아래에서 다음 작업을 이어가세요.'; selectedDoc = null; await loadAll(); }
  }
  async function reverseDocument() {
    if (!selectedDoc) return;
    const reason = form.reason?.trim();
    if (!reason) { localError = '역분개 사유가 필요합니다.'; return; }
    pendingOperation = 'reverse';
    const saved = await command.run(`/documents/${String(selectedDoc.id)}/reverse`, { reason });
    if (saved) { command.success = '문서를 역분개했습니다.'; selectedDoc = null; await loadAll(); }
  }
  function addLine() { lines = [...lines, { lot_id: '', location_id: '', quantity: '1' }]; }
  function removeLine(index: number) { lines = lines.filter((_, i) => i !== index); }
  function addInspection() { inspectionResults = [...inspectionResults, { item_code: '', result: '', note: '' }]; }
  function removeInspection(index: number) { inspectionResults = inspectionResults.filter((_, i) => i !== index); }
  function canPost(doc: Row) { return doc.status === 'draft' && can(...(documentRoles[String(doc.kind)] ?? [])); }
</script>

<svelte:head><title>수불 · 생산 · 품질 · 조립 제조 ERP</title></svelte:head>
<div class="heading"><div><p class="eyebrow">WORK RECORDS</p><h1>{tab === 'form' ? kinds[kind] + ' 등록' : '업무 처리 이력'}</h1><p>내용 입력 → 초안 확인 → 등록 확정 순서로 처리합니다. 초안만 저장하면 수량은 반영되지 않습니다.</p></div><a class="button secondary" href="/v2/work">업무 대기 목록</a></div>
{#if contextOrder}<OrderFlow order={contextOrder} references={refs} compact={tab === 'form'} />{/if}
{#if completedOrder}<div class="notice notice-info" role="status"><strong>다음 업무를 진행할 수 있습니다.</strong><a class="button secondary" href={workHref('orders', completedOrder)}>이 지시의 다음 작업 확인</a></div>{/if}

<Feedback {command} onRecovered={recovered} />
{#if detailResource.loading}<p role="status">등록 내용을 확인하는 중…</p>{/if}
{#if localError}<p class="notice notice-error" role="alert">{localError}</p>{/if}
<div class="tabs" aria-label="업무 기록 보기"><button disabled={command.busy || command.locked} class:selected={tab === 'list'} onclick={() => tab = 'list'}>처리 이력</button><button disabled={command.busy || command.locked} class:selected={tab === 'form'} onclick={() => { resetForm(kind); orderChanged(); tab = 'form'; }}>새 기록 등록</button></div>

{#if tab === 'list'}
  <section class="card">
    <h2>문서 목록</h2>
    <div class="filter"><div class="field"><label for="v2-doc-kind-filter">문서 종류</label><select id="v2-doc-kind-filter" bind:value={kindFilter}><option value="">전체</option>{#each allKinds as value}<option value={value}>{kinds[value]}</option>{/each}</select></div><div class="field"><label for="v2-doc-order-filter">지시</label><select id="v2-doc-order-filter" bind:value={orderFilter}><option value="">전체</option>{#each orders as row}<option value={String(row.id)}>{name(refs?.items, row.finished_item_id)} · {short(row.id)} · {label(row.status)}</option>{/each}</select></div></div>
    {#if docResource.loading}<p>로딩 중…</p>{:else if filteredDocuments().length === 0}<p class="empty">문서가 없습니다.</p>{:else}<div class="table-scroll"><table><thead><tr><th>종류</th><th>ID</th><th>상태</th><th>지시</th><th>일시</th><th>처리</th></tr></thead><tbody>{#each filteredDocuments() as doc (doc.id)}<tr><td>{label(doc.kind)}</td><td class="subtext">{short(doc.id)}</td><td><span class="status">{label(doc.status)}</span></td><td>{short(doc.order_id)}</td><td>{time(doc.occurred_at)}</td><td>{#if doc.status === 'draft'}<button class="button secondary compact" disabled={command.busy || command.locked} onclick={() => void openDocument(doc)}>{canPost(doc) ? '확정 준비' : '상세'}</button>{:else if doc.status === 'posted' && can('admin')}<button class="button secondary compact" disabled={command.busy || command.locked} onclick={() => void openDocument(doc)}>역분개 준비</button>{:else}<button class="button text compact" onclick={() => void openDocument(doc)}>상세</button>{/if}</td></tr>{/each}</tbody></table></div>{/if}
    <p class="muted">최근 500건과 미처리 잔량이 있는 기록을 표시합니다. 잘못 작성한 초안은 확정하지 말고 새로 등록하세요. 확정한 기록은 사유를 남겨 취소·정정할 수 있습니다.</p>
  </section>
{:else}
  <section class="card">
    <h2>{kinds[kind]} 입력</h2>
    {#if ['inspection','disposition','rework','goods_receipt'].includes(kind) && !sourceOptions().length}<p class="notice notice-info">처리 가능한 대상 기록이 없습니다. 이전 단계가 확정되었는지 확인하거나 업무 대기 목록을 새로고침하세요.</p>{/if}
    {#if availableKinds().length === 0}<p class="notice notice-info">현재 역할에는 작성 가능한 생산 전표가 없습니다. 조회 기능은 계속 사용할 수 있습니다.</p>
    {:else}<form onsubmit={createDraft}>
      <fieldset disabled={command.busy || command.locked}><div class="form-grid">
        <div class="field"><label for="v2-doc-kind">문서 종류</label><select id="v2-doc-kind" bind:value={kind} onchange={() => chooseKind(kind)}>{#each availableKinds() as value}<option value={value}>{kinds[value]}</option>{/each}</select></div>
        {#if kind !== 'component_receipt'}<div class="field"><label for="v2-doc-order">작업 지시</label><select id="v2-doc-order" bind:value={form.order_id} onchange={orderChanged} required><option value="" disabled>작업 지시를 선택하세요</option>{#each activeOrders() as row}<option value={String(row.id)}>{name(refs?.items, row.finished_item_id)} · {short(row.id)} · {label(row.status)}</option>{/each}</select></div>{/if}
        {#if sourceOptions().length || ['inspection','disposition','rework','goods_receipt'].includes(kind)}<div class="field"><label for="v2-doc-source">처리할 생산 · 검사 기록</label><select id="v2-doc-source" bind:value={form.source_document_id} onchange={sourceChanged} required><option value="" disabled>잔량이 있는 기록을 선택하세요</option>{#each sourceOptions() as source}<option value={String(source.id)}>{label(source.kind)} · {source.output_lot_code || short(source.id)} · 대기 {quantity(sourceRemaining(source, kind as WorkKind, documents))}</option>{/each}</select></div>{/if}
        {#if ['production','rework'].includes(kind)}<div class="field"><label for="v2-doc-session">진행 중인 작업</label><select id="v2-doc-session" bind:value={form.work_session_id} required><option value="" disabled>진행 중인 작업을 선택하세요</option>{#each ownSessions() as row}<option value={String(row.id)}>{short(row.id)} · {short(row.user_id)}</option>{/each}</select><span class="subtext">이 지시에서 시작한 내 작업에 생산 수량을 연결합니다.</span>{#if form.order_id && ownSessions().length === 0}<a href={workHref('sessions', form.order_id)}>먼저 작업 시작하기</a>{/if}</div>{/if}
        {#if ['issue','return'].includes(kind)}<div class="field"><label for="v2-doc-floor-location">{kind === 'issue' ? '불출 대상 현장 위치' : '반납 출발 현장 위치'}</label><select id="v2-doc-floor-location" bind:value={form.location_id} required><option value=""></option>{#each locationOptions('floor') as loc}<option value={String(loc.id)}>{loc.code} · {loc.name}</option>{/each}</select></div>{/if}
        {#if kind === 'goods_receipt'}<div class="field"><label for="v2-doc-finished-location">완제품 입고 위치</label><select id="v2-doc-finished-location" bind:value={form.location_id} required><option value=""></option>{#each locationOptions('finished') as loc}<option value={String(loc.id)}>{loc.code} · {loc.name}</option>{/each}</select></div>{/if}
        {#if ['production','disposition','rework','goods_receipt'].includes(kind)}<div class="field"><label for="v2-doc-quantity">수량</label><input id="v2-doc-quantity" type="number" min="1" max="1000000" step="1" bind:value={form.quantity} required /></div>{/if}
        {#if kind === 'component_receipt'}<div class="field"><label for="v2-doc-receipt-total">입고 합계</label><input id="v2-doc-receipt-total" value={String(receiptQuantity())} disabled /></div>{/if}
        {#if kind === 'inspection'}<div class="field"><label for="v2-doc-accepted">합격 수량</label><input id="v2-doc-accepted" type="number" min="0" max="1000000" step="1" bind:value={form.accepted_quantity} required /></div><div class="field"><label for="v2-doc-rejected">부적합 수량</label><input id="v2-doc-rejected" type="number" min="0" max="1000000" step="1" bind:value={form.rejected_quantity} required /></div><div class="field"><label for="v2-doc-defect">부적합 사유</label><select id="v2-doc-defect" bind:value={form.defect_reason_id} required={Number(form.rejected_quantity || 0) > 0}><option value=""></option>{#each refs?.defect_reasons ?? [] as reason}<option value={String(reason.id)}>{reason.code} · {reason.name}</option>{/each}</select></div>{/if}
        {#if kind === 'disposition'}<div class="field"><label for="v2-doc-disposition">처분</label><select id="v2-doc-disposition" bind:value={form.disposition} required><option value="" disabled>처분 방식을 선택하세요</option><option value="disposal">폐기</option><option value="rework">재작업</option></select></div>{/if}
        {#if ['production','material_loss','disposition','rework'].includes(kind)}<div class="field wide"><label for="v2-doc-reason">사유·비고{kind === 'disposition' ? ' (필수)' : ''}</label><textarea id="v2-doc-reason" bind:value={form.reason} required={kind === 'disposition'} placeholder={kind === 'production' ? 'BOM 대비 실제 소비 차이가 있으면 사유가 필수입니다.' : ''}></textarea></div>{/if}
        <div class="field"><label for="v2-doc-occurred">발생 일시 (선택)</label><input id="v2-doc-occurred" type="datetime-local" bind:value={form.occurred_at} /><span class="subtext">실제 작업한 시간을 입력하세요. 비워두면 등록 시각을 사용합니다.</span></div>
      </div>

      {#if lineKinds()}<fieldset><legend>{kind === 'component_receipt' ? '입고 LOT' : kind === 'issue' ? '창고 출고 LOT' : kind === 'return' ? '반납 대상 창고 LOT' : '실제 소비 LOT'}</legend>{#each lines as line, index}<div class="line-row"><div class="field"><label for={`v2-doc-lot-${index}`}>LOT</label><select id={`v2-doc-lot-${index}`} bind:value={line.lot_id} required={linesRequired()}><option value=""></option>{#each lots() as lot}<option value={String(lot.id)}>{lot.code ?? lot.lot_code ?? short(lot.id)} · {name(refs?.items, lot.item_id)}</option>{/each}</select></div><div class="field"><label for={`v2-doc-location-${index}`}>{kind === 'return' ? '반납 창고' : lineLocationKind() === 'warehouse' ? '창고 위치' : '현장 위치'}</label><select id={`v2-doc-location-${index}`} bind:value={line.location_id} required={linesRequired()}><option value=""></option>{#each locationOptions(lineLocationKind()) as loc}<option value={String(loc.id)}>{loc.code} · {loc.name}</option>{/each}</select></div><div class="field"><label for={`v2-doc-line-quantity-${index}`}>수량</label><input id={`v2-doc-line-quantity-${index}`} type="number" min="1" max="1000000" step="1" bind:value={line.quantity} required={linesRequired()} /></div><button class="button secondary compact" type="button" onclick={() => removeLine(index)}>삭제</button></div>{/each}<button class="button secondary" type="button" onclick={addLine}>LOT 행 추가</button>{#if !linesRequired()}<p class="subtext">생산·재작업의 추가 소비가 없으면 빈 행은 전송하지 않습니다.</p>{/if}</fieldset>{/if}

      {#if kind === 'inspection'}<fieldset><legend>검사 결과</legend>{#each inspectionResults as result, index}<div class="line-row"><div class="field"><label for={`v2-doc-inspection-code-${index}`}>항목 코드</label><input id={`v2-doc-inspection-code-${index}`} bind:value={result.item_code} required /></div><div class="field"><label for={`v2-doc-inspection-result-${index}`}>판정</label><select id={`v2-doc-inspection-result-${index}`} bind:value={result.result} required><option value="" disabled>판정 선택</option><option value="pass">합격</option><option value="fail">불합격</option></select></div><div class="field"><label for={`v2-doc-inspection-note-${index}`}>비고</label><input id={`v2-doc-inspection-note-${index}`} bind:value={result.note} /></div><button class="button secondary compact" type="button" onclick={() => removeInspection(index)}>삭제</button></div>{/each}<button class="button secondary" type="button" onclick={addInspection}>검사 항목 추가</button></fieldset>{/if}
      </fieldset><div class="actions"><button class="button" disabled={command.busy || command.locked || docResource.loading || !!docResource.error || !!orderResource.error || !!refsResource.error || !!sessionResource.error || !!inventoryResource.error}>초안 저장 후 확인</button></div>
    </form>{/if}

  </section>
{/if}

{#if selectedDoc}<section class="card" aria-label="등록 내용 확인">
  <div class="heading"><div><p class="eyebrow">문서 상세</p><h2 id="document-review-title" tabindex="-1">{label(selectedDoc.kind)} · {short(selectedDoc.id)}</h2><p>상태 {label(selectedDoc.status)} · 지시 {short(selectedDoc.order_id)} · 수량 {quantity(selectedDoc.quantity)}</p></div><button class="button secondary compact" disabled={command.busy || command.locked} onclick={() => selectedDoc = null}>닫기</button></div>
  {#if selectedDoc.status === 'draft'}
    {#if canPost(selectedDoc)}<div class="notice notice-info"><strong>등록 내용 확인</strong><p>확정하면 재고와 생산 현황에 반영됩니다. 확정 후에는 사유를 남겨 취소하고 새 기록으로 정정해야 합니다.</p></div><button class="button" disabled={command.busy || command.locked} onclick={() => void postDocument()}>등록 확정</button>{:else}<p class="notice notice-info">현재 계정에는 이 종류의 전표를 확정할 역할이 없습니다.</p>{/if}
  {:else if selectedDoc.status === 'posted' && can('admin')}
    <div class="field"><label for="v2-reverse-reason">취소·정정 사유</label><textarea id="v2-reverse-reason" bind:value={form.reason} required></textarea></div><div class="actions"><button class="button" disabled={command.busy || command.locked} onclick={() => void reverseDocument()}>등록 취소 (역분개)</button></div>
  {/if}
  <dl class="detail-list"><dt>작업 지시</dt><dd>{name(refs?.items, orders.find(row => row.id === selectedDoc?.order_id)?.finished_item_id)} · {short(selectedDoc.order_id)}</dd><dt>합격 / 부적합</dt><dd>{quantity(selectedDoc.accepted_quantity)} / {quantity(selectedDoc.rejected_quantity)}</dd><dt>대상 기록</dt><dd>{short(selectedDoc.source_document_id)}</dd><dt>위치</dt><dd>{name(refs?.locations, selectedDoc.location_id)}</dd><dt>사유</dt><dd>{selectedDoc.reason || '—'}</dd></dl>
  {#if Array.isArray(selectedDoc.lines) && selectedDoc.lines.length}<div class="table-scroll"><table><thead><tr><th>자재 로트</th><th>위치</th><th>수량</th></tr></thead><tbody>{#each selectedDoc.lines as line}<tr><td>{name(refs?.lots, line.lot_id)}</td><td>{name(refs?.locations, line.location_id)}</td><td>{quantity(line.quantity)}</td></tr>{/each}</tbody></table></div>{/if}
  {#if Array.isArray(selectedDoc.inspection_results) && selectedDoc.inspection_results.length}<div class="table-scroll"><table><thead><tr><th>검사 항목</th><th>판정</th><th>비고</th></tr></thead><tbody>{#each selectedDoc.inspection_results as result}<tr><td>{result.item_code}</td><td>{label(result.result)}</td><td>{result.note || '—'}</td></tr>{/each}</tbody></table></div>{/if}
  {#if Array.isArray(selectedDoc.dependencies) && selectedDoc.dependencies.length}<h3>의존 문서</h3><div class="table-scroll"><table><thead><tr><th>종류</th><th>ID</th></tr></thead><tbody>{#each selectedDoc.dependencies as dep}<tr><td>{label(dep.kind)}</td><td>{short(dep.id)}</td></tr>{/each}</tbody></table></div>{/if}

</section>{/if}
