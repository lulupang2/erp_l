<script lang="ts">
  import { onMount } from 'svelte';
  import { Resource, request, name, quantity, short, label, can, type Row, type References } from '$lib/v2/client.svelte';
  import { allowsWork, readyToClose, sourceCandidates, sourceRemaining, workHref, type WorkKind } from '$lib/v2/workflow';
  const data = new Resource<{orders: Row[]; documents: Row[]; refs: References}>();
  let queue = $state('production');
  let search = $state('');
  const queues = [
    {id:'production',label:'현장 작업',description:'작업할 지시를 열고 작업 시작과 생산 보고를 이어갑니다.'},
    {id:'materials',label:'자재 준비',description:'발행된 지시의 자재를 불출합니다. 이 목록은 자재 부족 판정이 아닙니다.'},
    {id:'inspection',label:'검사 대기',description:'아직 검사하지 않은 생산 기록을 선택하세요. 부분 검사도 가능합니다.'},
    {id:'goods_receipt',label:'입고 대기',description:'검사 합격 후 입고하지 않은 수량입니다. 보류 지시는 재개 후 입고하세요.'},
    {id:'disposition',label:'불량 처리',description:'처분이 필요한 부적합 수량을 폐기 또는 재작업으로 분류합니다.'},
    {id:'rework',label:'재작업',description:'재작업으로 처분된 수량입니다. 작업 시작 후 생산 기록을 등록하세요.'},
    {id:'closing',label:'마감 준비',description:'목표를 채우고 작업·자재·검사 잔량을 정리한 지시입니다. 내용을 확인한 뒤 마감하세요.'}
  ];
  onMount(() => { void load(); });
  async function load() {
    await data.load(async () => {
      const [orders, documents, refs] = await Promise.all([request<Row[]>('/orders'), request<Row[]>('/documents?pending=true'), request<References>('/reference')]);
      return {orders, documents, refs};
    });
  }
  function rows(id: string): {order: Row; source?: Row}[] {
    if (!data.value) return [];
    const {orders, documents} = data.value;
    if (id === 'closing') return orders.filter(readyToClose).map(order => ({order}));
    if (id === 'materials') return orders.filter(order => allowsWork(order, 'issue')).map(order => ({order}));
    if (id === 'production') return orders.filter(order => ['issued','in_progress'].includes(String(order.status)) && (Number(order.new_output_quantity) < Number(order.start_allowance) || Number(order.active_sessions) > 0)).map(order => ({order}));
    return sourceCandidates(documents, id as WorkKind).flatMap(source => {
      const order = orders.find(row => row.id === source.order_id);
      return order && ['issued','in_progress','held'].includes(String(order.status)) ? [{order, source}] : [];
    });
  }
  function href(order: Row, source?: Row) {
    if (queue === 'closing') return workHref('orders', order.id);
    if (queue === 'production' || (queue === 'rework' && !Number(order.active_sessions))) return workHref('sessions', order.id);
    return workHref('documents', order.id, queue === 'materials' ? 'issue' : queue, source?.id);
  }
  function allowed(order: Row) {
    if (queue === 'closing') return can('planner');
    if (queue === 'production') return can('operator');
    const role = ['materials','goods_receipt'].includes(queue) ? 'materials' : queue === 'rework' ? 'operator' : 'quality';
    return can(role) && allowsWork(order, (queue === 'materials' ? 'issue' : queue) as WorkKind);
  }
</script>

<svelte:head><title>업무 대기 · 조립 제조 ERP</title></svelte:head>
<div class="page-heading"><div><p class="eyebrow">FACTORY WORK</p><h1>업무 대기</h1><p>할 일을 선택하고, 같은 작업 지시에서 다음 업무까지 이어가세요.</p></div><div class="actions"><a class="button secondary" href="/v2/orders">작업 지시 관리</a><button class="button secondary" disabled={data.loading} onclick={load}>새로고침</button></div></div>
<nav class="work-queues" aria-label="업무 선택">{#each queues as item}<button class:active={queue === item.id} aria-pressed={queue === item.id} onclick={() => queue = item.id}><span>{item.label}</span><strong>{data.loading || data.error || !data.value ? '—' : rows(item.id).length}</strong></button>{/each}</nav>
<section class="card">
  <div class="heading"><div><h2>{queues.find(row => row.id === queue)?.label}</h2><p>{queues.find(row => row.id === queue)?.description}</p></div></div>
  <div class="field search"><label for="work-search">제품 · 지시 검색</label><input id="work-search" bind:value={search} placeholder="제품명, 품목 코드 또는 지시 ID" /></div>
  {#if data.loading}<p role="status">업무 목록을 불러오는 중…</p>{:else if data.error}<p class="notice notice-error" role="alert">{data.error}</p>{:else if data.value}
    <div class="work-list">
      {#each rows(queue).filter(({order}) => `${name(data.value?.refs.items, order.finished_item_id)} ${order.id}`.toLowerCase().includes(search.toLowerCase())) as {order, source}}
        <article class="work-row"><div><a class="record-link" href={workHref('orders', order.id)}>{name(data.value.refs.items, order.finished_item_id)}</a><p class="subtext">지시 {short(order.id)} · 예정 {String(order.planned_date ?? '—')} · {label(order.status)}</p>{#if source}<p class="subtext">{label(source.kind)} {short(source.id)}{source.output_lot_code ? ` · 로트 ${source.output_lot_code}` : ''}</p>{/if}</div>
          <div class="work-row-quantity"><strong>{quantity(source ? sourceRemaining(source, queue as WorkKind, data.value.documents) : order.remaining_quantity)}</strong><span>{source ? '처리 대기' : '입고 목표 잔여'}</span></div>
          {#if allowed(order)}<a class="button" href={href(order, source)}>{queue === 'production' ? '작업 열기' : queue === 'closing' ? '마감 확인' : queue === 'materials' ? '불출 등록' : queue === 'rework' && !Number(order.active_sessions) ? '작업 시작' : '처리하기'}</a>{:else}<a class="button secondary" href={workHref('orders', order.id)}>{order.status === 'held' ? '보류 지시 확인' : '지시 확인'}</a>{/if}
        </article>
      {:else}<div class="empty"><strong>해당하는 업무가 없습니다.</strong><p>다른 업무를 선택하거나 검색 조건을 확인하세요.</p><a href="/v2/orders">작업 지시 보기</a></div>{/each}
    </div>
  {/if}
</section>
