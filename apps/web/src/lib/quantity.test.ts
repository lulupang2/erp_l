import { describe, expect, it } from 'vitest';
import { consumption, normalizeCode, quantity, resultInput, seoulTime } from './quantity';

describe('quantity rules', () => {
  it.each(['-1', '1.5', '1e2', '1 2', '', '1000001'])('rejects invalid input %s', value => {
    expect(() => quantity(value, 0, '수량')).toThrow();
  });
  it('accepts integer boundaries and trims whitespace', () => {
    expect(quantity('0', 0, '수량')).toBe(0);
    expect(quantity(' 1 ', 1, '수량')).toBe(1);
    expect(quantity('1000000', 1, '수량')).toBe(1_000_000);
    expect(() => quantity('0', 1, '수량')).toThrow();
  });
  it('uses good plus defective against remaining plan', () => {
    expect(resultInput('4', '1', 5, ' note ')).toEqual({ good_quantity: 4, defective_quantity: 1, note: 'note' });
    expect(resultInput('0', '5', 5, '')).toEqual({ good_quantity: 0, defective_quantity: 5, note: '' });
    expect(() => resultInput('0', '0', 5, '')).toThrow();
    expect(() => resultInput('3', '3', 5, '')).toThrow();
    expect(() => resultInput('1', '0', 0, '')).toThrow();
  });
  it('calculates material use exactly through the 1e12 stock boundary', () => {
    const materials = [{ item_id: 'item', code: 'ITEM', name: '부품', unit: '개', quantity_per_unit: 1_000_000 }];
    expect(consumption(materials, 500_000, 500_000)[0].required_quantity).toBe(1_000_000_000_000);
  });
  it('normalizes item codes and formats a UTC instant in Seoul', () => {
    expect(normalizeCode(' ab _1- ')).toBe('AB_1-');
    expect(() => normalizeCode('한글')).toThrow();
    expect(seoulTime('2026-09-10T15:30:00Z')).toContain('2026. 09. 11.');
    expect(seoulTime('2026-09-10T15:30:00Z')).toContain('00:30');
  });
});
