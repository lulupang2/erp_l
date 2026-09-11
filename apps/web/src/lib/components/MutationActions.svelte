<script lang="ts">
  let { busy, pending, disabled = false, label, onreset }: {
    busy: boolean; pending: boolean; disabled?: boolean; label: string; onreset: () => void;
  } = $props();
</script>

{#if pending}
  <div class="notice notice-warning" role="status">
    <strong>저장 결과를 아직 확인하지 못했습니다.</strong>
    <p>입력과 요청 번호를 보관했습니다. 아래 버튼은 원래 입력 그대로 재전송하며, 이미 저장되었다면 기존 결과를 확인합니다.</p>
  </div>
{/if}
<div class="form-actions">
  <button type="submit" class="button" disabled={busy || (disabled && !pending)}>
    {busy ? '처리 중…' : pending ? '동일 요청 다시 확인' : label}
  </button>
  {#if pending}<button type="button" class="button quiet" disabled={busy} onclick={onreset}>요청 초기화</button>{/if}
</div>
