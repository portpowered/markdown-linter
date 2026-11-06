import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import test from 'node:test';
import { analyzeRequest } from './analyze.mjs';

test('MDX stream preserves isolated responses, byte offsets and failures across changing inputs', async () => {
  const script = fileURLToPath(new URL('./analyze.mjs', import.meta.url));
  const requests = [
    {mode:'mdx',source:'# Devices 😀\r\n\r\nRead **café** and `API`.\r\n'},
    {mode:'mdx',source:'<Unclosed>\n'},
    {mode:'mdx',source:'import Untrusted from "not-installed";\n\n<Untrusted>{neverExecute()}</Untrusted>\n'},
    {mode:'mdx',source:'Changed content.\n'},
  ];
  const expected = [];
  for (const request of requests) {
    const isolated = spawnSync(process.execPath, [script], {input:JSON.stringify(request),encoding:'utf8'});
    assert.equal(isolated.status, 0, isolated.stderr);
    const response = JSON.parse(isolated.stdout);
    expected.push(response);
    assert.deepEqual(JSON.parse(JSON.stringify(await analyzeRequest(request))), response);
  }
  const stream = spawnSync(process.execPath, [script, '--stream-mdx'], {
    input:requests.map(request=>JSON.stringify(request)).join('\n')+'\n', encoding:'utf8',
  });
  assert.equal(stream.status, 0, stream.stderr);
  assert.deepEqual(stream.stdout.trim().split('\n').map(line=>JSON.parse(line)), expected);
});

test('the MDX stream rejects invalid framing and stateful parser modes', () => {
  const script = fileURLToPath(new URL('./analyze.mjs', import.meta.url));
  for (const input of ['not-json\n', JSON.stringify({mode:'mermaid',source:'graph LR\nA-->B'})+'\n']) {
    const stream = spawnSync(process.execPath, [script, '--stream-mdx'], {input,encoding:'utf8'});
    assert.notEqual(stream.status, 0);
    assert.equal(stream.stdout, '');
  }
});
