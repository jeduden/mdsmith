// Unit tests for the hand-rolled JSON-RPC client.
//
// The client owns the JSON-RPC surface to `mdsmith lsp` — frames
// outgoing messages with `Content-Length` headers, parses incoming
// frames the same way, correlates request ids to pending promises,
// and fans notifications out to registered handlers. The lifecycle
// runs `initialize` then `initialized` before any other request, and
// drives `shutdown` + `exit` on stop.
//
// Tests drive the client against an in-process `Duplex` so we never
// spawn a real server. The Duplex feeds bytes the same way a child
// process's stdio would.

import { describe, expect, test } from "bun:test";
import { PassThrough } from "node:stream";

import { LspClient, encodeFrame, parseFrames } from "./lsp-client";

// makePair returns a pair of writable/readable PassThroughs the client
// can treat as the server stdio: the client writes to `serverIn` and
// reads from `serverOut`, while tests inspect `serverIn` (what the
// client sent) and push frames into `serverOut` (what the client should
// see).
function makePair(): {
  serverIn: PassThrough;
  serverOut: PassThrough;
} {
  return { serverIn: new PassThrough(), serverOut: new PassThrough() };
}

describe("encodeFrame", () => {
  test("prefixes a JSON body with Content-Length", () => {
    const out = encodeFrame({ jsonrpc: "2.0", id: 1, method: "test" });
    const text = out.toString("utf8");
    // The LSP spec mandates Content-Length plus a literal CRLF CRLF
    // separator; downstream LSPs reject LF-only framing.
    expect(text.startsWith("Content-Length: ")).toBe(true);
    expect(text).toContain("\r\n\r\n");
    const [header, body] = text.split("\r\n\r\n");
    const length = Number(header.replace("Content-Length: ", ""));
    // Header length must match the UTF-8 byte count of the body
    // (multi-byte characters would break a naive .length-based count).
    expect(Buffer.byteLength(body, "utf8")).toBe(length);
    expect(JSON.parse(body)).toEqual({ jsonrpc: "2.0", id: 1, method: "test" });
  });

  test("uses the UTF-8 byte length for multibyte payloads", () => {
    // Diagnostic messages can contain non-ASCII characters; a JS
    // .length count would under-report the Content-Length and the
    // server would hang waiting for more bytes.
    const out = encodeFrame({ msg: "héllo — 漢字" });
    const text = out.toString("utf8");
    const [header, body] = text.split("\r\n\r\n");
    const length = Number(header.replace("Content-Length: ", ""));
    expect(Buffer.byteLength(body, "utf8")).toBe(length);
  });
});

describe("parseFrames", () => {
  test("yields a single complete frame and keeps the remainder", () => {
    const body = JSON.stringify({ id: 1, result: "ok" });
    const buf = Buffer.from(
      `Content-Length: ${Buffer.byteLength(body, "utf8")}\r\n\r\n${body}`,
      "utf8",
    );
    const { frames, rest } = parseFrames(buf);
    expect(frames).toEqual([{ id: 1, result: "ok" }]);
    expect(rest.length).toBe(0);
  });

  test("yields multiple frames packed in one chunk", () => {
    const a = JSON.stringify({ id: 1, result: "a" });
    const b = JSON.stringify({ id: 2, result: "b" });
    const buf = Buffer.concat([
      Buffer.from(`Content-Length: ${a.length}\r\n\r\n${a}`, "utf8"),
      Buffer.from(`Content-Length: ${b.length}\r\n\r\n${b}`, "utf8"),
    ]);
    const { frames, rest } = parseFrames(buf);
    expect(frames).toEqual([
      { id: 1, result: "a" },
      { id: 2, result: "b" },
    ]);
    expect(rest.length).toBe(0);
  });

  test("returns the partial trailing bytes as 'rest' when a frame is incomplete", () => {
    // A real TCP-style stream can deliver a frame split across two
    // chunks. The parser must hand the partial bytes back so the
    // caller can prepend them to the next chunk.
    const body = JSON.stringify({ id: 1, result: "ok" });
    const full = `Content-Length: ${body.length}\r\n\r\n${body}`;
    // Cut mid-body — only half the payload arrived.
    const partial = Buffer.from(full.slice(0, full.length - 2), "utf8");
    const { frames, rest } = parseFrames(partial);
    expect(frames).toEqual([]);
    // The remainder is exactly the chunk we passed in — no parse
    // progress could be made without more bytes.
    expect(rest.equals(partial)).toBe(true);
  });
});

describe("LspClient request correlation", () => {
  test("resolves request(...) with the matching server response", async () => {
    const { serverIn, serverOut } = makePair();
    const client = new LspClient(serverIn, serverOut);
    const promise = client.request("textDocument/codeAction", { file: "x" });

    // Capture what the client sent so we can echo the same id back.
    const sentChunks: Buffer[] = [];
    serverIn.on("data", (b) => sentChunks.push(b));
    // Wait a microtask so the request frame lands.
    await new Promise<void>((r) => setImmediate(r));
    const text = Buffer.concat(sentChunks).toString("utf8");
    const body = text.split("\r\n\r\n").slice(1).join("\r\n\r\n");
    const req = JSON.parse(body);
    expect(req.method).toBe("textDocument/codeAction");
    expect(req.params).toEqual({ file: "x" });
    expect(typeof req.id).toBe("number");

    // Reply with the matching id; the promise should resolve with
    // the wire response's `result`.
    const reply = JSON.stringify({ jsonrpc: "2.0", id: req.id, result: 42 });
    serverOut.write(
      `Content-Length: ${reply.length}\r\n\r\n${reply}`,
    );
    const result = await promise;
    expect(result).toBe(42);
  });

  test("rejects request(...) with the server's error payload", async () => {
    const { serverIn, serverOut } = makePair();
    const client = new LspClient(serverIn, serverOut);
    const promise = client.request("textDocument/codeAction", {});

    // Read the id back.
    const sentChunks: Buffer[] = [];
    serverIn.on("data", (b) => sentChunks.push(b));
    await new Promise<void>((r) => setImmediate(r));
    const text = Buffer.concat(sentChunks).toString("utf8");
    const body = text.split("\r\n\r\n").slice(1).join("\r\n\r\n");
    const req = JSON.parse(body);
    const reply = JSON.stringify({
      jsonrpc: "2.0",
      id: req.id,
      error: { code: -32601, message: "method not found" },
    });
    serverOut.write(`Content-Length: ${reply.length}\r\n\r\n${reply}`);

    await expect(promise).rejects.toThrow("method not found");
  });

  test("dispatches notifications to the registered handler", async () => {
    const { serverIn, serverOut } = makePair();
    const client = new LspClient(serverIn, serverOut);
    const received: unknown[] = [];
    client.onNotification("textDocument/publishDiagnostics", (params) =>
      received.push(params),
    );

    const notif = JSON.stringify({
      jsonrpc: "2.0",
      method: "textDocument/publishDiagnostics",
      params: { uri: "file:///a.md", diagnostics: [] },
    });
    serverOut.write(`Content-Length: ${notif.length}\r\n\r\n${notif}`);
    await new Promise<void>((r) => setImmediate(r));
    expect(received).toEqual([{ uri: "file:///a.md", diagnostics: [] }]);
  });

  test("survives a frame split across two chunks", async () => {
    const { serverIn, serverOut } = makePair();
    const client = new LspClient(serverIn, serverOut);
    const promise = client.request("test", {});
    const sentChunks: Buffer[] = [];
    serverIn.on("data", (b) => sentChunks.push(b));
    await new Promise<void>((r) => setImmediate(r));
    const text = Buffer.concat(sentChunks).toString("utf8");
    const body = text.split("\r\n\r\n").slice(1).join("\r\n\r\n");
    const req = JSON.parse(body);

    // Reply in two chunks: the header alone, then the body. The
    // parser must buffer the partial frame until the body arrives.
    const reply = JSON.stringify({ jsonrpc: "2.0", id: req.id, result: "ok" });
    serverOut.write(`Content-Length: ${reply.length}\r\n\r\n`);
    await new Promise<void>((r) => setImmediate(r));
    serverOut.write(reply);
    const result = await promise;
    expect(result).toBe("ok");
  });

  test("notify(...) writes a request without an id", async () => {
    const { serverIn, serverOut } = makePair();
    const client = new LspClient(serverIn, serverOut);
    const sentChunks: Buffer[] = [];
    serverIn.on("data", (b) => sentChunks.push(b));
    client.notify("textDocument/didChange", { uri: "x", version: 1 });
    await new Promise<void>((r) => setImmediate(r));
    const text = Buffer.concat(sentChunks).toString("utf8");
    const body = text.split("\r\n\r\n").slice(1).join("\r\n\r\n");
    const parsed = JSON.parse(body);
    // Notifications carry no id (per JSON-RPC) so the server knows
    // not to send a reply.
    expect(parsed.id).toBeUndefined();
    expect(parsed.method).toBe("textDocument/didChange");
    expect(parsed.params).toEqual({ uri: "x", version: 1 });
  });

  test("monotonically increments ids across concurrent requests", async () => {
    const { serverIn, serverOut } = makePair();
    const client = new LspClient(serverIn, serverOut);
    const sentChunks: Buffer[] = [];
    serverIn.on("data", (b) => sentChunks.push(b));
    client.request("a", {});
    client.request("b", {});
    client.request("c", {});
    await new Promise<void>((r) => setImmediate(r));
    const text = Buffer.concat(sentChunks).toString("utf8");
    // Three Content-Length sections; the bodies are the JSON between
    // each `\r\n\r\n` and the next `Content-Length:` header.
    const parts = text.split("Content-Length: ").filter(Boolean);
    const ids: number[] = [];
    for (const p of parts) {
      const body = p.split("\r\n\r\n").slice(1).join("\r\n\r\n");
      ids.push(JSON.parse(body).id as number);
    }
    expect(ids).toEqual([ids[0], ids[0] + 1, ids[0] + 2]);
  });
});

describe("LspClient.initialize", () => {
  test("sends initialize then a one-shot initialized notification", async () => {
    const { serverIn, serverOut } = makePair();
    const client = new LspClient(serverIn, serverOut);
    const sentChunks: Buffer[] = [];
    serverIn.on("data", (b) => sentChunks.push(b));
    const promise = client.initialize({
      processId: process.pid,
      rootUri: "file:///vault",
      capabilities: {},
    });

    await new Promise<void>((r) => setImmediate(r));
    // First out: the initialize request.
    const firstBody = Buffer.concat(sentChunks)
      .toString("utf8")
      .split("Content-Length: ")
      .filter(Boolean)[0]
      .split("\r\n\r\n")
      .slice(1)
      .join("\r\n\r\n");
    const initReq = JSON.parse(firstBody);
    expect(initReq.method).toBe("initialize");
    expect(typeof initReq.id).toBe("number");

    // Reply with empty capabilities; the client should then push the
    // `initialized` notification.
    const reply = JSON.stringify({
      jsonrpc: "2.0",
      id: initReq.id,
      result: { capabilities: {} },
    });
    serverOut.write(`Content-Length: ${reply.length}\r\n\r\n${reply}`);
    await promise;

    // Drain to capture the post-reply send.
    await new Promise<void>((r) => setImmediate(r));
    const allBodies = Buffer.concat(sentChunks)
      .toString("utf8")
      .split("Content-Length: ")
      .filter(Boolean)
      .map((p) => JSON.parse(p.split("\r\n\r\n").slice(1).join("\r\n\r\n")));
    const methods = allBodies.map((m) => m.method);
    expect(methods).toEqual(["initialize", "initialized"]);
  });
});
