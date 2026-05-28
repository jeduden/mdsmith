// Hand-rolled JSON-RPC client for `mdsmith lsp`.
//
// Obsidian has no `vscode-languageclient` analog; plan 214 owns the
// framing, request correlation, and notification dispatch on the
// plugin side. Roughly 100 lines of plain TypeScript over a `Writable`
// + `Readable` pair, so the client works equally over a child process
// stdio pair or an in-memory `Duplex` (used in tests).
//
// Wire format follows the LSP base protocol:
//   - Each frame is `Content-Length: <N>\r\n\r\n<body>`.
//   - <N> is the UTF-8 byte length of <body> (not the JS .length).
//   - <body> is a single JSON object.
//
// The client tags every request with a monotonic integer id. Pending
// requests live in a map keyed by id; responses look the entry up,
// resolve or reject it, and free the slot. Notifications dispatch via
// a `method → handler[]` map.

import type { Readable, Writable } from "node:stream";

// JsonRpcRequest carries a `method` plus a `params` blob; an `id`
// elevates it from notification to request. Per JSON-RPC 2.0, a
// missing id signals "do not reply".
export interface JsonRpcRequest {
  jsonrpc: "2.0";
  id?: number;
  method: string;
  params?: unknown;
}

// JsonRpcResponse carries the matching `id` plus either `result` or
// `error`. The client treats `error` as a rejection and surfaces the
// `message` as the Error string.
export interface JsonRpcResponse {
  jsonrpc?: "2.0";
  id?: number;
  result?: unknown;
  error?: { code: number; message: string; data?: unknown };
}

// JsonRpcNotification is a server-sent message without an id. The
// client routes by `method` to registered handlers; one method can
// have multiple handlers (none today, but cheap to allow).
export interface JsonRpcNotification {
  jsonrpc?: "2.0";
  method: string;
  params?: unknown;
}

// encodeFrame serializes one outgoing message. The header has to use
// the UTF-8 byte length of the JSON body, not the JS .length — string
// length and byte length diverge as soon as the payload contains any
// multi-byte character (very common in diagnostic messages).
export function encodeFrame(msg: unknown): Buffer {
  const body = Buffer.from(JSON.stringify(msg), "utf8");
  const header = Buffer.from(`Content-Length: ${body.length}\r\n\r\n`, "utf8");
  return Buffer.concat([header, body]);
}

// parseFrames extracts as many complete frames as possible from `buf`,
// returning the leftover bytes for the caller to prepend to the next
// chunk. Designed so a single stream chunk can carry zero, one, or
// many frames, and a frame can also straddle two chunks — the caller
// is responsible for stitching them together with the returned `rest`.
export function parseFrames(buf: Buffer): {
  frames: unknown[];
  rest: Buffer;
} {
  const frames: unknown[] = [];
  let cursor = 0;
  while (cursor < buf.length) {
    const sep = buf.indexOf("\r\n\r\n", cursor, "utf8");
    if (sep < 0) break;
    const header = buf.subarray(cursor, sep).toString("utf8");
    // Multiple headers may stack (Content-Type, etc.); pick the one
    // we care about. The LSP base protocol mandates Content-Length.
    const m = /Content-Length:\s*(\d+)/i.exec(header);
    if (!m) {
      // Malformed header; drop everything up to and including the
      // separator and try again. Defensive — the mdsmith server
      // would never emit this.
      cursor = sep + 4;
      continue;
    }
    const length = Number(m[1]);
    const bodyStart = sep + 4;
    const bodyEnd = bodyStart + length;
    if (bodyEnd > buf.length) break; // body not yet fully delivered
    const body = buf.subarray(bodyStart, bodyEnd).toString("utf8");
    try {
      frames.push(JSON.parse(body));
    } catch {
      // Drop a single corrupt frame rather than locking the parser.
    }
    cursor = bodyEnd;
  }
  return { frames, rest: buf.subarray(cursor) };
}

// Pending is the resolve/reject pair for one outstanding request.
type Pending = {
  resolve: (value: unknown) => void;
  reject: (err: Error) => void;
};

// NotificationHandler is the receiver registered through
// `onNotification(method, handler)`. Params arrive parsed.
export type NotificationHandler = (params: unknown) => void;

// LspClient owns the JSON-RPC surface to a single mdsmith lsp server
// process. The constructor takes an arbitrary Writable/Readable pair
// so production code passes child.stdin/child.stdout and tests pass
// in-memory streams.
export class LspClient {
  private nextId = 1;
  private pending = new Map<number, Pending>();
  private handlers = new Map<string, NotificationHandler[]>();
  private buffer = Buffer.alloc(0);

  constructor(
    private readonly out: Writable,
    private readonly inp: Readable,
  ) {
    inp.on("data", (chunk: Buffer) => this.onData(chunk));
  }

  // request sends a JSON-RPC request and resolves with the `result`
  // field of the matching response (or rejects with the `error`).
  request(method: string, params?: unknown): Promise<unknown> {
    const id = this.nextId++;
    const frame = encodeFrame({ jsonrpc: "2.0", id, method, params });
    return new Promise<unknown>((resolve, reject) => {
      this.pending.set(id, { resolve, reject });
      this.out.write(frame);
    });
  }

  // notify sends a JSON-RPC notification — no id, no reply expected.
  notify(method: string, params?: unknown): void {
    const frame = encodeFrame({ jsonrpc: "2.0", method, params });
    this.out.write(frame);
  }

  // onNotification registers a handler for a method. Multiple
  // handlers per method are allowed; they are called in registration
  // order.
  onNotification(method: string, handler: NotificationHandler): void {
    const list = this.handlers.get(method);
    if (list) {
      list.push(handler);
    } else {
      this.handlers.set(method, [handler]);
    }
  }

  // initialize drives the LSP handshake: `initialize` followed by an
  // `initialized` notification once the server replies. Subsequent
  // requests should not be sent before this promise resolves.
  async initialize(params: unknown): Promise<unknown> {
    const result = await this.request("initialize", params);
    this.notify("initialized", {});
    return result;
  }

  // shutdown drives the LSP teardown handshake: `shutdown` request
  // then `exit` notification. The server is expected to terminate
  // shortly after `exit`; callers should still kill the child after
  // a short timeout in case it does not.
  async shutdown(): Promise<void> {
    try {
      await this.request("shutdown");
    } finally {
      // Always send `exit` even if `shutdown` rejected — the spec
      // says the server must exit on `exit` regardless of the
      // shutdown handshake state.
      this.notify("exit");
    }
  }

  // onData appends the incoming chunk to the rolling buffer and
  // drains as many complete frames as the buffer holds.
  private onData(chunk: Buffer): void {
    this.buffer = this.buffer.length === 0 ? chunk : Buffer.concat([this.buffer, chunk]);
    const { frames, rest } = parseFrames(this.buffer);
    this.buffer = rest;
    for (const frame of frames) {
      this.dispatch(frame);
    }
  }

  // dispatch routes a parsed frame to either a pending request slot
  // (when `id` is set) or to the notification handlers (when only
  // `method` is set). Server-to-client requests (id + method) are
  // not used by mdsmith today and silently no-op.
  private dispatch(frame: unknown): void {
    if (!frame || typeof frame !== "object") return;
    const msg = frame as JsonRpcResponse & JsonRpcNotification;
    if (typeof msg.id === "number" && msg.method === undefined) {
      const pending = this.pending.get(msg.id);
      if (!pending) return; // late reply for a cleared id
      this.pending.delete(msg.id);
      if (msg.error) {
        pending.reject(new Error(msg.error.message));
      } else {
        pending.resolve(msg.result);
      }
      return;
    }
    if (typeof msg.method === "string") {
      const list = this.handlers.get(msg.method);
      if (!list) return;
      for (const h of list) {
        try {
          h(msg.params);
        } catch {
          // Handler exceptions must not break the dispatch loop —
          // one bad listener should not stall every other listener.
        }
      }
    }
  }
}
