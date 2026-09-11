<script lang="ts">
  import { ApiError, errorMessage } from '$lib/api';
  import { formatQuantity } from '$lib/quantity';
  import type { Item } from '$lib/types';

  let { error, items }: { error: unknown; items?: Map<string, Item> } = $props();
  const shortages = $derived(error instanceof ApiError ? error.shortages : []);
</script>

{#if error}
  <div class="notice notice-error" role="alert">
    <strong>요청을 확인해 주세요</strong>
    <p>{errorMessage(error)}</p>
    {#if shortages.length}
      <ul class="shortage-list">
        {#each shortages as row (row.item_id)}
          <li><strong>{items?.get(row.item_id)?.name ?? row.item_id}</strong> · 필요 {formatQuantity(row.required_quantity)} / 현재 {formatQuantity(row.available_quantity)}</li>
        {/each}
      </ul>
    {/if}
  </div>
{/if}
