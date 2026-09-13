<script lang="ts">
  import { onMount } from 'svelte';
  import { Resource, Command, request, can, label, name } from '$lib/v2/client.svelte';
  import type { References, Row } from '$lib/v2/client.svelte';
  import Feedback from '$lib/v2/Feedback.svelte';

  type ReferenceKind = keyof References;
  type Field = { key: string; label: string; type?: 'text' | 'number' | 'select' | 'textarea' | 'ref' | 'fixed'; options?: string[]; ref?: ReferenceKind };
  type Definition = { label: string; fields: Field[] };
  type Line = { component_item_id?: string; quantity?: string; item_code?: string; name?: string; required?: boolean; acceptance?: string };

  const definitions: Record<ReferenceKind, Definition> = {
    items: { label: '품목', fields: [{ key: 'code', label: '품목 코드' }, { key: 'name', label: '품목 이름' }, { key: 'kind', label: '품목 구분', type: 'select', options: ['component', 'finished'] }, { key: 'unit', label: '단위', type: 'fixed' }] },
    locations: { label: '논리 위치', fields: [{ key: 'code', label: '위치 코드' }, { key: 'name', label: '위치 이름' }, { key: 'kind', label: '위치 용도', type: 'select', options: ['warehouse', 'floor', 'finished'] }] },
    lots: { label: '부품 로트', fields: [{ key: 'item_id', label: '품목', type: 'ref', ref: 'items' }, { key: 'code', label: '로트 코드' }] },
    defect_reasons: { label: '부적합 사유', fields: [{ key: 'code', label: '사유 코드' }, { key: 'name', label: '사유 이름' }] },
    bom_revisions: { label: 'BOM 개정', fields: [{ key: 'finished_item_id', label: '완제품', type: 'ref', ref: 'items' }, { key: 'revision', label: '개정 번호' }] },
    inspection_revisions: { label: '검사 기준 개정', fields: [{ key: 'finished_item_id', label: '완제품', type: 'ref', ref: 'items' }, { key: 'revision', label: '개정 번호' }] }
  };

  let refs = $state<References | null>(null);
  let resource = new Resource<References>();
  let command = new Command();
  let error = $state('');
  let kind = $state<ReferenceKind>('items');
  let form = $state<Record<string, string>>({ unit: 'ea' });
  let bomLines = $state<Line[]>([{ component_item_id: '', quantity: '1' }]);
  let inspectionLines = $state<Line[]>([{ item_code: '', name: '', required: true, acceptance: '' }]);

  onMount(() => { void load(); });
  async function load() { await resource.load(() => request<References>('/reference')); refs = resource.value; }
  function select(next: ReferenceKind) { kind = next; form = next === 'items' ? { unit: 'ea' } : {}; bomLines = [{ component_item_id: '', quantity: '1' }]; inspectionLines = [{ item_code: '', name: '', required: true, acceptance: '' }]; error = ''; }
  function endpointKind(value: ReferenceKind) { return value.replaceAll('_', '-'); }
  function payload() {
    if (kind === 'items') return { code: form.code, name: form.name, kind: form.kind, unit: 'ea' };
    if (kind === 'locations') return { code: form.code, name: form.name, kind: form.kind };
    if (kind === 'lots') return { code: form.code, item_id: form.item_id };
    if (kind === 'defect_reasons') return { code: form.code, name: form.name };
    if (kind === 'bom_revisions') return { finished_item_id: form.finished_item_id, revision: form.revision, lines: bomLines.filter(line => line.component_item_id).map(line => ({ component_item_id: line.component_item_id, quantity: Number(line.quantity) })) };
    return { finished_item_id: form.finished_item_id, revision: form.revision, items: inspectionLines.filter(line => line.item_code).map(line => ({ item_code: line.item_code, name: line.name ?? '', required: Boolean(line.required), acceptance: line.acceptance ?? '' })) };
  }
  async function submit(event: SubmitEvent) {
    event.preventDefault();
    const saved = await command.run(`/reference/${endpointKind(kind)}`, payload());
    if (saved) { command.success = `${definitions[kind].label} 초안을 저장했습니다.`; await load(); }
  }
  async function approve(id: string) { const saved = await command.run(`/reference/${endpointKind(kind)}/${encodeURIComponent(id)}/approve`, {}); if (saved) await load(); }
  async function retire(id: string) { const saved = await command.run(`/reference/${endpointKind(kind)}/${encodeURIComponent(id)}/retire`, {}); if (saved) await load(); }
  function addBomLine() { bomLines = [...bomLines, { component_item_id: '', quantity: '1' }]; }
  function addInspectionLine() { inspectionLines = [...inspectionLines, { item_code: '', name: '', required: true, acceptance: '' }]; }
</script>

<svelte:head><title>기준정보 · 조립 제조 ERP</title></svelte:head>
<div class="heading"><div><p class="eyebrow">MASTER DATA</p><h1>기준정보와 승인 개정</h1><p>품목·위치·부품 로트·부적합 사유와 승인된 BOM·검사 기준을 관리합니다.</p></div><span class="status">{refs ? '기준정보 연결됨' : '조회 중'}</span></div>
{#if resource.error}<p class="notice notice-error" role="alert">{resource.error} <button class="button secondary" onclick={() => void load()}>다시 조회</button></p>{/if}
<div class="tabs" role="tablist">{#each Object.entries(definitions) as [entryKind, definition]}<button class:selected={kind === entryKind} onclick={() => select(entryKind as ReferenceKind)}>{definition.label}</button>{/each}</div>
<div class="split">
  <section class="card">
    <h2>{definitions[kind].label} 등록</h2>
    <form onsubmit={submit}>
      <div class="form-grid">
        {#each definitions[kind].fields as field}
          {#if field.type === 'textarea'}<div class="field wide"><label for={`v2-${field.key}`}>{field.label}</label><textarea id={`v2-${field.key}`} bind:value={form[field.key]} required></textarea></div>
          {:else if field.type === 'select'}<div class="field"><label for={`v2-${field.key}`}>{field.label}</label><select id={`v2-${field.key}`} bind:value={form[field.key]} required><option value="" disabled>{field.label}을 선택하세요</option>{#each field.options ?? [] as option}<option value={option}>{label(option)}</option>{/each}</select></div>
          {:else if field.type === 'ref'}<div class="field"><label for={`v2-${field.key}`}>{field.label}</label><select id={`v2-${field.key}`} bind:value={form[field.key]} required><option value="" disabled>{field.label}을 선택하세요</option>{#each refs?.[field.ref ?? 'items'] ?? [] as option}<option value={String(option.id)}>{name(refs?.[field.ref ?? 'items'], option.id)}</option>{/each}</select></div>
          {:else if field.type === 'fixed'}<div class="field"><label for="v2-fixed-unit">{field.label}</label><input id="v2-fixed-unit" value="ea" disabled /></div>
          {:else}<div class="field"><label for={`v2-${field.key}`}>{field.label}</label><input id={`v2-${field.key}`} bind:value={form[field.key]} required /></div>{/if}
        {/each}
      </div>
      {#if kind === 'bom_revisions'}<fieldset><legend>BOM 투입 구성</legend>{#each bomLines as line, index}<div class="line-row"><div class="field"><label for={`v2-bom-component-${index}`}>부품 품목</label><select id={`v2-bom-component-${index}`} bind:value={line.component_item_id}><option value=""></option>{#each refs?.items.filter(item => item.kind === 'component' && item.active !== false) ?? [] as item}<option value={String(item.id)}>{name(refs?.items, item.id)}</option>{/each}</select></div><div class="field"><label for={`v2-bom-quantity-${index}`}>소요 수량</label><input id={`v2-bom-quantity-${index}`} type="number" min="1" max="1000000" step="1" bind:value={line.quantity} /></div><button class="button secondary compact" type="button" onclick={() => bomLines = bomLines.filter((_, itemIndex) => itemIndex !== index)}>삭제</button></div>{/each}<button class="button secondary" type="button" onclick={addBomLine}>투입 품목 추가</button></fieldset>{/if}
      {#if kind === 'inspection_revisions'}<fieldset><legend>검사 항목</legend>{#each inspectionLines as line, index}<div class="line-row"><div class="field"><label for={`v2-inspection-code-${index}`}>검사 항목 코드</label><input id={`v2-inspection-code-${index}`} bind:value={line.item_code} /></div><div class="field"><label for={`v2-inspection-name-${index}`}>표시 이름</label><input id={`v2-inspection-name-${index}`} bind:value={line.name} /></div><div class="field"><label for={`v2-inspection-acceptance-${index}`}>합격 기준</label><input id={`v2-inspection-acceptance-${index}`} bind:value={line.acceptance} /></div><label class="check"><input type="checkbox" bind:checked={line.required} />필수</label><button class="button secondary compact" type="button" onclick={() => inspectionLines = inspectionLines.filter((_, itemIndex) => itemIndex !== index)}>삭제</button></div>{/each}<button class="button secondary" type="button" onclick={addInspectionLine}>검사 항목 추가</button></fieldset>{/if}
      <div class="actions"><button class="button" disabled={command.busy || command.locked}>초안 저장</button><Feedback {command} /></div>
    </form>
  </section>
  <section class="card">
    <h2>승인·사용 중지</h2><p class="muted">BOM·검사 기준의 승인과 사용 중지는 관리자 권한이 필요하며 서버에서 다시 확인합니다. 승인된 개정은 수정하지 않고 새 개정을 만드세요.</p>
    {#if refs}<div class="table-scroll"><table><thead><tr><th>기준정보</th><th>상태</th><th>관리</th></tr></thead><tbody>{#each refs[kind] as row}<tr><td><strong>{String(row.code ?? row.revision ?? row.name ?? '기준정보')}</strong><span class="subtext">{String(row.id).slice(0, 8)}</span></td><td><span class="status">{label(row.status ?? (row.active === false ? 'retired' : 'active'))}</span></td><td>{#if ['bom_revisions', 'inspection_revisions'].includes(kind) && row.status === 'draft' && can('admin')}<button class="button secondary compact" disabled={command.busy || command.locked} onclick={() => approve(String(row.id))}>승인</button>{:else if ['bom_revisions', 'inspection_revisions'].includes(kind) && row.status === 'approved' && can('admin')}<button class="button secondary compact" disabled={command.busy || command.locked} onclick={() => retire(String(row.id))}>사용 중지</button>{:else}<span class="subtext">—</span>{/if}</td></tr>{/each}</tbody></table></div>{/if}
  </section>
</div>
{#if refs}<section class="card"><h2>{definitions[kind].label} 목록</h2><div class="table-scroll"><table><thead><tr>{#each definitions[kind].fields as field}<th>{field.label}</th>{/each}<th>ID</th></tr></thead><tbody>{#each refs[kind] as row}<tr>{#each definitions[kind].fields as field}<td>{field.type === 'ref' ? name(refs?.[field.ref ?? 'items'], row[field.key]) : String(row[field.key] ?? '—')}</td>{/each}<td class="subtext">{String(row.id).slice(0, 8)}</td></tr>{/each}</tbody></table></div></section>{/if}
{#if error}<p class="notice notice-error" role="alert">{error}</p>{/if}
