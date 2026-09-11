<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import { api, orderOperation } from '$lib/api';
  import { Mutation, Resource } from '$lib/state.svelte';
  import { formatQuantity, quantity } from '$lib/quantity';
  import type { Bom, Item } from '$lib/types';
  import Feedback from '$lib/components/Feedback.svelte';
  import LoadState from '$lib/components/LoadState.svelte';
  import MutationActions from '$lib/components/MutationActions.svelte';

  const items = new Resource<Item[]>(); const bom = new Resource<Bom>();
  const mutation = new Mutation(orderOperation());
  let selected = $state(untrack(() => mutation.draft?.finished_item_id ?? page.url.searchParams.get('item_id') ?? ''));
  let planned = $state(String(mutation.draft?.planned_quantity ?? ''));
  async function loadBom() {
    if (!selected) { bom.value = null; bom.loading = false; bom.error = null; return; }
    const id = selected; await bom.load(() => api.bom(id));
  }
  async function initialize() { await items.load(() => api.allItems({ kind: 'finished_good' })); await loadBom(); }
  async function submit(event: SubmitEvent) {
    event.preventDefault(); if (mutation.busy || mutation.saved) return;
    try {
      const pending = mutation.draft;
      if (!pending && (!items.value?.some(item => item.id === selected) || !bom.value || bom.loading || bom.error)) throw new Error('완제품과 등록된 BOM을 먼저 확인해 주세요.');
      const saved = await mutation.submit(pending ?? { finished_item_id: selected, planned_quantity: quantity(planned, 1, '계획 수량') });
      if (saved) await goto(`/orders/${saved.id}`);
    } catch (error) { mutation.error = error; }
  }
  function reset() { mutation.reset(); if (!mutation.pending) planned = ''; }
  onMount(() => { void initialize(); });
</script>

<svelte:head><title>생산 지시 생성 · 조립 제조 ERP</title></svelte:head>
<a class="back-link" href="/orders">← 생산 지시 목록</a>
<div class="page-heading"><div><p class="eyebrow">NEW PRODUCTION ORDER</p><h1>생산 지시 생성</h1><p>완제품의 현재 BOM을 복사해 생산 기준을 고정합니다.</p></div></div>
<section class="panel form-panel"><div class="panel-header"><div><h2>생산 계획</h2><p>계획 수량은 양품과 불량을 합친 처리 목표입니다.</p></div></div><form class="panel-body" onsubmit={submit}>
  <LoadState loading={items.loading} error={items.error} empty={items.value?.length === 0} emptyText="등록된 완제품이 없습니다. 품목과 BOM부터 준비해 주세요." retry={() => void initialize()} />
  <fieldset disabled={mutation.locked || Boolean(mutation.saved) || items.loading || Boolean(items.error)}><legend class="sr-only">생산 지시 입력</legend>
    <div class="field"><label for="order-product">생산 완제품<span class="required">*</span></label><select id="order-product" bind:value={selected} onchange={() => void loadBom()} required><option value="">완제품 선택</option>{#each items.value ?? [] as item (item.id)}<option value={item.id}>{item.code} · {item.name}</option>{/each}</select></div>
    <div class="field"><label for="order-planned">계획 수량<span class="required">*</span></label><input id="order-planned" bind:value={planned} inputmode="numeric" maxlength="7" required placeholder="1~1,000,000" /></div>
  </fieldset>
  {#if selected}<LoadState loading={bom.loading} error={bom.error} retry={() => void loadBom()} />{/if}
  {#if bom.value && selected && !bom.loading && !bom.error}<div class="consumption-preview"><h3>복사할 BOM · 완제품 1개 기준</h3>{#each bom.value.components as material (material.item_id)}<div class="summary-line"><span>{material.name}</span><strong>{formatQuantity(material.quantity_per_unit)} {material.unit}</strong></div>{/each}<p>저장 순간의 BOM이 최종 기준입니다. 이전에 생성한 지시의 복사본은 이후 BOM 변경과 무관합니다.</p></div>{/if}
  <Feedback error={mutation.error} />
  {#if mutation.saved}<div class="notice notice-success" role="status">생산 지시가 저장되었습니다. <a href={`/orders/${mutation.saved.id}`}>생산 지시 상세 보기</a></div>{/if}
  <MutationActions busy={mutation.busy} pending={mutation.pending} disabled={Boolean(mutation.saved) || items.loading || Boolean(items.error) || bom.loading || Boolean(bom.error) || !bom.value} label="생산 지시 생성" onreset={reset} />
</form></section>
