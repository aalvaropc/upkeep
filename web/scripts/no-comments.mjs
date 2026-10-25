import { readdirSync, readFileSync, statSync } from "node:fs";
import { extname, join } from "node:path";
import { parseSync } from "oxc-parser";

const scripts = new Set([".ts", ".tsx", ".js", ".jsx", ".mjs"]);
let found = 0;

function lineOf(text, offset) {
  return text.slice(0, offset).split("\n").length;
}

function report(path, text, offset, snippet) {
  found += 1;
  console.log(`${path}:${lineOf(text, offset)}: ${snippet.split("\n")[0]}`);
}

function withoutStrings(text) {
  return text.replace(/"(?:[^"\\\n]|\\.)*"|'(?:[^'\\\n]|\\.)*'/g, (s) => " ".repeat(s.length));
}

function check(path) {
  const ext = extname(path);
  const text = readFileSync(path, "utf8");
  if (scripts.has(ext)) {
    const result = parseSync(path, text);
    if (result.errors.length > 0) {
      console.error(`${path}: ${result.errors[0].message}`);
      process.exitCode = 2;
    }
    for (const c of result.comments) {
      report(path, text, c.start, text.slice(c.start, c.end));
    }
  } else if (ext === ".css") {
    for (const m of withoutStrings(text).matchAll(/\/\*/g)) {
      report(path, text, m.index, text.slice(m.index, m.index + 60));
    }
  } else if (ext === ".html") {
    for (const m of text.matchAll(/<!--/g)) {
      report(path, text, m.index, text.slice(m.index, m.index + 60));
    }
  }
}

function walk(path) {
  if (statSync(path).isDirectory()) {
    for (const name of readdirSync(path)) {
      if (name !== "node_modules" && !name.startsWith(".")) {
        walk(join(path, name));
      }
    }
    return;
  }
  check(path);
}

for (const root of process.argv.slice(2)) {
  walk(root);
}

if (found > 0) {
  console.error(`${found} comment(s) found; the code has no comments`);
  process.exitCode = 1;
}
