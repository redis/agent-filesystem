import { Button } from "@redis-ui/components";
import { useCallback, useEffect, useRef, useState } from "react";
import type { FormEvent } from "react";
import styled, { keyframes } from "styled-components";
import type { SaveDatabaseInput } from "../foundation/types/afs";
import type { AFSDatabaseScopeRecord } from "../foundation/database-scope";
import {
  DialogActions,
  DialogBody,
  DialogCard,
  DialogCloseButton,
  DialogError,
  DialogHeader,
  DialogOverlay,
  DialogTitle,
  Field,
  FieldGroup,
  FormGrid,
  TextInput,
  TwoColumnFields,
} from "./afs-kit";

/**
 * Parses a Redis connection string into form components.
 * Supports formats like:
 *   redis-cli -u redis://user:pass@host:port/db
 *   redis://user:pass@host:port/db
 *   rediss://user:pass@host:port  (TLS)
 */
function parseRedisConnectionString(
  raw: string,
): Partial<DatabaseFormState> | null {
  let s = raw.trim();

  // Strip leading "redis-cli -u " or similar CLI prefixes
  s = s.replace(/^redis-cli\s+(-[a-zA-Z]\s+)*/, "");

  // Must look like a redis:// or rediss:// URI
  const match = s.match(/^(rediss?):\/\/(.+)$/);
  if (!match) return null;

  const scheme = match[1]; // "redis" or "rediss"
  const rest = match[2]; // user:pass@host:port/db

  let username = "";
  let password = "";
  let hostPort = "";
  let dbIndex = "";
  const useTLS = scheme === "rediss";

  // Split on last @ to separate credentials from host (password may contain @)
  const atIdx = rest.lastIndexOf("@");
  let hostAndPath: string;

  if (atIdx !== -1) {
    const creds = rest.slice(0, atIdx);
    hostAndPath = rest.slice(atIdx + 1);

    const colonIdx = creds.indexOf(":");
    if (colonIdx !== -1) {
      username = decodeURIComponent(creds.slice(0, colonIdx));
      password = decodeURIComponent(creds.slice(colonIdx + 1));
    } else {
      username = decodeURIComponent(creds);
    }
  } else {
    hostAndPath = rest;
  }

  // Split host:port from /db
  const slashIdx = hostAndPath.indexOf("/");
  if (slashIdx !== -1) {
    hostPort = hostAndPath.slice(0, slashIdx);
    const db = hostAndPath.slice(slashIdx + 1);
    if (/^\d+$/.test(db)) dbIndex = db;
  } else {
    hostPort = hostAndPath;
  }

  // Strip trailing query params if any
  hostPort = hostPort.split("?")[0];

  if (!hostPort) return null;

  return {
    endpointLabel: hostPort,
    username,
    password,
    ...(dbIndex ? { dbIndex } : {}),
    useTLS,
  };
}

type DatabaseFormState = {
  displayName: string;
  description: string;
  endpointLabel: string;
  dbIndex: string;
  username: string;
  password: string;
  useTLS: boolean;
};

function createInitialFormState(
  value?: Partial<DatabaseFormState>,
): DatabaseFormState {
  return {
    displayName: value?.displayName ?? "",
    description: value?.description ?? "",
    endpointLabel: value?.endpointLabel ?? "",
    dbIndex: value?.dbIndex ?? "0",
    username: value?.username ?? "",
    password: value?.password ?? "",
    useTLS: value?.useTLS ?? false,
  };
}

export type AddDatabaseDialogProps = {
  isOpen: boolean;
  onClose: () => void;
  saveDatabase: (input: SaveDatabaseInput) => Promise<void>;
  mode?: "create" | "edit";
  editingDatabase?: AFSDatabaseScopeRecord | null;
  onDelete?: (databaseId: string) => void;
};

export function AddDatabaseDialog({
  isOpen,
  onClose,
  saveDatabase,
  mode = "create",
  editingDatabase = null,
  onDelete,
}: AddDatabaseDialogProps) {
  const [formError, setFormError] = useState<string | null>(null);
  const [form, setForm] = useState<DatabaseFormState>(createInitialFormState());
  const [showParseBanner, setShowParseBanner] = useState(false);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const nameInputRef = useRef<HTMLInputElement>(null);
  const parseBannerTimer = useRef<ReturnType<typeof setTimeout>>();

  const handlePaste = useCallback(
    (event: React.ClipboardEvent<HTMLInputElement>) => {
      const text = event.clipboardData.getData("text/plain");
      const parsed = parseRedisConnectionString(text);
      if (!parsed) return;

      // Prevent the raw string from landing in the input
      event.preventDefault();

      setForm((current) => ({
        ...current,
        ...parsed,
        // Auto-fill name from host if empty
        displayName:
          current.displayName || parsed.endpointLabel || current.displayName,
      }));

      // Show banner
      setShowParseBanner(true);
      clearTimeout(parseBannerTimer.current);
      parseBannerTimer.current = setTimeout(
        () => setShowParseBanner(false),
        2500,
      );
    },
    [],
  );

  useEffect(() => {
    if (isOpen) {
      if (mode === "edit" && editingDatabase) {
        setForm(
          createInitialFormState({
            displayName: editingDatabase.displayName,
            description: editingDatabase.description,
            endpointLabel: editingDatabase.endpointLabel,
            dbIndex: editingDatabase.dbIndex,
            username: editingDatabase.username,
            password: editingDatabase.password,
            useTLS: editingDatabase.useTLS,
          }),
        );
      } else {
        setForm(createInitialFormState());
      }
      setFormError(null);
      setIsSubmitting(false);
      const id = setTimeout(() => nameInputRef.current?.focus(), 60);
      return () => clearTimeout(id);
    }
  }, [isOpen, mode, editingDatabase]);

  function updateForm<TKey extends keyof DatabaseFormState>(
    key: TKey,
    value: DatabaseFormState[TKey],
  ) {
    setForm((current) => ({ ...current, [key]: value }));
  }

  async function submitForm(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (form.displayName.trim() === "" || form.endpointLabel.trim() === "") {
      return;
    }
    if (isSubmitting) return;

    try {
      setFormError(null);
      setIsSubmitting(true);
      await saveDatabase({
        id: mode === "edit" ? editingDatabase?.id : undefined,
        name: form.displayName,
        description: form.description,
        redisAddr: form.endpointLabel,
        redisUsername: form.username,
        redisPassword: form.password,
        redisDB: Number.parseInt(form.dbIndex || "0", 10) || 0,
        redisTLS: form.useTLS,
      });
      onClose();
    } catch (error) {
      setFormError(
        error instanceof Error ? error.message : "Unable to save database.",
      );
    } finally {
      setIsSubmitting(false);
    }
  }

  if (!isOpen) return null;

  return (
    <DialogOverlay
      onClick={(event) => {
        if (isSubmitting) return;
        if (event.target === event.currentTarget) onClose();
      }}
    >
      <DialogCard>
        <DialogHeader>
          <div>
            <DialogTitle>
              {mode === "create"
                ? "Add database"
                : `Edit ${editingDatabase?.displayName || editingDatabase?.databaseName || "database"}`}
            </DialogTitle>
            <DialogBody>
              Configure how this database appears in the app and what connection
              settings it uses.
            </DialogBody>
          </div>
          <DialogCloseButton
            type="button"
            aria-label="Close"
            onClick={onClose}
            disabled={isSubmitting}
          >
            &times;
          </DialogCloseButton>
        </DialogHeader>

        <FormGrid onSubmit={submitForm}>
          {showParseBanner && (
            <ParseBanner>
              <ParseBannerIcon>⚡</ParseBannerIcon>
              Pasting Redis Connection String
            </ParseBanner>
          )}

          <Field>
            Display Name
            <TextInput
              ref={nameInputRef}
              value={form.displayName}
              onPaste={handlePaste}
              onChange={(event) =>
                updateForm("displayName", event.target.value)
              }
              placeholder="localhost:6388"
            />
          </Field>

          <Field>
            Description
            <TextInput
              value={form.description}
              onChange={(event) =>
                updateForm("description", event.target.value)
              }
              placeholder="Shared Claude workspace database for local agent state."
            />
          </Field>

          <FieldGroup title="Connection Details">
            <Field>
              Redis Hostname or IP
              <TextInput
                value={form.endpointLabel}
                onPaste={handlePaste}
                onChange={(event) =>
                  updateForm("endpointLabel", event.target.value)
                }
                placeholder="localhost:6379"
              />
            </Field>

            <TwoColumnFields>
              <Field>
                Username
                <TextInput
                  autoComplete="off"
                  value={form.username}
                  onPaste={handlePaste}
                  onChange={(event) => updateForm("username", event.target.value)}
                  placeholder="default"
                />
              </Field>
              <Field>
                Password
                <TextInput
                  autoComplete="off"
                  type="password"
                  value={form.password}
                  onPaste={handlePaste}
                  onChange={(event) => updateForm("password", event.target.value)}
                  placeholder="••••••••"
                />
              </Field>
            </TwoColumnFields>

            <InlineRow>
              <Field style={{ maxWidth: 100 }}>
                DB index
                <TextInput
                  type="number"
                  min="0"
                  max="15"
                  value={form.dbIndex}
                  onChange={(event) => updateForm("dbIndex", event.target.value)}
                  placeholder="0"
                />
              </Field>
              <ToggleLabel>
                <ToggleSwitch>
                  <input
                    checked={form.useTLS}
                    type="checkbox"
                    onChange={(event) =>
                      updateForm("useTLS", event.target.checked)
                    }
                  />
                  <ToggleTrack />
                </ToggleSwitch>
                Use TLS
              </ToggleLabel>
            </InlineRow>
          </FieldGroup>

          {formError ? (
            <DialogError role="alert">{formError}</DialogError>
          ) : null}

          <DialogActions>
            <DialogActionGroup>
              {mode === "edit" && editingDatabase && onDelete ? (
                <RemoveDatabaseButton
                  size="medium"
                  type="button"
                  onClick={() => onDelete(editingDatabase.id)}
                  disabled={isSubmitting}
                >
                  Remove database
                </RemoveDatabaseButton>
              ) : null}
            </DialogActionGroup>
            <DialogActionGroup>
              <Button
                size="medium"
                type="button"
                variant="secondary-fill"
                onClick={onClose}
                disabled={isSubmitting}
              >
                Cancel
              </Button>
              <Button size="medium" type="submit" disabled={isSubmitting}>
                <SubmitButtonContent>
                  {isSubmitting && <Spinner aria-hidden="true" />}
                  {isSubmitting
                    ? mode === "create"
                      ? "Adding database…"
                      : "Saving changes…"
                    : mode === "create"
                      ? "Add database"
                      : "Save changes"}
                </SubmitButtonContent>
              </Button>
            </DialogActionGroup>
          </DialogActions>
        </FormGrid>
      </DialogCard>
    </DialogOverlay>
  );
}

const DialogActionGroup = styled.div`
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  align-items: center;
`;

const RemoveDatabaseButton = styled(Button)`
  && {
    background: ${({ theme }) => theme.semantic.color.background.danger500};
    border-color: ${({ theme }) => theme.semantic.color.background.danger500};
    color: ${({ theme }) => theme.semantic.color.text.inverse};
  }

  &&:hover:not(:disabled) {
    background: ${({ theme }) => theme.semantic.color.background.danger600};
    border-color: ${({ theme }) => theme.semantic.color.background.danger600};
  }
`;

const InlineRow = styled.div`
  display: flex;
  align-items: flex-end;
  gap: 18px;
`;

const ToggleLabel = styled.label`
  display: inline-flex;
  align-items: center;
  gap: 10px;
  color: var(--afs-ink);
  font-size: 13px;
  font-weight: 700;
  cursor: pointer;
  padding-bottom: 10px;
`;

const ToggleSwitch = styled.span`
  position: relative;
  display: inline-flex;
  width: 36px;
  height: 20px;
  flex-shrink: 0;

  input {
    position: absolute;
    opacity: 0;
    width: 0;
    height: 0;
  }

  input:checked + span {
    background: var(--afs-focus, #2563eb);
  }

  input:checked + span::after {
    transform: translateX(16px);
  }

  input:focus-visible + span {
    box-shadow: 0 0 0 3px var(--afs-focus-soft);
  }
`;

const bannerSlideIn = keyframes`
  from { opacity: 0; transform: translateY(-6px); }
  to   { opacity: 1; transform: translateY(0); }
`;

const spin = keyframes`
  to { transform: rotate(360deg); }
`;

const SubmitButtonContent = styled.span`
  display: inline-flex;
  align-items: center;
  gap: 8px;
`;

const Spinner = styled.span`
  width: 14px;
  height: 14px;
  border-radius: 50%;
  border: 2px solid currentColor;
  border-top-color: transparent;
  opacity: 0.85;
  animation: ${spin} 700ms linear infinite;
  flex-shrink: 0;
`;

const ParseBanner = styled.div`
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  border-radius: 8px;
  font-size: 13px;
  font-weight: 600;
  color: var(--afs-focus, #2563eb);
  background: color-mix(in srgb, var(--afs-focus, #2563eb) 10%, transparent);
  border: 1px solid color-mix(in srgb, var(--afs-focus, #2563eb) 25%, transparent);
  animation: ${bannerSlideIn} 200ms ease-out;
`;

const ParseBannerIcon = styled.span`
  font-size: 15px;
  line-height: 1;
`;

const ToggleTrack = styled.span`
  position: absolute;
  inset: 0;
  border-radius: 999px;
  background: var(--afs-line-strong, #ccc);
  transition: background 160ms ease;
  cursor: pointer;

  &::after {
    content: "";
    position: absolute;
    top: 2px;
    left: 2px;
    width: 16px;
    height: 16px;
    border-radius: 50%;
    background: white;
    box-shadow: 0 1px 3px rgba(0, 0, 0, 0.15);
    transition: transform 160ms ease;
  }
`;
