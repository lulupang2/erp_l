<script lang="ts">
  import { formatQuantity } from '$lib/quantity';
  import type { Material } from '$lib/types';
  let { materials, planned }: { materials: Material[]; planned?: number } = $props();
</script>

<div class="table-scroll">
  <table>
    <caption class="sr-only">BOM 부품 구성</caption>
    <thead><tr><th scope="col">부품</th><th scope="col" class="numeric">1개당 소요량</th>{#if planned !== undefined}<th scope="col" class="numeric">계획 전체 소요량</th>{/if}<th scope="col">단위</th></tr></thead>
    <tbody>
      {#each materials as row (row.item_id)}
        <tr><td><a href={`/items/${row.item_id}`} class="item-name">{row.name}</a><span class="subtext mono">{row.code}</span></td><td class="numeric">{formatQuantity(row.quantity_per_unit)}</td>{#if planned !== undefined}<td class="numeric">{formatQuantity(row.quantity_per_unit * planned)}</td>{/if}<td>{row.unit}</td></tr>
      {/each}
    </tbody>
  </table>
</div>
