// Drives the wiring helper.
//
// `Wiring` is the seam between Obsidian's plugin lifecycle and the
// pieces of plan 214. main.ts instantiates one Wiring per plugin
// load, hands it spawned-child seams (an LspClient + a kill
// function), and calls start/stop. The wiring:
//
//   - Runs the LSP `initialize` handshake during start().
//   - Registers the diagnostics notification listener so incoming
//     `publishDiagnostics` payloads land in a per-URI map.
//   - Wraps the buffer-modify event into a `vault.on('modify')`-
//     style debounced callback when fixOnSave is on.
//   - Drives `shutdown` + `exit` and the kill callback on stop().

import { describe, expect, test } from "bun:test";
import { PassThrough } from "node:stream";

import { LspClient } from "./lsp-client";
import { Wiring, type WiringDeps } from "./wiring";

function makeServerPair(): {
  client: LspClient;
  serverIn: PassThrough;
  serverOut: PassThrough;
} {
  const serverIn = new PassThrough();
  const serverOut = new PassThrough();
  const client = new LspClient(serverIn, serverOut);
  return { client, serverIn, serverOut };
}

async function tick(): Promise<void> {
  await new Promise<void>((r) => setImmediate(r));
}

describe("Wiring.start", () => {
  test("sends the LSP initialize handshake and resolves", async () => {
    const { client, serverIn, serverOut } = makeServerPair();
    const sent: Buffer[] = [];
    serverIn.on("data", (b) => sent.push(b));
    const killed: number[] = [];
    const deps: WiringDeps = {
      client,
      killChild: () => killed.push(Date.now()),
      rootUri: "file:///vault",
      onPublishDiagnostics: () => {},
    };
    const wiring = new Wiring(deps);
    const startPromise = wiring.start();

    await tick();
    // First frame is the initialize request. Reply with empty caps.
    const initBody = Buffer.concat(sent)
      .toString("utf8")
      .split("Content-Length: ")
      .filter(Boolean)[0]
      .split("\r\n\r\n")
      .slice(1)
      .join("\r\n\r\n");
    const init = JSON.parse(initBody);
    expect(init.method).toBe("initialize");
    expect(init.params.rootUri).toBe("file:///vault");
    expect(init.params.capabilities).toBeDefined();
    const reply = JSON.stringify({
      jsonrpc: "2.0",
      id: init.id,
      result: { capabilities: {} },
    });
    serverOut.write(`Content-Length: ${reply.length}\r\n\r\n${reply}`);
    await startPromise;
    expect(killed).toEqual([]);
  });

  test("routes publishDiagnostics into the supplied handler", async () => {
    const { client, serverIn, serverOut } = makeServerPair();
    const received: Array<{ uri: string; diagnostics: unknown[] }> = [];
    const wiring = new Wiring({
      client,
      killChild: () => {},
      rootUri: "file:///vault",
      onPublishDiagnostics: (uri, diagnostics) =>
        received.push({ uri, diagnostics }),
    });
    const sent: Buffer[] = [];
    serverIn.on("data", (b) => sent.push(b));
    const startPromise = wiring.start();
    await tick();
    const sentText = Buffer.concat(sent).toString("utf8");
    const body = sentText
      .split("Content-Length: ")
      .filter(Boolean)[0]
      .split("\r\n\r\n")
      .slice(1)
      .join("\r\n\r\n");
    const init = JSON.parse(body);
    const reply = JSON.stringify({
      jsonrpc: "2.0",
      id: init.id,
      result: { capabilities: {} },
    });
    serverOut.write(`Content-Length: ${reply.length}\r\n\r\n${reply}`);
    await startPromise;

    // Now push a publishDiagnostics notification.
    const notif = JSON.stringify({
      jsonrpc: "2.0",
      method: "textDocument/publishDiagnostics",
      params: {
        uri: "file:///vault/a.md",
        diagnostics: [{ message: "x", range: { start: { line: 0, character: 0 }, end: { line: 0, character: 1 } } }],
      },
    });
    serverOut.write(`Content-Length: ${notif.length}\r\n\r\n${notif}`);
    await tick();
    expect(received.length).toBe(1);
    expect(received[0].uri).toBe("file:///vault/a.md");
    expect(received[0].diagnostics).toHaveLength(1);
  });
});

describe("Wiring.stop", () => {
  test("sends shutdown + exit and calls killChild", async () => {
    const { client, serverIn, serverOut } = makeServerPair();
    const killed: string[] = [];
    const wiring = new Wiring({
      client,
      killChild: () => killed.push("kill"),
      rootUri: "file:///vault",
      onPublishDiagnostics: () => {},
    });

    // Race the start so the wiring is past initialize before stop.
    const startPromise = wiring.start();
    const sentChunks: Buffer[] = [];
    serverIn.on("data", (b) => sentChunks.push(b));
    await tick();
    const initBody = Buffer.concat(sentChunks)
      .toString("utf8")
      .split("Content-Length: ")
      .filter(Boolean)[0]
      .split("\r\n\r\n")
      .slice(1)
      .join("\r\n\r\n");
    const init = JSON.parse(initBody);
    const reply = JSON.stringify({
      jsonrpc: "2.0",
      id: init.id,
      result: { capabilities: {} },
    });
    serverOut.write(`Content-Length: ${reply.length}\r\n\r\n${reply}`);
    await startPromise;

    // Now stop and ensure shutdown reply is delivered.
    const stopPromise = wiring.stop();
    await tick();
    // After startPromise, the wiring has emitted initialize+initialized
    // already. Then stop emits shutdown — find the shutdown id.
    const allBodies = Buffer.concat(sentChunks)
      .toString("utf8")
      .split("Content-Length: ")
      .filter(Boolean)
      .map((p) => JSON.parse(p.split("\r\n\r\n").slice(1).join("\r\n\r\n")));
    const shutdown = allBodies.find((m) => m.method === "shutdown");
    expect(shutdown).toBeDefined();
    const shutReply = JSON.stringify({
      jsonrpc: "2.0",
      id: shutdown!.id,
      result: null,
    });
    serverOut.write(
      `Content-Length: ${shutReply.length}\r\n\r\n${shutReply}`,
    );
    await stopPromise;
    expect(killed).toEqual(["kill"]);

    // exit notification should follow shutdown.
    await tick();
    const final = Buffer.concat(sentChunks)
      .toString("utf8")
      .split("Content-Length: ")
      .filter(Boolean)
      .map((p) => JSON.parse(p.split("\r\n\r\n").slice(1).join("\r\n\r\n")));
    const methods = final.map((m) => m.method);
    expect(methods).toContain("exit");
  });
});

describe("Wiring document tracking", () => {
  test("notifyDidOpen sends textDocument/didOpen with text", async () => {
    const { client, serverIn } = makeServerPair();
    const wiring = new Wiring({
      client,
      killChild: () => {},
      rootUri: "file:///vault",
      onPublishDiagnostics: () => {},
    });
    const sent: Buffer[] = [];
    serverIn.on("data", (b) => sent.push(b));
    wiring.notifyDidOpen("file:///vault/a.md", "markdown", "# hi\n");
    await tick();
    const bodies = Buffer.concat(sent)
      .toString("utf8")
      .split("Content-Length: ")
      .filter(Boolean)
      .map((p) => JSON.parse(p.split("\r\n\r\n").slice(1).join("\r\n\r\n")));
    const didOpen = bodies.find(
      (m) => m.method === "textDocument/didOpen",
    );
    expect(didOpen).toBeDefined();
    expect(didOpen.params.textDocument.uri).toBe("file:///vault/a.md");
    expect(didOpen.params.textDocument.languageId).toBe("markdown");
    expect(didOpen.params.textDocument.text).toBe("# hi\n");
  });

  test("notifyDidChange sends incremented version per uri", async () => {
    const { client, serverIn } = makeServerPair();
    const wiring = new Wiring({
      client,
      killChild: () => {},
      rootUri: "file:///vault",
      onPublishDiagnostics: () => {},
    });
    const sent: Buffer[] = [];
    serverIn.on("data", (b) => sent.push(b));
    wiring.notifyDidChange("file:///vault/a.md", "v1");
    wiring.notifyDidChange("file:///vault/a.md", "v2");
    wiring.notifyDidChange("file:///vault/b.md", "v1b");
    await tick();
    const bodies = Buffer.concat(sent)
      .toString("utf8")
      .split("Content-Length: ")
      .filter(Boolean)
      .map((p) => JSON.parse(p.split("\r\n\r\n").slice(1).join("\r\n\r\n")));
    const changes = bodies.filter(
      (m) => m.method === "textDocument/didChange",
    );
    expect(changes).toHaveLength(3);
    // a.md monotonic; b.md independent counter.
    expect(changes[0].params.textDocument.uri).toBe("file:///vault/a.md");
    expect(changes[0].params.textDocument.version).toBe(1);
    expect(changes[1].params.textDocument.version).toBe(2);
    expect(changes[2].params.textDocument.uri).toBe("file:///vault/b.md");
    expect(changes[2].params.textDocument.version).toBe(1);
  });
});
