import type { Material, ResultInput } from './types';

export const MAX_INPUT = 1_000_000;
export const MAX_STOCK = 1_000_000_000_000;

export function quantity(value: string | number, minimum = 0, label = '수량'): number {
  const text = String(value).trim();
  if (!/^\d+$/.test(text)) throw new Error(`${label}은(는) 소수 없는 정수로 입력해 주세요.`);
  const result = Number(text);
  if (!Number.isSafeInteger(result) || result < minimum || result > MAX_INPUT) {
    throw new Error(`${label}은(는) ${minimum.toLocaleString('ko-KR')}~1,000,000 사이여야 합니다.`);
  }
  return result;
}

export function resultInput(good: string, defective: string, remaining: number, note = ''): ResultInput {
  const good_quantity = quantity(good, 0, '양품 수량');
  const defective_quantity = quantity(defective, 0, '불량 수량');
  const processed = good_quantity + defective_quantity;
  if (processed < 1) throw new Error('양품과 불량의 합계는 1 이상이어야 합니다.');
  if (processed > remaining) throw new Error(`처리 수량이 잔여 ${formatQuantity(remaining)}개를 초과합니다.`);
  if (note.length > 500) throw new Error('메모는 500자까지 입력할 수 있습니다.');
  return { good_quantity, defective_quantity, note: note.trim() };
}

export function consumption(materials: Material[], good: number, defective: number) {
  quantity(good, 0, '양품 수량');
  quantity(defective, 0, '불량 수량');
  return materials.map((material) => {
    quantity(material.quantity_per_unit, 1, 'BOM 소요량');
    const required_quantity = material.quantity_per_unit * (good + defective);
    if (!Number.isSafeInteger(required_quantity)) throw new Error('소비량을 정확히 계산할 수 없습니다.');
    return { ...material, required_quantity };
  });
}

export function normalizeCode(value: string): string {
  const code = value.replace(/\s/g, '').toUpperCase();
  if (!/^[A-Z0-9_-]{1,40}$/.test(code)) throw new Error('코드는 영문, 숫자, _ 또는 - 조합으로 1~40자 입력해 주세요.');
  return code;
}

export function formatQuantity(value: number): string {
  return new Intl.NumberFormat('ko-KR', { maximumFractionDigits: 0 }).format(value);
}

export function seoulTime(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '일시 확인 불가';
  return new Intl.DateTimeFormat('ko-KR', {
    timeZone: 'Asia/Seoul', year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit', second: '2-digit', hourCycle: 'h23'
  }).format(date);
}
