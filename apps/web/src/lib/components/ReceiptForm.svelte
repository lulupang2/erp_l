<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { api, receiptOperation } from '$lib/api';
  import { Mutation, Resource } from '$lib/state.svelte';
  import { quantity } from '$lib/quantity';
  import type { Item } from '$lib/types';
  import Feedback from './Feedback.svelte';
  import LoadState from './LoadState.svelte';
  import MutationActions from './MutationActions.svelte';
  import Icon from './Icon.svelte';

  let { initialItemId = '', onsaved }: { initialItemId?: string; onsaved: () => Promise<void> } = $props();
  const items = new Resource<Item[]>();
  const mutation = new Mutation(receiptOperation());
  let selected = $state(mutation.draft?.item_id ?? untrack(() => initialItemId));
  let quantityText = $state(String(mutation.draft?.quantity ?? ''));
  let note = $state(mutation.draft?.note ?? '');
  let refreshing = $state(false);
  const itemMap = $derived(new Map(items.value?.map((item) => [item.id, item]) ?? []));

  function loadItems() { return items.load(() => api.allItems({ kind: 'component' })); }
  async function submit(event: SubmitEvent) {
    event.preventDefault(); if (mutation.busy || refreshing) return;
    try {
      // An unresolved request must replay before any current-form validation.
      const pending = mutation.draft;
      if (!pending && !items.value?.some((item) => item.id === selected)) throw new Error('입고할 부품을 선택해 주세요.');
      const body = pending ?? { item_id: selected, quantity: quantity(quantityText, 1, '입고 수량'), note: note.trim() };
      const saved = await mutation.submit(body);
      if (saved) {
        quantityText = ''; note = ''; refreshing = true;
        await onsaved();
      }
    } catch (error) { mutation.error = error; }
    finally { refreshing = false; }
  }
  function reset() {
    mutation.reset();
    if (!mutation.pending) { quantityText = ''; note = ''; }
  }
  onMount(() => { void loadItems(); });
</script>

<section class="panel" aria-label="부품 입고">
  <div class="panel-header"><div><h2 class="panel-label"><Icon name="plus" size={18} />부품 입고</h2><p>입고와 재고 증가는 함께 저장됩니다.</p></div></div>
  <form class="panel-body" onsubmit={submit}>
    <LoadState loading={items.loading} error={items.error} empty={items.value?.length === 0} emptyText="등록된 부품이 없습니다." retry={() => void loadItems()} />
    <fieldset disabled={mutation.locked || refreshing || items.loading || Boolean(items.error)}><legend class="sr-only">입고 정보</legend>
      <div class="field"><label for="receipt-item">입고 부품<span class="required">*</span></label><select id="receipt-item" bind:value={selected} required><option value="">부품 선택</option>{#each items.value ?? [] as item (item.id)}<option value={item.id}>{item.code} · {item.name}</option>{/each}</select></div>
      <div class="field"><label for="receipt-quantity">입고 수량<span class="required">*</span></label><input id="receipt-quantity" inputmode="numeric" bind:value={quantityText} required maxlength="7" placeholder="1~1,000,000" /><small>양수 정수만 입력합니다. 단위: {itemMap.get(selected)?.unit ?? '부품 선택 후 표시'}</small></div>
      <div class="field"><label for="receipt-note">입고 메모</label><textarea id="receipt-note" bind:value={note} maxlength="500" placeholder="입고 사유 또는 참고 사항 (선택)"></textarea><small>{note.length}/500자</small></div>
    </fieldset>
    <Feedback error={mutation.error} items={itemMap} />
    {#if mutation.saved}<div class="notice notice-success" role="status">입고가 저장되었습니다. <a href={`/receipts/${mutation.saved.id}`}>입고 상세 보기 ↗</a></div>{/if}
    <MutationActions busy={mutation.busy || refreshing} pending={mutation.pending} disabled={items.loading || Boolean(items.error) || !items.value?.length} label="입고 등록" onreset={reset} />
  </form>
</section>
