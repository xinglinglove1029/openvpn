import { useState } from 'react';
import { AlertTriangle } from 'lucide-react';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/ui/alert-dialog';
import { useIsMobile } from '@/hooks/useIsMobile';
import { cn } from '@/lib/utils';

export interface ConfirmState {
  title: string;
  message: string;
  danger?: boolean;
  onConfirm: () => void | Promise<void>;
}

export function ConfirmDialog({
  state,
  onClose,
}: {
  state: ConfirmState;
  onClose: () => void;
}) {
  const [saving, setSaving] = useState(false);
  const isMobile = useIsMobile();

  async function handleConfirm() {
    setSaving(true);
    try {
      await state.onConfirm();
      onClose();
    } catch {
      // 错误由调用方 notify 处理
    } finally {
      setSaving(false);
    }
  }

  const isOpen = !!state && (state.title !== '' || state.message !== '');

  return (
    <AlertDialog open={isOpen} onOpenChange={(open) => !open && onClose()}>
      <AlertDialogContent
        className={cn(
          isMobile && 'w-[95vw] max-w-[95vw] p-4 gap-3',
        )}
      >
        <AlertDialogHeader>
          <AlertDialogTitle
            className={cn(
              'flex items-center gap-2',
              isMobile && 'flex-col text-center gap-2',
            )}
          >
            <AlertTriangle
              className={cn(
                state.danger ? 'text-destructive' : 'text-warning',
                isMobile ? 'h-6 w-6' : 'h-5 w-5',
              )}
            />
            {state.title}
          </AlertDialogTitle>
          <AlertDialogDescription
            className={cn(
              isMobile && 'text-xs',
            )}
          >
            {state.message}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter
          className={cn(
            isMobile && 'flex-col gap-2 sm:flex-row sm:justify-end sm:space-x-0',
          )}
        >
          <AlertDialogCancel
            disabled={saving}
            className={cn(isMobile && 'w-full')}
          >
            取消
          </AlertDialogCancel>
          <AlertDialogAction
            onClick={handleConfirm}
            disabled={saving}
            className={cn(
              state.danger && 'bg-destructive text-destructive-foreground hover:bg-destructive/90',
              isMobile && 'w-full',
            )}
          >
            {saving ? '处理中...' : '确认执行'}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
