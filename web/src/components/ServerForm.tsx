import { useState, type FormEvent } from "react";

interface ServerFormProps {
  disabled: boolean;
  submitting: boolean;
  onSubmit: (name: string) => Promise<void>;
  onCancel: () => void;
}

export function ServerForm({
  disabled,
  submitting,
  onSubmit,
  onCancel,
}: ServerFormProps) {
  const [name, setName] = useState("");
  const trimmedName = name.trim();

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (trimmedName === "" || disabled) {
      return;
    }
    void onSubmit(trimmedName);
  };

  return (
    <form className="server-create-form" onSubmit={submit}>
      <div className="field">
        <label htmlFor="server-name">服务器名称</label>
        <input
          id="server-name"
          autoFocus
          autoComplete="off"
          disabled={disabled}
          maxLength={120}
          placeholder="例如 home-lab"
          value={name}
          onChange={(event) => setName(event.target.value)}
        />
        <p className="field-hint">使用容易辨认的名称，之后可以随时重命名。</p>
      </div>
      <div className="management-actions">
        <button
          className="button button-primary"
          type="submit"
          disabled={disabled || trimmedName === ""}
        >
          {submitting ? "正在创建…" : "创建"}
        </button>
        <button
          className="button button-quiet"
          type="button"
          disabled={disabled}
          onClick={onCancel}
        >
          取消
        </button>
      </div>
    </form>
  );
}
