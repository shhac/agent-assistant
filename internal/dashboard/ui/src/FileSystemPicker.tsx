import { useEffect, useRef, useState, type FormEvent } from "react";
import { useTree } from "@headless-tree/react";
import {
  asyncDataLoaderFeature,
  buildProxiedInstance,
  hotkeysCoreFeature,
  selectionFeature,
  type TreeInstance,
} from "@headless-tree/core";
import { useVirtualizer } from "@tanstack/react-virtual";
import {
  api,
  APIError,
  errorText,
  type FileSystemEntry,
  type FileSystemPage,
  type FileSystemKind,
} from "./api";

export interface FileSystemPickerProps {
  kind: FileSystemKind;
  multiple?: boolean;
  initialPath?: string;
  onSelect: (paths: string[]) => void;
  onCancel: () => void;
}
export function listDirectory(
  path: string,
  kind: FileSystemKind,
  hidden: boolean,
  signal?: AbortSignal,
  cursor?: string,
) {
  const params = new URLSearchParams({ path, kind, hidden: String(hidden) });
  if (cursor) params.set("cursor", cursor);
  return api<FileSystemPage>(`/api/filesystem?${params}`, { signal });
}

/** Browses the daemon's filesystem; no browser upload or local filesystem API. */
export function FileSystemPicker({
  kind,
  multiple = false,
  initialPath = "",
  onSelect,
  onCancel,
}: FileSystemPickerProps) {
  const dialog = useRef<HTMLDialogElement>(null);
  const [target, setTarget] = useState(initialPath);
  const [draft, setDraft] = useState(initialPath);
  const [hidden, setHidden] = useState(false);
  const [revision, setRevision] = useState(0);
  const [page, setPage] = useState<FileSystemPage | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [selected, setSelected] = useState<string[]>([]);
  useEffect(() => {
    const element = dialog.current;
    element?.showModal();
    return () => element?.close();
  }, []);
  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setError("");
    setPage(null);
    listDirectory(target, kind, hidden, controller.signal)
      .then((value) => {
        if (!controller.signal.aborted) {
          setPage(value);
          setDraft(value.path);
        }
      })
      .catch((error) => {
        if (!controller.signal.aborted) setError(errorText(error));
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [target, kind, hidden, revision]);
  function jump(e: FormEvent) {
    e.preventDefault();
    setTarget(draft.trim());
    setRevision((value) => value + 1);
  }
  function chooseCurrent() {
    if (!page) return;
    setSelected((previous) =>
      multiple ? [...new Set([...previous, page.path])] : [page.path],
    );
  }
  return (
    <dialog
      ref={dialog}
      className="filesystem-dialog"
      aria-labelledby="filesystem-title"
      onCancel={(e) => {
        e.preventDefault();
        onCancel();
      }}
    >
      <div className="filesystem-header">
        <div>
          <p className="eyebrow">ON THE DAEMON’S COMPUTER</p>
          <h2 id="filesystem-title">
            Choose{" "}
            {kind === "directory"
              ? "folders"
              : kind === "file"
                ? "files"
                : "files or folders"}
          </h2>
        </div>
        <button
          type="button"
          className="icon-button"
          aria-label="Close filesystem picker"
          onClick={onCancel}
        >
          ×
        </button>
      </div>
      <div className="filesystem-body">
        <p className="field-hint">
          Browse one folder at a time. File contents stay unopened.
        </p>
        <form className="filesystem-location" onSubmit={jump}>
          <label htmlFor="filesystem-path">
            Folder path
            <input
              id="filesystem-path"
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              placeholder="Server home directory"
              spellCheck={false}
            />
          </label>
          <button className="button secondary" disabled={loading}>
            Go
          </button>
        </form>
        <div className="filesystem-toolbar">
          <button
            type="button"
            className="text-button"
            disabled={loading || !page?.parent}
            onClick={() => page?.parent && setTarget(page.parent)}
          >
            ↑ Parent folder
          </button>
          <label className="filesystem-hidden">
            <input
              type="checkbox"
              checked={hidden}
              onChange={(e) => setHidden(e.target.checked)}
            />
            Show hidden entries
          </label>
        </div>
        {error && (
          <div className="error-notice" role="alert">
            {error}
            <button
              type="button"
              className="text-button"
              onClick={() => setRevision((value) => value + 1)}
            >
              Retry folder
            </button>
          </div>
        )}
        {loading && (
          <div className="filesystem-loading" role="status">
            Loading folder…
          </div>
        )}
        {page && !loading && (
          <DirectoryTree
            key={`${page.path}:${hidden}:${revision}`}
            page={page}
            kind={kind}
            hidden={hidden}
            multiple={multiple}
            selected={selected}
            onChange={setSelected}
          />
        )}
        {page && kind !== "file" && (
          <div className="filesystem-current-row">
            <p className="field-hint">
              Clicking a folder in the list <strong>opens</strong> it. To choose
              the folder you are looking at, use the button.
            </p>
            <button
              type="button"
              className="button secondary filesystem-current"
              onClick={chooseCurrent}
            >
              Select this folder: <span>{page.path}</span>
            </button>
          </div>
        )}
        <div className="filesystem-selection" aria-live="polite">
          <strong>
            {selected.length
              ? `${selected.length} selected`
              : "Nothing selected"}
          </strong>
          {selected.length > 0 && (
            <ul>
              {selected.map((path) => (
                <li key={path}>
                  <span title={path}>{path}</span>
                  <button
                    type="button"
                    aria-label={`Remove selection ${path}`}
                    onClick={() =>
                      setSelected((previous) =>
                        previous.filter((value) => value !== path),
                      )
                    }
                  >
                    ×
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>
      </div>
      <div className="dialog-footer">
        <p>
          {multiple
            ? "Click an entry to select it and open it; Shift selects a range. Arrow keys navigate, Enter opens, and the button above selects the folder you are in."
            : "Click an entry to select it. Arrow keys navigate, Enter opens, and the button above selects the folder you are in."}
        </p>
        <button type="button" className="button secondary" onClick={onCancel}>
          Cancel
        </button>
        <button
          type="button"
          className="button primary"
          disabled={!selected.length}
          onClick={() => onSelect(selected)}
        >
          Use selection
        </button>
      </div>
    </dialog>
  );
}

type TreeData = FileSystemEntry & {
  action?: "more" | "retry";
  directory?: string;
  ownerID?: string;
  detail?: string;
};
function DirectoryTree({
  page,
  kind,
  hidden,
  multiple,
  selected,
  onChange,
}: {
  page: FileSystemPage;
  kind: FileSystemKind;
  hidden: boolean;
  multiple: boolean;
  selected: string[];
  onChange: (paths: string[]) => void;
}) {
  const scroll = useRef<HTMLDivElement>(null);
  const pages = useRef(new Map([["root", page]]));
  const nodes = useRef(new Map<string, TreeData>());
  const controllers = useRef(new Set<AbortController>());
  const pending = useRef(new Set<string>());
  const [status, setStatus] = useState("");
  const treeRef = useRef<TreeInstance<TreeData> | null>(null);
  useEffect(
    () => () => {
      controllers.current.forEach((controller) => controller.abort());
    },
    [],
  );
  async function fetchPage(path: string, cursor?: string) {
    const controller = new AbortController();
    controllers.current.add(controller);
    try {
      return await listDirectory(path, kind, hidden, controller.signal, cursor);
    } finally {
      controllers.current.delete(controller);
    }
  }
  function dataRows(value: FileSystemPage, ownerID: string) {
    const rows: { id: string; data: TreeData }[] = value.entries.map(
      (data) => ({ id: `${ownerID}/${encodeURIComponent(data.path)}`, data }),
    );
    if (value.next_cursor)
      rows.push({
        id: `more:${ownerID}`,
        data: {
          name: "Load more entries",
          path: "",
          kind: "file",
          selectable: false,
          action: "more",
          directory: value.path,
          ownerID,
        },
      });
    rows.forEach((row) => nodes.current.set(row.id, row.data));
    return rows;
  }
  async function children(id: string) {
    const path = id === "root" ? page.path : nodes.current.get(id)?.path;
    if (!path) return [];
    try {
      let value = pages.current.get(id);
      if (!value) {
        value = await fetchPage(path);
        pages.current.set(id, value);
      }
      return dataRows(value, id);
    } catch (error) {
      const data: TreeData = {
        name: "Retry loading folder",
        path: "",
        kind: "file",
        selectable: false,
        action: "retry",
        directory: path,
        ownerID: id,
        detail: errorText(error),
      };
      const row = { id: `retry:${id}`, data };
      nodes.current.set(row.id, data);
      setStatus(data.detail || "Folder unavailable");
      return [row];
    }
  }
  async function activate(data: TreeData) {
    if (!data.action || !data.directory || pending.current.has(data.directory))
      return;
    const path = data.directory;
    const ownerID = data.ownerID || "root";
    pending.current.add(path);
    setStatus("Loading entries…");
    try {
      const previous = pages.current.get(ownerID);
      const next = await fetchPage(
        path,
        data.action === "more" ? previous?.next_cursor || undefined : undefined,
      );
      const seen = new Set<string>();
      const merged =
        previous && data.action === "more"
          ? {
              ...next,
              entries: [...previous.entries, ...next.entries].filter(
                (entry) => {
                  if (seen.has(entry.path)) return false;
                  seen.add(entry.path);
                  return true;
                },
              ),
            }
          : next;
      pages.current.set(ownerID, merged);
      const item = treeRef.current?.getItemInstance(ownerID);
      await item?.invalidateChildrenIds();
      setStatus("Entries loaded.");
    } catch (error) {
      if (error instanceof APIError && error.status === 409) {
        pages.current.delete(ownerID);
        setStatus(
          "This folder listing expired. Activate the row again to reload this folder.",
        );
        return;
      }
      setStatus(
        `Could not load entries: ${errorText(error)}. Activate the row to retry.`,
      );
    } finally {
      pending.current.delete(path);
    }
  }
  const tree = useTree<TreeData>({
    rootItemId: "root",
    instanceBuilder: buildProxiedInstance,
    state: {
      selectedItems: [...nodes.current]
        .filter(([, data]) => data.selectable && selected.includes(data.path))
        .map(([id]) => id),
    },
    setSelectedItems: (update) => {
      const previous = [...nodes.current]
        .filter(([, data]) => data.selectable && selected.includes(data.path))
        .map(([id]) => id);
      const next = typeof update === "function" ? update(previous) : update;
      const removed = new Set(
        previous
          .filter((id) => !next.includes(id))
          .map((id) => nodes.current.get(id)?.path),
      );
      const eligible = next
        .map((id) => nodes.current.get(id))
        .filter((data) => data?.selectable && !removed.has(data.path))
        .map((data) => data!.path);
      const outside = multiple
        ? selected.filter(
            (path) =>
              ![...nodes.current.values()].some((data) => data.path === path),
          )
        : [];
      const paths = [...new Set([...outside, ...eligible])];
      onChange(multiple ? paths : paths.slice(-1));
    },
    getItemName: (item) => item.getItemData()?.name || "Loading…",
    isItemFolder: (item) => item.getItemData()?.kind === "directory",
    createLoadingItemData: () => ({
      name: "Loading…",
      path: "",
      kind: "file",
      selectable: false,
    }),
    dataLoader: {
      getItem: async (id) =>
        nodes.current.get(id) || {
          name: page.path,
          path: page.path,
          kind: "directory",
          selectable: kind !== "file",
        },
      getChildrenWithData: children,
    },
    onPrimaryAction: (item) => {
      void activate(item.getItemData());
    },
    scrollToItem: (item) =>
      virtualizer.scrollToIndex(item.getItemMeta().index, { align: "auto" }),
    features: [asyncDataLoaderFeature, selectionFeature, hotkeysCoreFeature],
  });
  treeRef.current = tree;
  const items = tree.getItems();
  const virtualizer = useVirtualizer({
    count: items.length,
    getScrollElement: () => scroll.current,
    estimateSize: () => 38,
    overscan: 8,
    initialRect: { width: 600, height: 342 },
  });
  const containerProps = tree.getContainerProps("Server filesystem entries");
  return (
    <>
      <div
        {...containerProps}
        ref={(element) => {
          scroll.current = element;
          containerProps.ref?.(element);
        }}
        className="filesystem-tree"
        aria-multiselectable={multiple}
      >
        <div
          style={{
            height: virtualizer.getTotalSize(),
            width: "100%",
            position: "relative",
          }}
        >
          {virtualizer.getVirtualItems().map((row) => {
            const item = items[row.index],
              data = item.getItemData(),
              props = item.getProps();
            return (
              <button
                {...props}
                type="button"
                onClick={(event) => {
                  if (
                    !multiple ||
                    event.shiftKey ||
                    event.ctrlKey ||
                    event.metaKey ||
                    data.action
                  ) {
                    props.onClick?.(event);
                    return;
                  }
                  item.setFocused();
                  item.toggleSelect();
                  if (item.isFolder()) {
                    if (item.isExpanded()) item.collapse();
                    else item.expand();
                  }
                }}
                key={row.key}
                className={`filesystem-row ${item.isSelected() ? "selected" : ""} ${data.action ? "filesystem-action" : ""}`}
                style={{
                  position: "absolute",
                  top: 0,
                  left: 0,
                  width: "100%",
                  height: row.size,
                  transform: `translateY(${row.start}px)`,
                  paddingLeft: `${12 + item.getItemMeta().level * 18}px`,
                }}
                title={data.detail || data.path}
              >
                <span aria-hidden="true" className="filesystem-chevron">
                  {item.isFolder() ? (item.isExpanded() ? "▾" : "▸") : ""}
                </span>
                <span aria-hidden="true">
                  {data.action ? "↻" : item.isFolder() ? "▱" : "·"}
                </span>
                <span className="filesystem-name">{data.name}</span>
                {item.isLoading() && (
                  <span className="filesystem-row-hint">Loading…</span>
                )}
                {data.selectable && (
                  <span aria-hidden="true" className="filesystem-choice">
                    {item.isSelected() ? "✓" : ""}
                  </span>
                )}
              </button>
            );
          })}
        </div>
      </div>
      {!items.length && (
        <p className="field-hint">This folder has no matching entries.</p>
      )}
      <p className="filesystem-status" role="status">
        {status || "Only expanded folders are loaded."}
      </p>
    </>
  );
}
