// A second, different MCP client.
//
// Everything the vault has been tested against so far is the Go SDK talking to
// itself: the same library on both ends of the pipe, so a shared assumption
// would agree with itself and look like a passing test. This is the TypeScript
// SDK, a different language, a different implementation of the framing and the
// schema validation, and different maintainers, reading the same vault over the
// same stdio binary.
//
// It prints what it finds and exits non-zero if anything it needs is missing.

import { Client } from "@modelcontextprotocol/sdk/client/index.js";
import { StdioClientTransport } from "@modelcontextprotocol/sdk/client/stdio.js";

const binary = process.env.MNEMOSIA_MCP_BIN;
if (!binary) {
  console.error("set MNEMOSIA_MCP_BIN to the mnemosia-mcp binary");
  process.exit(2);
}

const failures = [];
function check(condition, message) {
  if (!condition) failures.push(message);
  console.log(`  ${condition ? "ok  " : "FAIL"}  ${message}`);
}

const transport = new StdioClientTransport({
  command: binary,
  env: {
    PATH: process.env.PATH,
    HOME: process.env.HOME,
    MNEMOSIA_PHRASE: process.env.MNEMOSIA_PHRASE,
    MNEMOSIA_APP_KEY: process.env.MNEMOSIA_APP_KEY,
    MNEMOSIA_HOME: process.env.MNEMOSIA_HOME,
    MNEMOSIA_MODEL_DIR: process.env.MNEMOSIA_MODEL_DIR,
    MNEMOSIA_INDEXER: process.env.MNEMOSIA_INDEXER ?? "",
  },
  stderr: "pipe",
});

const client = new Client(
  { name: "mnemosia-cross-agent-check", version: "1.0.0" },
  { capabilities: {} },
);

const started = Date.now();
await client.connect(transport);

const server = client.getServerVersion();
const capabilities = client.getServerCapabilities();
const instructions = client.getInstructions() ?? "";
console.log(`\nconnected to ${server?.name} ${server?.version} in ${Date.now() - started} ms`);
// The negotiated revision. Every shipping client is on 2025-11-25 by
// construction: the current revision deprecates `initialize`, so a handshake
// that uses it cannot reach the newer one whatever the client claims.
console.log(`  protocol      ${transport.protocolVersion ?? "(not reported by this transport)"}`);
console.log(`  capabilities  ${Object.keys(capabilities ?? {}).sort().join(", ")}`);
console.log(`  instructions  ${instructions.length} B`);

console.log("\nsurface");
const { tools } = await client.listTools();
const { resources } = await client.listResources();
const { resourceTemplates } = await client.listResourceTemplates();
const { prompts } = await client.listPrompts();
console.log(`  tools     ${tools.map((t) => t.name).sort().join(", ")}`);
console.log(`  resources ${resources.map((r) => r.uri).join(", ")}`);
console.log(`  templates ${resourceTemplates.map((r) => r.uriTemplate).join(", ")}`);
console.log(`  prompts   ${prompts.map((p) => p.name).join(", ")}`);

console.log("\nchecks");
check(instructions.length > 0, "the server sends instructions");
for (const want of ["recall", "remember", "browse", "open", "save_session", "forget"]) {
  check(tools.some((t) => t.name === want), `tool ${want} is advertised`);
}
check(prompts.some((p) => p.name === "resume"), "the resume prompt is advertised");

// A user's own memories must not be advertised as publicly cacheable. The Go
// SDK stamps "public" after a handler returns, so this is the field a different
// client would see if the middleware that overrides it were ever dropped.
const listed = await client.request(
  { method: "resources/list", params: {} },
  (await import("@modelcontextprotocol/sdk/types.js")).ListResourcesResultSchema,
);
check(listed.cacheScope === "private",
  `resources/list is cacheable as ${JSON.stringify(listed.cacheScope)}, want "private"`);

console.log("\nreading the vault");
const sessions = await client.callTool({
  name: "browse",
  arguments: { kinds: ["session"], limit: 10 },
});
const sessionText = sessions.content.map((c) => c.text ?? "").join("\n");
console.log(sessionText.split("\n").slice(0, 12).map((l) => "  " + l).join("\n"));
check(!sessions.isError, "browse returned a result rather than an error");

const memories = await client.callTool({
  name: "recall",
  arguments: { query: process.env.CROSS_AGENT_QUERY ?? "storage", limit: 3 },
});
const memoryText = memories.content.map((c) => c.text ?? "").join("\n");
console.log("\n" + memoryText.split("\n").slice(0, 14).map((l) => "  " + l).join("\n"));
check(!memories.isError, "recall returned a result rather than an error");

// The address space, read as a resource rather than through a tool. Both doors
// go through one resolver on the server; a second client is where that stops
// being an internal claim.
const vaultResource = await client.readResource({ uri: "mnemosia://vault" });
check(vaultResource.contents.length > 0, "mnemosia://vault reads as a resource");
check(vaultResource.cacheScope === "private",
  `resources/read is cacheable as ${JSON.stringify(vaultResource.cacheScope)}, want "private"`);

const resume = await client.getPrompt({ name: "resume", arguments: {} });
const resumeText = JSON.stringify(resume);
console.log(`\n  resume prompt: ${resume.messages.length} message(s), ${resumeText.length} B`);
check(resume.messages.length > 0, "the resume prompt returns a conversation to continue");

await client.close();

console.log(`\n${failures.length === 0 ? "PASS" : "FAIL"}: ${failures.length} problem(s)`);
for (const failure of failures) console.log(`  - ${failure}`);
process.exit(failures.length === 0 ? 0 : 1);
