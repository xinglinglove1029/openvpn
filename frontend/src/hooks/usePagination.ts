import { useEffect, useState } from 'react';

export function usePagination<T>(items: T[], resetKey = '', initialPageSize = 10) {
  const [page, setPage] = useState(1);
  const [pageSize, setPageSizeState] = useState(initialPageSize);
  const total = items.length;
  const pageCount = Math.max(1, Math.ceil(total / pageSize));
  const currentPage = Math.min(page, pageCount);
  const start = (currentPage - 1) * pageSize;
  const end = Math.min(start + pageSize, total);
  const pagedItems = items.slice(start, end);

  useEffect(() => { setPage(1); }, [resetKey, pageSize]);
  useEffect(() => { if (page !== currentPage) setPage(currentPage); }, [page, currentPage]);

  function setPageSize(next: number) { setPageSizeState(next); }

  return { page: currentPage, pageSize, setPageSize, total, pageCount, start, end, pagedItems, setPage };
}
