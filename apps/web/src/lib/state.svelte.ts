import { IdempotentOperation } from './api';

export class Resource<T> {
  value = $state<T | null>(null);
  loading = $state(true);
  error = $state<unknown>(null);
  private revision = 0;

  async load(loader: () => Promise<T>): Promise<void> {
    const revision = ++this.revision;
    this.loading = true;
    this.error = null;
    try {
      const value = await loader();
      if (revision === this.revision) this.value = value;
    } catch (error) {
      if (revision === this.revision) this.error = error;
    } finally {
      if (revision === this.revision) this.loading = false;
    }
  }
}

export class Mutation<Body extends object, Result> {
  busy = $state(false);
  pending = $state(false);
  error = $state<unknown>(null);
  saved = $state<Result | null>(null);

  constructor(private readonly operation: IdempotentOperation<Body, Result>) {
    this.pending = operation.pending !== null;
  }

  get draft(): Body | null { return this.operation.draft; }
  get locked(): boolean { return this.busy || this.pending; }

  async submit(body: Body): Promise<Result | null> {
    if (this.busy) return null;
    this.busy = true;
    this.error = null;
    this.saved = null;
    try {
      this.saved = await this.operation.submit(body);
      return this.saved;
    } catch (error) {
      this.error = error;
      return null;
    } finally {
      this.pending = this.operation.pending !== null;
      this.busy = false;
    }
  }

  reset(): void {
    if (this.busy) return;
    if (this.pending && !window.confirm('저장 여부가 아직 불확실합니다. 먼저 이력이나 동일 요청 재확인으로 결과를 확인하는 것이 안전합니다. 기존 요청을 버리고 새 요청을 만들면 중복 처리될 수 있습니다. 그래도 초기화할까요?')) return;
    this.operation.reset();
    this.pending = false;
    this.error = null;
    this.saved = null;
  }
}
