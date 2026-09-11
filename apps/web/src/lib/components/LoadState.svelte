<script lang="ts">
  import Feedback from './Feedback.svelte';
  let { loading, error, empty = false, emptyText = '등록된 항목이 없습니다.', retry }: {
    loading: boolean; error: unknown; empty?: boolean; emptyText?: string; retry?: () => void;
  } = $props();
</script>

{#if loading}
  <div class="empty-state" role="status" aria-live="polite"><span class="spinner"></span>서버에서 최신 정보를 불러오고 있습니다.</div>
{:else if error}
  <div class="load-error"><Feedback {error} />{#if retry}<button type="button" class="button secondary" onclick={retry}>다시 조회</button>{/if}</div>
{:else if empty}
  <div class="empty-state"><span class="empty-symbol" aria-hidden="true">＋</span><p>{emptyText}</p></div>
{/if}
