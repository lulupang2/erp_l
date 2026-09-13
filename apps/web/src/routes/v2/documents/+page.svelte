<script lang="ts">
  import { onMount } from 'svelte';
  import { Resource, Command, request, can, documentRoles, kinds, label, name, quantity, time, short, session } from '$lib/v2/client.svelte';
  import type { Row, References } from '$lib/v2/client.svelte';
  import Feedback from '$lib/v2/Feedback.svelte';

  type DocKind = 'component_receipt' | 'issue' | 'return' | 'material_loss' | 'production' | 'inspection' | 'disposition' | 'rework' | 'goods_receipt';
  type Line = { lot_id: string; location_id: string; quantity: string };
  type InspectionResult = { item_code: string; result: 'pass' | 'fail'; note: string };
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

  let selectedDoc = $state<Row | null>(null);
  let orderFilter = $state('');
  let kindFilter = $state<DocKind | ''>('');
  let kind = $state<DocKind>('component_receipt');
  let form = $state<Record<string, string>>({});
  let lines = $state<Line[]>([{ lot_id: '', location_id: '', quantity: '1' }]);
  let inspectionResults = $state<InspectionResult[]>([]);
  let tab = $state<'list' | 'form'>('list');
  let localError = $state('');

  onMount(() => { void loadAll(); });

  async function loadAll() {
    localError = '';
    await Promise.all([
      docResource.load(() => request<Row[]>('/documents')),
      orderResource.load(() => request<Row[]>('/orders')),
      inventoryResource.load(() => request<Row[]>('/inventory')),
      can('admin', 'planner', 'materials', 'quality') ? refsResource.load(() => request<References>('/reference')) : Promise.resolve(),
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
    form = {};
    lines = [{ lot_id: '', location_id: '', quantity: '1' }];
    inspectionResults = [];
    if (nextKind) kind = nextKind;
    ensureKind();
    localError = '';
  }
  function chooseKind(next: DocKind) { resetForm(next); }
  function filteredDocuments() { return documents.filter(doc => (!kindFilter || doc.kind === kindFilter) && (!orderFilter || String(doc.order_id ?? '') === orderFilter)); }

  function orderAllowed(row: Row) {
    const status = String(row.status);
    if (['issue', 'production', 'rework', 'goods_receipt'].includes(kind)) return ['issued', 'in_progress'].includes(status);
    return ['issued', 'in_progress', 'held'].includes(status);
  }
  function activeOrders() { return orders.filter(orderAllowed); }
  function selectedSource() { return documents.find(doc => String(doc.id) === form.source_document_id); }
  function sourceOptions() {
    const posted = documents.filter(doc => doc.status === 'posted');
    if (kind === 'inspection') return posted.filter(doc => ['production', 'rework'].includes(String(doc.kind)));
    if (kind === 'disposition') return posted.filter(doc => doc.kind === 'inspection');
    if (kind === 'rework') return posted.filter(doc => doc.kind === 'disposition' && doc.disposition === 'rework');
    if (kind === 'goods_receipt') return posted.filter(doc => doc.kind === 'inspection');
    return [];
  }
  function sourceChanged() {
    const source = selectedSource();
    if (source?.order_id) form.order_id = String(source.order_id);
    if (kind === 'inspection') {
      const order = orders.find(row => String(row.id) === String(source?.order_id ?? ''));
      const revision = refs?.inspection_revisions.find(row => String(row.id) === String(order?.inspection_revision_id ?? ''));
      const items = Array.isArray(revision?.items) ? revision.items as Row[] : [];
      inspectionResults = items.map(item => ({ item_code: String(item.item_code ?? ''), result: 'pass', note: '' }));
    }
  }

  function uniqueById(rows: Row[]) {
    const seen = new Set<string>();
    return rows.filter(row => { const id = String(row.id ?? ''); if (!id || seen.has(id)) return false; seen.add(id); return true; });
  }
  function lots(): Row[] {
    if (refs?.lots?.length) return refs.lots.filter(row => row.active !== false);
    return uniqueById(inventory.map(row => ({ id: row.lot_id, code: row.lot_code, item_id: row.item_id })));
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
    const saved = await command.run<Row>('/documents', payload());
    if (saved) {
      command.success = `${kinds[kind]} 초안을 저장했습니다.`;
      await loadAll();
      resetForm(kind);
      tab = 'list';
    }
  }

  async function openDocument(doc: Row) {
    await detailResource.load(() => request<Row>(`/documents/${String(doc.id)}`));
    selectedDoc = detailResource.value ?? doc;
    form = { reason: '' };
  }
  async function postDocument() {
    if (!selectedDoc) return;
    const saved = await command.run(`/documents/${String(selectedDoc.id)}/post`, {});
    if (saved) { command.success = '문서를 확정했습니다.'; selectedDoc = null; await loadAll(); }
  }
  async function reverseDocument() {
    if (!selectedDoc) return;
    const reason = form.reason?.trim();
    if (!reason) { localError = '역분개 사유가 필요합니다.'; return; }
    const saved = await command.run(`/documents/${String(selectedDoc.id)}/reverse`, { reason });
    if (saved) { command.success = '문서를 역분개했습니다.'; selectedDoc = null; await loadAll(); }
  }
  function addLine() { lines = [...lines, { lot_id: '', location_id: '', quantity: '1' }]; }
  function removeLine(index: number) { lines = lines.filter((_, i) => i !== index); }
  function addInspection() { inspectionResults = [...inspectionResults, { item_code: '', result: 'pass', note: '' }]; }
  function removeInspection(index: number) { inspectionResults = inspectionResults.filter((_, i) => i !== index); }
  function canPost(doc: Row) { return doc.status === 'draft' && can(...(documentRoles[String(doc.kind)] ?? [])); }
</script>

<svelte:head><title>수불 · 생산 · 품질 · 공장 v2</title></svelte:head>
<div class="heading"><div><p class="eyebrow">F2-04 · DOCUMENTS</p><h1>수불 · 생산 · 품질</h1><p>부품 입고, 불출·반납, 생산, 검사, 부적합 처분·재작업과 합격품 입고를 역할별 전표로 처리합니다.</p></div><span class="status">{session.user?.username ?? '—'}</span></div>
{#if localError}<p class="notice notice-error" role="alert">{localError}</p>{/if}
<div class="tabs" role="tablist"><button class:selected={tab === 'list'} onclick={() => tab = 'list'}>문서 목록</button><button class:selected={tab === 'form'} onclick={() => { ensureKind(); tab = 'form'; }}>문서 작성</button></div>

{#if tab === 'list'}
  <section class="card">
    <h2>문서 목록</h2>
    <div class="filter"><div class="field"><label for="v2-doc-kind-filter">문서 종류</label><select id="v2-doc-kind-filter" bind:value={kindFilter}><option value="">전체</option>{#each allKinds as value}<option value={value}>{kinds[value]}</option>{/each}</select></div><div class="field"><label for="v2-doc-order-filter">지시</label><select id="v2-doc-order-filter" bind:value={orderFilter}><option value="">전체</option>{#each orders as row}<option value={String(row.id)}>{short(row.id)} · {label(row.status)}</option>{/each}</select></div></div>
    {#if docResource.loading}<p>로딩 중…</p>{:else if filteredDocuments().length === 0}<p class="empty">문서가 없습니다.</p>{:else}<div class="table-scroll"><table><thead><tr><th>종류</th><th>ID</th><th>상태</th><th>지시</th><th>일시</th><th>처리</th></tr></thead><tbody>{#each filteredDocuments() as doc (doc.id)}<tr><td>{label(doc.kind)}</td><td class="subtext">{short(doc.id)}</td><td><span class="status">{label(doc.status)}</span></td><td>{short(doc.order_id)}</td><td>{time(doc.occurred_at)}</td><td>{#if doc.status === 'draft'}<button class="button secondary compact" disabled={command.locked} onclick={() => void openDocument(doc)}>{canPost(doc) ? '확정 준비' : '상세'}</button>{:else if doc.status === 'posted' && can('admin')}<button class="button secondary compact" disabled={command.locked} onclick={() => void openDocument(doc)}>역분개 준비</button>{:else}<button class="button text compact" onclick={() => void openDocument(doc)}>상세</button>{/if}</td></tr>{/each}</tbody></table></div>{/if}
    <p class="muted">현재 서버의 초안 수정 엔드포인트는 수정 요청을 거절합니다. 잘못 작성한 초안은 확정하지 말고 새 초안을 작성해야 하며, 이 제한은 백엔드/공개 계약 정합화가 필요합니다.</p>
  </section>
{:else}
  <section class="card">
    <h2>새 전표 초안</h2>
    {#if availableKinds().length === 0}<p class="notice notice-info">현재 역할에는 작성 가능한 생산 전표가 없습니다. 조회 기능은 계속 사용할 수 있습니다.</p>
    {:else}<form onsubmit={createDraft}>
      <div class="form-grid">
        <div class="field"><label for="v2-doc-kind">문서 종류</label><select id="v2-doc-kind" bind:value={kind} onchange={() => chooseKind(kind)}>{#each availableKinds() as value}<option value={value}>{kinds[value]}</option>{/each}</select></div>
        {#if kind !== 'component_receipt'}<div class="field"><label for="v2-doc-order">작업 지시</label><select id="v2-doc-order" bind:value={form.order_id} required={['issue','return','material_loss','production','rework','goods_receipt'].includes(kind)}><option value="" disabled>작업 지시를 선택하세요</option>{#each activeOrders() as row}<option value={String(row.id)}>{short(row.id)} · {label(row.status)}</option>{/each}</select></div>{/if}
        {#if sourceOptions().length || ['inspection','disposition','rework','goods_receipt'].includes(kind)}<div class="field"><label for="v2-doc-source">원본 문서</label><select id="v2-doc-source" bind:value={form.source_document_id} onchange={sourceChanged} required><option value="" disabled>원본 문서를 선택하세요</option>{#each sourceOptions() as source}<option value={String(source.id)}>{label(source.kind)} · {short(source.id)}</option>{/each}</select></div>{/if}
        {#if ['production','rework'].includes(kind)}<div class="field"><label for="v2-doc-session">작업 세션</label><select id="v2-doc-session" bind:value={form.work_session_id}><option value="">작업 세션을 연결하지 않음</option>{#each sessions.filter(row => row.status === 'active' && (!form.order_id || String(row.order_id) === form.order_id)) as row}<option value={String(row.id)}>{short(row.id)} · {short(row.user_id)}</option>{/each}</select><span class="subtext">현재 서버는 전표 확정 시 세션 연결을 강제 검증하지 않으므로 선택값은 추적 보조 정보입니다.</span></div>{/if}
        {#if ['issue','return'].includes(kind)}<div class="field"><label for="v2-doc-floor-location">{kind === 'issue' ? '불출 대상 현장 위치' : '반납 출발 현장 위치'}</label><select id="v2-doc-floor-location" bind:value={form.location_id} required><option value=""></option>{#each locationOptions('floor') as loc}<option value={String(loc.id)}>{loc.code} · {loc.name}</option>{/each}</select></div>{/if}
        {#if kind === 'goods_receipt'}<div class="field"><label for="v2-doc-finished-location">완제품 입고 위치</label><select id="v2-doc-finished-location" bind:value={form.location_id} required><option value=""></option>{#each locationOptions('finished') as loc}<option value={String(loc.id)}>{loc.code} · {loc.name}</option>{/each}</select></div>{/if}
        {#if ['production','disposition','rework','goods_receipt'].includes(kind)}<div class="field"><label for="v2-doc-quantity">수량</label><input id="v2-doc-quantity" type="number" min="1" max="1000000" step="1" bind:value={form.quantity} required /></div>{/if}
        {#if kind === 'component_receipt'}<div class="field"><label for="v2-doc-receipt-total">입고 합계</label><input id="v2-doc-receipt-total" value={String(receiptQuantity())} disabled /></div>{/if}
        {#if kind === 'inspection'}<div class="field"><label for="v2-doc-accepted">합격 수량</label><input id="v2-doc-accepted" type="number" min="0" max="1000000" step="1" bind:value={form.accepted_quantity} required /></div><div class="field"><label for="v2-doc-rejected">부적합 수량</label><input id="v2-doc-rejected" type="number" min="0" max="1000000" step="1" bind:value={form.rejected_quantity} required /></div><div class="field"><label for="v2-doc-defect">부적합 사유</label><select id="v2-doc-defect" bind:value={form.defect_reason_id} required={Number(form.rejected_quantity || 0) > 0}><option value=""></option>{#each refs?.defect_reasons ?? [] as reason}<option value={String(reason.id)}>{reason.code} · {reason.name}</option>{/each}</select></div>{/if}
        {#if kind === 'disposition'}<div class="field"><label for="v2-doc-disposition">처분</label><select id="v2-doc-disposition" bind:value={form.disposition} required><option value="" disabled>처분 방식을 선택하세요</option><option value="disposal">폐기</option><option value="rework">재작업</option></select></div>{/if}
        {#if ['production','material_loss','disposition','rework'].includes(kind)}<div class="field wide"><label for="v2-doc-reason">사유·비고{kind === 'disposition' ? ' (필수)' : ''}</label><textarea id="v2-doc-reason" bind:value={form.reason} required={kind === 'disposition'} placeholder={kind === 'production' ? 'BOM 대비 실제 소비 차이가 있으면 사유가 필수입니다.' : ''}></textarea></div>{/if}
        <div class="field"><label for="v2-doc-occurred">발생 일시 (선택)</label><input id="v2-doc-occurred" type="datetime-local" bind:value={form.occurred_at} /><span class="subtext">입력 시 브라우저 현지시각을 RFC3339 UTC로 변환해 전송합니다.</span></div>
      </div>

      {#if lineKinds()}<fieldset><legend>{kind === 'component_receipt' ? '입고 LOT' : kind === 'issue' ? '창고 출고 LOT' : kind === 'return' ? '반납 대상 창고 LOT' : '실제 소비 LOT'}</legend>{#each lines as line, index}<div class="line-row"><div class="field"><label for={`v2-doc-lot-${index}`}>LOT</label><select id={`v2-doc-lot-${index}`} bind:value={line.lot_id} required={linesRequired()}><option value=""></option>{#each lots() as lot}<option value={String(lot.id)}>{lot.code ?? lot.lot_code ?? short(lot.id)} · {name(refs?.items, lot.item_id)}</option>{/each}</select></div><div class="field"><label for={`v2-doc-location-${index}`}>{kind === 'return' ? '반납 창고' : lineLocationKind() === 'warehouse' ? '창고 위치' : '현장 위치'}</label><select id={`v2-doc-location-${index}`} bind:value={line.location_id} required={linesRequired()}><option value=""></option>{#each locationOptions(lineLocationKind()) as loc}<option value={String(loc.id)}>{loc.code} · {loc.name}</option>{/each}</select></div><div class="field"><label for={`v2-doc-line-quantity-${index}`}>수량</label><input id={`v2-doc-line-quantity-${index}`} type="number" min="1" max="1000000" step="1" bind:value={line.quantity} required={linesRequired()} /></div><button class="button secondary compact" type="button" onclick={() => removeLine(index)}>삭제</button></div>{/each}<button class="button secondary" type="button" onclick={addLine}>LOT 행 추가</button>{#if !linesRequired()}<p class="subtext">생산·재작업의 추가 소비가 없으면 빈 행은 전송하지 않습니다.</p>{/if}</fieldset>{/if}

      {#if kind === 'inspection'}<fieldset><legend>검사 결과</legend>{#each inspectionResults as result, index}<div class="line-row"><div class="field"><label for={`v2-doc-inspection-code-${index}`}>항목 코드</label><input id={`v2-doc-inspection-code-${index}`} bind:value={result.item_code} required /></div><div class="field"><label for={`v2-doc-inspection-result-${index}`}>판정</label><select id={`v2-doc-inspection-result-${index}`} bind:value={result.result}><option value="pass">합격</option><option value="fail">불합격</option></select></div><div class="field"><label for={`v2-doc-inspection-note-${index}`}>비고</label><input id={`v2-doc-inspection-note-${index}`} bind:value={result.note} /></div><button class="button secondary compact" type="button" onclick={() => removeInspection(index)}>삭제</button></div>{/each}<button class="button secondary" type="button" onclick={addInspection}>검사 항목 추가</button></fieldset>{/if}
      <div class="actions"><button class="button" disabled={command.busy || command.locked}>초안 저장</button></div>
    </form>{/if}
    <Feedback {command} />
  </section>
{/if}

{#if selectedDoc}<section class="card">
  <div class="heading"><div><p class="eyebrow">문서 상세</p><h2>{label(selectedDoc.kind)} · {short(selectedDoc.id)}</h2><p>상태 {label(selectedDoc.status)} · 지시 {short(selectedDoc.order_id)} · 수량 {quantity(selectedDoc.quantity)}</p></div><button class="button secondary compact" onclick={() => selectedDoc = null}>닫기</button></div>
  {#if selectedDoc.status === 'draft'}
    {#if canPost(selectedDoc)}<div class="notice notice-info"><strong>확정 전 확인</strong><p>확정 후 원문은 수정하지 않고 역분개 후 새 문서로 정정합니다. 보류 상태·재고·산출 잔량·역할은 서버가 다시 검사합니다.</p></div><button class="button" disabled={command.busy || command.locked} onclick={() => void postDocument()}>이 초안 확정</button>{:else}<p class="notice notice-info">현재 계정에는 이 종류의 전표를 확정할 역할이 없습니다.</p>{/if}
  {:else if selectedDoc.status === 'posted' && can('admin')}
    <div class="field"><label for="v2-reverse-reason">역분개 사유</label><textarea id="v2-reverse-reason" bind:value={form.reason} required></textarea></div><div class="actions"><button class="button" disabled={command.busy || command.locked} onclick={() => void reverseDocument()}>역분개</button></div>
  {/if}
  {#if Array.isArray(selectedDoc.dependencies) && selectedDoc.dependencies.length}<h3>의존 문서</h3><div class="table-scroll"><table><thead><tr><th>종류</th><th>ID</th></tr></thead><tbody>{#each selectedDoc.dependencies as dep}<tr><td>{label(dep.kind)}</td><td>{short(dep.id)}</td></tr>{/each}</tbody></table></div>{/if}
  <Feedback {command} />
</section>{/if}
