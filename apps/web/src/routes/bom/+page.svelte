<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/state';
  import { ApiError, api } from '$lib/api';
  import { Resource } from '$lib/state.svelte';
  import { quantity, seoulTime } from '$lib/quantity';
  import type { Bom, Item } from '$lib/types';
  import Feedback from '$lib/components/Feedback.svelte';
  import LoadState from '$lib/components/LoadState.svelte';

  type Line = { key: number; item_id: string; quantity: string };
  const items = new Resource<Item[]>();
  const products = $derived(items.value?.filter((item) => item.kind === 'finished_good') ?? []);
  const components = $derived(items.value?.filter((item) => item.kind === 'component') ?? []);
  let selected = $state(page.url.searchParams.get('item_id') ?? '');
  let lines = $state<Line[]>([]); let sequence = 0; let revision = 0;
  let loading = $state(false); let saving = $state(false); let error = $state<unknown>(null); let loadError = $state<unknown>(null);
  let updated = $state(''); let success = $state(false);

  function useBom(bom: Bom | null) {
    lines = bom?.components.map((row) => ({ key: ++sequence, item_id: row.item_id, quantity: String(row.quantity_per_unit) })) ?? [];
    if (!lines.length) add();
    updated = bom?.updated_at ?? '';
  }
  function add() { lines = [...lines, { key: ++sequence, item_id: '', quantity: '1' }]; }
  async function loadBom() {
    const version = ++revision; const id = selected;
    error = null; loadError = null; success = false; updated = '';
    if (!id) { lines = []; loading = false; return; }
    loading = true;
    try {
      let bom: Bom | null;
      try { bom = await api.bom(id); }
      catch (reason) { if (reason instanceof ApiError && reason.status === 404) bom = null; else throw reason; }
      if (version === revision) useBom(bom);
    } catch (reason) { if (version === revision) loadError = reason; }
    finally { if (version === revision) loading = false; }
  }
  async function initialize() {
    await items.load(() => api.allItems());
    if (selected && !items.error) await loadBom();
  }
  async function submit(event: SubmitEvent) {
    event.preventDefault(); if (saving || loading || !selected) return; error = null; success = false;
    try {
      if (!lines.length) throw new Error('BOM에는 부품이 하나 이상 필요합니다.');
      const ids = new Set<string>();
      const rows = lines.map((line, index) => {
        if (!components.some((item) => item.id === line.item_id)) throw new Error(`${index + 1}번째 행의 부품을 선택해 주세요.`);
        if (ids.has(line.item_id)) throw new Error('같은 부품을 중복으로 등록할 수 없습니다.');
        ids.add(line.item_id);
        return { item_id: line.item_id, quantity_per_unit: quantity(line.quantity, 1, '부품 소요량') };
      });
      saving = true;
      await api.replaceBom(selected, { components: rows });
      success = true;
      // A refetch failure is not a failed write: keep the saved status and inputs.
      try { useBom(await api.bom(selected)); loadError = null; }
      catch (reason) { loadError = reason; }
    } catch (reason) { error = reason; }
    finally { saving = false; }
  }
  onMount(() => { void initialize(); });
</script>

<svelte:head><title>BOM 구성 · 조립 제조 ERP</title></svelte:head>
<div class="page-heading"><div><p class="eyebrow">BILL OF MATERIALS</p><h1>BOM 구성</h1><p>완제품 한 개를 만들 때 필요한 부품과 정수 소요량을 정의합니다.</p></div></div>
<section class="panel form-panel"><div class="panel-header"><div><h2>완제품 선택</h2><p>모든 페이지의 완제품과 부품을 불러와 표시합니다.</p></div></div><div class="panel-body">
  <LoadState loading={items.loading} error={items.error} empty={!products.length} emptyText="먼저 완제품 품목을 등록해 주세요." retry={() => void initialize()} />
  {#if !items.loading && !items.error && products.length}
    <div class="field"><label for="bom-product">완제품</label><select id="bom-product" bind:value={selected} disabled={saving} onchange={() => void loadBom()}><option value="">완제품 선택</option>{#each products as item (item.id)}<option value={item.id}>{item.code} · {item.name}</option>{/each}</select></div>
    {#if selected}
      <LoadState {loading} error={loadError} retry={() => void loadBom()} />
      {#if !loading && (!loadError || success)}
        <form onsubmit={submit}>
          <fieldset disabled={saving || components.length === 0}><legend class="sr-only">BOM 부품 구성 편집</legend>
            {#if !components.length}<div class="notice notice-warning">등록된 부품이 없습니다. <a href="/items/new">부품 등록</a> 후 다시 조회해 주세요.</div>{/if}
            {#each lines as line, index (line.key)}
              <div class="bom-line"><div class="field"><label for={`component-${line.key}`}>부품 {index + 1}</label><select id={`component-${line.key}`} bind:value={line.item_id} required><option value="">부품 선택</option>{#each components as item (item.id)}<option value={item.id}>{item.code} · {item.name} ({item.unit})</option>{/each}</select></div><div class="field"><label for={`quantity-${line.key}`}>소요량 {index + 1}</label><input id={`quantity-${line.key}`} inputmode="numeric" bind:value={line.quantity} required maxlength="7" /></div><button type="button" class="button danger compact" aria-label={`${index + 1}번째 부품 제거`} onclick={() => lines = lines.filter((row) => row.key !== line.key)}>제거</button></div>
            {/each}
            <button type="button" class="button secondary compact" onclick={add}>＋ 부품 추가</button>
          </fieldset>
          <Feedback {error} />
          {#if success}<div class="notice notice-success" role="status">BOM이 저장되었습니다.{#if loadError} 저장은 완료했지만 최신 구성 조회에 실패했습니다.{/if}</div>{/if}
          <div class="form-actions"><button type="submit" class="button" disabled={saving || !components.length}>{saving ? '저장 중…' : 'BOM 전체 저장'}</button><a class="button quiet" href={`/items/${selected}`}>품목 상세</a></div>
          {#if updated}<p class="help" style="margin-top: 15px">최종 저장 {seoulTime(updated)}</p>{/if}
        </form>
      {/if}
    {/if}
  {/if}
</div></section>
<div class="notice notice-info form-panel"><strong>기존 지시는 바뀌지 않습니다.</strong><p>BOM 저장은 선택한 완제품의 전체 구성을 교체합니다. 기존 생산 지시는 생성 당시 복사본을 사용하며, 변경 사항은 새 지시부터 적용됩니다.</p></div>
