export const LOG_ID_HEADER = 'X-Log-ID';

let latestResponseLogID = '';

export function validLogID(value: unknown): value is string {
  return typeof value === 'string' && /^[0-9a-f]{32}$/.test(value);
}

export function createLogID(): string {
  const bytes = new Uint8Array(16);
  globalThis.crypto.getRandomValues(bytes);
  return Array.from(bytes, (value) => value.toString(16).padStart(2, '0')).join('');
}

export function requestLogID(value?: unknown): string {
  return validLogID(value) ? value : createLogID();
}

export function recordResponseLogID(value: unknown) {
  if (validLogID(value)) latestResponseLogID = value;
}

export function latestLogID(): string {
  return latestResponseLogID;
}

export function clearLatestLogID() {
  latestResponseLogID = '';
}
