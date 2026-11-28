// Generates docs/api.md from the bundled OpenAPI document (pnpm gen:docs).
// No dependencies: the input is the JSON that `redocly bundle` writes.
//   node scripts/api-docs.mjs <openapi.json> <output.md>
import { readFileSync, writeFileSync } from "node:fs";

const [input, output] = process.argv.slice(2);
if (!input || !output) {
  console.error("usage: node scripts/api-docs.mjs <openapi.json> <output.md>");
  process.exit(2);
}
const spec = JSON.parse(readFileSync(input, "utf8"));
const methods = ["get", "post", "put", "patch", "delete"];
const lines = [];
const out = (s = "") => lines.push(s);
const oneLine = (s = "") => s.replace(/\s+/g, " ").trim();
const cell = (s = "") => oneLine(s).replaceAll("|", "\\|");
const refName = (ref) => ref.split("/").pop();
const schemaRef = (s) => {
  if (!s) return "";
  if (s.$ref) return `[\`${refName(s.$ref)}\`](#${refName(s.$ref).toLowerCase()})`;
  if (s.type === "array" && s.items) return `array of ${schemaRef(s.items)}`;
  return `\`${s.type ?? "object"}\``;
};
const resolve = (o) => (o && o.$ref ? resolvePointer(o.$ref) : o);
function resolvePointer(ref) {
  return ref
    .slice(2)
    .split("/")
    .reduce((node, key) => node[key.replaceAll("~1", "/").replaceAll("~0", "~")], spec);
}

out("# API reference");
out();
out(
  "<!-- Generated from api/openapi.yaml by `pnpm gen:docs` (scripts/generate-api.sh). Do not edit by hand; CI fails when it is out of date. -->",
);
out();
out(
  `**${spec.info.title}** ${spec.info.version}, OpenAPI ${spec.openapi}. Base path: \`${spec.servers?.[0]?.url ?? "/api/v1"}\`.`,
);
out();
out(spec.info.description?.trim() ?? "");
out();
out("## Endpoints");
out();
out("| Method | Path | Summary | Access |");
out("|---|---|---|---|");
const ops = [];
for (const [path, item] of Object.entries(spec.paths)) {
  for (const m of methods) {
    const op = item[m];
    if (!op) continue;
    ops.push({ path, method: m, op, item });
    const sec = op.security ?? spec.security ?? [];
    const access =
      sec.length === 0 ? "public" : sec.map((s) => Object.keys(s).join(" + ")).join(" or ");
    out(
      `| ${m.toUpperCase()} | [\`${path}\`](#${op.operationId.toLowerCase()}) | ${cell(op.summary)} | ${access} |`,
    );
  }
}
for (const { path, method, op, item } of ops) {
  out();
  out(`### ${op.operationId}`);
  out();
  out(`\`${method.toUpperCase()} ${path}\`: ${oneLine(op.summary)}`);
  out();
  if (op.description) {
    out(op.description.trim());
    out();
  }
  const params = [...(item.parameters ?? []), ...(op.parameters ?? [])].map(resolve);
  if (params.length) {
    out("| Parameter | In | Required | Description |");
    out("|---|---|---|---|");
    for (const p of params) {
      out(`| \`${p.name}\` | ${p.in} | ${p.required ? "yes" : "no"} | ${cell(p.description)} |`);
    }
    out();
  }
  const body = resolve(op.requestBody);
  if (body) {
    const [type, media] = Object.entries(body.content)[0];
    out(`Request body (\`${type}\`): ${schemaRef(media.schema)}`);
    out();
  }
  out("| Status | Response |");
  out("|---|---|");
  for (const [status, r] of Object.entries(op.responses)) {
    const res = resolve(r);
    const media = res.content ? Object.entries(res.content)[0] : null;
    const schema = media ? ` ${schemaRef(media[1].schema)}` : "";
    out(`| ${status} | ${cell(res.description)}${schema} |`);
  }
}
out();
out("## Schemas");
for (const [name, s] of Object.entries(spec.components.schemas)) {
  out();
  out(`### ${name}`);
  out();
  if (s.description) {
    out(s.description.trim());
    out();
  }
  if (s.enum) {
    out(`One of: ${s.enum.map((v) => `\`${v}\``).join(", ")}.`);
    continue;
  }
  if (!s.properties) {
    out(`Type: ${schemaRef(s)}.`);
    continue;
  }
  out("| Property | Type | Required | Description |");
  out("|---|---|---|---|");
  for (const [prop, p] of Object.entries(s.properties)) {
    const enumText = p.enum ? ` One of: ${p.enum.map((v) => `\`${v}\``).join(", ")}.` : "";
    out(
      `| \`${prop}\` | ${schemaRef(p)} | ${s.required?.includes(prop) ? "yes" : "no"} | ${cell(p.description)}${enumText} |`,
    );
  }
}
writeFileSync(output, lines.join("\n").replace(/\n{3,}/g, "\n\n") + "\n");
