// Response DTOs follow contracts/README.md; never expose SQL/database models.
export type ItemKind = 'component' | 'finished_good';
export type OrderStatus = 'pending' | 'in_progress' | 'completed';
export type MovementType = 'manual_receipt' | 'production_consumption' | 'production_receipt';

export interface Item {
  id: string;
  code: string;
  name: string;
  kind: ItemKind;
  unit: string;
  created_at: string;
}

export interface Material {
  item_id: string;
  code: string;
  name: string;
  unit: string;
  quantity_per_unit: number;
}

export interface Bom {
  id: string;
  finished_item_id: string;
  updated_at: string;
  components: Material[];
}

export interface Inventory {
  item_id: string;
  code: string;
  name: string;
  kind: ItemKind;
  unit: string;
  quantity: number;
  updated_at: string;
}

export interface Receipt {
  id: string;
  item_id: string;
  quantity: number;
  note: string;
  created_at: string;
}

export interface Movement {
  id: string;
  item_id: string;
  delta: number;
  movement_type: MovementType;
  receipt_id: string | null;
  result_id: string | null;
  created_at: string;
}

export interface Order {
  id: string;
  finished_item_id: string;
  planned_quantity: number;
  good_quantity: number;
  defective_quantity: number;
  remaining_quantity: number;
  status: OrderStatus;
  created_at: string;
}

export interface OrderDetail extends Order { materials: Material[] }

export interface ProductionResult {
  id: string;
  order_id: string;
  good_quantity: number;
  defective_quantity: number;
  note: string;
  created_at: string;
}

export interface Pagination { page: number; page_size: number; total: number }
export interface Page<T> { data: T[]; pagination: Pagination }
export interface Data<T> { data: T }
export interface Shortage { item_id: string; required_quantity: number; available_quantity: number }

export type ItemInput = Pick<Item, 'code' | 'name' | 'kind' | 'unit'>;
export interface BomInput { components: Pick<Material, 'item_id' | 'quantity_per_unit'>[] }
export interface ReceiptInput { item_id: string; quantity: number; note?: string }
export interface OrderInput { finished_item_id: string; planned_quantity: number }
export interface ResultInput { good_quantity: number; defective_quantity: number; note?: string }
export type ListQuery = Record<string, string | number | undefined>;
