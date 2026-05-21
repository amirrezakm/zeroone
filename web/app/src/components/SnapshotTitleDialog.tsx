import { useEffect, useRef, useState } from "react";
import { X } from "lucide-react";

type Props = {
  open: boolean;
  title: string;
  description?: string;
  defaultValue: string;
  confirmLabel: string;
  confirmIcon?: React.ReactNode;
  pending?: boolean;
  onCancel: () => void;
  onConfirm: (value: string) => void;
};

// Modal that captures a snapshot title before a mutating action. The
// default value is pre-filled and editable; the operator must keep at
// least one non-whitespace character.
export default function SnapshotTitleDialog({
  open,
  title,
  description,
  defaultValue,
  confirmLabel,
  confirmIcon,
  pending,
  onCancel,
  onConfirm,
}: Props) {
  const [value, setValue] = useState(defaultValue);
  const inputRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    if (!open) return;
    setValue(defaultValue);
    queueMicrotask(() => {
      inputRef.current?.focus();
      inputRef.current?.select();
    });
  }, [open, defaultValue]);

  if (!open) return null;
  const trimmed = value.trim();
  const submit = () => {
    if (!trimmed || pending) return;
    onConfirm(trimmed);
  };
  return (
    <div className="fixed inset-0 z-50 grid place-items-center bg-black/50 p-4" onClick={onCancel}>
      <form
        className="panel flex w-full max-w-md flex-col"
        onClick={(e) => e.stopPropagation()}
        onSubmit={(e) => {
          e.preventDefault();
          submit();
        }}
      >
        <div className="border-border dark:border-border-dark flex items-center justify-between border-b px-5 py-3">
          <div>
            <h2 className="font-semibold">{title}</h2>
            {description && (
              <p className="text-muted dark:text-muted-dark text-xs">{description}</p>
            )}
          </div>
          <button type="button" onClick={onCancel} className="btn px-2">
            <X size={14} />
          </button>
        </div>
        <div className="px-5 py-4">
          <label className="text-muted dark:text-muted-dark block text-xs">Snapshot title</label>
          <input
            ref={inputRef}
            type="text"
            className="input mt-1 w-full"
            value={value}
            onChange={(e) => setValue(e.target.value)}
            placeholder="e.g. Add CDN endpoint"
            maxLength={120}
          />
          <p className="text-muted dark:text-muted-dark mt-2 text-xs">
            A snapshot of stack.json + xray config will be saved under this title before the change
            is applied.
          </p>
        </div>
        <div className="border-border dark:border-border-dark flex items-center justify-end gap-2 border-t px-5 py-3">
          <button type="button" className="btn" onClick={onCancel}>
            Cancel
          </button>
          <button type="submit" className="btn btn-primary" disabled={!trimmed || pending}>
            {confirmIcon}
            {pending ? "Working…" : confirmLabel}
          </button>
        </div>
      </form>
    </div>
  );
}
