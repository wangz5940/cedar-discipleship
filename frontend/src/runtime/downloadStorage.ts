type StoredChunk = {
  taskID: string;
  index: number;
  blob: Blob;
};

const databaseName = 'agp-downloads-v1';
const storeName = 'chunks';
const memoryFallbackLimit = 128 * 1024 * 1024;

export class DownloadChunkStorage {
  private databasePromise: Promise<IDBDatabase | null> | null = null;
  private memory = new Map<string, Map<number, Blob>>();

  async putChunk(taskID: string, index: number, blob: Blob): Promise<void> {
    const database = await this.database();
    if (!database) {
      const chunks = this.memory.get(taskID) || new Map<number, Blob>();
      const existingSize = [...chunks.values()].reduce((total, chunk) => total + chunk.size, 0);
      if (existingSize + blob.size > memoryFallbackLimit) throw new Error('download_storage_write_failed');
      chunks.set(index, blob);
      this.memory.set(taskID, chunks);
      return;
    }
    await runTransaction(database, 'readwrite', (store) => {
      store.put({ taskID, index, blob } satisfies StoredChunk);
    });
  }

  async listChunks(taskID: string): Promise<Blob[]> {
    const database = await this.database();
    if (!database) {
      return [...(this.memory.get(taskID)?.entries() || [])]
        .sort(([left], [right]) => left - right)
        .map(([, blob]) => blob);
    }
    return new Promise((resolve, reject) => {
      const transaction = database.transaction(storeName, 'readonly');
      const request = transaction.objectStore(storeName).index('taskID').getAll(IDBKeyRange.only(taskID));
      request.onsuccess = () => {
        const records = (request.result as StoredChunk[]).sort((left, right) => left.index - right.index);
        resolve(records.map((record) => record.blob));
      };
      request.onerror = () => reject(request.error || new Error('download_storage_read_failed'));
    });
  }

  async chunkSize(taskID: string): Promise<number> {
    const chunks = await this.listChunks(taskID);
    return chunks.reduce((total, chunk) => total + chunk.size, 0);
  }

  async clearChunks(taskID: string): Promise<void> {
    this.memory.delete(taskID);
    const database = await this.database();
    if (!database) return;
    await new Promise<void>((resolve, reject) => {
      const transaction = database.transaction(storeName, 'readwrite');
      const index = transaction.objectStore(storeName).index('taskID');
      const request = index.openKeyCursor(IDBKeyRange.only(taskID));
      request.onsuccess = () => {
        const cursor = request.result;
        if (!cursor) return;
        transaction.objectStore(storeName).delete(cursor.primaryKey);
        cursor.continue();
      };
      transaction.oncomplete = () => resolve();
      transaction.onerror = () => reject(transaction.error || new Error('download_storage_clear_failed'));
      transaction.onabort = () => reject(transaction.error || new Error('download_storage_clear_failed'));
    });
  }

  private database(): Promise<IDBDatabase | null> {
    if (this.databasePromise) return this.databasePromise;
    this.databasePromise = openDatabase().catch(() => null);
    return this.databasePromise;
  }
}

function openDatabase(): Promise<IDBDatabase | null> {
  if (typeof indexedDB === 'undefined') return Promise.resolve(null);
  return new Promise((resolve, reject) => {
    const request = indexedDB.open(databaseName, 1);
    request.onupgradeneeded = () => {
      const database = request.result;
      if (database.objectStoreNames.contains(storeName)) return;
      const store = database.createObjectStore(storeName, { keyPath: ['taskID', 'index'] });
      store.createIndex('taskID', 'taskID');
    };
    request.onsuccess = () => resolve(request.result);
    request.onerror = () => reject(request.error || new Error('download_storage_open_failed'));
    request.onblocked = () => reject(new Error('download_storage_blocked'));
  });
}

function runTransaction(
  database: IDBDatabase,
  mode: IDBTransactionMode,
  action: (store: IDBObjectStore) => void,
): Promise<void> {
  return new Promise((resolve, reject) => {
    const transaction = database.transaction(storeName, mode);
    action(transaction.objectStore(storeName));
    transaction.oncomplete = () => resolve();
    transaction.onerror = () => reject(transaction.error || new Error('download_storage_write_failed'));
    transaction.onabort = () => reject(transaction.error || new Error('download_storage_write_failed'));
  });
}
