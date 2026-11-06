// Installed parser bridge. Input is data; MDX, imports and components never execute.
import {readFileSync, realpathSync} from 'node:fs';
import {createInterface} from 'node:readline';
import {pathToFileURL} from 'node:url';
export async function analyzeRequest(request) {
try {
  if (request.mode === 'mermaid') {
    const {JSDOM} = await import('jsdom');
    const dom = new JSDOM('');
    globalThis.window = dom.window;
    globalThis.document = dom.window.document;
    const {default: mermaid} = await import('mermaid');
    mermaid.initialize({startOnLoad: false, securityLevel: 'strict'});
    const result = await mermaid.parse(request.source);
    return {ok: !!result};
  } else if (request.mode === 'mdx') {
    const [{unified}, {default: parse}, {default: mdx}, {default: gfm}] = await Promise.all([
      import('unified'), import('remark-parse'), import('remark-mdx'), import('remark-gfm')]);
    const tree = unified().use(parse).use(mdx).use(gfm).parse(request.source);
    // Convert parser UTF-16 offsets to the wire contract's UTF-8 bytes explicitly.
    const convert = node => {
      const start = node.position?.start.offset ?? 0;
      const end = node.position?.end.offset ?? start;
      return {type: node.type, value: node.value, name: node.name, lang: node.lang,
        depth: node.depth, attributes: node.attributes?.length ?? 0,
        start: Buffer.byteLength(request.source.slice(0, start)),
        end: Buffer.byteLength(request.source.slice(0, end)),
        children: (node.children ?? []).map(convert)};
    };
    return {ok: true, tree: convert(tree)};
  } else { throw new Error('Unknown parser mode'); }
} catch (error) {
  return {ok: false, error: String(error.message),
    line: error.line ?? error.hash?.loc?.first_line ?? 1,
    column: error.column ?? error.hash?.loc?.first_column ?? 1};
}
}

if (process.argv[1] && import.meta.url === pathToFileURL(realpathSync(process.argv[1])).href) {
  if (process.argv.includes('--stream-mdx')) {
    const lines = createInterface({input: process.stdin, crlfDelay: Infinity});
    for await (const line of lines) {
      const request = JSON.parse(line);
      if (request.mode !== 'mdx') throw new Error('Streaming supports MDX only');
      process.stdout.write(JSON.stringify(await analyzeRequest(request)) + '\n');
    }
  } else process.stdout.write(JSON.stringify(await analyzeRequest(JSON.parse(readFileSync(0, 'utf8')))));
}
