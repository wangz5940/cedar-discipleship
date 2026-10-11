export type MobileViewMode = 'masonry' | 'stacked';

export function normalizeMobileViewMode(value: unknown): MobileViewMode {
  return value === 'stacked' ? 'stacked' : 'masonry';
}

export function personalBookErrorMessage(error: Error): string {
  const messages: Record<string, string> = {
    invalid_personal_book: '请检查日期、书籍、标题及篇号或页码范围',
    personal_books_failed: '个人书籍操作失败，请稍后重试',
    asset_not_found: '当前书籍不可访问，请重新选择文件',
    group_membership_required: '请加入当前小组后设置个人书籍',
    personal_book_not_found: '当天个人书籍计划已删除，请刷新后重试',
    future_date_forbidden: '禁止打卡未来日期内容',
  };
  return messages[error.message] || error.message;
}
