<script lang="ts">
  import { formatQuantity } from '$lib/quantity';
  import type { Pagination } from '$lib/types';
  let { pagination, loading = false, onchange }: {
    pagination: Pagination; loading?: boolean; onchange: (page: number, size: number) => void;
  } = $props();
  const pages = $derived(Math.max(1, Math.ceil(pagination.total / pagination.page_size)));
</script>

<div class="pagination" aria-label="목록 페이지">
  <span>총 <strong>{formatQuantity(pagination.total)}</strong>건</span>
  <div class="pagination-controls">
    <label>표시 수
      <select aria-label="페이지당 표시 수" value={String(pagination.page_size)} disabled={loading} onchange={(event) => onchange(1, Number(event.currentTarget.value))}>
        <option value="20">20건</option><option value="50">50건</option><option value="100">100건</option>
      </select>
    </label>
    <button type="button" class="button secondary compact" disabled={loading || pagination.page <= 1} onclick={() => onchange(pagination.page - 1, pagination.page_size)} aria-label="이전 페이지">이전</button>
    <span class="page-count">{pagination.page} / {pages}</span>
    <button type="button" class="button secondary compact" disabled={loading || pagination.page >= pages} onclick={() => onchange(pagination.page + 1, pagination.page_size)} aria-label="다음 페이지">다음</button>
  </div>
</div>
