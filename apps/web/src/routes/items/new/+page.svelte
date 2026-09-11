<script lang="ts">
  import { goto } from '$app/navigation';
  import { api } from '$lib/api';
  import { normalizeCode } from '$lib/quantity';
  import type { ItemKind } from '$lib/types';
  import Feedback from '$lib/components/Feedback.svelte';

  let code = $state(''); let name = $state(''); let kind = $state<ItemKind>('component'); let unit = $state('개');
  let busy = $state(false); let error = $state<unknown>(null); let savedId = $state('');

  async function submit(event: SubmitEvent) {
    event.preventDefault(); if (busy || savedId) return; error = null;
    try {
      const normalized = normalizeCode(code);
      if (!name.trim() || name.trim().length > 100) throw new Error('품목명은 1~100자로 입력해 주세요.');
      if (!unit.trim() || unit.trim().length > 20) throw new Error('단위는 1~20자로 입력해 주세요.');
      busy = true;
      const item = await api.createItem({ code: normalized, name: name.trim(), kind, unit: unit.trim() });
      savedId = item.id;
      await goto(`/items/${item.id}`);
    } catch (reason) { error = reason; }
    finally { busy = false; }
  }
</script>

<svelte:head><title>품목 등록 · 조립 제조 ERP</title></svelte:head>
<a class="back-link" href="/items">← 품목 목록</a>
<div class="page-heading"><div><p class="eyebrow">NEW ITEM</p><h1>품목 등록</h1><p>부품과 완제품을 구분해 등록합니다. 수량은 입고 또는 생산으로 반영됩니다.</p></div></div>
<section class="panel form-panel"><div class="panel-header"><div><h2>기본 정보</h2><p>별표 표시 항목은 필수입니다.</p></div></div><form class="panel-body" onsubmit={submit}>
  <fieldset disabled={busy || Boolean(savedId)}><legend class="sr-only">품목 기본 정보</legend><div class="form-grid">
    <div class="field"><label for="code">품목 코드<span class="required">*</span></label><input id="code" bind:value={code} maxlength="40" required placeholder="예: PART-CASE" autocomplete="off" /><small>영문 대문자, 숫자, _ 또는 -. 공백을 제거하고 대문자로 저장합니다.</small></div>
    <div class="field"><label for="kind">종류<span class="required">*</span></label><select id="kind" bind:value={kind}><option value="component">부품</option><option value="finished_good">완제품</option></select><small>부품은 소비 자재, 완제품은 생산 결과입니다.</small></div>
    <div class="field"><label for="name">품목명<span class="required">*</span></label><input id="name" bind:value={name} maxlength="100" required placeholder="예: 키보드 케이스" /></div>
    <div class="field"><label for="unit">단위<span class="required">*</span></label><input id="unit" bind:value={unit} maxlength="20" required placeholder="예: 개, EA" /><small>표시 단위입니다. 소수나 단위 환산은 지원하지 않습니다.</small></div>
  </div></fieldset>
  <Feedback {error} />
  {#if savedId}<div class="notice notice-success" role="status">품목이 등록되었습니다. <a href={`/items/${savedId}`}>품목 상세 보기</a></div>{/if}
  <div class="form-actions"><button class="button" type="submit" disabled={busy || Boolean(savedId)}>{busy ? '등록 중…' : '품목 등록'}</button><a class="button quiet" href="/items">목록으로</a></div>
</form></section>
