export interface DateRange {
  start: string;
  end: string;
}

export function todayString(date = new Date()): string {
  return formatLocalDate(date);
}

export function parseLocalDate(value: string): Date {
  const [year, month, day] = String(value || todayString()).split('-').map(Number);
  return new Date(year, (month || 1) - 1, day || 1);
}

export function formatLocalDate(date: Date): string {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, '0');
  const day = String(date.getDate()).padStart(2, '0');
  return `${year}-${month}-${day}`;
}

export function currentCalendarWeekRange(value = todayString()): DateRange {
  const start = parseLocalDate(value);
  start.setDate(start.getDate() - start.getDay());
  const end = new Date(start);
  end.setDate(start.getDate() + 6);
  return { start: formatLocalDate(start), end: formatLocalDate(end) };
}

export function currentMonthString(date = new Date()): string {
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}`;
}

export function formatMonthLabel(month: string): string {
  const [year, value] = String(month || currentMonthString()).split('-').map(Number);
  return `${year}年${value}月`;
}

export function dayOffsetFrom(startDate: string, date: string): number {
  const start = new Date(`${startDate}T12:00:00`);
  const current = new Date(`${date}T12:00:00`);
  return Math.floor((current.getTime() - start.getTime()) / 86400000);
}

export function numberToChinese(input: number): string {
  const digits = ['零', '一', '二', '三', '四', '五', '六', '七', '八', '九'];
  const value = Number(input || 0);
  if (value <= 10) return value === 10 ? '十' : digits[value];
  if (value < 20) return `十${digits[value - 10]}`;
  if (value < 100) {
    const tens = Math.floor(value / 10);
    const ones = value % 10;
    return `${digits[tens]}十${ones ? digits[ones] : ''}`;
  }
  return String(value);
}

export function toChineseMonthDay(value: string): string {
  const current = parseLocalDate(value);
  const months = ['一', '二', '三', '四', '五', '六', '七', '八', '九', '十', '十一', '十二'];
  return `${months[current.getMonth()]}月${numberToChinese(current.getDate())}号`;
}
