import { test, expect, type Page } from '@playwright/test';

const actor = '00000000-0000-0000-0000-000000000002';
async function factory(page: Page, uncertain = false) {
  const orders = ['one','two'].map((id,index) => ({id,finished_item_id:`item-${id}`,assigned_user_id:actor,status:index ? 'issued':'in_progress',target_quantity:10,start_allowance:12,planned_date:'2026-09-14',new_output_quantity:index ? 0:5,pending_quantity:index ? 0:5,accepted_quantity:0,rejected_quantity:0,rework_quantity:0,received_quantity:0,remaining_quantity:10,floor_quantity:20,active_sessions:index ? 0:1,inspection_revision_id:'criteria'}));
  const sessions: any[] = [{id:'work-one',order_id:'one',user_id:actor,status:'active',started_at:'2026-09-14T00:00:00Z'}];
  const documents: any[] = [{id:'source-one',order_id:'one',kind:'production',status:'posted',quantity:5,available_pending_quantity:5}, {id:'source-other',order_id:'two',kind:'production',status:'posted',quantity:9,available_pending_quantity:9}];
  const references = {items:[{id:'item-one',code:'KB-01',name:'조립 키보드'},{id:'item-two',code:'SW-02',name:'스위치 모듈'},{id:'part',code:'CASE',name:'케이스'}],locations:[{id:'floor',kind:'floor',code:'F1',name:'조립 현장'},{id:'finished',kind:'finished',code:'FG',name:'완제품 창고'}],lots:[{id:'lot',item_id:'part',code:'CASE-LOT'}],defect_reasons:[],bom_revisions:[],inspection_revisions:[{id:'criteria',items:[{item_code:'VISUAL',required:true}]}]};
  const writes: any[] = [];
  let firstCreate = true;
  let draft: any;
  await page.route('**/api/v2/**', async route => {
    const req = route.request(); const url = new URL(req.url()); const path = url.pathname.replace('/api/v2','');
    const body = req.method() === 'POST' ? req.postDataJSON() : undefined;
    if (body) writes.push({path,body,key:req.headers()['idempotency-key']});
    const reply = (data: unknown) => route.fulfill({json:{data}});
    if (path === '/orders') return reply(orders);
    if (path.startsWith('/orders/')) return reply(orders.find(row => row.id === path.split('/')[2]));
    if (path === '/reference') return reply(references);
    if (path === '/inventory') return reply([{lot_id:'lot',item_id:'part',location_id:'floor',location_kind:'floor',order_id:'one',quantity:20}]);
    if (path === '/work-sessions' && body) {
      const work = {id:'work-new',order_id:body.order_id,user_id:actor,status:'active',started_at:'2026-09-14T01:00:00Z'};
      sessions.push(work); const order=orders.find(row=>row.id===body.order_id)!;order.status='in_progress';order.active_sessions=1;
      return reply(work);
    }
    if (path === '/work-sessions') return reply(sessions);
    if (path === '/documents' && body) {
      if (!draft) {draft={...body,id:'new-draft',status:'draft'};documents.push(draft);}
      if (uncertain && firstCreate) { firstCreate=false;return route.fulfill({status:503,json:{error:{code:'UNAVAILABLE',message:'응답 확인 필요'}}}); }
      return reply(draft);
    }
    if (path === '/documents') return reply(documents);
    if (path.endsWith('/post')) {
      draft.status='posted';
      if (draft.kind === 'inspection') {draft.available_accepted_quantity=draft.accepted_quantity;documents[0].available_pending_quantity-=draft.accepted_quantity+draft.rejected_quantity;orders[0].accepted_quantity+=draft.accepted_quantity;}
      return reply(draft);
    }
    if (path.startsWith('/documents/')) return reply(documents.find(row=>row.id===path.split('/')[2]));
    return route.fulfill({status:404,json:{error:{code:'NOT_FOUND',message:path}}});
  });
  return {orders,documents,writes};
}

test('work queue keeps order context through start and production report', async ({page}) => {
  const state=await factory(page);
  await page.goto('/v2/work');
  await page.locator('.work-row').filter({hasText:'스위치 모듈'}).getByRole('link',{name:'작업 열기'}).click();
  await expect(page).toHaveURL(/sessions\?order=two/);
  await expect(page.locator('#v2-session-order')).toHaveValue('two');
  await page.getByRole('button',{name:'작업 시작',exact:true}).click();
  await page.getByRole('link',{name:'생산 수량 보고'}).click();
  await expect(page.locator('#v2-doc-order')).toHaveValue('two');
  await expect(page.locator('#v2-doc-session')).toHaveValue('work-new');
  await page.locator('#v2-doc-quantity').fill('2');
  await page.getByRole('button',{name:'초안 저장 후 확인'}).click();
  await expect(page.getByRole('button',{name:'등록 확정',exact:true})).toBeVisible();
  expect(state.writes.find(row=>row.path==='/documents').body).toMatchObject({kind:'production',order_id:'two',work_session_id:'work-new',quantity:2});
  await page.getByRole('button',{name:'등록 확정',exact:true}).click();
  await expect(page.getByRole('link',{name:'이 지시의 다음 작업 확인'})).toHaveAttribute('href','/v2/orders?order=two');
});

test('inspection queue scopes sources, requires explicit judgment and connects receipt', async ({page}) => {
  const state=await factory(page);
  await page.goto('/v2/work');
  await page.getByRole('button',{name:/검사 대기/}).click();
  await page.locator('.work-row').filter({hasText:'조립 키보드'}).getByRole('link',{name:'처리하기'}).click();
  await expect(page.locator('#v2-doc-source')).toHaveValue('source-one');
  await expect(page.locator('#v2-doc-source option[value="source-other"]')).toHaveCount(0);
  await expect(page.locator('#v2-doc-inspection-result-0')).toHaveValue('');
  await page.locator('#v2-doc-accepted').fill('6');await page.locator('#v2-doc-rejected').fill('0');
  await page.locator('#v2-doc-inspection-result-0').selectOption('pass');
  await page.getByRole('button',{name:'초안 저장 후 확인'}).click();
  await expect(page.getByRole('alert')).toContainText('잔량을 초과');
  expect(state.writes).toHaveLength(0);
  await page.locator('#v2-doc-accepted').fill('3');
  await page.getByRole('button',{name:'초안 저장 후 확인'}).click();
  await page.getByRole('button',{name:'등록 확정',exact:true}).click();
  await page.getByRole('link',{name:'업무 대기 목록',exact:true}).click();
  await page.getByRole('button',{name:/입고 대기/}).click();
  await page.locator('.work-row').getByRole('link',{name:'처리하기'}).click();
  await expect(page.locator('#v2-doc-order')).toHaveValue('one');
  await expect(page.locator('#v2-doc-source')).toHaveValue('new-draft');
});

test('uncertain draft retries the same key and resumes confirmation', async ({page}) => {
  const state=await factory(page,true);
  await page.goto('/v2/documents?order=one&kind=production&session=work-one');
  await page.locator('#v2-doc-quantity').fill('2');
  await page.getByRole('button',{name:'초안 저장 후 확인'}).click();
  await expect(page.locator('#v2-doc-kind')).toBeDisabled();
  await page.getByRole('button',{name:/같은 요청/}).click();
  await expect(page.getByRole('button',{name:'등록 확정',exact:true})).toBeVisible();
  expect(state.writes).toHaveLength(2);expect(state.writes[0].key).toBe(state.writes[1].key);
});

test('held orders keep inspection but block finished goods receipt', async ({page}) => {
  const state=await factory(page);state.orders[0].status='held';
  state.documents.push({id:'accepted',order_id:'one',kind:'inspection',status:'posted',available_accepted_quantity:3});
  await page.goto('/v2/work');await page.getByRole('button',{name:/입고 대기/}).click();
  await expect(page.getByRole('link',{name:'보류 지시 확인'})).toBeVisible();
  await expect(page.getByRole('link',{name:'처리하기'})).toHaveCount(0);
  await page.goto('/v2/documents?order=one&kind=goods_receipt');
  await expect(page.getByRole('alert')).toContainText('현재 지시 상태');
  await expect(page.locator('#v2-doc-quantity')).toHaveCount(0);
});

test('queue error does not show a false empty result', async ({page}) => {
  await factory(page);
  await page.route('**/api/v2/documents?pending=true', route=>route.fulfill({status:503,json:{error:{code:'UNAVAILABLE',message:'대기 조회 실패'}}}));
  await page.goto('/v2/work');
  await expect(page.getByRole('alert')).toContainText('대기 조회 실패');
  await expect(page.locator('.work-queues strong').first()).toHaveText('—');
  await expect(page.getByText('해당하는 업무가 없습니다.')).toHaveCount(0);
});

test('switching orders clears the prior production source and judgment', async ({page}) => {
  const state=await factory(page);state.orders[1].status='in_progress';
  await page.goto('/v2/documents?order=one&kind=inspection&source=source-one');
  await expect(page.locator('#v2-doc-source')).toHaveValue('source-one');
  await page.locator('#v2-doc-order').selectOption('two');
  await expect(page.locator('#v2-doc-source')).toHaveValue('');
  await expect(page.locator('#v2-doc-source option[value="source-one"]')).toHaveCount(0);
  await expect(page.locator('#v2-doc-inspection-result-0')).toHaveCount(0);
});

test('shop floor and inspection form remain readable on mobile', async ({page}) => {
  await factory(page);await page.setViewportSize({width:390,height:844});
  await page.goto('/v2/sessions?order=one');
  await expect(page.getByRole('link',{name:'생산 수량 보고'})).toBeVisible();
  await expect.poll(()=>page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true);
  await page.screenshot({path:'.local/work-sessions-mobile.png',fullPage:true,animations:'disabled'});
  await page.goto('/v2/documents?order=one&kind=inspection&source=source-one');
  await expect(page.locator('#v2-doc-source')).toHaveValue('source-one');
  await expect.poll(()=>page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true);
  await page.screenshot({path:'.local/work-inspection-mobile.png',fullPage:true,animations:'disabled'});
});

test('queue fits mobile and desktop in both themes', async ({page}) => {
  await factory(page);
  for (const width of [1440,390]) {
    await page.setViewportSize({width,height:960});await page.goto('/v2/work');
    await expect(page.locator('.work-row').first()).toBeVisible();
    if (await page.locator('html').getAttribute('data-theme') === 'dark') await page.locator('.theme-toggle').click();
    await expect(page.locator('html')).toHaveAttribute('data-theme','light');
    await expect.poll(()=>page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true);
    await page.screenshot({path:`.local/work-queue-${width}-light.png`,fullPage:true,animations:'disabled'});
    await page.locator('.theme-toggle').click();
    await expect(page.locator('html')).toHaveAttribute('data-theme','dark');
    await page.screenshot({path:`.local/work-queue-${width}-dark.png`,fullPage:true,animations:'disabled'});
  }
});
